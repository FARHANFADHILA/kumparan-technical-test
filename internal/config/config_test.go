package config

import (
	"testing"
	"time"
)

func TestLoadMissingDatabaseURL(t *testing.T) {
	// Pastikan tidak ada DATABASE_URL dari environment luar yang bocor
	// ke tes ini. t.Setenv mengembalikan nilai lama setelah tes selesai.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PORT", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load seharusnya error kalau DATABASE_URL kosong, dapat nil")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:password@localhost:5432/kumparan_articles?sslmode=disable")
	t.Setenv("PORT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load mengembalikan error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default salah: dapat %q, mau %q", cfg.Port, "8080")
	}
	if cfg.MaxConns != 25 {
		t.Errorf("MaxConns default salah: dapat %d, mau 25", cfg.MaxConns)
	}
	if cfg.MinConns != 2 {
		t.Errorf("MinConns default salah: dapat %d, mau 2", cfg.MinConns)
	}
	if cfg.MaxConnLifetime != 30*time.Minute {
		t.Errorf("MaxConnLifetime salah: dapat %v", cfg.MaxConnLifetime)
	}
	if cfg.HealthCheckPeriod != time.Minute {
		t.Errorf("HealthCheckPeriod salah: dapat %v", cfg.HealthCheckPeriod)
	}
}

func TestLoadCustomPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:password@localhost:5432/kumparan_articles?sslmode=disable")
	t.Setenv("PORT", "9000")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load mengembalikan error: %v", err)
	}
	if cfg.Port != "9000" {
		t.Errorf("Port kustom tidak dipakai: dapat %q, mau %q", cfg.Port, "9000")
	}
}
