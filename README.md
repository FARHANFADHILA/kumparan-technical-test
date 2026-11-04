# kumparan-articles

Layanan artikel ringan untuk technical test Backend Engineer kumparan (MagangHub Batch 3).
Dua endpoint saja, sesuai soal: `POST /articles` dan `GET /articles`, dengan filter
`query` (pencarian di judul dan body) dan `author` (filter berdasarkan nama penulis),
diurutkan terbaru dulu.

## Menjalankan dari nol

### Prasyarat

- Go 1.25 atau lebih baru (proyek ini memakai toolchain Go 1.25; `go get` terbaru
  sempat beralih otomatis dari Go 1.23.6 ke toolchain 1.26).
- PostgreSQL 14 atau lebih baru, lokal atau lewat Docker Compose di bawah.

### 1. Siapkan database

Jalankan migrasi dan seed penulis. Di Windows, dari folder proyek, dengan
PostgreSQL lokal:

```cmd
"D:\postgre\18\bin\psql.exe" -U postgres -d kumparan_articles -f migrations\001_init.sql
"D:\postgre\18\bin\psql.exe" -U postgres -d kumparan_articles -f migrations\002_seed.sql
```

Nama file migrasi harus `001_init.sql`. Kalau di folder `migrations` masih ada
`001_iit.sql` (salah ketik lama), ganti dulu namanya dengan
`ren migrations\001_iit.sql 001_init.sql`.

### 2. Siapkan konfigurasi

Salin `.env.example` menjadi `.env`, lalu isi password database lokalmu.
Jangan pernah meng-commit `.env` (sudah ada di `.gitignore`).
Kalau tidak memakai `.env`, set variabelnya di shell sebelum menjalankan server,
misalnya di CMD: `set DATABASE_URL=postgres://postgres:password@localhost:5432/kumparan_articles?sslmode=disable`

Isi `.env`:

```
DATABASE_URL=postgres://postgres:password@localhost:5432/kumparan_articles?sslmode=disable
PORT=8080
```

### 3. Jalankan server

```cmd
go vet ./...
go run .
```

Server jalan di `:8080` (atau sesuai `PORT`).

### 4. Tes cepat

Di PowerShell, pakai `Invoke-RestMethod` (jangan `curl` polos, karena di
PowerShell `curl` adalah alias `Invoke-WebRequest` dan sintaks `-H`/`-d`-nya
berbeda; kalau tetap mau `curl`, pakai `curl.exe` di CMD):

```powershell
Invoke-RestMethod -Uri http://localhost:8080/health
Invoke-RestMethod -Method Post -Uri http://localhost:8080/articles -ContentType "application/json" -Body '{"author_id":"author-001","title":"Uji artikel pertama","body":"Ini isi artikel uji tentang banjir di Pekanbaru."}'
Invoke-RestMethod -Uri "http://localhost:8080/articles?limit=5"
Invoke-RestMethod -Uri "http://localhost:8080/articles?query=banjir"
Invoke-RestMethod -Uri "http://localhost:8080/articles?author=Budi%20Santoso"
```

Di CMD, perintah yang setara memakai `curl.exe` dengan JSON yang di-escape
(`\"author_id\":\"author-001\"`, dan seterusnya).

## Menjalankan database lewat Docker (untuk reviewer tanpa Postgres lokal)

File `docker-compose.yml` hanya menjalankan database, bukan aplikasinya,
supaya reviewer tetap bisa membaca kode Go-nya dan menjalankannya sendiri.
Port host sengaja `5433`, bukan `5432`, supaya tidak bentrok dengan
Postgres lokal yang sudah memakai `5432`.

```cmd
docker compose up -d
```

Lalu, dengan database Docker, jalankan aplikasinya dengan:

```
DATABASE_URL=postgres://kumparan:kumparan_dev@localhost:5433/kumparan_articles?sslmode=disable
```

Password `kumparan_dev` di Compose hanya untuk pengembangan lokal, bukan
password produksi. Volume `pgdata` membuat data bertahan walau container
dihentikan. Folder `migrations` di-mount ke `/docker-entrypoint-initdb.d`,
sehingga Postgres menjalankan `001_init.sql` lalu `002_seed.sql` (urut abjad)
hanya pada inisialisasi pertama. Kalau migrasi diubah setelah volume dibuat,
jalankan `docker compose down -v` lalu `up` lagi untuk inisialisasi ulang.

