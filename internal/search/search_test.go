package search

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kumparan-articles/internal/repository"
)

func TestBuildFilter(t *testing.T) {
	if got := buildFilter(nil, nil, ""); got != "" {
		t.Fatalf("tanpa syarat harus kosong, dapat %q", got)
	}
	got := buildFilter([]string{"author-001", "author-002"}, nil, "")
	want := `author_id IN ["author-001", "author-002"]`
	if got != want {
		t.Fatalf("author saja: dapat %q, mau %q", got, want)
	}
	ts := time.Date(2026, 1, 5, 2, 0, 45, 0, time.UTC)
	got = buildFilter(nil, &ts, "synth-0000005")
	// Unix (detik) untuk waktu di atas adalah 1767578445.
	want = `(created_at_ts < 1767578445 OR (created_at_ts = 1767578445 AND id < "synth-0000005"))`
	if got != want {
		t.Fatalf("cursor saja: dapat %q, mau %q", got, want)
	}
	got = buildFilter([]string{"author-001"}, &ts, "synth-0000005")
	if !strings.Contains(got, "author_id IN") || !strings.Contains(got, "created_at_ts <") {
		t.Fatalf("author + cursor harus gabung dengan AND, dapat %q", got)
	}
}

func TestSearchIDs_OrderPreservedAndLimitPlusOne(t *testing.T) {
	var gotReq searchRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/indexes/articles/search" {
			t.Errorf("method/path salah: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits": [{"id": "synth-0000003"}, {"id": "synth-0000001"}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "kunci-tes", "articles")
	ids, err := c.SearchIDs(context.Background(), "riau", nil, nil, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer kunci-tes" {
		t.Fatalf("Authorization salah: %q", gotAuth)
	}
	if gotReq.Q != "riau" || gotReq.Limit != 21 {
		t.Fatalf("request salah: q=%q limit=%d, mau q=riau limit=21", gotReq.Q, gotReq.Limit)
	}
	if len(gotReq.Sort) != 2 || gotReq.Sort[0] != "created_at_ts:desc" || gotReq.Sort[1] != "id:desc" {
		t.Fatalf("sort salah: %v", gotReq.Sort)
	}
	// Urutan harus sama persis dengan urutan Meilisearch, bukan diurutkan ulang.
	if len(ids) != 2 || ids[0] != "synth-0000003" || ids[1] != "synth-0000001" {
		t.Fatalf("urutan id salah: %v", ids)
	}
}

func TestIndexArticle_RequestShape(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotDocs []meiliDoc
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.String(), r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotDocs)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"taskUid": 1, "status": "enqueued"}`))
	}))
	defer srv.Close()

	created := time.Date(2026, 1, 5, 2, 0, 45, 0, time.UTC)
	c := New(srv.URL, "kunci-tes", "articles")
	err := c.IndexArticle(context.Background(), &repository.Article{
		ID: "synth-0000005", AuthorID: "author-004", AuthorName: "Kumparan Sindikasi",
		Title: "Pasar Riau", Body: "Isi", CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || !strings.HasPrefix(gotPath, "/indexes/articles/documents") {
		t.Fatalf("method/path salah: %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer kunci-tes" {
		t.Fatalf("Authorization salah: %q", gotAuth)
	}
	if len(gotDocs) != 1 || gotDocs[0].ID != "synth-0000005" || gotDocs[0].CreatedAtTs != created.Unix() {
		t.Fatalf("dokumen salah: %+v", gotDocs)
	}
}

func TestDisabled(t *testing.T) {
	c := New("", "", "")
	if c.Enabled() {
		t.Fatal("client tanpa host/index harus tidak enabled")
	}
	if err := c.IndexArticle(context.Background(), &repository.Article{}); err != nil {
		t.Fatalf("IndexArticle saat disabled harus no-op, dapat %v", err)
	}
	if _, err := c.SearchIDs(context.Background(), "riau", nil, nil, "", 20); !errors.Is(err, ErrDisabled) {
		t.Fatalf("SearchIDs saat disabled harus ErrDisabled, dapat %v", err)
	}
}
