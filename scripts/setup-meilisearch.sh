#!/bin/sh
# Setup Meilisearch untuk kumparan-articles.
# Dijalankan sekali oleh service meili-setup di docker-compose.yml,
# SEBELUM app jalan. Tidak mengubah main.go dan tidak mengubah dockerfile.
# - Buat indeks articles (primaryKey id) kalau belum ada.
# - Terapkan setting searchable / filterable / sortable.
# - Tunggu task setting sampai succeeded.
# Aman dijalankan ulang: buat indeks yang sudah ada (409) diabaikan,
# PATCH setting itu idempotent.
set -e

MEILI_HOST="${MEILI_HOST:-http://meilisearch:7700}"

# curl dengan Authorization hanya kalau MEILI_MASTER_KEY diisi.
# Di compose dev, key kosong, jadi tidak ada header Authorization.
meili_curl() {
  if [ -n "${MEILI_MASTER_KEY:-}" ]; then
    curl -s -H "Authorization: Bearer $MEILI_MASTER_KEY" "$@"
  else
    curl -s "$@"
  fi
}

# 1. Tunggu Meilisearch sehat, maksimal 30 detik.
echo "menunggu meilisearch di $MEILI_HOST ..."
i=0
until meili_curl "$MEILI_HOST/health" | grep -q available; do
  i=$((i + 1))
  if [ "$i" -ge 15 ]; then
    echo "meilisearch tidak siap setelah 30 detik"
    exit 1
  fi
  sleep 2
done
echo "meilisearch sehat"

# 2. Buat indeks articles kalau belum ada.
# 202 = dibuat, 409 = sudah ada (wajar, lanjut).
CODE=$(meili_curl -o /tmp/create-index.json -w "%{http_code}" -X POST "$MEILI_HOST/indexes" \
  -H "Content-Type: application/json" \
  -d '{"uid":"articles","primaryKey":"id"}')
echo "buat indeks articles: HTTP $CODE"
case "$CODE" in
  200|201|202|409) ;;
  *)
    echo "gagal buat indeks articles"
    cat /tmp/create-index.json
    exit 1
    ;;
esac

# 3. Terapkan setting indeks.
TASK_JSON=$(meili_curl -X PATCH "$MEILI_HOST/indexes/articles/settings" \
  -H "Content-Type: application/json" \
  -d '{"searchableAttributes":["title","body"],"filterableAttributes":["author_id","created_at_ts","id"],"sortableAttributes":["created_at_ts","id"]}')
echo "settings task: $TASK_JSON"
TASK_UID=$(echo "$TASK_JSON" | grep -o '"taskUid":[0-9]*' | cut -d: -f2)
if [ -z "$TASK_UID" ]; then
  echo "gagal baca taskUid dari jawaban settings"
  exit 1
fi

# 4. Tunggu task setting sampai succeeded, maksimal 60 detik.
i=0
while [ "$i" -lt 60 ]; do
  STATUS=$(meili_curl "$MEILI_HOST/tasks/$TASK_UID" | grep -o '"status":"[a-z]*"' | head -n 1 | cut -d'"' -f4)
  if [ "$STATUS" = "succeeded" ]; then
    echo "meilisearch settings ok (task $TASK_UID)"
    exit 0
  fi
  if [ "$STATUS" = "failed" ] || [ "$STATUS" = "canceled" ]; then
    echo "task settings $STATUS (task $TASK_UID)"
    meili_curl "$MEILI_HOST/tasks/$TASK_UID"
    echo ""
    exit 1
  fi
  i=$((i + 1))
  sleep 1
done

echo "task settings $TASK_UID tidak selesai dalam 60 detik"
exit 1
