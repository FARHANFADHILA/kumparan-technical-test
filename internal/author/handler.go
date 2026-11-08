package author

import (
	"net/http"

	"kumparan-articles/internal/platform/response"
)

// Handler menangani HTTP request untuk domain author.
type Handler struct {
	repo Repository
}

// NewHandler membuat instance HTTP handler baru untuk author.
func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

// RegisterRoutes mendaftarkan endpoint HTTP author ke ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /authors", h.ListAuthors)
}

// ListAuthors mengembalikan daftar semua author terurut berdasarkan id ASC.
func (h *Handler) ListAuthors(w http.ResponseWriter, r *http.Request) {
	authors, err := h.repo.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "terjadi kesalahan server")
		return
	}
	if authors == nil {
		authors = []*Author{}
	}
	response.JSON(w, http.StatusOK, map[string]any{"data": authors})
}
