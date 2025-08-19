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
}

func Load() (Config, error) {
	var cfg Config

	cfg.HTTPAddr = os.Getenv("HTTP_ADDR")
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8081"
	}

	cfg.PostgresDSN = os.Getenv("POSTGRES_DSN")
	if cfg.PostgresDSN == "" {
		return Config{}, errors.New("set POSTGRES_DSN")

	}

	if c := os.Getenv("CACHE_WARM_LIMIT"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n >= 0 {
			cfg.CacheWarmLimit = n
		}
	}
	if cfg.CacheWarmLimit == 0 {
		cfg.CacheWarmLimit = 100
	}

	return cfg, nil

}