## API

### POST /articles

Body yang dikirim klien hanya:

```json
{"author_id": "author-001", "title": "Judul", "body": "Isi artikel."}
```

Klien TIDAK mengirim `id` dan TIDAK mengirim `created_at`. Server yang
membuat `id` (UUIDv7 sebagai string) dan database yang mengisi `created_at`
dengan `now()`. Penulis harus sudah ada (tidak ada `POST /authors` di soal);
kalau `author_id` tidak ditemukan, responsnya 404 `author_not_found`, bukan
penulis dibuat diam-diam. Respons sukses 201 berisi `id`, `author_id`,
`author_name`, `title`, `body`, dan `created_at`.

Bentuk error selalu sama: `{"error": {"code": "...", "message": "..."}}`.

### GET /articles

Parameter: `query` (opsional), `author` (opsional, nama penulis),
`limit` (default 20, maksimal 100), `cursor` (opsional, untuk halaman berikut).

Respons: `{"data": [...], "next_cursor": "..."}` dengan `next_cursor: null`
kalau sudah halaman terakhir. Tidak ada total `COUNT(*)` dan tidak ada
nomor halaman, sengaja, karena menghitung total untuk penulis sindikasi
yang punya jutaan artikel akan sangat mahal.

Pagination memakai keyset, bukan OFFSET: cursor berisi artikel terakhir
di halaman (`created_at` + `id`), dan halaman berikut memakai
`WHERE (created_at, id) < (cursor_created_at, cursor_id)`
dengan `ORDER BY created_at DESC, id DESC`. `id` dipakai sebagai tie-breaker
karena banyak artikel bisa berbagi `created_at` yang sama. Server mengambil
`limit+1` baris; baris ekstra itu hanya penanda masih ada halaman berikut.

## Keputusan desain

- **REST dengan Go standard library (`net/http`), bukan framework.**
  Endpoint soal sudah berbentuk HTTP, reviewer bisa mengetes dengan
  curl/Postman, tidak ada streaming dan tidak ada kontrak antar-layanan
  internal yang membutuhkan gRPC, dan hanya ada satu resource dengan
  field tetap sehingga GraphQL hanya menambah kompleksitas dan risiko N+1.
  Go 1.22+ sudah bisa membedakan `POST /articles` dan `GET /articles`
  lewat ServeMux bawaan.
- **PostgreSQL + `pgx` dengan SQL mentah, tanpa ORM.**
  `pgx` adalah driver, bukan ORM: ia tidak membuat SQL dari struct.
  PostgreSQL dipilih sekaligus sebagai database dan mesin pencari
  (full-text search bawaan), sehingga hanya satu layanan yang perlu
  dijalankan. Menambah Elasticsearch/OpenSearch akan memaksa penjelasan
  sinkronisasi saat artikel dibuat/diubah, kalau tidak, hasil pencariannya
  basi. Itu dicatat sebagai future work, bukan dibangun sekarang.
- **`id` tetap TEXT sesuai skema soal, diisi UUIDv7 di server.**
  Mengubah tipe kolom sendiri berisiko: id eksternal/sindikasi tidak
  dijamin berbentuk UUID (misalnya `author-1` adalah TEXT yang valid),
  dan harness reviewer bisa saja memasukkan id string sederhana.
  Mengisi TEXT dengan UUIDv4 acak adalah jebakan performa: sisipan acak
  menyebar di B-tree primary key, menyebabkan page split dan fragmentasi
  di 50 juta baris. UUIDv7 yang terurut waktu menghindari itu tanpa
  mengubah tipe yang diberikan. Trade-off-nya jujur: kolom `uuid`
  16 byte vs UUID sebagai teks ~37 byte, selisihnya kira-kira 3-4 GB
  indeks tambahan di 50 juta baris. Untuk proyek greenfield tanpa skema
  yang diberikan dan semua id dijamin UUID, `uuid` lebih hemat, dan
  migrasinya harus bertahap (kolom baru, backfill per batch, validasi,
  tukar). Untuk soal ini, kepatuhan skema dan fleksibilitas id eksternal
  lebih penting daripada penghematan itu.
