package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kumparan-articles/internal/repository"
	"kumparan-articles/internal/search"
)

type Handler struct {
	repo   *repository.Repository
	search *search.Client
}

func New(repo *repository.Repository) *Handler {
	return &Handler{repo: repo}
}

// NewWithSearch sama seperti New, ditambah client Meilisearch.
// Kalau client-nya nil atau tidak enabled, handler tetap jalan
// dengan Postgres saja, jadi konstruktor lama di tes tidak rusak.
func NewWithSearch(repo *repository.Repository, searchClient *search.Client) *Handler {
	return &Handler{repo: repo, search: searchClient}
}

type createRequest struct {
	AuthorID string `json:"author_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type listResponse struct {
	Data       []*repository.Article `json:"data"`
	NextCursor *string               `json:"next_cursor"`
}

type cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeCursor(t time.Time, id string) string {
	b, _ := json.Marshal(cursor{CreatedAt: t, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", err
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return time.Time{}, "", err
	}
	if c.ID == "" || c.CreatedAt.IsZero() {
		return time.Time{}, "", errors.New("cursor tidak valid")
	}
	return c.CreatedAt, c.ID, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (h *Handler) CreateArticle(w http.ResponseWriter, r *http.Request) {
	// Batasi body sebelum di-decode: 1.000.000 byte + 1 byte pendeteksi
	// kelebihan. Tanpa ini, validasi panjang body di bawah baru jalan
	// setelah seluruh body (yang bisa sangat besar) dibaca ke memori.
	r.Body = http.MaxBytesReader(w, r.Body, 1000001)
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusBadRequest, "validation_error", "body terlalu besar, maksimal 1.000.000 byte")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "body JSON tidak valid")
		return
	}
	req.AuthorID = strings.TrimSpace(req.AuthorID)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)

	if req.AuthorID == "" || req.Title == "" || req.Body == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "author_id, title, dan body wajib diisi")
		return
	}
	if len(req.Title) > 1000 {
		writeError(w, http.StatusBadRequest, "validation_error", "title terlalu panjang, maksimal 1000 byte")
		return
	}
	if len(req.Body) >= 1000000 {
		writeError(w, http.StatusBadRequest, "validation_error", "body terlalu panjang, maksimal kurang dari 1.000.000 byte")
		return
	}

	art, err := h.repo.Create(r.Context(), req.AuthorID, req.Title, req.Body)
	if err != nil {
		if errors.Is(err, repository.ErrAuthorNotFound) {
			writeError(w, http.StatusNotFound, "author_not_found", "author_id tidak ditemukan")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "terjadi kesalahan server")
		return
	}
	// Salinan ke Meilisearch, kalau dikonfigurasi. Gagal di sini TIDAK
	// membatalkan POST, karena data asli sudah aman di Postgres.
	// Artikel yang gagal diindeks bisa diindeks ulang lewat bulk.
	if h.search != nil && h.search.Enabled() {
		if err := h.search.IndexArticle(r.Context(), art); err != nil {
			log.Printf("meilisearch index gagal untuk %s: %v (artikel tetap tersimpan di Postgres)", art.ID, err)
		}
	}
	writeJSON(w, http.StatusCreated, art)
}

// listViaSearch adalah jalur Meilisearch untuk GET dengan query:
// cari id di Meilisearch, ambil artikel lengkap dari Postgres,
// susun ulang sesuai urutan Meilisearch. Bentuk hasil dan logika
// limit+1-nya sama persis dengan repo.List, supaya handler di bawah
// bisa memakai logika next_cursor yang sama tanpa perubahan.
func (h *Handler) listViaSearch(ctx context.Context, filter repository.ListFilter) ([]*repository.Article, error) {
	var authorIDs []string
	if filter.Author != "" {
		ids, err := h.repo.AuthorIDs(ctx, filter.Author)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []*repository.Article{}, nil
		}
		authorIDs = ids
	}
	ids, err := h.search.SearchIDs(ctx, filter.Query, authorIDs, filter.CursorCreatedAt, filter.CursorID, filter.Limit)
	if err != nil {
		return nil, err
	}
	return h.repo.GetByIDs(ctx, ids)
}

func (h *Handler) ListArticles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := repository.ListFilter{
		Query:  strings.TrimSpace(q.Get("query")),
		Author: strings.TrimSpace(q.Get("author")),
		Limit:  20,
	}
	if len(filter.Query) > 1000 {
		writeError(w, http.StatusBadRequest, "validation_error", "query terlalu panjang, maksimal 1000 byte")
		return
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "validation_error", "limit harus angka positif")
			return
		}
		if n > 100 {
			n = 100
		}
		filter.Limit = n
	}
	if cur := q.Get("cursor"); cur != "" {
		t, id, err := decodeCursor(cur)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor tidak valid")
			return
		}
		filter.CursorCreatedAt = &t
		filter.CursorID = id
	}

	var arts []*repository.Article
	var err error
	// Jalur Meilisearch hanya untuk GET dengan query dan hanya kalau
	// client dikonfigurasi. Kalau Meilisearch gagal apa pun alasannya,
	// fallback ke Postgres (MATERIALIZED), bukan 500, supaya frontend
	// tetap dapat hasil dengan bentuk yang sama.
	if filter.Query != "" && h.search != nil && h.search.Enabled() {
		arts, err = h.listViaSearch(r.Context(), filter)
		if err != nil {
			log.Printf("meilisearch search gagal, fallback ke Postgres: %v", err)
			arts, err = h.repo.List(r.Context(), filter)
		}
	} else {
		arts, err = h.repo.List(r.Context(), filter)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "terjadi kesalahan server")
		return
	}

	var next *string
	if len(arts) > filter.Limit {
		s := encodeCursor(arts[filter.Limit-1].CreatedAt, arts[filter.Limit-1].ID)
		next = &s
		arts = arts[:filter.Limit]
	}
	if arts == nil {
		arts = []*repository.Article{}
	}
	writeJSON(w, http.StatusOK, listResponse{Data: arts, NextCursor: next})
}
