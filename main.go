package main

import (
	"context"
	_ "embed"
	"log"
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

const scalarDocsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>kumparan-articles API Docs (Scalar)</title>
</head>
<body>
<script id="api-reference" data-url="/openapi.yaml"></script>
<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

func main() {
	// untuk config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	// untuk koneksi ke database
	ctx := context.Background()
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
	handler := handler.NewWithSearch(repo, searchClient)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /articles", handler.CreateArticle)
	mux.HandleFunc("GET /articles", handler.ListArticles)
	mux.HandleFunc("GET /authors", handler.ListAuthors)
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(openapiYAML)
	})
	mux.HandleFunc("GET /scalar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(scalarDocsHTML))
	})
	// mux.HandleFunc("GET /scalar/", func(w http.ResponseWriter, r *http.Request) {
	// 	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 	w.Write([]byte(scalarDocsHTML))
	// })
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