- **Pencarian memakai konfigurasi `simple`, bukan stemmer satu bahasa.**
  Kontennya campuran Indonesia dan Inggris, bahkan bisa tercampur dalam
  satu artikel, dan bahasa tidak dicatat di skema. Konfigurasi Inggris
  akan merusak kata Indonesia dan sebaliknya. Keterbatasan yang harus
  ditulis jujur: `simple` tidak melakukan stemming, jadi mencari `makan`
  tidak menemukan `memakan`, dan `berlari` tidak menemukan `lari`.
  PostgreSQL bawaan juga tidak menyediakan konfigurasi `indonesian`;
  itu butuh kamus kustom (misalnya berbasis Sastrawi) yang akan merusak
  reproduksibilitas satu perintah untuk reviewer.
  `search_vector` adalah kolom generated STORED dari judul + body dengan
  indeks GIN. Biayanya dibayar sekali saat tulis, dan dibaca 100 kali
  lebih sering daripada tulis, jadi STORED masuk akal di rasio ini.
  Tidak ada ranking (`setweight`/`ts_rank`) karena soal memerintahkan
  urutan terbaru dulu, bukan relevansi. Memberi bobot tanpa mengubah
  urutan tidak ada efeknya; mengubah urutan akan melanggar soal.
- **Filter penulis: partial contains, mengembang ke SEMUA id yang namanya mengandung fragment itu.**
  Nama penulis tidak unik di soal, jadi `author=Budi` harus mengembalikan
  artikel dari semua `authors.id` yang namanya mengandung "Budi" (misalnya
  kedua Budi Santoso), lewat `author_id = ANY($ids)`, tidak pernah diam-diam
  memilih satu id. Pencocokan case-insensitive dengan
  `lower(name) LIKE '%' || lower($1) || '%'`, dengan wildcard `%`/`_` dari
  input di-escape (karakter escape `!`) supaya input seperti `%` tidak
  mencocokkan semua penulis. Index trigram
  `idx_authors_name_trgm ON authors USING gin (lower(name) gin_trgm_ops)`
  (extension `pg_trgm`, lihat `migrations/001_init.sql`) melayani pencarian
  contains ini tanpa sequential scan; index `lower(name)` lama tetap ada.
- **Kolom `NOT NULL`, autovacuum agresif, dan guard body.**
  `DEFAULT now()` saja masih mengizinkan NULL eksplisit, dan NULL akan
  merusak pagination keyset karena perbandingan dengan NULL tidak pernah
  benar. `author_id` dan `created_at` dibuat `NOT NULL`.
  Dengan autovacuum default, vacuum karena insert di 50 juta baris
  menunggu ~10 juta insert (~50 hari), dan statistik planner menjadi basi.
  Karena itu `articles` memakai scale factor insert dan analyze 0,01.
  `CHECK (length(body) < 1000000)` adalah jaring pengaman terakhir di
  database terhadap batas ~1 MB `tsvector`; validasi utama tetap di
  aplikasi (400) supaya klien mendapat pesan yang jelas. `NOT NULL`
  bukan berarti tidak kosong: string kosong tetap lolos `NOT NULL`,
  sehingga handler juga memeriksa judul/body setelah trim.
- **Tidak ada endpoint update di soal.**
  "Artikel diedit" adalah kondisi data, bukan endpoint yang diminta.
  Konsekuensinya: saat judul/body berubah, indeks pencarian harus berubah
  dan `created_at` tidak boleh berubah. Kolom generated STORED menangani
  ini otomatis, dan tes integrasi membuktikannya.
- **Partisi ditunda, sengaja.**
  Tabel tunggal masih layak di 50 juta baris. Partisi bulanan berdasarkan
  `created_at` baru relevan di atas kira-kira 300 juta baris, atau saat
  VACUUM/backup mulai menyakitkan, atau saat ada kebijakan retensi untuk
  menghapus partisi lama. Konsekuensinya kalau nanti dipakai: primary key
  harus memuat kolom partisi, foreign key ke tabel terpartisi menjadi
  canggung, dan query penulis/pencarian tanpa filter tanggal tetap
  menyapu banyak partisi. Itu perubahan besar yang didokumentasikan,
  bukan kelalaian.

