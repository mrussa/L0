package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mrussa/L0/internal/config"
)

func TestLoad_ErrWhenPostgresDSNMissing(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	cfg, err := config.Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "set POSTGRES_DSN")
	_ = cfg
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://u:p@h/db?sslmode=disable")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("CACHE_WARM_LIMIT", "")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("KAFKA_TOPIC", "")
	t.Setenv("KAFKA_GROUP", "")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, ":8081", cfg.HTTPAddr)
	require.Equal(t, "postgres://u:p@h/db?sslmode=disable", cfg.PostgresDSN)
	require.Equal(t, 100, cfg.CacheWarmLimit)
	require.Equal(t, "localhost:9092", cfg.KafkaBrokers)
	require.Equal(t, "orders", cfg.KafkaTopic)
	require.Equal(t, "orders-consumer", cfg.KafkaGroup)
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9999")
	t.Setenv("POSTGRES_DSN", "postgres://user:pass@host:5432/db?sslmode=disable")
	t.Setenv("CACHE_WARM_LIMIT", "42")
	t.Setenv("KAFKA_BROKERS", "rp:9092")
	t.Setenv("KAFKA_TOPIC", "mytopic")
	t.Setenv("KAFKA_GROUP", "mygroup")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, ":9999", cfg.HTTPAddr)
	require.Equal(t, "postgres://user:pass@host:5432/db?sslmode=disable", cfg.PostgresDSN)
	require.Equal(t, 42, cfg.CacheWarmLimit)
	require.Equal(t, "rp:9092", cfg.KafkaBrokers)
	require.Equal(t, "mytopic", cfg.KafkaTopic)
	require.Equal(t, "mygroup", cfg.KafkaGroup)
}

func TestLoad_CacheWarmLimit_InvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"non_number", "abc"},
		{"negative", "-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("POSTGRES_DSN", "postgres://u:p@h/db?sslmode=disable")
			t.Setenv("CACHE_WARM_LIMIT", tt.value)
			t.Setenv("HTTP_ADDR", "")
			t.Setenv("KAFKA_BROKERS", "")
			t.Setenv("KAFKA_TOPIC", "")
			t.Setenv("KAFKA_GROUP", "")

			cfg, err := config.Load()
			require.NoError(t, err)
			require.Equal(t, 100, cfg.CacheWarmLimit)
		})
	}
}
