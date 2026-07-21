package projecttemplate

import (
	"testing"
	"time"
)

func TestRelativeTimeKeepsWallClockAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	source := time.Date(2026, 3, 9, 9, 30, 0, 0, loc).Unix()
	rel, err := ToRelativeLocalTime(source, "2026-03-07", loc)
	if err != nil {
		t.Fatal(err)
	}
	if rel != (RelativeLocalTimeV1{DayOffset: 2, LocalTime: "09:30:00"}) {
		t.Fatalf("relative = %#v", rel)
	}
	got, err := FromRelativeLocalTime(rel, "2026-10-31", loc)
	if err != nil {
		t.Fatal(err)
	}
	if text := time.Unix(got, 0).In(loc).Format("2006-01-02 15:04:05"); text != "2026-11-02 09:30:00" {
		t.Fatalf("restored = %s", text)
	}
}

func TestRelativeTimeRejectsInvalidDateAndNonexistentLocalTime(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ToRelativeLocalTime(0, "not-a-date", loc); ErrorCode(err) != "project_template_date_out_of_range" {
		t.Fatalf("invalid anchor error = %v", err)
	}
	if _, err := FromRelativeLocalTime(RelativeLocalTimeV1{LocalTime: "02:30:00"}, "2026-03-08", loc); ErrorCode(err) != "project_template_date_out_of_range" {
		t.Fatalf("nonexistent time error = %v", err)
	}
}
