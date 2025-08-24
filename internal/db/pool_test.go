package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestNewPool_ParseConfigError(t *testing.T) {
	_, err := NewPool(context.Background(), "://bad_dsn")
	require.Error(t, err)
}

func TestNewPool_SetsConfigValues(t *testing.T) {
	pool, err := NewPool(context.Background(), "postgres://user:pass@localhost:5432/dbname?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	cfg := pool.Config()
	require.EqualValues(t, int32(20), cfg.MaxConns)
	require.EqualValues(t, int32(0), cfg.MinConns)
	require.Equal(t, 2*time.Minute, cfg.MaxConnIdleTime)
	require.Equal(t, 45*time.Minute, cfg.MaxConnLifetime)
}

func TestNewPool_NewWithConfigError(t *testing.T) {
	orig := newPoolWithConfig
	defer func() { newPoolWithConfig = orig }()

	newPoolWithConfig = func(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
		return nil, errors.New("boom")
	}

	_, err := NewPool(context.Background(), "postgres://user:pass@localhost:5432/dbname?sslmode=disable")
	require.Error(t, err)
}

func TestNewPool_ContextCanceled_NoError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pool, err := NewPool(ctx, "postgres://user:pass@localhost:5432/dbname?sslmode=disable")
	require.NoError(t, err)
	pool.Close()
}

func TestPing_Unreachable(t *testing.T) {
	pool, err := NewPool(context.Background(), "postgres://user:pass@127.0.0.1:1/dbname?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	start := time.Now()
	err = Ping(context.Background(), pool)
	require.Error(t, err)
	require.LessOrEqual(t, time.Since(start), 4*time.Second)
}
