package app

import (
	"sort"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type HomeTaskReason string

const (
	HomeTaskReasonStarted      HomeTaskReason = "started"
	HomeTaskReasonOverdue      HomeTaskReason = "overdue"
	HomeTaskReasonDueToday     HomeTaskReason = "due_today"
	HomeTaskReasonHighPriority HomeTaskReason = "high_priority"
)

type HomeTaskItemView struct {
	Task    TaskOccurrenceView
	Reasons []HomeTaskReason
}

type HomeMyWorkView struct {
	OpenCount             int
	StartedCount          int
	OverdueCount          int
	DueTodayCount         int
	HighPriorityOpenCount int
	Items                 []HomeTaskItemView
}

type HomeProjectAttentionView struct {
	Project               ProjectView
	OverdueCount          int
	HighPriorityOpenCount int
	WaitReadyCount        int
	UnassignedOpenCount   int
	SeriesMetrics         ProjectSeriesMetricsView
	LatestUpdate          *ProjectAnnotationInfo
}

type HomeView struct {
	GeneratedAt      int64
	Today            string
	ActorType        string
	MyWork           *HomeMyWorkView
	ProjectAttention []HomeProjectAttentionView
}

// Home 返回 Web Console 登录首页的权限内权威摘要。
func (s *Service) Home() (HomeView, error) {
	now := s.clock.Unix()
	location := s.clock.Location()
	current := time.Unix(now, 0).In(location)
	todayStart := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, location)
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	view := HomeView{
		GeneratedAt:      now,
		Today:            todayStart.Format("2006-01-02"),
		ActorType:        s.runtime.ActorType,
		ProjectAttention: []HomeProjectAttentionView{},
	}

	if !s.runtime.IsTenantActor() && s.Require(PermissionTaskRead) == nil {
		myWork, err := s.homeMyWork(todayStart.Unix(), tomorrowStart.Unix())
		if err != nil {
			return HomeView{}, err
		}
		view.MyWork = &myWork
	}
	if s.Require(PermissionProjectRead) == nil {
		attention, err := s.homeProjectAttention()
		if err != nil {
			return HomeView{}, err
		}
		view.ProjectAttention = attention
	}
	return view, nil
}

func (s *Service) homeMyWork(todayStart, tomorrowStart int64) (HomeMyWorkView, error) {
	assignee := query.Predicate{Attribute: query.AttrAssignee, Operator: query.OpEqual, Value: query.StringValue(s.runtime.ActorUserID)}
	open := query.Or(
		query.Predicate{Attribute: query.AttrStatus, Operator: query.OpEqual, Value: query.StringValue("pending")},
		query.Predicate{Attribute: query.AttrStatus, Operator: query.OpEqual, Value: query.StringValue("waiting")},
	)
	page, err := s.QueryTaskViews(TaskViewQuery{
		WorkspaceID: s.workspaceID, OccurrenceMode: OccurrenceModeMaterialized,
		Sort: "urgency", NoContext: true, Query: query.And(assignee, open),
	})
	if err != nil {
		return HomeMyWorkView{}, err
	}
	result := HomeMyWorkView{OpenCount: len(page.Items), Items: make([]HomeTaskItemView, 0, min(len(page.Items), 8))}
	all := make([]HomeTaskItemView, 0, len(page.Items))
	for _, taskView := range page.Items {
		reasons := homeTaskReasons(taskView, todayStart, tomorrowStart)
		for _, reason := range reasons {
			switch reason {
			case HomeTaskReasonStarted:
				result.StartedCount++
			case HomeTaskReasonOverdue:
				result.OverdueCount++
			case HomeTaskReasonDueToday:
				result.DueTodayCount++
			case HomeTaskReasonHighPriority:
				result.HighPriorityOpenCount++
			}
		}
		all = append(all, HomeTaskItemView{Task: taskView, Reasons: reasons})
	}
	sort.SliceStable(all, func(i, j int) bool {
		left, right := all[i], all[j]
		leftBucket, rightBucket := homeTaskBucket(left.Reasons), homeTaskBucket(right.Reasons)
		if leftBucket != rightBucket {
			return leftBucket < rightBucket
		}
		leftUrgency, rightUrgency := homeUrgency(left.Task), homeUrgency(right.Task)
		if leftUrgency != rightUrgency {
			return leftUrgency > rightUrgency
		}
		if dueLess(left.Task.Due, right.Task.Due) {
			return true
		}
		if dueLess(right.Task.Due, left.Task.Due) {
			return false
		}
		leftEntry, rightEntry := optionalInt64(left.Task.Entry), optionalInt64(right.Task.Entry)
		if leftEntry != rightEntry {
			return leftEntry < rightEntry
		}
		return left.Task.ID < right.Task.ID
	})
	if len(all) > 8 {
		all = all[:8]
	}
	result.Items = all
	return result, nil
}

