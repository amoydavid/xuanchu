package app

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const (
	taskActivityCursorVersion = 1
	activityAuditRank         = 30
	activityAnnotationRank    = 20
	activitySnapshotRank      = 10
)

var taskActivityAuditActions = []string{
	"task.add", "task.recurrence.generated", "task.start", "task.stop",
	"task.done", "task.reopen", "task.delete", "task.modify",
	"task.link.add", "task.link.update", "task.link.remove",
}

type TaskActivityInput struct {
	Limit  int
	Cursor string
}

type TaskActivityAnnotation struct {
	ID          string
	Description string
}

type TaskActivityLink struct {
	ID    string
	Type  string
	URL   string
	Title string
}

type TaskActivityEntry struct {
	ID         string
	Kind       string
	Action     string
	Actor      task.ActorInfo
	OccurredAt int64
	Changes    []TaskFieldChange
	Annotation *TaskActivityAnnotation
	Link       *TaskActivityLink
}

type TaskActivityPage struct {
	Entries    []TaskActivityEntry
	NextCursor *string
}

type taskActivityCursor struct {
	Version    int    `json:"v"`
	OccurredAt int64  `json:"occurred_at"`
	SourceRank int    `json:"source_rank"`
	SourceID   string `json:"source_id"`
}

type taskActivityCandidate struct {
	Entry      TaskActivityEntry
	SourceRank int
	SourceID   string
	Actor      actorColumns
}

