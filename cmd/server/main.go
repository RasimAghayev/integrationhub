package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/api"
	"github.com/RasimAghayev/integrationhub/internal/auth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/execution"
	"github.com/RasimAghayev/integrationhub/internal/executor"
	"github.com/RasimAghayev/integrationhub/internal/oauth"
	"github.com/RasimAghayev/integrationhub/internal/rbac"
	"github.com/RasimAghayev/integrationhub/internal/secret"
	"github.com/RasimAghayev/integrationhub/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

var httpRequests = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "integrationhub_http_requests_total",
		Help: "Total number of HTTP requests processed.",
	},
	[]string{"method", "path"},
)

func main() {
	slog.SetDefault(slog.New(newSlogHandler()))
	prometheus.MustRegister(httpRequests)

	addr := ":8080"
	if v := os.Getenv("INTEGRATIONHUB_SERVER_PORT"); v != "" {
		addr = ":" + v
	}

	initCtx, initCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer initCancel()

	pool, err := newPool(initCtx)
	if err != nil {
		slog.Error("postgres pool init failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	prometheus.MustRegister(newPoolStatsCollector(pool))

	if err := store.Migrate(initCtx, pool); err != nil {
		slog.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	var cipher *secret.Cipher
	var oauthMgr *oauth.Manager
	if key := os.Getenv("INTEGRATIONHUB_SECRET_KEY"); key != "" {
		keyBytes, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			slog.Error("invalid INTEGRATIONHUB_SECRET_KEY", "error", err)
			os.Exit(1)
		}
		cipher, err = secret.New(keyBytes)
		if err != nil {
			slog.Error("invalid INTEGRATIONHUB_SECRET_KEY", "error", err)
			os.Exit(1)
		}
		oauthMgr = oauth.NewManager(keyBytes)
	} else {
		slog.Warn("INTEGRATIONHUB_SECRET_KEY not set; credential encryption disabled")
	}

	connStore := store.NewConnectionStore(pool)
	connHandler := api.NewConnectionHandler(connStore, cipher, oauthMgr)

	keyStore := store.NewAPIKeyStore(pool)
	var authenticator *auth.Authenticator
	var authHandler *api.AuthHandler
	if jwtSecret := os.Getenv("INTEGRATIONHUB_JWT_SECRET"); jwtSecret != "" {
		jwtMgr := auth.NewJWTManager([]byte(jwtSecret), envDuration("INTEGRATIONHUB_JWT_TTL", time.Hour), "integrationhub")
		authenticator = auth.NewAuthenticator(jwtMgr, keyStore)
		authHandler = api.NewAuthHandler(jwtMgr, keyStore)
	} else {
		authenticator = auth.NewAuthenticator(nil, keyStore)
		slog.Warn("INTEGRATIONHUB_JWT_SECRET not set; JWT authentication disabled")
	}
	if raw := os.Getenv("INTEGRATIONHUB_ADMIN_API_KEY"); raw != "" {
		if err := seedAdminKey(initCtx, keyStore, raw); err != nil {
			slog.Error("failed to seed admin api key", "error", err)
			os.Exit(1)
		}
	}

	enforcer, err := rbac.New(rbacPolicy())
	if err != nil {
		slog.Error("failed to build rbac enforcer", "error", err)
		os.Exit(1)
	}
	execMetrics := executor.NewMetrics()
	prometheus.MustRegister(execMetrics.Collectors()...)
	exec := executor.New(executor.DefaultConfig(), nil)
	exec.SetMetrics(execMetrics)
	intStore := store.NewIntegrationStore(pool)
	execSvc := execution.New(store.NewExecutionStore(pool), intStore, connStore, exec, cipher)
	execSvc.SetWorkers(envInt("INTEGRATIONHUB_EXEC_WORKERS", 4))
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	execSvc.StartWorker(workerCtx, time.Duration(envInt("INTEGRATIONHUB_EXEC_POLL_MS", 1000))*time.Millisecond)
	intHandler := api.NewIntegrationHandler(intStore)
	execHandler := api.NewExecutionHandler(execSvc, intStore)
	limiter := api.NewRateLimiter(
		rate.Limit(envInt("INTEGRATIONHUB_RATE_PER_SEC", 100)),
		envInt("INTEGRATIONHUB_RATE_BURST", 200),
		5*time.Minute,
	)
	router := newRouter(routerDeps{
		conn:    connHandler,
		int:     intHandler,
		exec:    execHandler,
		auth:    authenticator,
		authAPI: authHandler,
		rbac:    enforcer,
		limiter: limiter,
		cors:    splitCSV(os.Getenv("INTEGRATIONHUB_CORS_ORIGINS")),
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}

	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("server stopped")
}

type routerDeps struct {
	conn    *api.ConnectionHandler
	int     *api.IntegrationHandler
	exec    *api.ExecutionHandler
	auth    *auth.Authenticator
	authAPI *api.AuthHandler
	rbac    *rbac.Enforcer
	limiter *api.RateLimiter
	cors    []string
}

func newRouter(d routerDeps) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(api.RequestIDHeader)
	router.Use(api.SecurityHeaders)
	router.Use(d.limiter.Middleware)
	router.Use(api.CORS(d.cors))
	router.Use(slogRequestLogger)
	router.Use(middleware.Recoverer)
	router.Use(metricsMiddleware)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Get("/", handleRoot)
	router.Get("/healthz", handleHealthz)
	router.Get("/readyz", handleReadyz)
	router.Get("/metrics", promhttp.Handler().ServeHTTP)
	if d.authAPI != nil {
		router.Post("/api/v1/auth/token", d.authAPI.Token)
	}
	router.Group(func(r chi.Router) {
		r.Use(d.auth.Middleware)
		r.Use(d.rbac.Middleware(resourceFor))
		if d.conn != nil {
			r.Route("/api/v1/connections", func(r chi.Router) {
				r.Get("/", d.conn.List)
				r.Post("/", d.conn.Create)
				r.Get("/{id}", d.conn.Get)
				r.Put("/{id}", d.conn.Update)
				r.Delete("/{id}", d.conn.Delete)
				r.Post("/{id}/test", d.conn.Test)
				r.Get("/{id}/oauth/authorize", d.conn.OAuthAuthorize)
				r.Get("/{id}/oauth/callback", d.conn.OAuthCallback)
				r.Post("/{id}/oauth/refresh", d.conn.OAuthRefresh)
			})
		}
		if d.int != nil {
			r.Route("/api/v1/integrations", func(r chi.Router) {
				r.Get("/", d.int.List)
				r.Post("/", d.int.Create)
				r.Get("/{id}", d.int.Get)
				r.Put("/{id}", d.int.Update)
				r.Delete("/{id}", d.int.Delete)
				if d.exec != nil {
					r.Post("/{id}/execute", d.exec.Execute)
				}
			})
		}
		if d.exec != nil {
			r.Route("/api/v1/executions", func(r chi.Router) {
				r.Get("/", d.exec.List)
				r.Get("/counts", d.exec.Counts)
				r.Get("/dead", d.exec.Dead)
				r.Get("/{id}", d.exec.Get)
				r.Post("/{id}/replay", d.exec.Replay)
			})
		}
	})
	return router
}

func newSlogHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
}

func slogRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		defer func() {
			slog.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		}()
		next.ServeHTTP(ww, r)
	})
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		// Label by route pattern, not the concrete URL, to keep cardinality low.
		pattern := "unmatched"
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if p := rctx.RoutePattern(); p != "" {
				pattern = p
			}
		}
		httpRequests.WithLabelValues(r.Method, pattern).Inc()
	})
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"service": "integrationhub",
		"version": "0.1.0",
	})
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	checks := map[string]bool{
		"postgres": checkPostgres(ctx),
		"redis":    checkRedis(ctx),
		"nats":     checkNATS(ctx),
	}
	ok := true
	for _, v := range checks {
		if !v {
			ok = false
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(boolStatus(ok))
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  statusString(ok),
		"checks":  checks,
		"version": "0.1.0",
	})
}

func handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ready",
		"version": "0.1.0",
	})
}

func boolStatus(ok bool) int {
	if ok {
		return http.StatusOK
	}
	return http.StatusServiceUnavailable
}

func statusString(ok bool) string {
	if ok {
		return "up"
	}
	return "degraded"
}

func newPool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dbDSN())
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = envInt32("INTEGRATIONHUB_DB_MAX_CONNS", 10)
	cfg.MinConns = envInt32("INTEGRATIONHUB_DB_MIN_CONNS", 0)
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}

// poolStatsCollector exports pgxpool statistics as Prometheus gauges so pool
// saturation (AcquireDuration / empty acquires) is observable.
type poolStatsCollector struct {
	pool *pgxpool.Pool
	desc *prometheus.Desc
}

func newPoolStatsCollector(pool *pgxpool.Pool) prometheus.Collector {
	return &poolStatsCollector{
		pool: pool,
		desc: prometheus.NewDesc(
			"integrationhub_pgxpool_stats",
			"pgxpool connection statistics.",
			[]string{"stat"}, nil,
		),
	}
}

func (c *poolStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *poolStatsCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(s.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(s.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(s.TotalConns()), "total")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(s.EmptyAcquireCount()), "empty_acquire_total")
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(s.CanceledAcquireCount()), "canceled_acquire_total")
}

