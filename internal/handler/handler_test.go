package handler

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestEncodeDecodeCursorRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, 11, 3, 11, 16, 32, 735380000, time.FixedZone("WIB", 7*60*60))
	id := "01a19f9f-7fde-77d5-b9bc-0154e2f5acff"

	encoded := encodeCursor(createdAt, id)
	gotTime, gotID, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor mengembalikan error: %v", err)
	}
	if gotID != id {
		t.Fatalf("id tidak sama: dapat %q, mau %q", gotID, id)
	}
	if !gotTime.Equal(createdAt) {
		t.Fatalf("created_at tidak sama: dapat %v, mau %v", gotTime, createdAt)
	}
}

func TestDecodeCursorInvalid(t *testing.T) {
	encode := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	cases := map[string]string{
		"kosong":              "",
		"bukan base64":        "!!!bukan-base64!!!",
		"bukan JSON":          encode("bukan-json"),
		"id kosong":           encode(`{"created_at":"2026-11-03T11:16:32Z","id":""}`),
		"created_at kosong":   encode(`{"id":"abc-123"}`),
		"created_at nol":      encode(`{"created_at":"0001-01-01T00:00:00Z","id":"abc-123"}`),
		"created_at tidak valid": encode(`{"created_at":"bukan-waktu","id":"abc-123"}`),
	}
	for name, input := range cases {
		_, _, err := decodeCursor(input)
		if err == nil {
			t.Errorf("kasus %q: diharapkan error, dapat nil", name)
		}
	}
}
