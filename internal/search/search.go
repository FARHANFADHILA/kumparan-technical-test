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

	"kumparan-articles/internal/repository"
)

// ErrDisabled berarti Meilisearch belum dikonfigurasi (host atau index
// kosong). Handler yang memakai paket ini harus fallback ke Postgres
// kalau dapat error ini, bukan mengembalikan 500 ke frontend.
var ErrDisabled = errors.New("meilisearch tidak dikonfigurasi")

// Client adalah koneksi ke satu index Meilisearch.
// Host contoh: http://localhost:7700 (di laptop) atau
// http://meilisearch:7700 (di dalam jaringan Docker Compose).
type Client struct {
	host       string
	key        string
	index      string
	httpClient *http.Client
}

// New membuat client baru. Tidak ada koneksi yang dibuka di sini,
// koneksi baru dibuka waktu request pertama, jadi New tidak akan
// gagal hanya karena Meilisearch sedang mati.
func New(host, key, index string) *Client {
	return &Client{
		host:       strings.TrimRight(host, "/"),
		key:        key,
		index:      index,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled benar kalau host dan index sudah diisi. Kalau tidak,
// IndexArticle menjadi no-op dan SearchIDs mengembalikan ErrDisabled.
func (c *Client) Enabled() bool {
	return c != nil && c.host != "" && c.index != ""
}

// meiliDoc adalah bentuk dokumen yang dikirim ke Meilisearch.
// created_at_ts dalam detik, SAMA dengan bulk, lihat catatan di atas.
type meiliDoc struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	AuthorID    string `json:"author_id"`
	AuthorName  string `json:"author_name"`
	CreatedAt   string `json:"created_at"`
	CreatedAtTs int64  `json:"created_at_ts"`
}

// IndexArticle mengirim satu artikel ke Meilisearch.
// Ia hanya mengirim (PUT documents), tidak menunggu task-nya selesai,
// karena pengindeksan Meilisearch itu asynchronous: server menjawab
// 202 Accepted begitu dokumen diterima, lalu mengindeks di belakang
// layar, biasanya kurang dari 1 detik untuk satu dokumen.
// Kalau PUT-nya gagal (jaringan putus, kunci salah, index tidak ada),
// error dikembalikan supaya handler bisa mencatatnya di log, tapi
// handler TIDAK boleh membatalkan POST Postgres hanya karena ini
// gagal, karena data asli sudah aman di Postgres.
func (c *Client) IndexArticle(ctx context.Context, a *repository.Article) error {
	if !c.Enabled() {
		return nil
	}
	if a == nil {
		return errors.New("artikel nil")
	}
	doc := meiliDoc{
		ID:          a.ID,
		Title:       a.Title,
		Body:        a.Body,
		AuthorID:    a.AuthorID,
		AuthorName:  a.AuthorName,
		CreatedAt:   a.CreatedAt.UTC().Format(time.RFC3339),
		CreatedAtTs: a.CreatedAt.Unix(),
	}
	payload, err := json.Marshal([]meiliDoc{doc})
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

// searchRequest adalah body POST /indexes/{index}/search.
// Filter dikirim sebagai satu string, dengan AND di antara syarat,
// dan OR di dalam syarat cursor. Limit di sini sudah limit+1,
// supaya pemanggil bisa tahu apakah masih ada halaman berikutnya.
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

// SearchIDs mencari di Meilisearch dan mengembalikan daftar id saja,
// dalam urutan Meilisearch (created_at_ts terbaru, lalu id terbaru).
// Artikel lengkapnya TIDAK dikembalikan di sini, pemanggil harus
// mengambilnya dari Postgres dengan id-id ini, lalu menyusun ulang
// sesuai urutan ini, karena Postgres mengembalikan id dalam urutan
// acak kalau memakai WHERE id = ANY(...).
//
// Parameter cursorCreatedAt dan cursorID berasal dari cursor handler
// lama (decodeCursor). Kalau cursorCreatedAt nil, berarti halaman
// pertama, tidak ada filter tanggal.
// Limit yang diminta adalah limit frontend (misalnya 20), fungsi ini
// meminta limit+1 ke Meilisearch, supaya pemanggil bisa membedakan
// "sudah habis" (dapat <= limit) dengan "masih ada" (dapat limit+1).
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
		return nil, fmt.Errorf("gagal baca hasil search: %v", err)
	}
	ids := make([]string, 0, len(resp.Hits))
	for _, h := range resp.Hits {
		ids = append(ids, h.ID)
	}
	return ids, nil
}

