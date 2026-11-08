package article

import (
	"context"
	"log"
	"time"

	"kumparan-articles/internal/platform/pagination"
	"kumparan-articles/internal/platform/search"
)

// SearchClient mendefinisikan kontrak interaksi dengan mesin pencarian (Meilisearch).
type SearchClient interface {
	Enabled() bool
	IndexArticle(ctx context.Context, doc search.Document) error
	SearchIDs(ctx context.Context, query string, authorIDs []string, cursorCreatedAt *time.Time, cursorID string, limit int) ([]string, error)
}

// Service mendefinisikan usecase / logika bisnis domain artikel.
type Service interface {
	Create(ctx context.Context, authorID, title, body string) (*Article, error)
	List(ctx context.Context, filter ListFilter) ([]*Article, *string, error)
}

type service struct {
	repo   Repository
	search SearchClient
}

// NewService membuat instance Service artikel dengan dependency repository dan search client.
func NewService(repo Repository, searchClient SearchClient) Service {
	return &service{
		repo:   repo,
		search: searchClient,
	}
}

func (s *service) Create(ctx context.Context, authorID, title, body string) (*Article, error) {
	art, err := s.repo.Create(ctx, authorID, title, body)
	if err != nil {
		return nil, err
	}

	// Salinan ke Meilisearch jika aktif
	if s.search != nil && s.search.Enabled() {
		doc := search.Document{
			ID:         art.ID,
			Title:      art.Title,
			Body:       art.Body,
			AuthorID:   art.AuthorID,
			AuthorName: art.AuthorName,
			CreatedAt:  art.CreatedAt,
		}
		if err := s.search.IndexArticle(ctx, doc); err != nil {
			log.Printf("meilisearch index gagal untuk %s: %v (artikel tetap tersimpan di Postgres)", art.ID, err)
		}
	}

	return art, nil
}

func (s *service) List(ctx context.Context, filter ListFilter) ([]*Article, *string, error) {
	var arts []*Article
	var err error

	// Jalur Meilisearch untuk pencarian dengan query ATAU author, jika aktif.
	// Jika Meilisearch gagal, fallback ke Postgres agar client tetap mendapat data.
	if (filter.Query != "" || filter.Author != "") && s.search != nil && s.search.Enabled() {
		arts, err = s.listViaSearch(ctx, filter)
		if err != nil {
			log.Printf("meilisearch search gagal, fallback ke Postgres: %v", err)
			arts, err = s.repo.List(ctx, filter)
		}
	} else {
		arts, err = s.repo.List(ctx, filter)
	}

	if err != nil {
		return nil, nil, err
	}

	var nextCursor *string
	if len(arts) > filter.Limit {
		encoded := pagination.EncodeCursor(arts[filter.Limit-1].CreatedAt, arts[filter.Limit-1].ID)
		nextCursor = &encoded
		arts = arts[:filter.Limit]
	}
	if arts == nil {
		arts = []*Article{}
	}

	return arts, nextCursor, nil
}

func (s *service) listViaSearch(ctx context.Context, filter ListFilter) ([]*Article, error) {
	var authorIDs []string
	if filter.Author != "" {
		ids, err := s.repo.AuthorIDs(ctx, filter.Author)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []*Article{}, nil
		}
		authorIDs = ids
	}

	ids, err := s.search.SearchIDs(ctx, filter.Query, authorIDs, filter.CursorCreatedAt, filter.CursorID, filter.Limit)
	if err != nil {
		return nil, err
	}

	return s.repo.GetByIDs(ctx, ids)
}
