package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrDisabled = errors.New("meilisearch tidak dikonfigurasi")

// Document adalah representasi data dokumen yang diindeks ke Meilisearch.
type Document struct {
	ID         string
	Title      string
	Body       string
	AuthorID   string
	AuthorName string
	CreatedAt  time.Time
}

// Client adalah koneksi ke satu index Meilisearch.
type Client struct {
	host       string
	key        string
	index      string
	httpClient *http.Client
}

// New membuat client Meilisearch baru.
func New(host, key, index string) *Client {
	return &Client{
		host:       strings.TrimRight(host, "/"),
		key:        key,
		index:      index,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled bernilai true jika host dan index sudah terisi.
func (c *Client) Enabled() bool {
	return c != nil && c.host != "" && c.index != ""
}

type meiliDoc struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	AuthorID    string `json:"author_id"`
	AuthorName  string `json:"author_name"`
	CreatedAt   string `json:"created_at"`
	CreatedAtTs int64  `json:"created_at_ts"`
}

// IndexArticle mengirim satu dokumen ke Meilisearch index.
func (c *Client) IndexArticle(ctx context.Context, doc Document) error {
	if !c.Enabled() {
		return nil
	}
	if doc.ID == "" {
		return errors.New("id dokumen kosong")
	}
	mdoc := meiliDoc{
		ID:          doc.ID,
		Title:       doc.Title,
		Body:        doc.Body,
		AuthorID:    doc.AuthorID,
		AuthorName:  doc.AuthorName,
		CreatedAt:   doc.CreatedAt.UTC().Format(time.RFC3339),
		CreatedAtTs: doc.CreatedAt.Unix(),
	}
	payload, err := json.Marshal([]meiliDoc{mdoc})
	if err != nil {
		return err
	}
	code, body, err := c.do(ctx, http.MethodPut, "/indexes/"+c.index+"/documents?primaryKey=id", payload)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("meilisearch index: HTTP %d: %s", code, strings.TrimSpace(string(body)))
	}
	return nil
}

type searchRequest struct {
	Q      string   `json:"q"`
	Filter string   `json:"filter,omitempty"`
	Sort   []string `json:"sort"`
	Limit  int      `json:"limit"`
}

type searchResponse struct {
	Hits []struct {
		ID string `json:"id"`
	} `json:"hits"`
}

// SearchIDs mencari di Meilisearch dan mengembalikan daftar ID yang sesuai.
func (c *Client) SearchIDs(ctx context.Context, query string, authorIDs []string, cursorCreatedAt *time.Time, cursorID string, limit int) ([]string, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	req := searchRequest{
		Q:     query,
		Sort:  []string{"created_at_ts:desc", "id:desc"},
		Limit: limit + 1,
	}
	if f := buildFilter(authorIDs, cursorCreatedAt, cursorID); f != "" {
		req.Filter = f
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	code, body, err := c.do(ctx, http.MethodPost, "/indexes/"+c.index+"/search", payload)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("meilisearch search: HTTP %d: %s", code, strings.TrimSpace(string(body)))
	}
	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("gagal baca hasil search: %w", err)
	}
	ids := make([]string, 0, len(resp.Hits))
	for _, h := range resp.Hits {
		ids = append(ids, h.ID)
	}
	return ids, nil
}

// buildFilter membuat string filter Meilisearch dari syarat yang ada.
func buildFilter(authorIDs []string, cursorCreatedAt *time.Time, cursorID string) string {
	parts := []string{}
	if len(authorIDs) > 0 {
		quoted := make([]string, 0, len(authorIDs))
		for _, id := range authorIDs {
			quoted = append(quoted, strconv.Quote(id))
		}
		parts = append(parts, fmt.Sprintf("author_id IN [%s]", strings.Join(quoted, ", ")))
	}
	if cursorCreatedAt != nil {
		ts := cursorCreatedAt.Unix()
		parts = append(parts, fmt.Sprintf("(created_at_ts < %d OR (created_at_ts = %d AND id < %s))", ts, ts, strconv.Quote(cursorID)))
	}
	return strings.Join(parts, " AND ")
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.host+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}
