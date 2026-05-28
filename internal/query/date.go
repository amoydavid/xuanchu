package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ParseDate(value string, now time.Time, loc *time.Location) (int64, error) {
	if loc == nil {
		loc = time.Local
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.Unix(), nil
	}
	if ts, err := time.ParseInLocation("2006-01-02", value, loc); err == nil {
		return ts.Unix(), nil
	}
	start := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	switch value {
	case "today":
		return start.Unix(), nil
	case "tomorrow":
		return start.AddDate(0, 0, 1).Unix(), nil
	case "eod":
		return start.Add(24*time.Hour - time.Second).Unix(), nil
	case "eow":
		days := (7 - int(start.Weekday())) % 7
		return start.AddDate(0, 0, days).Add(24*time.Hour - time.Second).Unix(), nil
	case "eom":
		return time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, loc).Add(-time.Second).Unix(), nil
	}
	if strings.HasSuffix(value, "days") {
		n, err := strconv.Atoi(strings.TrimSuffix(value, "days"))
		if err != nil {
			return 0, err
		}
		return start.AddDate(0, 0, n).Unix(), nil
	}
	return 0, fmt.Errorf("unsupported date %q", value)
}
