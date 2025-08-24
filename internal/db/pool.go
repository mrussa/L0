package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxConnections    = 20
	minConnections    = 0
	maxConnIdleTime   = 2 * time.Minute
	maxConnLifetime   = 45 * time.Minute
	connectionTimeout = 3 * time.Second
)

var newPoolWithConfig = pgxpool.NewWithConfig

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	cfg.MaxConns = maxConnections
	cfg.MinConns = minConnections
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.MaxConnLifetime = maxConnLifetime

	pool, err := newPoolWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return pool, nil

}

func Ping(parent context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(parent, connectionTimeout)
	defer cancel()
	return pool.Ping(ctx)
}