func homeTaskReasons(taskView TaskOccurrenceView, todayStart, tomorrowStart int64) []HomeTaskReason {
	reasons := make([]HomeTaskReason, 0, 4)
	if taskView.Start != nil {
		reasons = append(reasons, HomeTaskReasonStarted)
	}
	if taskView.Due != nil {
		switch {
		case *taskView.Due < todayStart:
			reasons = append(reasons, HomeTaskReasonOverdue)
		case *taskView.Due < tomorrowStart:
			reasons = append(reasons, HomeTaskReasonDueToday)
		}
	}
	if taskView.Priority != nil && *taskView.Priority == "H" {
		reasons = append(reasons, HomeTaskReasonHighPriority)
	}
	return reasons
}

func homeTaskBucket(reasons []HomeTaskReason) int {
	for bucket, reason := range []HomeTaskReason{
		HomeTaskReasonStarted, HomeTaskReasonOverdue, HomeTaskReasonDueToday, HomeTaskReasonHighPriority,
	} {
		for _, actual := range reasons {
			if actual == reason {
				return bucket
			}
		}
	}
	return 4
}

func homeUrgency(taskView TaskOccurrenceView) float64 {
	if taskView.Urgency == nil {
		return 0
	}
	return *taskView.Urgency
}

func dueLess(left, right *int64) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	return *left < *right
}

func optionalInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func (s *Service) homeProjectAttention() ([]HomeProjectAttentionView, error) {
	projects, err := s.ListProjectsByStatus("open")
	if err != nil {
		return nil, err
	}
	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ID)
	}
	metrics, err := s.homeRepo.ProjectMetrics(s.workspaceID, projectIDs, s.clock.Unix())
	if err != nil {
		return nil, err
	}
	latest, err := s.homeRepo.LatestProjectAnnotations(projectIDs)
	if err != nil {
		return nil, err
	}
	annotationRows := make([]storage.ProjectAnnotation, 0, len(latest))
	for _, annotation := range latest {
		annotationRows = append(annotationRows, annotation)
	}
	users, err := s.resolveUserInfos(projectAnnotationUserIDs(annotationRows))
	if err != nil {
		return nil, err
	}
	attention := make([]HomeProjectAttentionView, 0, len(projects))
	for _, project := range projects {
		metric := metrics[project.ID]
		item := HomeProjectAttentionView{
			Project: project, OverdueCount: metric.OverdueCount,
			HighPriorityOpenCount: metric.HighPriorityOpenCount,
			WaitReadyCount:        metric.WaitReadyCount, UnassignedOpenCount: metric.UnassignedOpenCount,
			SeriesMetrics: ProjectSeriesMetricsView{
				RecurringSeriesCount: metric.RecurringSeriesCount, ActiveRecurringSeriesCount: metric.ActiveRecurringSeriesCount,
				OpenRecurringOccurrenceCount:    metric.OpenRecurringOccurrenceCount,
				OverdueRecurringOccurrenceCount: metric.OverdueRecurringOccurrenceCount,
			},
		}
		if annotation, ok := latest[project.ID]; ok {
			converted := projectAnnotationInfoFromModel(annotation, users)
			item.LatestUpdate = &converted
		}
		attention = append(attention, item)
	}
	sort.SliceStable(attention, func(i, j int) bool {
		left, right := attention[i], attention[j]
		if left.OverdueCount != right.OverdueCount {
			return left.OverdueCount > right.OverdueCount
		}
		if left.HighPriorityOpenCount != right.HighPriorityOpenCount {
			return left.HighPriorityOpenCount > right.HighPriorityOpenCount
		}
		if left.WaitReadyCount != right.WaitReadyCount {
			return left.WaitReadyCount > right.WaitReadyCount
		}
		if left.UnassignedOpenCount != right.UnassignedOpenCount {
			return left.UnassignedOpenCount > right.UnassignedOpenCount
		}
		if left.Project.ModifiedAt != right.Project.ModifiedAt {
			return left.Project.ModifiedAt > right.Project.ModifiedAt
		}
		return left.Project.Slug < right.Project.Slug
	})
	hasRisk := false
	for _, item := range attention {
		if homeProjectHasRisk(item) {
			hasRisk = true
			break
		}
	}
	if hasRisk {
		risky := attention[:0]
		for _, item := range attention {
			if homeProjectHasRisk(item) {
				risky = append(risky, item)
			}
		}
		attention = risky
		if len(attention) > 5 {
			attention = attention[:5]
		}
	} else if len(attention) > 3 {
		attention = attention[:3]
	}
	return attention, nil
}

func homeProjectHasRisk(item HomeProjectAttentionView) bool {
	return item.OverdueCount > 0 || item.HighPriorityOpenCount > 0 || item.WaitReadyCount > 0 || item.UnassignedOpenCount > 0
}