## Skala: angka yang benar

Soal menyebut ~50 juta artikel, tumbuh ~200.000 per hari, satu penulis
sindikasi ~30%, rasio baca:tulis ~100:1 dan bursty.

- 200.000 per hari = sekitar 2,3 tulis per detik rata-rata, bukan
  200.000 per detik. Di 100:1, itu sekitar 231 baca per detik rata-rata,
  lebih tinggi saat burst.
- Pertumbuhan: +73 juta per tahun, jadi ~123 juta dalam 1 tahun dan
  ~270 juta dalam 3 tahun dari 50 juta sekarang.
- Bottleneck-nya BUKAN TPS mentah (Postgres menangani ribuan TPS).
  Bottleneck-nya adalah write amplification (setiap insert menulis heap
  + primary key + 3 indeks B-tree + GIN + WAL), pertumbuhan storage, dan
  perawatan indeks/tabel.
- Storage: soal tidak memberi rata-rata ukuran artikel, jadi tidak ada
  satu angka total yang bisa diklaim. Rumusnya: rata-rata ukuran artikel
  × jumlah baris, ditambah indeks. Ilustrasi: pada rata-rata 5 KB,
  data saja ~250 GB di 50 juta; pada 2 KB, ~100 GB. Angka-angka ini
  adalah ilustrasi, bukan hasil ukur.
- Validasi skala di proyek ini TIDAK memakai 50 juta baris sintetis:
  itu diperkirakan 200-400 GB dan tidak muat di server 80 GB, dan build
  indeks GIN-nya bisa berjam-jam, di luar tenggat. Sebagai gantinya,
  pakai seed miring yang lebih kecil (misalnya ~100 ribu artikel dengan
  satu penulis dominan) dan buktikan dengan `EXPLAIN ANALYZE` bahwa
  query memakai indeks dan hanya mengambil satu halaman, lalu
  ekstrapolasikan (biaya indeks tumbuh lambat, full scan tumbuh linear).
- Deployment demo di server teman (8 GB RAM, 80 GB disk) LEBIH dari
  cukup untuk demo dengan seed ratusan ribu sampai ~1 juta artikel,
  dengan pool kecil (misalnya 20-25, sesuai `MaxConns` di `main.go`).
  Itu BUKAN klaim produksi untuk 50 juta artikel. Repo + Docker Compose
  yang bisa dijalankan lokal tetap menjadi submission utama; URL live
  hanya bonus. Spesifikasi tidak meminta autentikasi, jadi `POST`
  publik di demo bisa di-spam; jangan tambah auth yang akan merusak
  pengujian reviewer, tapi pantau demo selama penilaian dan siap
  reset seed.

## Batasan yang diketahui

- List mengembalikan body penuh untuk sampai 100 artikel. Untuk artikel
  kecil ini tidak masalah, tapi satu halaman bisa besar kalau body besar.
  Kontrak ini dipertahankan karena soal tidak meminta endpoint detail
  terpisah; endpoint ringkasan/excerpt adalah future work.
- Pencarian kata yang sangat umum harus menyortir banyak kecocokan
  sebelum mengambil 20 teratas, karena urutannya terbaru dulu, bukan
  relevansi. Ini konsekuensi desain, bukan bug: planner harus memilih
  antara GIN dulu untuk kata langka vs indeks `created_at` dulu untuk
  kata umum, dan tidak ada indeks gabungan GIN + B-tree yang sederhana
  untuk kasus ini.
- Pool database diatur eksplisit (`MaxConns 25`), tapi angka itu harus
  diselaraskan dengan `max_connections` Postgres dan kapasitas server
  kalau beban naik jauh di atas demo.
- `/health` mengecek database dengan timeout 2 detik. Kalau database
  lambat, health bisa 503 walau servernya sendiri hidup.
- Tes integrasi membutuhkan database tes terpisah (`TEST_DATABASE_URL`)
  dan akan menghapus lalu membuat ulang tabel `articles`/`authors` di
  database tes itu. Jangan pernah mengarahkannya ke database utama.

