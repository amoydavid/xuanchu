package urgency

import (
	"slices"
	"sort"

	"git.dajee.net/dajee/xuanchu/internal/task"
)

const (
	coefTagNext     = 15.0
	coefDue         = 12.0
	coefPriorityH   = 6.0
	coefPriorityM   = 3.9
	coefPriorityL   = 1.8
	coefBlocking    = 8.0
	coefActive      = 4.0
	coefAnnotations = 1.0
	coefWaiting     = -3.0
	coefBlocked     = -5.0
	coefAge         = 2.0
	coefTags        = 1.0
	coefProject     = 1.0
	ageMaxDays      = 365.0
)

// isTerminated 判断任务是否处于终态（completed/deleted）。
// 终态任务 urgency 恒为 0，不再累计 due/priority/age 等任何加分项。
// waiting 仍是活状态（可被唤醒），不计入终态。
func isTerminated(status string) bool {
	return status == task.StatusCompleted || status == task.StatusDeleted
}

func Explain(tsk task.Task, opts Options) ExplainResult {
	var result ExplainResult
	result.UUID = tsk.UUID
	// 终态任务（completed/deleted）urgency 恒为 0：它们无需再排优先级，
	// 与 Taskwarrior 行为一致（终态任务不进入 next 等 urgency 报表）。
	// 保留一条说明项，便于 urgency 命令解释为何是 0。
	if isTerminated(tsk.Status) {
		result.Items = []ExplainItem{{
			Name: "urgency_terminated", Reason: "task is " + tsk.Status + ", urgency is fixed at 0",
		}}
		return result
	}
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
	names := make([]string, 0, len(tsk.UDAs))
	for name := range tsk.UDAs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := tsk.UDAs[name]
		if coef, ok := opts.UDACoefficients[name]; ok {
			add(ExplainItem{Name: "uda." + name, Coefficient: coef, Raw: value.Raw, Contribution: coef, Reason: "task has UDA " + name})
		}
		key := name + "." + value.Raw
		if coef, ok := opts.UDAValueCoefficients[key]; ok {
			add(ExplainItem{Name: "uda." + key, Coefficient: coef, Raw: value.Raw, Contribution: coef, Reason: "task UDA value matches " + key})
		}
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

// TaskValue 是解耦 identity 的 urgency 计算输入（spec §13.4）。
// App 层从 TaskOccurrenceView 映射；projected 的 Entry 为 nil（age 不计）。
type TaskValue struct {
	Status           string
	Entry            *int64
	Start            *int64
	Wait             *int64
	Due              *int64
	Priority         *string
	Project          *string
	Tags             []string
	AnnotationCount  int
	UDAs             map[string]string
}

// Result 是无 identity 的 urgency 结果（spec §13.4）。
// App 层负责用 TaskOccurrenceView 的 id/uuid 包装为 UrgencyView。
type Result struct {
	Total float64
	Items []ExplainItem
}

// ExplainValue 对无 identity 的 TaskValue 计算 urgency（spec §13.4）。
// projected 的 Entry 为 nil → age 项不计；blocked/blocking 固定 false（由 Options 传）。
func ExplainValue(tv TaskValue, opts Options) Result {
	var result Result
	// 终态任务（completed/deleted）urgency 恒为 0，与 Taskwarrior 一致。
	// 保留一条说明项，便于 urgency 命令解释为何是 0。
	if isTerminated(tv.Status) {
		result.Items = []ExplainItem{{
			Name: "urgency_terminated", Reason: "task is " + tv.Status + ", urgency is fixed at 0",
		}}
		return result
	}
	add := func(item ExplainItem) {
		result.Items = append(result.Items, item)
		result.Total += item.Contribution
	}
	if slices.Contains(tv.Tags, "next") {
		add(ExplainItem{Name: "tag.next", Coefficient: coefTagNext, Raw: "next", Contribution: coefTagNext, Reason: "task has +next"})
	}
	if tv.Start != nil {
		add(ExplainItem{Name: "active", Coefficient: coefActive, Raw: *tv.Start, Contribution: coefActive, Reason: "task is active"})
	}
	if (tv.Wait != nil && *tv.Wait > opts.NowUnix) || tv.Status == "waiting" {
		add(ExplainItem{Name: "waiting", Coefficient: coefWaiting, Raw: tv.Wait, Contribution: coefWaiting, Reason: "task is waiting"})
	}
	if opts.Blocked {
		add(ExplainItem{Name: "blocked", Coefficient: coefBlocked, Raw: true, Contribution: coefBlocked, Reason: "task is blocked by dependencies"})
	}
	if opts.Blocking {
		add(ExplainItem{Name: "blocking", Coefficient: coefBlocking, Raw: true, Contribution: coefBlocking, Reason: "task blocks other tasks"})
	}
	if tv.Due != nil {
		contribution := dueContribution(*tv.Due, opts.NowUnix)
		if contribution > 0 {
			add(ExplainItem{Name: "due", Coefficient: coefDue, Raw: *tv.Due, Contribution: contribution, Reason: "task has due date"})
		}
	}
	if tv.Priority != nil {
		switch *tv.Priority {
		case "H":
			add(ExplainItem{Name: "priority.H", Coefficient: coefPriorityH, Raw: "H", Contribution: coefPriorityH, Reason: "priority is H"})
		case "M":
			add(ExplainItem{Name: "priority.M", Coefficient: coefPriorityM, Raw: "M", Contribution: coefPriorityM, Reason: "priority is M"})
		case "L":
			add(ExplainItem{Name: "priority.L", Coefficient: coefPriorityL, Raw: "L", Contribution: coefPriorityL, Reason: "priority is L"})
		}
	}
	// projected 的 Entry 为 nil → age 不计。
	if tv.Entry != nil && opts.NowUnix > *tv.Entry {
		days := float64(opts.NowUnix-*tv.Entry) / 86400.0
		if days > ageMaxDays {
			days = ageMaxDays
		}
		contribution := coefAge * (days / ageMaxDays)
		if contribution > 0 {
			add(ExplainItem{Name: "age", Coefficient: coefAge, Raw: days, Contribution: contribution, Reason: "task age"})
		}
	}
	if len(tv.Tags) > 0 {
		add(ExplainItem{Name: "tags", Coefficient: coefTags, Raw: len(tv.Tags), Contribution: tagCountContribution(len(tv.Tags)), Reason: "task has tags"})
	}
	if tv.AnnotationCount > 0 {
		add(ExplainItem{Name: "annotations", Coefficient: coefAnnotations, Raw: tv.AnnotationCount, Contribution: coefAnnotations * tagCountContribution(tv.AnnotationCount), Reason: "task has annotations"})
	}
	if tv.Project != nil && *tv.Project != "" {
		add(ExplainItem{Name: "project", Coefficient: coefProject, Raw: *tv.Project, Contribution: coefProject, Reason: "task has project"})
	}
	names := make([]string, 0, len(tv.UDAs))
	for name := range tv.UDAs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := tv.UDAs[name]
		if coef, ok := opts.UDACoefficients[name]; ok {
			add(ExplainItem{Name: "uda." + name, Coefficient: coef, Raw: value, Contribution: coef, Reason: "task has UDA " + name})
		}
		key := name + "." + value
		if coef, ok := opts.UDAValueCoefficients[key]; ok {
			add(ExplainItem{Name: "uda." + key, Coefficient: coef, Raw: value, Contribution: coef, Reason: "task UDA value matches " + key})
		}
	}
	return result
}
