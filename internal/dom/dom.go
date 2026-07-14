package dom

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

func Resolve(tsk task.Task, field string, urgency float64) (string, error) {
	switch field {
	case "uuid":
		return tsk.UUID, nil
	case "title":
		return tsk.Title, nil
	case "description":
		if tsk.Description == nil {
			return "", nil
		}
		return *tsk.Description, nil
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
	case "start":
		if tsk.Start == nil {
			return "", nil
		}
		return strconv.FormatInt(*tsk.Start, 10), nil
	case "wait":
		if tsk.Wait == nil {
			return "", nil
		}
		return strconv.FormatInt(*tsk.Wait, 10), nil
	case "scheduled":
		if tsk.Scheduled == nil {
			return "", nil
		}
		return strconv.FormatInt(*tsk.Scheduled, 10), nil
	case "until":
		if tsk.Until == nil {
			return "", nil
		}
		return strconv.FormatInt(*tsk.Until, 10), nil
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
	case "depends":
		return strings.Join(tsk.Depends, ","), nil
	case "annotations":
		lines := make([]string, 0, len(tsk.Annotations))
		for _, annotation := range tsk.Annotations {
			if annotation.ID == "" {
				lines = append(lines, strconv.FormatInt(annotation.Entry, 10)+":"+annotation.Description)
				continue
			}
			lines = append(lines, annotation.ID+" "+strconv.FormatInt(annotation.Entry, 10)+":"+annotation.Description)
		}
		return strings.Join(lines, "\n"), nil
	case "parent":
		if tsk.Parent == nil {
			return "", nil
		}
		return *tsk.Parent, nil
	case "series_id":
		if tsk.SeriesID == nil {
			return "", nil
		}
		return *tsk.SeriesID, nil
	case "recurrence_at":
		if tsk.RecurrenceAt == nil {
			return "", nil
		}
		return strconv.FormatInt(*tsk.RecurrenceAt, 10), nil
	case "recurrence_rule_snapshot":
		if tsk.RecurrenceRuleSnapshot == nil {
			return "", nil
		}
		return *tsk.RecurrenceRuleSnapshot, nil
	case "urgency":
		return strconv.FormatFloat(urgency, 'f', 3, 64), nil
	}
	if strings.HasPrefix(field, "uda.") {
		field = strings.TrimPrefix(field, "uda.")
	}
	if value, ok := tsk.UDAs[field]; ok {
		return value.Raw, nil
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
