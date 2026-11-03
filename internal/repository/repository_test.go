package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Tes integrasi ini membutuhkan database Postgres terpisah, BUKAN database
// pengembangan utama. Buat dulu database tesnya, misalnya:
//   createdb kumparan_articles_test
// Lalu jalankan dengan:
//   TEST_DATABASE_URL=postgres://postgres:password@localhost:5432/kumparan_articles_test?sslmode=disable go test ./...
// Kalau TEST_DATABASE_URL tidak di-set, semua tes di file ini di-skip,
// sehingga `go test ./...` tetap lolos di mesin tanpa database tes.

const testSchema = `
DROP TABLE IF EXISTS articles CASCADE;
DROP TABLE IF EXISTS authors CASCADE;
CREATE TABLE authors (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL
);
CREATE TABLE articles (
  id TEXT PRIMARY KEY,
  author_id TEXT NOT NULL REFERENCES authors(id),
  title TEXT NOT NULL,
  body TEXT NOT NULL CHECK (length(body) < 1000000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  search_vector TSVECTOR GENERATED ALWAYS AS (
    to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(body, ''))
  ) STORED
);
CREATE INDEX idx_articles_search ON articles USING GIN (search_vector);
CREATE INDEX idx_articles_created ON articles (created_at DESC, id DESC);
CREATE INDEX idx_articles_author_created ON articles (author_id, created_at DESC, id DESC);
CREATE INDEX idx_authors_lower_name ON authors (lower(name));
INSERT INTO authors (id, name) VALUES
  ('author-001', 'Budi Santoso'),
  ('author-002', 'Budi Santoso'),
  ('author-003', 'Siti Rahma')
ON CONFLICT (id) DO NOTHING;
`

func setupTestRepo(t *testing.T) (*Repository, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak di-set, tes integrasi di-skip")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("gagal konek database tes: %v", err)
	}
	if _, err := pool.Exec(ctx, testSchema); err != nil {
		pool.Close()
		t.Fatalf("gagal menyiapkan skema tes: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return New(pool), pool
}

func TestListExpandsDuplicateAuthorNames(t *testing.T) {
	repo, _ := setupTestRepo(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, "author-001", "Artikel Budi pertama", "Isi pertama."); err != nil {
		t.Fatalf("Create author-001 gagal: %v", err)
	}
	if _, err := repo.Create(ctx, "author-002", "Artikel Budi kedua", "Isi kedua."); err != nil {
		t.Fatalf("Create author-002 gagal: %v", err)
	}
	if _, err := repo.Create(ctx, "author-003", "Artikel Siti", "Isi Siti."); err != nil {
		t.Fatalf("Create author-003 gagal: %v", err)
	}

	// Nama dicari dengan huruf kecil: pencocokan harus case-insensitive
	// dan mengembalikan KEDUA Budi, bukan diam-diam hanya satu.
	arts, err := repo.List(ctx, ListFilter{Author: "budi santoso", Limit: 20})
	if err != nil {
		t.Fatalf("List gagal: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("diharapkan 2 artikel Budi, dapat %d", len(arts))
	}
	seen := map[string]bool{}
	for _, a := range arts {
		seen[a.AuthorID] = true
		if a.AuthorName != "Budi Santoso" {
			t.Errorf("author_name salah: %q", a.AuthorName)
		}
	}
	if !seen["author-001"] || !seen["author-002"] {
		t.Errorf("kedua id Budi harus muncul, dapat %v", seen)
	}
}

func TestSearchAfterEditKeepsCreatedAt(t *testing.T) {
	repo, pool := setupTestRepo(t)
	ctx := context.Background()

	art, err := repo.Create(ctx, "author-003", "Judul lama", "Isi lama tentang hujan.")
	if err != nil {
		t.Fatalf("Create gagal: %v", err)
	}
	original := art.CreatedAt

	// Edit judul dan body, created_at TIDAK boleh berubah. search_vector
	// yang digenerate harus ikut berubah sehingga kata baru bisa dicari.
	if _, err := pool.Exec(ctx, "UPDATE articles SET title = $1, body = $2 WHERE id = $3",
		"Judul baru", "Isi baru tentang banjir di Pekanbaru.", art.ID); err != nil {
		t.Fatalf("UPDATE gagal: %v", err)
	}

	var createdAt time.Time
	if err := pool.QueryRow(ctx, "SELECT created_at FROM articles WHERE id = $1", art.ID).Scan(&createdAt); err != nil {
		t.Fatalf("SELECT created_at gagal: %v", err)
	}
	if !createdAt.Equal(original) {
		t.Fatalf("created_at berubah setelah edit: awal %v, sesudah %v", original, createdAt)
	}

	found, err := repo.List(ctx, ListFilter{Query: "banjir", Limit: 20})
	if err != nil {
		t.Fatalf("List query banjir gagal: %v", err)
	}
	if len(found) != 1 || found[0].ID != art.ID {
		t.Fatalf("pencarian kata baru harus menemukan artikel yang diedit, dapat %d hasil", len(found))
	}

	old, err := repo.List(ctx, ListFilter{Query: "hujan", Limit: 20})
	if err != nil {
		t.Fatalf("List query hujan gagal: %v", err)
	}
	if len(old) != 0 {
		t.Fatalf("kata lama seharusnya sudah tidak ditemukan, dapat %d hasil", len(old))
	}
}

func TestKeysetPaginationNoDuplicatesOrMissing(t *testing.T) {
	repo, _ := setupTestRepo(t)
	ctx := context.Background()

	ids := []string{}
	for i := 1; i <= 5; i++ {
		art, err := repo.Create(ctx, "author-001", "Artikel pagination", "Isi pagination.")
		if err != nil {
			t.Fatalf("Create ke-%d gagal: %v", i, err)
		}
		ids = append(ids, art.ID)
		// Jeda kecil agar created_at antar artikel tidak semuanya identik
		// di mesin yang sangat cepat; keyset tetap harus benar walau
		// waktunya sama, karena id dipakai sebagai tie-breaker.
		time.Sleep(2 * time.Millisecond)
	}

	seen := map[string]bool{}
	var cursorTime *time.Time
	var cursorID string
	for page := 1; page <= 4; page++ {
		f := ListFilter{Limit: 2, CursorCreatedAt: cursorTime, CursorID: cursorID}
		arts, err := repo.List(ctx, f)
		if err != nil {
			t.Fatalf("List halaman %d gagal: %v", page, err)
		}
		if len(arts) == 0 {
			break
		}
		// List mengembalikan Limit+1 baris kalau masih ada halaman berikut.
		// Di sini repo.List mengembalikan semua baris itu, jadi handler-lah
		// yang memotongnya; untuk tes ini, ambil maksimal 2 per halaman
		// dengan cara yang sama seperti handler.
		hasNext := len(arts) > 2
		pageArts := arts
		if hasNext {
			pageArts = arts[:2]
		}
		for _, a := range pageArts {
			if seen[a.ID] {
				t.Fatalf("id %s muncul dua kali di pagination", a.ID)
			}
			seen[a.ID] = true
		}
		if !hasNext {
			break
		}
		last := pageArts[len(pageArts)-1]
		cursorTime = &last.CreatedAt
		cursorID = last.ID
	}
	if len(seen) != 5 {
		t.Fatalf("diharapkan 5 artikel unik dari pagination, dapat %d", len(seen))
	}
	// Pastikan tidak ada id di luar yang dibuat di tes ini.
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("id %s hilang dari pagination", id)
		}
	}
}
