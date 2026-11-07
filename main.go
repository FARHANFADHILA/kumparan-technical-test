package main

import (
	"context"
	"log"
	_ "embed"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"kumparan-articles/internal/config"
	"kumparan-articles/internal/handler"
	"kumparan-articles/internal/repository"
	"kumparan-articles/internal/search"
)

//go:embed openapi.yaml
var openapiYAML []byte

func main() {
	// untuk log config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	
	// untuk koneksi ke database
	pgcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("gagal parsing DATABASE_URL: %v", err)
	}
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
	searchClient := search.New(cfg.MeiliHost, cfg.MeiliMasterKey, cfg.MeiliIndex)
	if searchClient.Enabled() {
		log.Printf("meilisearch aktif: %s index %s (pencarian dengan query pakai Meilisearch, fallback ke Postgres kalau gagal)", cfg.MeiliHost, cfg.MeiliIndex)
	} else {
		log.Printf("meilisearch tidak dikonfigurasi (MEILI_HOST kosong), pencarian pakai Postgres saja")
	}
	handler := handler.NewWithSearch(repo, searchClient, openapiYAML)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /articles", handler.CreateArticle)
	mux.HandleFunc("GET /articles", handler.ListArticles)
	mux.HandleFunc("GET /authors", handler.ListAuthors)
	mux.HandleFunc("GET /openapi.yaml", handler.DonwloadOpenApi)
	mux.HandleFunc("GET /scalar", handler.ScalarDocs)
	mux.HandleFunc("GET /scalar/", handler.ScalarDocs)
	
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy"}`))
			return
		}
		w.Write([]byte(`{"status":"ok", "version":"v1.0.0"}`))
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
