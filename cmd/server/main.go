package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/nats-io/nats.go"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	addr := ":8080"
	if v := os.Getenv("INTEGRATIONHUB_SERVER_PORT"); v != "" {
		addr = ":" + v
	}

	router := chi.NewRouter()
	router.Get("/", handleRoot)
	router.Get("/healthz", handleHealthz)
	router.Get("/readyz", handleReadyz)

	srv := &http.Server{
		Addr: addr,
		Handler: router,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("server stopped")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"service":  "integrationhub",
		"version":  "0.1.0",
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
	name := env("INTEGRATIONHUB_DB_NAME", "integrationhub")
	dsn := "host=" + host + " port=" + port + " user=" + user + " dbname=" + name + " sslmode=disable connect_timeout=2s"
	_, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return false
	}
	return true
}

func checkRedis(ctx context.Context) bool {
	addr := env("INTEGRATIONHUB_REDIS_ADDR", "localhost:6379")
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	defer rdb.Close()
	_, err := rdb.Ping(ctx).Result()
	return err == nil
}

func checkNATS(ctx context.Context) bool {
	url := env("INTEGRATIONHUB_NATS_URL", "nats://localhost:4222")
	nc, err := nats.Connect(url, nats.ConnectTimeout(2*time.Second))
	if err != nil {
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
