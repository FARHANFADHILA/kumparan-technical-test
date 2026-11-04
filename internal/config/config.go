package config

import (
	"errors"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Config adalah seluruh konfigurasi aplikasi yang dibaca dari environment.
// Sengaja berupa struct biasa, bukan variabel global: main.go yang memanggil
// Load() sekali, lalu mengoper nilai-nilainya ke pembuat pool dan server.
// Dengan begitu, dependensi database tetap terlihat di constructor
// (repository.New(pool)), dan tes bisa memakai konfigurasi atau pool lain
// tanpa menimpa state global.
type Config struct {
	DatabaseURL       string
	Port              string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	HealthCheckPeriod time.Duration
	// Meilisearch, opsional: kalau MeiliHost kosong, pencarian tetap
	// jalan lewat Postgres saja, dan handler akan fallback ke Postgres
	// kalau Meilisearch mati. Jadi field ini TIDAK wajib diisi.
	MeiliHost      string
	MeiliMasterKey string
	MeiliIndex     string
}

// Load membaca .env kalau ada (error karena file tidak ada diabaikan,
// supaya cara set env lewat shell tetap bisa jalan), lalu membaca
// environment dan mengisi nilai default yang masuk akal untuk demo.
// Load TIDAK menimpa environment yang sudah di-set di shell: godotenv
// hanya mengisi variabel yang belum ada.
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
