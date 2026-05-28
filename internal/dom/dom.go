package dom

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dajee/taskg/internal/task"
)

func Resolve(tsk task.Task, field string, urgency float64) (string, error) {
	switch field {
	case "uuid":
		return tsk.UUID, nil
	case "description":
		return tsk.Description, nil
	case "status":
		return tsk.Status, nil
	case "entry":
		return strconv.FormatInt(tsk.Entry, 10), nil
	case "modified":
		return strconv.FormatInt(tsk.Modified, 10), nil
	case "due":
		if tsk.Due == nil {
			return "", nil
		}
		return strconv.FormatInt(*tsk.Due, 10), nil
	case "project":
		if tsk.Project == nil {
			return "", nil
		}
		return *tsk.Project, nil
	case "priority":
		if tsk.Priority == nil {
			return "", nil
		}
		return *tsk.Priority, nil
	case "tags":
		return strings.Join(tsk.Tags, ","), nil
	case "urgency":
		return strconv.FormatFloat(urgency, 'f', 3, 64), nil
	}
	if strings.HasPrefix(field, "tag.") {
		tag := strings.TrimPrefix(field, "tag.")
		if slices.Contains(tsk.Tags, tag) {
			return tag, nil
		}
		return "", nil
	}
	return "", fmt.Errorf("unknown DOM field %q", field)
}
