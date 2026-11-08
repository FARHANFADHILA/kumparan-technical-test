package article

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository mendefinisikan interface operasi database untuk artikel.
type Repository interface {
	Create(ctx context.Context, authorID, title, body string) (*Article, error)
	AuthorIDs(ctx context.Context, name string) ([]string, error)
	GetByIDs(ctx context.Context, ids []string) ([]*Article, error)
	List(ctx context.Context, f ListFilter) ([]*Article, error)
}

type pgRepository struct {
	pool *pgxpool.Pool
}

// NewRepository membuat instance baru PostgreSQL repository untuk artikel.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

func (r *pgRepository) Create(ctx context.Context, authorID, title, body string) (*Article, error) {
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

	a := &Article{
		AuthorID:   authorID,
		AuthorName: authorName,
		Title:      title,
		Body:       body,
	}
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

func escapeLike(s string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(s)
}

func (r *pgRepository) AuthorIDs(ctx context.Context, name string) ([]string, error) {
	rows, err := r.pool.Query(ctx, "SELECT id FROM authors WHERE lower(name) LIKE '%' || lower($1) || '%' ESCAPE '!'", escapeLike(name))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	authorIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		authorIDs = append(authorIDs, id)
	}
	return authorIDs, rows.Err()
}

func (r *pgRepository) GetByIDs(ctx context.Context, ids []string) ([]*Article, error) {
	if len(ids) == 0 {
		return []*Article{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT a.id, a.author_id, au.name, a.title, a.body, a.created_at FROM articles a JOIN authors au ON au.id = a.author_id WHERE a.id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[string]*Article{}
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.AuthorID, &a.AuthorName, &a.Title, &a.Body, &a.CreatedAt); err != nil {
			return nil, err
		}
		art := a
		byID[art.ID] = &art
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	articles := []*Article{}
	for _, id := range ids {
		if a, ok := byID[id]; ok {
			articles = append(articles, a)
		}
	}
	return articles, nil
}

func (r *pgRepository) List(ctx context.Context, f ListFilter) ([]*Article, error) {
	var authorIDs []string
	if f.Author != "" {
		ids, err := r.AuthorIDs(ctx, f.Author)
		if err != nil {
			return nil, err
		}
		authorIDs = ids
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

	query := ""
	if f.Query != "" {
		query = "WITH matches AS MATERIALIZED (SELECT a.id FROM articles a"
		if len(conds) > 0 {
			query += " WHERE " + strings.Join(conds, " AND ")
		}
		query += ") SELECT a.id, a.author_id, au.name, a.title, a.body, a.created_at FROM articles a JOIN matches m ON m.id = a.id JOIN authors au ON au.id = a.author_id"
	} else {
		query = "SELECT a.id, a.author_id, au.name, a.title, a.body, a.created_at FROM articles a JOIN authors au ON au.id = a.author_id"
		if len(conds) > 0 {
			query += " WHERE " + strings.Join(conds, " AND ")
		}
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
