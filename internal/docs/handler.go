package docs

import (
	"net/http"
)

const scalarDocsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>kumparan-articles API Docs (Scalar)</title>
</head>
<body>
<script 
  id="api-reference" 
  data-url="/openapi.yaml"
  data-configuration='{"hideModels": true}'>
</script>
<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

// Handler menangani endpoint dokumentasi API (OpenAPI spec dan Scalar UI).
type Handler struct {
	openapiYAML []byte
}

// NewHandler membuat instance Handler dokumentasi.
func NewHandler(openapiYAML []byte) *Handler {
	return &Handler{openapiYAML: openapiYAML}
}

// RegisterRoutes mendaftarkan endpoint dokumentasi ke ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.yaml", h.DownloadOpenAPI)
	mux.HandleFunc("GET /scalar", h.ScalarDocs)
	mux.HandleFunc("GET /scalar/", h.ScalarDocs)
}

// ScalarDocs menampilkan antarmuka web interaktif Scalar docs.
func (h *Handler) ScalarDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(scalarDocsHTML))
}

// DownloadOpenAPI menyajikan file openapi.yaml mentah.
func (h *Handler) DownloadOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(h.openapiYAML)
}
