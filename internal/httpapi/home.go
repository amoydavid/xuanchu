package httpapi

import (
	"net/http"

	"git.dajee.net/dajee/xuanchu/internal/app"
)

type homeTaskItemResponse struct {
	Task    taskOccurrenceJSON `json:"task"`
	Reasons []string           `json:"reasons"`
}

type homeMyWorkResponse struct {
	OpenCount             int                    `json:"open_count"`
	StartedCount          int                    `json:"started_count"`
	OverdueCount          int                    `json:"overdue_count"`
	DueTodayCount         int                    `json:"due_today_count"`
	HighPriorityOpenCount int                    `json:"high_priority_open_count"`
	Items                 []homeTaskItemResponse `json:"items"`
}

type homeProjectAttentionResponse struct {
	Project               projectResponse              `json:"project"`
	OverdueCount          int                          `json:"overdue_count"`
	HighPriorityOpenCount int                          `json:"high_priority_open_count"`
	WaitReadyCount        int                          `json:"wait_ready_count"`
	UnassignedOpenCount   int                          `json:"unassigned_open_count"`
	SeriesMetrics         projectSeriesMetricsResponse `json:"series_metrics"`
	LatestUpdate          *projectAnnotationResponse   `json:"latest_update"`
}

type homeResponse struct {
	GeneratedAt      int64                          `json:"generated_at"`
	Today            string                         `json:"today"`
	ActorType        string                         `json:"actor_type"`
	MyWork           *homeMyWorkResponse            `json:"my_work"`
	ProjectAttention []homeProjectAttentionResponse `json:"project_attention"`
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	scoped, _, err := s.scopedService(r, "", "", "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.Home()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, homeResponseFromView(view), nil)
}

func homeResponseFromView(view app.HomeView) homeResponse {
	out := homeResponse{
		GeneratedAt: view.GeneratedAt, Today: view.Today, ActorType: view.ActorType,
		ProjectAttention: make([]homeProjectAttentionResponse, 0, len(view.ProjectAttention)),
	}
	if view.MyWork != nil {
		myWork := &homeMyWorkResponse{
			OpenCount: view.MyWork.OpenCount, StartedCount: view.MyWork.StartedCount,
			OverdueCount: view.MyWork.OverdueCount, DueTodayCount: view.MyWork.DueTodayCount,
			HighPriorityOpenCount: view.MyWork.HighPriorityOpenCount,
			Items:                 make([]homeTaskItemResponse, 0, len(view.MyWork.Items)),
		}
		for _, item := range view.MyWork.Items {
			reasons := make([]string, 0, len(item.Reasons))
			for _, reason := range item.Reasons {
				reasons = append(reasons, string(reason))
			}
			myWork.Items = append(myWork.Items, homeTaskItemResponse{Task: occurrenceViewToJSON(item.Task), Reasons: reasons})
		}
		out.MyWork = myWork
	}
	for _, item := range view.ProjectAttention {
		response := homeProjectAttentionResponse{
			Project: projectResponseFromView(item.Project), OverdueCount: item.OverdueCount,
			HighPriorityOpenCount: item.HighPriorityOpenCount, WaitReadyCount: item.WaitReadyCount,
			UnassignedOpenCount: item.UnassignedOpenCount,
			SeriesMetrics: projectSeriesMetricsResponse{
				RecurringSeriesCount:            item.SeriesMetrics.RecurringSeriesCount,
				ActiveRecurringSeriesCount:      item.SeriesMetrics.ActiveRecurringSeriesCount,
				OpenRecurringOccurrenceCount:    item.SeriesMetrics.OpenRecurringOccurrenceCount,
				OverdueRecurringOccurrenceCount: item.SeriesMetrics.OverdueRecurringOccurrenceCount,
			},
		}
		if item.LatestUpdate != nil {
			latest := projectAnnotationToJSON(*item.LatestUpdate)
			response.LatestUpdate = &latest
		}
		out.ProjectAttention = append(out.ProjectAttention, response)
	}
	return out
}
