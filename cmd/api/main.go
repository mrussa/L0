package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/mrussa/L0/internal/cache"
	"github.com/mrussa/L0/internal/config"
	"github.com/mrussa/L0/internal/db"
	"github.com/mrussa/L0/internal/httpapi"
	"github.com/mrussa/L0/internal/kafka"
	"github.com/mrussa/L0/internal/repo"
)

var version = "dev"

func warmCache(ctx context.Context, r *repo.OrdersRepo, c *cache.OrdersCache, limit int, logf func(string, ...any)) {
	if limit <= 0 {
		return
	}

	ctxList, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	uids, err := r.ListRecentOrderUIDs(ctxList, limit)
	if err != nil {
		logf("[CACHE] warm list: %v", err)
		return
	}

	ok, failed := 0, 0
	for _, uid := range uids {
		ctxOne, cancelOne := context.WithTimeout(ctx, 2*time.Second)
		o, err := r.GetOrder(ctxOne, uid)
		cancelOne()
		if err != nil {
			failed++
			logf("[CACHE] warm %s: %v", uid, err)
			continue
		}
		c.Set(uid, o)
		ok++
	}
	logf("[CACHE] warmed: %d ok, %d failed, size=%d", ok, failed, c.Len())
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[CFG] %v", err)
	}
	log.Printf("[APP] version=%s", version)
	log.Printf("[CFG] http=%s dsn_present=%t cache_warm=%d", cfg.HTTPAddr, cfg.PostgresDSN != "", cfg.CacheWarmLimit)
	log.Printf("[KAFKA] brokers=%s topic=%s group=%s", cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroup)

	rootCtx := context.Background()

	pool, err := db.NewPool(rootCtx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("[DB] new pool: %v", err)
	}
	if err := db.Ping(rootCtx, pool); err != nil {
		pool.Close()
		log.Fatalf("[DB] ping: %v", err)
	}
	defer pool.Close()
	log.Println("[DB] connected")

	rpo := repo.NewOrdersRepo(pool)
	c := cache.New()

	warmCache(rootCtx, rpo, c, cfg.CacheWarmLimit, log.Printf)

	api := httpapi.New(rpo, c, log.Printf, version)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	cons := kafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroup, rpo, c, log.Printf)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cons.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
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

	wg.Wait()
	log.Printf("[HTTP] bye")
}
