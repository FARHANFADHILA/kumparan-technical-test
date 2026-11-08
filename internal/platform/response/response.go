package response

import (
	"encoding/json"
	"net/http"
)

// ErrorEnvelope mendefinisikan format standar respon error API.
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail memuat kode dan pesan error.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON mengirimkan payload JSON dengan status code HTTP yang diberikan.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Error mengirimkan respon error berformat JSON standar:
// {"error": {"code": "...", "message": "..."}}
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorEnvelope{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}
