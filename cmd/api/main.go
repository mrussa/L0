package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mrussa/L0/internal/cache"
	"github.com/mrussa/L0/internal/config"
	"github.com/mrussa/L0/internal/db"
	"github.com/mrussa/L0/internal/repo"
)

type httpError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, httpError{Error: msg})
}

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

	rpo := repo.NewOrdersRepo(pool)
	c := cache.New()

	warmCtx, cancel := context.WithTimeout(startCtx, 15*time.Second)
	defer cancel()

	if cfg.CacheWarmLimit > 0 {
		uids, err := rpo.ListRecentOrderUIDs(warmCtx, cfg.CacheWarmLimit)
		if err != nil {
			log.Printf("[CACHE] warm list: %v", err)
		} else {
			ok, failed := 0, 0
			for _, uid := range uids {
				o, err := rpo.GetOrder(warmCtx, uid)
				if err != nil {
					failed++
					log.Printf("[CACHE] warm %s: %v", uid, err)
					continue
				}
				c.Set(uid, o)
				ok++
			}
			log.Printf("[CACHE] warmed: %d ok, %d failed, size=%d", ok, failed, c.Len())
		}
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"cache_size": c.Len(),
		})
	})

	mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeErr(w, http.StatusBadRequest, "use /order/{order_uid}")
	})

	mux.HandleFunc("/order/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/order/")
		if id == "" || len(id) > 100 {
			writeErr(w, http.StatusBadRequest, "bad order_uid")
			return
		}

		if o, ok := c.Get(id); ok {
			log.Printf("[CACHE] hit %s", id)
			writeJSON(w, http.StatusOK, o)
			return
		}
		log.Printf("[CACHE] miss %s", id)

		o, err := rpo.GetOrder(r.Context(), id)
		if err != nil {
			switch {
			case errors.Is(err, repo.ErrBadUID):
				writeErr(w, http.StatusBadRequest, "bad order_uid")
			case errors.Is(err, repo.ErrNotFound):
				writeErr(w, http.StatusNotFound, "not found")
			default:
				log.Printf("[HTTP] /order/%s: %v", id, err)
				writeErr(w, http.StatusInternalServerError, "internal")
			}
			return
		}

		c.Set(id, o)
		writeJSON(w, http.StatusOK, o)
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
