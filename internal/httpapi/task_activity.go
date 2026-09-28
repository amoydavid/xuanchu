package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const taskActivityMaxLimit = 100

type taskActivityAnnotationResponse struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type taskActivityLinkResponse struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type taskActivityEntryResponse struct {
	ID         string                          `json:"id"`
	Kind       string                          `json:"kind"`
	Action     string                          `json:"action"`
	Actor      task.JSONActorInfo              `json:"actor"`
	OccurredAt string                          `json:"occurred_at"`
	Changes    []any                           `json:"changes,omitempty"`
	Annotation *taskActivityAnnotationResponse `json:"annotation,omitempty"`
	Link       *taskActivityLinkResponse       `json:"link,omitempty"`
}

type taskActivityPageResponse struct {
	Entries    []taskActivityEntryResponse `json:"entries"`
	NextCursor *string                     `json:"next_cursor"`
}

func (s *Server) handleTaskActivity(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(s, w, r)
	if !ok {
		return
	}
	limit, ok := parseTaskActivityLimit(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	page, err := scoped.ListTaskActivity(taskRef, app.TaskActivityInput{
		Limit: limit, Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
	})
	if err != nil {
		s.writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, taskActivityPageToResponse(page), nil)
}

func parseTaskActivityLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > taskActivityMaxLimit {
			writeError(w, http.StatusBadRequest, "api_bad_limit", fmt.Sprintf("limit must be between 1 and %d", taskActivityMaxLimit), nil)
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}

func taskActivityPageToResponse(page app.TaskActivityPage) taskActivityPageResponse {
	entries := make([]taskActivityEntryResponse, 0, len(page.Entries))
	for _, entry := range page.Entries {
		response := taskActivityEntryResponse{
			ID: entry.ID, Kind: entry.Kind, Action: entry.Action,
			Actor:      task.ActorInfoToJSON(entry.Actor),
			OccurredAt: time.Unix(entry.OccurredAt, 0).UTC().Format(time.RFC3339),
			Changes:    taskFieldChangesToJSON(entry.Changes),
		}
		if entry.Annotation != nil {
			response.Annotation = &taskActivityAnnotationResponse{ID: entry.Annotation.ID, Description: entry.Annotation.Description}
		}
		if entry.Link != nil {
			response.Link = &taskActivityLinkResponse{ID: entry.Link.ID, Type: entry.Link.Type, URL: entry.Link.URL, Title: entry.Link.Title}
		}
		entries = append(entries, response)
	}
	return taskActivityPageResponse{Entries: entries, NextCursor: page.NextCursor}
}
