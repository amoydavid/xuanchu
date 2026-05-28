package recurrence

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func Validate(expr string) error {
	_, _, err := parse(expr)
	return err
}

func Next(fromUnix int64, expr string, loc *time.Location) (int64, error) {
	if loc == nil {
		loc = time.Local
	}

	n, unit, err := parse(expr)
	if err != nil {
		return 0, err
	}

	from := time.Unix(fromUnix, 0).In(loc)
	switch unit {
	case "days":
		return from.AddDate(0, 0, n).Unix(), nil
	case "weeks":
		return from.AddDate(0, 0, 7*n).Unix(), nil
	case "months":
		return from.AddDate(0, n, 0).Unix(), nil
	default:
		return 0, fmt.Errorf("unsupported recurrence %q", expr)
	}
}

func parse(expr string) (int, string, error) {
	switch expr {
	case "daily":
		return 1, "days", nil
	case "weekly":
		return 1, "weeks", nil
	case "monthly":
		return 1, "months", nil
	}

	for _, unit := range []string{"days", "weeks", "months"} {
		if strings.HasSuffix(expr, unit) {
			n, err := strconv.Atoi(strings.TrimSuffix(expr, unit))
			if err != nil || n <= 0 {
				return 0, "", fmt.Errorf("invalid recurrence %q", expr)
			}
			return n, unit, nil
		}
	}

	return 0, "", fmt.Errorf("unsupported recurrence %q", expr)
}
