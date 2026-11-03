package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAuthorNotFound = errors.New("author tidak ditemukan")

type Article struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type ListFilter struct {
	Query           string
	Author          string
	CursorCreatedAt *time.Time
	CursorID        string
	Limit           int
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, authorID, title, body string) (*Article, error) {
	var authorName string
	err := r.pool.QueryRow(ctx, "SELECT name FROM authors WHERE id = $1", authorID).Scan(&authorName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAuthorNotFound
		}
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	a := &Article{AuthorID: authorID, AuthorName: authorName, Title: title, Body: body}
	err = r.pool.QueryRow(ctx,
		`INSERT INTO articles (id, author_id, title, body)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, author_id, title, body, created_at`,
		id.String(), authorID, title, body,
	).Scan(&a.ID, &a.AuthorID, &a.Title, &a.Body, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *Repository) List(ctx context.Context, f ListFilter) ([]*Article, error) {
	var authorIDs []string
	if f.Author != "" {
		rows, err := r.pool.Query(ctx, "SELECT id FROM authors WHERE lower(name) = lower($1)", f.Author)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			authorIDs = append(authorIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(authorIDs) == 0 {
			return []*Article{}, nil
		}
	}

	args := []any{}
	conds := []string{}

	if f.Query != "" {
		args = append(args, f.Query)
		conds = append(conds, fmt.Sprintf("a.search_vector @@ plainto_tsquery('simple'::regconfig, $%d)", len(args)))
	}
	if len(authorIDs) > 0 {
		args = append(args, authorIDs)
		conds = append(conds, fmt.Sprintf("a.author_id = ANY($%d)", len(args)))
	}
	if f.CursorCreatedAt != nil {
		args = append(args, *f.CursorCreatedAt, f.CursorID)
		conds = append(conds, fmt.Sprintf("(a.created_at, a.id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	args = append(args, f.Limit+1)

	query := "SELECT a.id, a.author_id, au.name, a.title, a.body, a.created_at FROM articles a JOIN authors au ON au.id = a.author_id"
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY a.created_at DESC, a.id DESC LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	articles := []*Article{}
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.AuthorID, &a.AuthorName, &a.Title, &a.Body, &a.CreatedAt); err != nil {
			return nil, err
		}
		articles = append(articles, &a)
	}
	return articles, rows.Err()
}
