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
	if rel, ok, err := parseRelativeDate(value, now, loc); ok || err != nil {
		return rel, err
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

func parseRelativeDate(value string, now time.Time, loc *time.Location) (int64, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false, nil
	}
	if strings.HasPrefix(trimmed, "now") {
		offset := strings.TrimPrefix(trimmed, "now")
		if offset == "" {
			return now.In(loc).Unix(), true, nil
		}
		dur, err := time.ParseDuration(offset)
		if err != nil {
			return 0, true, err
		}
		return now.Add(dur).In(loc).Unix(), true, nil
	}
	if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") {
		dur, err := time.ParseDuration(trimmed)
		if err != nil {
			return 0, true, err
		}
		return now.Add(dur).In(loc).Unix(), true, nil
	}
	if dur, err := time.ParseDuration(trimmed); err == nil {
		return now.Add(dur).In(loc).Unix(), true, nil
	}
	return 0, false, nil
}

func ParseDateValue(raw string) Value {
	return DateValue(raw)
}

func ResolveDateValue(value Value, nowUnix int64, loc *time.Location) (int64, error) {
	now := time.Unix(nowUnix, 0).In(loc)
	return ParseDate(value.Raw, now, loc)
}

func ResolveDateRange(value Value, nowUnix int64, loc *time.Location) (int64, int64, error) {
	if loc == nil {
		loc = time.Local
	}
	start, err := ResolveDateValue(value, nowUnix, loc)
	if err != nil {
		return 0, 0, err
	}
	startTime := time.Unix(start, 0).In(loc)
	dayStart := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, loc)
	nextDayStart := dayStart.AddDate(0, 0, 1)
	return dayStart.Unix(), nextDayStart.Unix(), nil
}

func ResolveDeadlineDateValue(value Value, nowUnix int64, loc *time.Location) (int64, error) {
	_, end, err := ResolveDateRange(value, nowUnix, loc)
	if err != nil {
		return 0, err
	}
	return end - 1, nil
}
