package query

import (
	"testing"
	"time"
)

func TestParseDateTomorrow(t *testing.T) {
	loc := time.FixedZone("TEST", 8*60*60)
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, loc)
	got, err := ParseDate("tomorrow", now, loc)
	if err != nil {
		t.Fatalf("ParseDate() error = %v", err)
	}
	want := time.Date(2026, 5, 29, 0, 0, 0, 0, loc).Unix()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestParseDateYYYYMMDD(t *testing.T) {
	loc := time.UTC
	got, err := ParseDate("2026-05-28", time.Now(), loc)
	if err != nil {
		t.Fatalf("ParseDate() error = %v", err)
	}
	want := time.Date(2026, 5, 28, 0, 0, 0, 0, loc).Unix()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}
