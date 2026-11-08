package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"kumparan-articles/internal/config"
)

// NewPostgresPool membuat pool koneksi PostgreSQL berdasarkan konfigurasi yang diberikan.
func NewPostgresPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	pgcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("gagal parsing DATABASE_URL: %w", err)
	}

	pgcfg.MaxConns = cfg.MaxConns
	pgcfg.MinConns = cfg.MinConns
	pgcfg.MaxConnLifetime = cfg.MaxConnLifetime
	pgcfg.HealthCheckPeriod = cfg.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, pgcfg)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat pool database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("gagal ping database: %w", err)
	}

	return pool, nil
}
