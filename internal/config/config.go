package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr       string
	PostgresDSN    string
	CacheWarmLimit int

	KafkaBrokers string
	KafkaTopic   string
	KafkaGroup   string
}

func Load() (Config, error) {
	var cfg Config

	cfg.HTTPAddr = getEnv("HTTP_ADDR", ":8081")
	cfg.PostgresDSN = getEnv("POSTGRES_DSN", "")
	if cfg.PostgresDSN == "" {
		return Config{}, errors.New("set POSTGRES_DSN")
	}

	cfg.CacheWarmLimit = getEnvInt("CACHE_WARM_LIMIT", 100)
	cfg.KafkaBrokers = getEnv("KAFKA_BROKERS", "localhost:9092")
	cfg.KafkaTopic = getEnv("KAFKA_TOPIC", "orders")
	cfg.KafkaGroup = getEnv("KAFKA_GROUP", "orders-consumer")

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}
