package projecttemplate

import (
	"fmt"
	"time"
)

func ToRelativeLocalTime(unix int64, anchorDate string, loc *time.Location) (RelativeLocalTimeV1, error) {
	if loc == nil {
		return RelativeLocalTimeV1{}, dateOutOfRange("timezone is required")
	}
	anchor, err := parseLocalDate(anchorDate, loc)
	if err != nil {
		return RelativeLocalTimeV1{}, err
	}
	source := time.Unix(unix, 0).In(loc)
	if !validYear(source.Year()) {
		return RelativeLocalTimeV1{}, dateOutOfRange("source date is outside supported range")
	}
	offset := calendarDay(source.Year(), source.Month(), source.Day()) - calendarDay(anchor.Year(), anchor.Month(), anchor.Day())
	return RelativeLocalTimeV1{
		DayOffset: int(offset),
		LocalTime: source.Format("15:04:05"),
	}, nil
}

func FromRelativeLocalTime(relative RelativeLocalTimeV1, startDate string, loc *time.Location) (int64, error) {
	if loc == nil {
		return 0, dateOutOfRange("timezone is required")
	}
	start, err := parseLocalDate(startDate, loc)
	if err != nil {
		return 0, err
	}
	clock, err := time.Parse("15:04:05", relative.LocalTime)
	if err != nil || clock.Format("15:04:05") != relative.LocalTime {
		return 0, dateOutOfRange("local time must be HH:MM:SS")
	}
	date := start.AddDate(0, 0, relative.DayOffset)
	if !validYear(date.Year()) {
		return 0, dateOutOfRange("restored date is outside supported range")
	}
	restored := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, loc)
	if restored.Year() != date.Year() || restored.Month() != date.Month() || restored.Day() != date.Day() || restored.Hour() != clock.Hour() || restored.Minute() != clock.Minute() || restored.Second() != clock.Second() {
		return 0, dateOutOfRange("local time does not exist in timezone")
	}
	return restored.Unix(), nil
}

func parseLocalDate(value string, loc *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil || parsed.Format("2006-01-02") != value || !validYear(parsed.Year()) {
		return time.Time{}, dateOutOfRange("date must be YYYY-MM-DD")
	}
	return parsed, nil
}

func calendarDay(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / int64(24*time.Hour/time.Second)
}

func validYear(year int) bool { return year >= 1 && year <= 9999 }

func dateOutOfRange(message string) Error {
	return Error{Code: "project_template_date_out_of_range", Message: fmt.Sprintf("template date: %s", message)}
}
