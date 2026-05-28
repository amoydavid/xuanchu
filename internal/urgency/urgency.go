package urgency

import (
	"slices"

	"github.com/dajee/taskg/internal/task"
)

const (
	coefTagNext      = 15.0
	coefDue          = 12.0
	coefPriorityH    = 6.0
	coefPriorityM    = 3.9
	coefPriorityL    = 1.8
	coefBlocking     = 8.0
	coefActive       = 4.0
	coefAnnotations  = 1.0
	coefWaiting      = -3.0
	coefBlocked      = -5.0
	coefAge          = 2.0
	coefTags         = 1.0
	coefProject      = 1.0
	ageMaxDays       = 365.0
)

func Explain(tsk task.Task, opts Options) ExplainResult {
	var result ExplainResult
	result.UUID = tsk.UUID
	add := func(item ExplainItem) {
		result.Items = append(result.Items, item)
		result.Total += item.Contribution
	}
	if slices.Contains(tsk.Tags, "next") {
		add(ExplainItem{Name: "tag.next", Coefficient: coefTagNext, Raw: "next", Contribution: coefTagNext, Reason: "task has +next"})
	}
	if tsk.Start != nil {
		add(ExplainItem{Name: "active", Coefficient: coefActive, Raw: *tsk.Start, Contribution: coefActive, Reason: "task is active"})
	}
	if (tsk.Wait != nil && *tsk.Wait > opts.NowUnix) || tsk.Status == task.StatusWaiting {
		add(ExplainItem{Name: "waiting", Coefficient: coefWaiting, Raw: tsk.Wait, Contribution: coefWaiting, Reason: "task is waiting"})
	}
	if opts.Blocked {
		add(ExplainItem{Name: "blocked", Coefficient: coefBlocked, Raw: true, Contribution: coefBlocked, Reason: "task is blocked by dependencies"})
	}
	if opts.Blocking {
		add(ExplainItem{Name: "blocking", Coefficient: coefBlocking, Raw: true, Contribution: coefBlocking, Reason: "task blocks other tasks"})
	}
	if tsk.Due != nil {
		contribution := dueContribution(*tsk.Due, opts.NowUnix)
		if contribution > 0 {
			add(ExplainItem{Name: "due", Coefficient: coefDue, Raw: *tsk.Due, Contribution: contribution, Reason: "task has due date"})
		}
	}
	if tsk.Priority != nil {
		switch *tsk.Priority {
		case "H":
			add(ExplainItem{Name: "priority.H", Coefficient: coefPriorityH, Raw: "H", Contribution: coefPriorityH, Reason: "priority is H"})
		case "M":
			add(ExplainItem{Name: "priority.M", Coefficient: coefPriorityM, Raw: "M", Contribution: coefPriorityM, Reason: "priority is M"})
		case "L":
			add(ExplainItem{Name: "priority.L", Coefficient: coefPriorityL, Raw: "L", Contribution: coefPriorityL, Reason: "priority is L"})
		}
	}
	if opts.NowUnix > tsk.Entry {
		days := float64(opts.NowUnix-tsk.Entry) / 86400.0
		if days > ageMaxDays {
			days = ageMaxDays
		}
		contribution := coefAge * (days / ageMaxDays)
		if contribution > 0 {
			add(ExplainItem{Name: "age", Coefficient: coefAge, Raw: days, Contribution: contribution, Reason: "task age"})
		}
	}
	if len(tsk.Tags) > 0 {
		add(ExplainItem{Name: "tags", Coefficient: coefTags, Raw: len(tsk.Tags), Contribution: tagCountContribution(len(tsk.Tags)), Reason: "task has tags"})
	}
	if len(tsk.Annotations) > 0 {
		add(ExplainItem{Name: "annotations", Coefficient: coefAnnotations, Raw: len(tsk.Annotations), Contribution: coefAnnotations * tagCountContribution(len(tsk.Annotations)), Reason: "task has annotations"})
	}
	if tsk.Project != nil && *tsk.Project != "" {
		add(ExplainItem{Name: "project", Coefficient: coefProject, Raw: *tsk.Project, Contribution: coefProject, Reason: "task has project"})
	}
	return result
}

func dueContribution(due, now int64) float64 {
	if due <= now {
		return coefDue
	}
	days := float64(due-now) / 86400.0
	if days > 7 {
		return 0
	}
	return coefDue * ((7 - days) / 7)
}

func tagCountContribution(n int) float64 {
	switch {
	case n <= 0:
		return 0
	case n == 1:
		return 0.8
	case n == 2:
		return 0.9
	default:
		return 1.0
	}
}