## Tes

```cmd
go vet ./...
go test ./...
```

Tes unit handler (`handler_test.go`) mencakup round-trip cursor dan
cursor tidak valid. Tes integrasi repository (`repository_test.go`)
mencakup: filter nama penulis ganda, pencarian setelah edit dengan
`created_at` tidak berubah, dan pagination keyset tanpa duplikat atau
hilang. Tes integrasi di-skip kalau `TEST_DATABASE_URL` tidak di-set.

## Transparansi penggunaan AI

Bagian yang dibantu AI: eksplorasi ide desain, draf awal kode dan
dokumentasi, peninjauan kode, serta paket pencarian Meilisearch
(`internal/search`), wiring handler ke Meilisearch, dan program
bulk 1 juta dokumen (`cmd/meili-bulk`). Keputusan akhir, pengujian,
dan perbaikan adalah tanggung jawab penulis, dan penulis memahami
solusi ini.

Satu kasus nyata di mana AI memberi jawaban yang tidak cocok, bagaimana
penulis menyadarinya, dan apa yang dipakai sebagai gantinya:
saat mengetes di PowerShell, AI sempat menyarankan `curl.exe` dengan JSON
dibungkus kutip satu. Perintah itu mengembalikan `invalid_json` dari
server, padahal JSON yang ditulis sudah benar. Penyebabnya adalah
PowerShell yang merusak kutip ganda di dalam argumen saat diteruskan ke
program luar. Penulis menyadarinya dari respons `invalid_json` itu
sendiri, padahal server sudah jalan dan endpoint sudah merespons.
Sebagai gantinya dipakai `Invoke-RestMethod` di PowerShell (atau `curl`
asli di CMD dengan escape `\"...\"`), dan POST pun berhasil dengan 201.
Pelajaran yang diambil: perintah shell tidak bisa disalin mentah antar
shell; sintaks kutipnya harus disesuaikan dengan shell yang dipakai.

Kasus kedua, yang lebih relevan dengan soal ini: saat membuat data
sintetis 1 juta artikel untuk uji skala, AI pertama kali memberi
generator dengan subquery judul dan body yang tidak terikat ke
nomor baris, sehingga semua artikel sintetis mendapat judul dan
body yang persis sama. Penulis menyadarinya dari hasil GET yang
menampilkan judul/body identik berulang, padahal datanya
seharusnya beragam. Sebagai gantinya, generator diperbaiki dengan
`CROSS JOIN LATERAL` yang terikat ke nomor baris (`gs`), sehingga
setiap baris mendapat kalimat subjek-predikat-objek-tempat yang
berbeda, dan dicek dengan `SELECT count(DISTINCT title) FROM
articles WHERE id LIKE 'synth-%'`. Pelajaran yang diambil:
hasil AI untuk data uji harus dicek distribusinya, bukan hanya
apakah perintahnya berhasil jalan (INSERT 0 1000000 saja tidak
cukup sebagai bukti).

## Future work (tidak dibangun sekarang)

- Partisi bulanan berdasarkan `created_at` di atas ~300 juta baris atau
  saat perawatan mulai menyakitkan, dengan konsekuensi primary key dan
  foreign key di atas.
- OpenSearch/Elasticsearch/Typesense atau ekstensi BM25 (misalnya
  ParadeDB) hanya kalau ranking relevansi yang baik, toleransi typo,
  highlighting, atau facet benar-benar diminta.
- Deteksi bahasa per artikel saat tulis dan simpan bahasanya, atau
  simpan dua vektor (Indonesia + Inggris) dengan biaya indeks ganda,
  atau kamus Indonesia kustom, untuk memperbaiki stemming.
- Ranking dengan bobot judul vs body hanya kalau urutan relevansi
  pernah diminta, menggantikan urutan terbaru dulu.
- Endpoint detail terpisah dan excerpt untuk list, supaya list tidak
  harus mengembalikan body penuh.
- Rate limiting untuk `POST` publik di demo, tanpa mengubah kontrak
  pengujian reviewer (misalnya limit per IP yang longgar, plus reset seed).
