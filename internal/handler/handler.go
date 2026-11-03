package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kumparan-articles/internal/repository"
)

type Handler struct {
	repo *repository.Repository
}

func New(repo *repository.Repository) *Handler {
	return &Handler{repo: repo}
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
	if c.ID == "" {
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
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (h *Handler) CreateArticle(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	writeJSON(w, http.StatusCreated, art)
}

func (h *Handler) ListArticles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := repository.ListFilter{
		Query:  strings.TrimSpace(q.Get("query")),
		Author: strings.TrimSpace(q.Get("author")),
		Limit:  20,
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

	arts, err := h.repo.List(r.Context(), filter)
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