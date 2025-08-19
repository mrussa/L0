package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrussa/L0/internal/config"
	"github.com/mrussa/L0/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[CFG] %v", err)
	}
	log.Printf("[CFG] http=%s dsn_present=%t cache_warm=%d", cfg.HTTPAddr, cfg.PostgresDSN != "", cfg.CacheWarmLimit)

	startCtx := context.Background()
	pool, err := db.NewPool(startCtx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("[DB] new pool: %v", err)
	}

	if err := db.Ping(startCtx, pool); err != nil {
		pool.Close()
		log.Fatalf("[DB] ping: %v", err)
	}
	defer pool.Close()
	log.Println("[DB] connected")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("[HTTP] listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[HTTP] %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("[HTTP] shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[HTTP] shutdown error: %v", err)
	}
	log.Printf("[HTTP] bye")
}