// buildFilter membuat string filter Meilisearch dari syarat yang ada.
// Contoh hasil, kalau ada author dan cursor:
//   author_id IN ["author-001", "author-002"] AND (created_at_ts < 1767578445 OR (created_at_ts = 1767578445 AND id < "synth-0000005"))
//
// Syarat cursor ini penting untuk infinite scroll: ambil yang
// tanggalnya lebih lama dari pembatas, ATAU tanggalnya sama persis
// tapi id-nya lebih kecil dari pembatas. Tanpa bagian OR itu,
// artikel yang tanggalnya sama persis dengan pembatas akan hilang
// di halaman berikutnya.
//
// TES WAJIB SEBELUM HANDLER DIALIHKAN, di Postman:
//   POST /indexes/articles/search, body:
//   {"q": "banjir", "filter": "created_at_ts < 1790000000 OR (created_at_ts = 1790000000 AND id < \"synth-0000100\")", "sort": ["created_at_ts:desc", "id:desc"], "limit": 20}
//   Kalau Meilisearch menolak id < "...", jangan pakai paket ini
//   untuk halaman berikutnya dulu, kirim error-nya, dan kita ubah
//   strateginya (misalnya filter created_at_ts saja + batasan tie
//   yang dicatat di README).
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

// settingsRequest adalah setting index yang harus ada sebelum
// SearchIDs dipakai dengan sort dan filter. Tanpa setting ini,
// Meilisearch akan menolak sort/filter dengan error "not sortable"
// atau "not filterable", bukan diam-diam mengembalikan hasil salah.
type settingsRequest struct {
	SearchableAttributes []string `json:"searchableAttributes"`
	FilterableAttributes []string `json:"filterableAttributes"`
	SortableAttributes   []string `json:"sortableAttributes"`
}

type taskCreated struct {
	TaskUID int `json:"taskUid"`
}

type taskStatus struct {
	Status string `json:"status"`
	Error  *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// SetSettings mengirim setting index dan menunggu task-nya sampai
// succeeded. Fungsi ini dipanggil sekali waktu setup, BUKAN per
// request, karena setiap perubahan setting membuat Meilisearch
// mengindeks ulang dan task-nya asynchronous.
func (c *Client) SetSettings(ctx context.Context) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	payload, err := json.Marshal(settingsRequest{
		SearchableAttributes: []string{"title", "body"},
		FilterableAttributes: []string{"author_id", "created_at_ts", "id"},
		SortableAttributes:   []string{"created_at_ts", "id"},
	})
	if err != nil {
		return err
	}
	code, body, err := c.do(ctx, http.MethodPatch, "/indexes/"+c.index+"/settings", payload)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("meilisearch settings: HTTP %d: %s", code, strings.TrimSpace(string(body)))
	}
	var created taskCreated
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("gagal baca taskUid settings: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		code, body, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/tasks/%d", created.TaskUID), nil)
		if err != nil {
			return err
		}
		if code < 200 || code >= 300 {
			return fmt.Errorf("meilisearch cek task settings: HTTP %d: %s", code, strings.TrimSpace(string(body)))
		}
		var st taskStatus
		if err := json.Unmarshal(body, &st); err != nil {
			return err
		}
		switch st.Status {
		case "succeeded":
			return nil
		case "failed", "canceled":
			if st.Error != nil {
				return fmt.Errorf("task settings %s: %s (%s)", st.Status, st.Error.Message, st.Error.Code)
			}
			return fmt.Errorf("task settings %s", st.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return fmt.Errorf("task settings %d tidak selesai dalam 60 detik", created.TaskUID)
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