func (s *Service) ListTaskActivity(taskRef string, input TaskActivityInput) (TaskActivityPage, error) {
	if err := s.Require(PermissionTaskRead); err != nil {
		return TaskActivityPage{}, err
	}
	resolved, err := s.ResolveTaskReferenceForRead(taskRef)
	if err != nil {
		return TaskActivityPage{}, err
	}
	if resolved.Task == nil {
		return TaskActivityPage{Entries: []TaskActivityEntry{}}, nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	var cursor *taskActivityCursor
	if strings.TrimSpace(input.Cursor) != "" {
		decoded, err := decodeTaskActivityCursor(input.Cursor)
		if err != nil {
			return TaskActivityPage{}, err
		}
		cursor = &decoded
	}

	actionExister, ok := s.auditRepo.(interface {
		ExistsForTaskActions(workspaceID, taskID string, actions []string) (bool, error)
	})
	if !ok {
		return TaskActivityPage{}, RuntimeError{Code: "activity_unavailable", Message: "activity audit source is unavailable"}
	}
	hasGenerated, err := actionExister.ExistsForTaskActions(s.workspaceID, resolved.Task.UUID, []string{"task.recurrence.generated"})
	if err != nil {
		return TaskActivityPage{}, err
	}
	hasCreated, err := actionExister.ExistsForTaskActions(s.workspaceID, resolved.Task.UUID, []string{"task.add"})
	if err != nil {
		return TaskActivityPage{}, err
	}

	candidates, err := s.listTaskAuditActivityCandidates(resolved.Task.UUID, cursor, limit+1, hasGenerated)
	if err != nil {
		return TaskActivityPage{}, err
	}
	annotationCursor, err := annotationStorageCursor(cursor)
	if err != nil {
		return TaskActivityPage{}, err
	}
	annotationRows, err := s.repo.ListAnnotationActivity(s.workspaceID, resolved.Task.UUID, annotationCursor, limit+1)
	if err != nil {
		return TaskActivityPage{}, err
	}
	for _, row := range annotationRows {
		occurredAt := row.CreatedAt
		if occurredAt == 0 {
			occurredAt = row.Entry
		}
		candidate := taskActivityCandidate{
			Entry: TaskActivityEntry{
				ID: "annotation:" + row.ID, Kind: "annotation", Action: "commented", OccurredAt: occurredAt,
				Annotation: &TaskActivityAnnotation{ID: row.ID, Description: row.Description},
			},
			SourceRank: activityAnnotationRank, SourceID: row.ID,
			Actor: actorColumns{Type: row.CreatedByActorType, UserID: row.CreatedByUserID, TokenID: row.CreatedByTokenID, TokenName: row.CreatedByTokenName, TokenPrefix: row.CreatedByTokenPrefix},
		}
		if cursor == nil || activityKeyBefore(candidate, *cursor) {
			candidates = append(candidates, candidate)
		}
	}
	if !hasGenerated && !hasCreated {
		snapshot := taskActivityCandidate{
			Entry:      TaskActivityEntry{ID: "snapshot:created", Kind: "lifecycle", Action: "created", OccurredAt: resolved.Task.Entry},
			SourceRank: activitySnapshotRank, SourceID: "created", Actor: actorColumns{Type: "unknown"},
		}
		if cursor == nil || activityKeyBefore(snapshot, *cursor) {
			candidates = append(candidates, snapshot)
		}
	}

	if err := resolveTaskActivityActors(candidates, s); err != nil {
		return TaskActivityPage{}, err
	}
	sort.SliceStable(candidates, func(i, j int) bool { return activityKeyAfter(candidates[i], candidates[j]) })
	hasNext := len(candidates) > limit
	if hasNext {
		candidates = candidates[:limit]
	}
	entries := make([]TaskActivityEntry, 0, len(candidates))
	for _, candidate := range candidates {
		entries = append(entries, candidate.Entry)
	}
	page := TaskActivityPage{Entries: entries}
	if hasNext && len(candidates) > 0 {
		last := candidates[len(candidates)-1]
		encoded := encodeTaskActivityCursor(taskActivityCursor{
			Version: taskActivityCursorVersion, OccurredAt: last.Entry.OccurredAt,
			SourceRank: last.SourceRank, SourceID: last.SourceID,
		})
		page.NextCursor = &encoded
	}
	return page, nil
}

func (s *Service) listTaskAuditActivityCandidates(taskID string, cursor *taskActivityCursor, want int, suppressCreated bool) ([]taskActivityCandidate, error) {
	storageCursor, err := auditStorageCursor(cursor)
	if err != nil {
		return nil, err
	}
	targetType := "task"
	out := make([]taskActivityCandidate, 0, want)
	batchSize := want
	if batchSize < 30 {
		batchSize = 30
	}
	for len(out) < want {
		rows, err := s.auditRepo.List(storage.AuditListOptions{
			WorkspaceID: &s.workspaceID, TargetType: &targetType, TargetID: &taskID,
			Actions: taskActivityAuditActions, Cursor: storageCursor, Limit: batchSize,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			candidate, ok := taskActivityCandidateFromAudit(row, suppressCreated)
			if ok && (cursor == nil || activityKeyBefore(candidate, *cursor)) {
				out = append(out, candidate)
			}
		}
		if len(rows) < batchSize {
			break
		}
		last := rows[len(rows)-1]
		lastID := last.ID
		storageCursor = &storage.AuditListCursor{CreatedAt: last.CreatedAt, ID: &lastID}
	}
	return out, nil
}

func taskActivityCandidateFromAudit(row storage.AuditLogEntry, suppressCreated bool) (taskActivityCandidate, bool) {
	kind, action := "", ""
	switch row.Action {
	case "task.add":
		if suppressCreated {
			return taskActivityCandidate{}, false
		}
		kind, action = "lifecycle", "created"
	case "task.recurrence.generated":
		kind, action = "lifecycle", "generated"
	case "task.start":
		kind, action = "lifecycle", "started"
	case "task.stop":
		kind, action = "lifecycle", "stopped"
	case "task.done":
		kind, action = "lifecycle", "completed"
	case "task.reopen":
		kind, action = "lifecycle", "reopened"
	case "task.delete":
		kind, action = "lifecycle", "deleted"
	case "task.modify":
		changes := parseTaskFieldChanges(row.PayloadJSON)
		if len(changes) == 0 {
			return taskActivityCandidate{}, false
		}
		return auditCandidate(row, "change", "fields_changed", changes, nil), true
	case "task.link.add", "task.link.update", "task.link.remove":
		actionByAudit := map[string]string{"task.link.add": "link_added", "task.link.update": "link_updated", "task.link.remove": "link_removed"}
		link := taskActivityLinkFromPayload(row.PayloadJSON)
		return auditCandidate(row, "relation", actionByAudit[row.Action], nil, link), true
	default:
		return taskActivityCandidate{}, false
	}
	return auditCandidate(row, kind, action, nil, nil), true
}

func auditCandidate(row storage.AuditLogEntry, kind, action string, changes []TaskFieldChange, link *TaskActivityLink) taskActivityCandidate {
	return taskActivityCandidate{
		Entry:      TaskActivityEntry{ID: "audit:" + strconv.FormatInt(row.ID, 10), Kind: kind, Action: action, OccurredAt: row.CreatedAt, Changes: changes, Link: link},
		SourceRank: activityAuditRank, SourceID: strconv.FormatInt(row.ID, 10),
		Actor: actorColumns{Type: auditActorType(row), UserID: row.ActorUserID, TokenID: row.ActorTokenID, TokenName: row.ActorTokenName, TokenPrefix: row.ActorTokenPrefix},
	}
}

func taskActivityLinkFromPayload(raw string) *TaskActivityLink {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return &TaskActivityLink{}
	}
	return &TaskActivityLink{
		ID: activityStringValue(payload["link_id"]), Type: activityStringValue(payload["type"]),
		URL: activityStringValue(payload["url"]), Title: activityStringValue(payload["title"]),
	}
}

func activityStringValue(value any) string {
	valueString, _ := value.(string)
	return valueString
}

func resolveTaskActivityActors(candidates []taskActivityCandidate, service *Service) error {
	userIDs := make([]string, 0)
	for _, candidate := range candidates {
		if (candidate.Actor.Type == "" || candidate.Actor.Type == actorTypeUser) && candidate.Actor.UserID != nil && *candidate.Actor.UserID != "" {
			userIDs = append(userIDs, *candidate.Actor.UserID)
		}
	}
	users, err := service.resolveUserInfos(userIDs)
	if err != nil {
		return err
	}
	for index := range candidates {
		candidates[index].Entry.Actor = actorInfoFromColumns(candidates[index].Actor, "", users)
	}
	return nil
}

func auditStorageCursor(cursor *taskActivityCursor) (*storage.AuditListCursor, error) {
	if cursor == nil {
		return nil, nil
	}
	result := &storage.AuditListCursor{CreatedAt: cursor.OccurredAt}
	if cursor.SourceRank == activityAuditRank {
		id, err := strconv.ParseInt(cursor.SourceID, 10, 64)
		if err != nil || id <= 0 {
			return nil, badTaskActivityCursor()
		}
		result.ID = &id
	} else if activityAuditRank < cursor.SourceRank {
		result.IncludeCreatedAt = true
	}
	return result, nil
}

func annotationStorageCursor(cursor *taskActivityCursor) (*storage.TaskAnnotationListCursor, error) {
	if cursor == nil {
		return nil, nil
	}
	result := &storage.TaskAnnotationListCursor{CreatedAt: cursor.OccurredAt}
	if cursor.SourceRank == activityAnnotationRank {
		if strings.TrimSpace(cursor.SourceID) == "" {
			return nil, badTaskActivityCursor()
		}
		id := cursor.SourceID
		result.ID = &id
	} else if activityAnnotationRank < cursor.SourceRank {
		result.IncludeCreatedAt = true
	}
	return result, nil
}

func activityKeyAfter(left, right taskActivityCandidate) bool {
	if left.Entry.OccurredAt != right.Entry.OccurredAt {
		return left.Entry.OccurredAt > right.Entry.OccurredAt
	}
	if left.SourceRank != right.SourceRank {
		return left.SourceRank > right.SourceRank
	}
	if left.SourceRank == activityAuditRank {
		leftID, _ := strconv.ParseInt(left.SourceID, 10, 64)
		rightID, _ := strconv.ParseInt(right.SourceID, 10, 64)
		return leftID > rightID
	}
	return left.SourceID > right.SourceID
}

func activityKeyBefore(candidate taskActivityCandidate, cursor taskActivityCursor) bool {
	if candidate.Entry.OccurredAt != cursor.OccurredAt {
		return candidate.Entry.OccurredAt < cursor.OccurredAt
	}
	if candidate.SourceRank != cursor.SourceRank {
		return candidate.SourceRank < cursor.SourceRank
	}
	if candidate.SourceRank == activityAuditRank {
		candidateID, err1 := strconv.ParseInt(candidate.SourceID, 10, 64)
		cursorID, err2 := strconv.ParseInt(cursor.SourceID, 10, 64)
		return err1 == nil && err2 == nil && candidateID < cursorID
	}
	return candidate.SourceID < cursor.SourceID
}

func encodeTaskActivityCursor(cursor taskActivityCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeTaskActivityCursor(value string) (taskActivityCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return taskActivityCursor{}, badTaskActivityCursor()
	}
	var cursor taskActivityCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.Version != taskActivityCursorVersion || cursor.OccurredAt < 0 || cursor.SourceID == "" {
		return taskActivityCursor{}, badTaskActivityCursor()
	}
	if cursor.SourceRank != activityAuditRank && cursor.SourceRank != activityAnnotationRank && cursor.SourceRank != activitySnapshotRank {
		return taskActivityCursor{}, badTaskActivityCursor()
	}
	if cursor.SourceRank == activityAuditRank {
		id, err := strconv.ParseInt(cursor.SourceID, 10, 64)
		if err != nil || id <= 0 {
			return taskActivityCursor{}, badTaskActivityCursor()
		}
	}
	if cursor.SourceRank == activitySnapshotRank && cursor.SourceID != "created" {
		return taskActivityCursor{}, badTaskActivityCursor()
	}
	return cursor, nil
}

func badTaskActivityCursor() error {
	return RuntimeError{Code: "api_bad_cursor", Message: "invalid activity cursor"}
}
