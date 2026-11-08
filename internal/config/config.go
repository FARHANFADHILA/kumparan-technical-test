package config

import (
	"errors"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL       string
	Port              string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	HealthCheckPeriod time.Duration
	MeiliHost      string
	MeiliMasterKey string
	MeiliIndex     string
}

func Load() (Config, error) {
	_ = godotenv.Load()
	cfg := Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		Port:              os.Getenv("PORT"),
		MaxConns:          25,
		MinConns:          2,
		MaxConnLifetime:   30 * time.Minute,
		HealthCheckPeriod: time.Minute,
		MeiliHost:         os.Getenv("MEILI_HOST"),
		MeiliMasterKey:    os.Getenv("MEILI_MASTER_KEY"),
		MeiliIndex:        os.Getenv("MEILI_INDEX"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL belum di-set")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.MeiliIndex == "" {
		cfg.MeiliIndex = "articles"
	}
	return cfg, nil
}
