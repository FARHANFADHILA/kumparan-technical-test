package system

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler menangani endpoint kesehatan dan status sistem.
type Handler struct {
	pool *pgxpool.Pool
}

// NewHandler membuat instance Handler sistem.
func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// RegisterRoutes mendaftarkan endpoint sistem ke ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.HealthCheck)
}

// HealthCheck memeriksa ketersediaan koneksi database dengan timeout 2 detik.
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(pingCtx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
		return
	}

	_, _ = w.Write([]byte(`{"status":"ok", "version":"v1.0.6"}`))
}
