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

func TestParseDateNow(t *testing.T) {
	loc := time.FixedZone("TEST", 8*60*60)
	now := time.Date(2026, 6, 8, 14, 30, 0, 0, loc)
	got, err := ParseDate("now", now, loc)
	if err != nil {
		t.Fatalf("ParseDate() error = %v", err)
	}
	if got != now.Unix() {
		t.Fatalf("got %d, want %d", got, now.Unix())
	}
}

func TestParseDateNowRelativeDuration(t *testing.T) {
	loc := time.FixedZone("TEST", 8*60*60)
	now := time.Date(2026, 6, 8, 14, 30, 0, 0, loc)
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{name: "加 24 小时", input: "now+24h", want: now.Add(24 * time.Hour)},
		{name: "减 2 小时", input: "now-2h", want: now.Add(-2 * time.Hour)},
		{name: "复合 duration", input: "now+2h30m", want: now.Add(2*time.Hour + 30*time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDate(tt.input, now, loc)
			if err != nil {
				t.Fatalf("ParseDate() error = %v", err)
			}
			if got != tt.want.Unix() {
				t.Fatalf("got %d, want %d", got, tt.want.Unix())
			}
		})
	}
}

func TestParseDateRejectsInvalidNowRelativeDuration(t *testing.T) {
	loc := time.FixedZone("TEST", 8*60*60)
	now := time.Date(2026, 6, 8, 14, 30, 0, 0, loc)
	for _, input := range []string{"now+1d", "now+90min", "now+", "now-", "now+-2h", "now--2h"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseDate(input, now, loc); err == nil {
				t.Fatal("ParseDate() error = nil, want error")
			}
		})
	}
}

func TestResolveStartDateValueUsesStartOfDay(t *testing.T) {
	loc := time.FixedZone("TEST", 8*60*60)
	now := time.Date(2026, 7, 5, 14, 30, 0, 0, loc)
	got, err := ResolveStartDateValue(ParseDateValue("2026-07-05"), now.Unix(), loc)
	if err != nil {
		t.Fatalf("ResolveStartDateValue() error = %v", err)
	}
	want := time.Date(2026, 7, 5, 0, 0, 0, 0, loc).Unix()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}
