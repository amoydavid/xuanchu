package query

import (
	"errors"
	"strings"
	"time"

	"github.com/dajee/taskg/internal/task"
)

func ParseAddArgs(args []string) (ParsedAdd, error) {
	var desc []string
	mod := task.Modification{}
	for _, arg := range args {
		applied, err := applyModificationToken(arg, &mod)
		if err != nil {
			return ParsedAdd{}, err
		}
		if applied {
			continue
		}
		desc = append(desc, arg)
	}
	description := strings.TrimSpace(strings.Join(desc, " "))
	if description == "" {
		return ParsedAdd{}, errors.New("description is required")
	}
	return ParsedAdd{Description: description, Mod: mod}, nil
}

func ParseModifyArgs(args []string) (task.Modification, error) {
	mod := task.Modification{}
	for _, arg := range args {
		applied, err := applyModificationToken(arg, &mod)
		if err != nil {
			return task.Modification{}, err
		}
		if !applied {
			return task.Modification{}, errors.New("unsupported modification: " + arg)
		}
	}
	if mod.Empty() {
		return task.Modification{}, errors.New("at least one modification is required")
	}
	return mod, nil
}

func ParseFilters(args []string) (Filter, error) {
	filter := Filter{}
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "+") && len(arg) > 1:
			filter.Tags = append(filter.Tags, strings.TrimPrefix(arg, "+"))
		case strings.HasPrefix(arg, "status:"):
			value := strings.TrimPrefix(arg, "status:")
			filter.Status = &value
		case strings.HasPrefix(arg, "project:"):
			value := strings.TrimPrefix(arg, "project:")
			filter.Project = &value
		case strings.HasPrefix(arg, "priority:"):
			value := strings.TrimPrefix(arg, "priority:")
			filter.Priority = &value
		case strings.HasPrefix(arg, "/") && strings.HasSuffix(arg, "/") && len(arg) >= 2:
			value := strings.TrimSuffix(strings.TrimPrefix(arg, "/"), "/")
			filter.Text = &value
		default:
			value := arg
			filter.Target = &value
		}
	}
	return filter, nil
}

func applyModificationToken(arg string, mod *task.Modification) (bool, error) {
	switch {
	case strings.HasPrefix(arg, "+") && len(arg) > 1:
		mod.AddTags = append(mod.AddTags, strings.TrimPrefix(arg, "+"))
		return true, nil
	case strings.HasPrefix(arg, "-") && len(arg) > 1:
		mod.RemoveTags = append(mod.RemoveTags, strings.TrimPrefix(arg, "-"))
		return true, nil
	case strings.HasPrefix(arg, "project:"):
		value := strings.TrimPrefix(arg, "project:")
		mod.Project = &value
		return true, nil
	case strings.HasPrefix(arg, "priority:"):
		value := strings.TrimPrefix(arg, "priority:")
		mod.Priority = &value
		return true, nil
	case strings.HasPrefix(arg, "description:"):
		value := strings.TrimPrefix(arg, "description:")
		mod.Description = &value
		return true, nil
	case strings.HasPrefix(arg, "due:"):
		value := strings.TrimPrefix(arg, "due:")
		due, err := ParseDate(value, time.Now(), time.Local)
		if err != nil {
			return false, err
		}
		mod.Due = &due
		return true, nil
	default:
		return false, nil
	}
}
