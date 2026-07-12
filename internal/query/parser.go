package query

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

// ParsedAdd is the structured result of ParseAddArgs.
type ParsedAdd struct {
	Title string
	Mod   task.Modification
}

func ParseAddArgs(args []string) (ParsedAdd, error) {
	var titleParts []string
	mod := task.Modification{}
	for _, arg := range args {
		applied, err := applyModificationToken(arg, &mod)
		if err != nil {
			return ParsedAdd{}, err
		}
		if applied {
			continue
		}
		titleParts = append(titleParts, arg)
	}
	title := strings.TrimSpace(strings.Join(titleParts, " "))
	if title == "" {
		return ParsedAdd{}, errors.New("title is required")
	}
	return ParsedAdd{Title: title, Mod: mod}, nil
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

func ParseFilterExpr(args []string) (Expr, error) {
	if len(args) == 0 {
		return nil, nil
	}
	return ParseQuery(strings.Join(args, " "))
}

func applyModificationToken(arg string, mod *task.Modification) (bool, error) {
	switch {
	case strings.HasPrefix(arg, "+@") && len(arg) > 2:
		mod.AddAssignees = append(mod.AddAssignees, strings.TrimPrefix(arg, "+@"))
		return true, nil
	case strings.HasPrefix(arg, "-@") && len(arg) > 2:
		mod.RemoveAssignees = append(mod.RemoveAssignees, strings.TrimPrefix(arg, "-@"))
		return true, nil
	case strings.HasPrefix(arg, "@") && len(arg) > 1:
		mod.AddAssignees = append(mod.AddAssignees, strings.TrimPrefix(arg, "@"))
		return true, nil
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
	case strings.HasPrefix(arg, "title:"):
		value := strings.TrimPrefix(arg, "title:")
		mod.Title = &value
		return true, nil
	case strings.HasPrefix(arg, "description:"):
		value := strings.TrimPrefix(arg, "description:")
		mod.Description = &value
		return true, nil
	case strings.HasPrefix(arg, "due:"):
		value := strings.TrimPrefix(arg, "due:")
		if value == "" {
			mod.ClearDue = true
			return true, nil
		}
		due, err := ResolveDeadlineDateValue(ParseDateValue(value), time.Now().Unix(), time.Local)
		if err != nil {
			return false, err
		}
		mod.Due = &due
		return true, nil
	case strings.HasPrefix(arg, "wait:"):
		value := strings.TrimPrefix(arg, "wait:")
		if value == "" {
			mod.ClearWait = true
			return true, nil
		}
		wait, err := ResolveStartDateValue(ParseDateValue(value), time.Now().Unix(), time.Local)
		if err != nil {
			return false, err
		}
		mod.Wait = &wait
		return true, nil
	case strings.HasPrefix(arg, "scheduled:"):
		value := strings.TrimPrefix(arg, "scheduled:")
		if value == "" {
			mod.ClearScheduled = true
			return true, nil
		}
		sched, err := ResolveStartDateValue(ParseDateValue(value), time.Now().Unix(), time.Local)
		if err != nil {
			return false, err
		}
		mod.Scheduled = &sched
		return true, nil
	case strings.HasPrefix(arg, "until:"):
		value := strings.TrimPrefix(arg, "until:")
		if value == "" {
			mod.ClearUntil = true
			return true, nil
		}
		until, err := ResolveDeadlineDateValue(ParseDateValue(value), time.Now().Unix(), time.Local)
		if err != nil {
			return false, err
		}
		mod.Until = &until
		return true, nil
	case strings.HasPrefix(arg, "depends:"):
		value := strings.TrimPrefix(arg, "depends:")
		if value == "" {
			mod.ClearDepends = true
			return true, nil
		}
		mod.AddDepends = append(mod.AddDepends, value)
		return true, nil
		case strings.HasPrefix(arg, "recur:"), strings.HasPrefix(arg, "mask:"), strings.HasPrefix(arg, "imask:"):
			// 旧循环任务 token 在 spec 2026-07-11 后不再支持（spec §11.1、§13.6）。
			// 返回错误而非静默忽略，避免用户以为循环仍然生效。
			key := arg
			if i := strings.Index(arg, ":"); i >= 0 {
				key = arg[:i]
			}
			return false, fmt.Errorf("%s is no longer supported; use 'xuanchu series' commands to manage recurring tasks", key)
		default:
		if name, value, ok := strings.Cut(arg, ":"); ok && isPotentialUDAName(name) {
			if mod.UDAs == nil {
				mod.UDAs = map[string]string{}
			}
			name = strings.TrimPrefix(name, "uda.")
			if value == "" {
				mod.ClearUDAs = append(mod.ClearUDAs, name)
				return true, nil
			}
			mod.UDAs[name] = value
			return true, nil
		}
		return false, nil
	}
}

func isPotentialUDAName(name string) bool {
	name = strings.TrimPrefix(name, "uda.")
	if strings.TrimSpace(name) == "" {
		return false
	}
	switch name {
	case "uuid", "title", "description", "status", "entry", "modified", "end", "due", "start", "wait", "scheduled", "until", "project", "project_seq", "task_slug", "priority", "depends", "annotations", "recur", "parent", "assignee", "tag", "mask", "imask", "series_id", "recurrence_at", "task_type":
		return false
	default:
		return true
	}
}
