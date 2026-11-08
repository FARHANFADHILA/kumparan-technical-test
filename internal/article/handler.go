package article

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"kumparan-articles/internal/platform/pagination"
	"kumparan-articles/internal/platform/response"
)

type createRequest struct {
	AuthorID string `json:"author_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type listResponse struct {
	Data       []*Article `json:"data"`
	NextCursor *string    `json:"next_cursor"`
}

// Handler menangani request HTTP untuk domain artikel.
type Handler struct {
	svc Service
}

// NewHandler membuat instance Handler artikel baru.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mendaftarkan endpoint HTTP artikel ke ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /articles", h.CreateArticle)
	mux.HandleFunc("GET /articles", h.ListArticles)
}

// CreateArticle menangani pembuatan artikel baru (POST /articles).
func (h *Handler) CreateArticle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1000001)
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			response.Error(w, http.StatusBadRequest, "validation_error", "body terlalu besar, maksimal 1.000.000 byte")
			return
		}
		response.Error(w, http.StatusBadRequest, "invalid_json", "body JSON tidak valid")
		return
	}

	req.AuthorID = strings.TrimSpace(req.AuthorID)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)

	if req.AuthorID == "" || req.Title == "" || req.Body == "" {
		response.Error(w, http.StatusBadRequest, "validation_error", "author_id, title, dan body wajib diisi")
		return
	}
	if len(req.Title) > 1000 {
		response.Error(w, http.StatusBadRequest, "validation_error", "title terlalu panjang, maksimal 1000 byte")
		return
	}
	if len(req.Body) >= 1000000 {
		response.Error(w, http.StatusBadRequest, "validation_error", "body terlalu panjang, maksimal kurang dari 1.000.000 byte")
		return
	}

	art, err := h.svc.Create(r.Context(), req.AuthorID, req.Title, req.Body)
	if err != nil {
		if errors.Is(err, ErrAuthorNotFound) {
			response.Error(w, http.StatusNotFound, "author_not_found", "author_id tidak ditemukan")
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal_error", "terjadi kesalahan server")
		return
	}

	response.JSON(w, http.StatusCreated, art)
}

// ListArticles menangani pencarian dan listing artikel dengan pagination keyset (GET /articles).
func (h *Handler) ListArticles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := ListFilter{
		Query:  strings.TrimSpace(q.Get("query")),
		Author: strings.TrimSpace(q.Get("author")),
		Limit:  10,
	}

	if len(filter.Query) > 1000 {
		response.Error(w, http.StatusBadRequest, "validation_error", "query terlalu panjang, maksimal 1000 byte")
		return
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			response.Error(w, http.StatusBadRequest, "validation_error", "limit harus angka positif")
			return
		}
		if n > 100 {
			n = 100
		}
		filter.Limit = n
	}

	if cur := q.Get("cursor"); cur != "" {
		t, id, err := pagination.DecodeCursor(cur)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_cursor", "cursor tidak valid")
			return
		}
		filter.CursorCreatedAt = &t
		filter.CursorID = id
	}

	arts, nextCursor, err := h.svc.List(r.Context(), filter)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "terjadi kesalahan server")
		return
	}

	response.JSON(w, http.StatusOK, listResponse{
		Data:       arts,
		NextCursor: nextCursor,
	})
}
