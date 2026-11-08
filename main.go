package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"time"

	"kumparan-articles/internal/article"
	"kumparan-articles/internal/author"
	"kumparan-articles/internal/config"
	"kumparan-articles/internal/docs"
	"kumparan-articles/internal/platform/database"
	"kumparan-articles/internal/platform/search"
	"kumparan-articles/internal/system"
)

//go:embed openapi.yaml
var openapiYAML []byte

func main() {
	// 1. Muat konfigurasi environment
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("gagal memuat konfigurasi: %v", err)
	}

	ctx := context.Background()

	// 2. Inisialisasi pool koneksi database PostgreSQL
	pool, err := database.NewPostgresPool(ctx, cfg)
	if err != nil {
		log.Fatalf("gagal inisialisasi database: %v", err)
	}
	defer pool.Close()

	// 3. Inisialisasi search client (Meilisearch)
	searchClient := search.New(cfg.MeiliHost, cfg.MeiliMasterKey, cfg.MeiliIndex)
	if searchClient.Enabled() {
		log.Printf("meilisearch aktif: %s index %s (pencarian dengan query pakai Meilisearch, fallback ke Postgres kalau gagal)", cfg.MeiliHost, cfg.MeiliIndex)
	} else {
		log.Printf("meilisearch tidak dikonfigurasi (MEILI_HOST kosong), pencarian pakai Postgres saja")
	}

	// 4. Inisialisasi per fitur (Feature-Based Architecture)
	// Fitur Author
	authorRepo := author.NewRepository(pool)
	authorHandler := author.NewHandler(authorRepo)

	// Fitur Article
	articleRepo := article.NewRepository(pool)
	articleService := article.NewService(articleRepo, searchClient)
	articleHandler := article.NewHandler(articleService)

	// Fitur Docs & System
	docsHandler := docs.NewHandler(openapiYAML)
	systemHandler := system.NewHandler(pool)

	// 5. Registrasi router
	mux := http.NewServeMux()
	articleHandler.RegisterRoutes(mux)
	authorHandler.RegisterRoutes(mux)
	docsHandler.RegisterRoutes(mux)
	systemHandler.RegisterRoutes(mux)

	// 6. Jalankan HTTP server
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("server jalan di :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server berhenti dengan error: %v", err)
	}
}
