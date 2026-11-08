package article

import (
	"context"
	"errors"
	"testing"
	"time"

	"kumparan-articles/internal/platform/search"
)

type mockRepository struct {
	createFn    func(ctx context.Context, authorID, title, body string) (*Article, error)
	authorIDsFn func(ctx context.Context, name string) ([]string, error)
	getByIDsFn  func(ctx context.Context, ids []string) ([]*Article, error)
	listFn      func(ctx context.Context, f ListFilter) ([]*Article, error)
}

func (m *mockRepository) Create(ctx context.Context, authorID, title, body string) (*Article, error) {
	if m.createFn != nil {
		return m.createFn(ctx, authorID, title, body)
	}
	return nil, nil
}

func (m *mockRepository) AuthorIDs(ctx context.Context, name string) ([]string, error) {
	if m.authorIDsFn != nil {
		return m.authorIDsFn(ctx, name)
	}
	return nil, nil
}

func (m *mockRepository) GetByIDs(ctx context.Context, ids []string) ([]*Article, error) {
	if m.getByIDsFn != nil {
		return m.getByIDsFn(ctx, ids)
	}
	return nil, nil
}

func (m *mockRepository) List(ctx context.Context, f ListFilter) ([]*Article, error) {
	if m.listFn != nil {
		return m.listFn(ctx, f)
	}
	return nil, nil
}

type mockSearchClient struct {
	enabledFn      func() bool
	indexArticleFn func(ctx context.Context, doc search.Document) error
	searchIDsFn    func(ctx context.Context, query string, authorIDs []string, cursorCreatedAt *time.Time, cursorID string, limit int) ([]string, error)
}

func (m *mockSearchClient) Enabled() bool {
	if m.enabledFn != nil {
		return m.enabledFn()
	}
	return false
}

func (m *mockSearchClient) IndexArticle(ctx context.Context, doc search.Document) error {
	if m.indexArticleFn != nil {
		return m.indexArticleFn(ctx, doc)
	}
	return nil
}

func (m *mockSearchClient) SearchIDs(ctx context.Context, query string, authorIDs []string, cursorCreatedAt *time.Time, cursorID string, limit int) ([]string, error) {
	if m.searchIDsFn != nil {
		return m.searchIDsFn(ctx, query, authorIDs, cursorCreatedAt, cursorID, limit)
	}
	return nil, nil
}

func TestService_CreateIndexesToSearch(t *testing.T) {
	indexed := false
	now := time.Now().UTC()

	repo := &mockRepository{
		createFn: func(ctx context.Context, authorID, title, body string) (*Article, error) {
			return &Article{
				ID:         "art-01",
				AuthorID:   authorID,
				AuthorName: "Budi",
				Title:      title,
				Body:       body,
				CreatedAt:  now,
			}, nil
		},
	}

	searchClient := &mockSearchClient{
		enabledFn: func() bool { return true },
		indexArticleFn: func(ctx context.Context, doc search.Document) error {
			if doc.ID == "art-01" && doc.AuthorName == "Budi" {
				indexed = true
			}
			return nil
		},
	}

	svc := NewService(repo, searchClient)
	art, err := svc.Create(context.Background(), "author-01", "Judul", "Konten")
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if art.ID != "art-01" {
		t.Errorf("art.ID = %q, want art-01", art.ID)
	}
	if !indexed {
		t.Errorf("artikel seharusnya diindeks ke Meilisearch")
	}
}

func TestService_ListFallbackToPostgresOnSearchError(t *testing.T) {
	postgresCalled := false
	repo := &mockRepository{
		authorIDsFn: func(ctx context.Context, name string) ([]string, error) {
			return []string{"author-01"}, nil
		},
		listFn: func(ctx context.Context, f ListFilter) ([]*Article, error) {
			postgresCalled = true
			return []*Article{
				{ID: "art-from-pg", Title: "Postgres Article", CreatedAt: time.Now()},
			}, nil
		},
	}

	searchClient := &mockSearchClient{
		enabledFn: func() bool { return true },
		searchIDsFn: func(ctx context.Context, query string, authorIDs []string, cursorCreatedAt *time.Time, cursorID string, limit int) ([]string, error) {
			return nil, errors.New("meilisearch down")
		},
	}

	svc := NewService(repo, searchClient)
	arts, _, err := svc.List(context.Background(), ListFilter{
		Query: "teknologi",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if !postgresCalled {
		t.Errorf("fallback ke Postgres seharusnya dipanggil saat Meilisearch error")
	}
	if len(arts) != 1 || arts[0].ID != "art-from-pg" {
		t.Errorf("hasil tidak sesuai, dapat %v", arts)
	}
}
