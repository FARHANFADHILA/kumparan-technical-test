package article

import (
	"errors"
	"time"
)

var ErrAuthorNotFound = errors.New("author tidak ditemukan")

// Article merepresentasikan data entitas artikel.
type Article struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListFilter memuat parameter filter dan pagination untuk query daftar artikel.
type ListFilter struct {
	Query           string
	Author          string
	CursorCreatedAt *time.Time
	CursorID        string
	Limit           int
}