func envInt32(key string, def int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return int32(n)
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// resourceFor maps a request path to the RBAC resource it touches.
func resourceFor(r *http.Request) string {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/connections"):
		return "connections"
	case strings.HasPrefix(p, "/api/v1/integrations"):
		return "integrations"
	case strings.HasPrefix(p, "/api/v1/executions"):
		return "executions"
	}
	return ""
}

// rbacPolicy loads the policy from a file, an inline CSV, or the defaults.
func rbacPolicy() string {
	if path := os.Getenv("INTEGRATIONHUB_RBAC_POLICY_FILE"); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			return string(b)
		}
		slog.Warn("INTEGRATIONHUB_RBAC_POLICY_FILE unreadable", "path", path)
	}
	if p := os.Getenv("INTEGRATIONHUB_RBAC_POLICY"); p != "" {
		return p
	}
	return rbac.DefaultPolicy
}

// seedAdminKey creates the bootstrap admin API key when it does not exist yet.
func seedAdminKey(ctx context.Context, s *store.APIKeyStore, raw string) error {
	hash := auth.HashKey(raw)
	if _, err := s.GetByHash(ctx, hash); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return s.Create(ctx, &domain.APIKey{Subject: "admin", KeyHash: hash, Roles: []string{"admin"}})
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func dbDSN() string {
	host := env("INTEGRATIONHUB_DB_HOST", "localhost")
	port := env("INTEGRATIONHUB_DB_PORT", "5432")
	user := env("INTEGRATIONHUB_DB_USER", "integrationhub")
	pass := env("INTEGRATIONHUB_DB_PASSWORD", "")
	name := env("INTEGRATIONHUB_DB_NAME", "integrationhub")
	return "host=" + host + " port=" + port + " user=" + user + " password=" + pass + " dbname=" + name + " sslmode=disable connect_timeout=2 statement_timeout=5000 idle_in_transaction_session_timeout=10000"
}

func checkPostgres(ctx context.Context) bool {
	_, err := pgx.Connect(ctx, dbDSN())
	if err != nil {
		slog.Error("postgres check failed", "error", err)
		return false
	}
	return true
}

func checkRedis(ctx context.Context) bool {
	addr := env("INTEGRATIONHUB_REDIS_ADDR", "localhost:6379")
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     env("INTEGRATIONHUB_REDIS_PASSWORD", ""),
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	defer func() { _ = rdb.Close() }()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		slog.Error("redis check failed", "error", err)
	}
	return err == nil
}

func checkNATS(ctx context.Context) bool {
	url := env("INTEGRATIONHUB_NATS_URL", "nats://localhost:4222")
	nc, err := nats.Connect(url, nats.Timeout(2*time.Second))
	if err != nil {
		slog.Error("nats check failed", "error", err)
		return false
	}
	defer nc.Close()
	_ = ctx
	return true
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
