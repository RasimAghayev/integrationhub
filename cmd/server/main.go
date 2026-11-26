package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
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

	router := chi.NewRouter()
	router.Use(metricsMiddleware)
	router.Get("/", handleRoot)
	router.Get("/healthz", handleHealthz)
	router.Get("/readyz", handleReadyz)
	router.Get("/metrics", promhttp.Handler().ServeHTTP)

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

func checkPostgres(ctx context.Context) bool {
	host := env("INTEGRATIONHUB_DB_HOST", "localhost")
	port := env("INTEGRATIONHUB_DB_PORT", "5432")
	user := env("INTEGRATIONHUB_DB_USER", "integrationhub")
	pass := env("INTEGRATIONHUB_DB_PASSWORD", "")
	name := env("INTEGRATIONHUB_DB_NAME", "integrationhub")
	dsn := "host=" + host + " port=" + port + " user=" + user + " password=" + pass + " dbname=" + name + " sslmode=disable connect_timeout=2"
	_, err := pgx.Connect(ctx, dsn)
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
