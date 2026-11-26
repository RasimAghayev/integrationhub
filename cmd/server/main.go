package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/api"
	"github.com/RasimAghayev/integrationhub/internal/oauth"
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

	connHandler := api.NewConnectionHandler(store.NewConnectionStore(pool), cipher, oauthMgr)
	intHandler := api.NewIntegrationHandler(store.NewIntegrationStore(pool))
	router := newRouter(routerDeps{conn: connHandler, int: intHandler})

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
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
	conn *api.ConnectionHandler
	int  *api.IntegrationHandler
}

func newRouter(d routerDeps) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)
	router.Use(middleware.RequestID)
	router.Use(metricsMiddleware)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Get("/", handleRoot)
	router.Get("/healthz", handleHealthz)
	router.Get("/readyz", handleReadyz)
	router.Get("/metrics", promhttp.Handler().ServeHTTP)
	if d.conn != nil {
		router.Route("/api/v1/connections", func(r chi.Router) {
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
		router.Route("/api/v1/integrations", func(r chi.Router) {
			r.Get("/", d.int.List)
			r.Post("/", d.int.Create)
			r.Get("/{id}", d.int.Get)
			r.Put("/{id}", d.int.Update)
			r.Delete("/{id}", d.int.Delete)
		})
	}
	return router
}

func newSlogHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		httpRequests.WithLabelValues(r.Method, r.URL.Path).Inc()
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

func envInt32(key string, def int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return int32(n)
		}
	}
	return def
}

func dbDSN() string {
	host := env("INTEGRATIONHUB_DB_HOST", "localhost")
	port := env("INTEGRATIONHUB_DB_PORT", "5432")
	user := env("INTEGRATIONHUB_DB_USER", "integrationhub")
	pass := env("INTEGRATIONHUB_DB_PASSWORD", "")
	name := env("INTEGRATIONHUB_DB_NAME", "integrationhub")
	return "host=" + host + " port=" + port + " user=" + user + " password=" + pass + " dbname=" + name + " sslmode=disable connect_timeout=2"
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
	defer rdb.Close()
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
