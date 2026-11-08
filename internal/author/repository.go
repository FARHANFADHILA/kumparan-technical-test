package author

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository mendefinisikan interface akses data author.
type Repository interface {
	List(ctx context.Context) ([]*Author, error)
	FindIDsByName(ctx context.Context, name string) ([]string, error)
	FindNameByID(ctx context.Context, id string) (string, error)
}

type pgRepository struct {
	pool *pgxpool.Pool
}

// NewRepository membuat instance baru PostgreSQL repository untuk author.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

func (r *pgRepository) List(ctx context.Context) ([]*Author, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name FROM authors ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	authors := []*Author{}
	for rows.Next() {
		var a Author
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		authors = append(authors, &a)
	}
	return authors, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(s)
}

func (r *pgRepository) FindIDsByName(ctx context.Context, name string) ([]string, error) {
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

func (r *pgRepository) FindNameByID(ctx context.Context, id string) (string, error) {
	var name string
	err := r.pool.QueryRow(ctx, "SELECT name FROM authors WHERE id = $1", id).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}
