package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"kumparan-articles/internal/config"
	"kumparan-articles/internal/handler"
	"kumparan-articles/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pgcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("gagal parsing DATABASE_URL: %v", err)
	}
	// Pool eksplisit, bukan default pgxpool. Angka ini untuk layanan demo
	// dengan beban soal (~2,3 tulis/detik rata-rata, burst puluhan per detik):
	// cukup besar untuk menampung burst baca 100:1, cukup kecil agar tidak
	// membanjiri satu Postgres. Naikkan MaxConns hanya bersamaan dengan
	// kenaikan max_connections Postgres dan kapasitas CPU/memori DB.
	// Nilai defaultnya tinggal di internal/config/config.go.
	pgcfg.MaxConns = cfg.MaxConns
	pgcfg.MinConns = cfg.MinConns
	pgcfg.MaxConnLifetime = cfg.MaxConnLifetime
	pgcfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	pool, err := pgxpool.NewWithConfig(ctx, pgcfg)
	if err != nil {
		log.Fatalf("gagal membuat pool database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("gagal konek database: %v", err)
	}

	repo := repository.New(pool)
	h := handler.New(repo)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /articles", h.CreateArticle)
	mux.HandleFunc("GET /articles", h.ListArticles)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy"}`))
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("server jalan di :%s", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
