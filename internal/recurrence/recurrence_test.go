package recurrence

import (
	"testing"
	"time"
)

func TestNextDailyWeeklyMonthlyAndN(t *testing.T) {
	loc := time.UTC
	base := time.Date(2030, 1, 1, 23, 59, 59, 0, loc).Unix()

	tests := map[string]time.Time{
		"daily":   time.Date(2030, 1, 2, 23, 59, 59, 0, loc),
		"weekly":  time.Date(2030, 1, 8, 23, 59, 59, 0, loc),
		"monthly": time.Date(2030, 2, 1, 23, 59, 59, 0, loc),
		"3days":   time.Date(2030, 1, 4, 23, 59, 59, 0, loc),
	}

	for expr, want := range tests {
		got, err := Next(base, expr, loc)
		if err != nil {
			t.Fatalf("Next(%q) error = %v", expr, err)
		}
		if got != want.Unix() {
			t.Fatalf("Next(%q) = %v, want %v", expr, time.Unix(got, 0).UTC(), want)
		}
	}
}

func TestValidateRejectsUnsupportedRecurrence(t *testing.T) {
	if err := Validate("fortnightly"); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}
