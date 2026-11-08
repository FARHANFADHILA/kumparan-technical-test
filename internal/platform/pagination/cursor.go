package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalidCursor = errors.New("cursor tidak valid")

type cursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

// EncodeCursor meng-encode created_at dan id menjadi string cursor base64 URL-safe.
func EncodeCursor(t time.Time, id string) string {
	b, _ := json.Marshal(cursorPayload{CreatedAt: t, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor men-decode string cursor base64 URL-safe menjadi created_at dan id.
func DecodeCursor(s string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", err
	}
	var c cursorPayload
	if err := json.Unmarshal(b, &c); err != nil {
		return time.Time{}, "", err
	}
	if c.ID == "" || c.CreatedAt.IsZero() {
		return time.Time{}, "", ErrInvalidCursor
	}
	return c.CreatedAt, c.ID, nil
}
