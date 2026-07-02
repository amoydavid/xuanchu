package app

import (
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
)

const actorTypeUser = "user"

type actorColumns struct {
	Type        string
	UserID      *string
	TokenID     *string
	TokenName   *string
	TokenPrefix *string
}

func (rt RuntimeContext) actorColumns() actorColumns {
	if rt.IsTenantActor() {
		return actorColumns{
			Type:        auth.TokenTypeTenantAccess,
			TokenID:     stringPtr(rt.ActorTokenID),
			TokenName:   stringPtr(rt.ActorTokenName),
			TokenPrefix: stringPtr(rt.ActorTokenPrefix),
		}
	}
	return actorColumns{
		Type:   actorTypeUser,
		UserID: stringPtr(rt.ActorUserID),
	}
}

func actorInfoFromRuntime(rt RuntimeContext) task.ActorInfo {
	if rt.IsTenantActor() {
		return task.ActorInfo{
			Type: auth.TokenTypeTenantAccess,
			ID:   rt.ActorTokenID,
			Name: rt.ActorTokenName,
			Token: &task.TokenActorInfo{
				ID:     rt.ActorTokenID,
				Name:   rt.ActorTokenName,
				Prefix: rt.ActorTokenPrefix,
			},
		}
	}
	return task.ActorInfo{
		Type: actorTypeUser,
		ID:   rt.ActorUserID,
		Name: rt.ActorName,
		User: &task.UserInfo{ID: rt.ActorUserID, Name: rt.ActorName},
	}
}

func actorInfoFromColumns(cols actorColumns, fallbackUserID string, users map[string]task.UserInfo) task.ActorInfo {
	actorType := cols.Type
	if actorType == "" {
		actorType = actorTypeUser
	}
	if actorType == auth.TokenTypeTenantAccess {
		token := task.TokenActorInfo{}
		if cols.TokenID != nil {
			token.ID = *cols.TokenID
		}
		if cols.TokenName != nil {
			token.Name = *cols.TokenName
		}
		if cols.TokenPrefix != nil {
			token.Prefix = *cols.TokenPrefix
		}
		return task.ActorInfo{Type: auth.TokenTypeTenantAccess, ID: token.ID, Name: token.Name, Token: &token}
	}
	userID := fallbackUserID
	if cols.UserID != nil && *cols.UserID != "" {
		userID = *cols.UserID
	}
	ui := users[userID]
	if ui.ID == "" {
		ui = task.UserInfo{ID: userID, Name: userID}
	}
	return task.ActorInfo{Type: actorTypeUser, ID: ui.ID, Name: ui.Name, User: &ui}
}

func projectAnnotationActorColumns(row storage.ProjectAnnotation) actorColumns {
	return actorColumns{
		Type:        row.CreatedByActorType,
		UserID:      row.CreatedByUserID,
		TokenID:     row.CreatedByTokenID,
		TokenName:   row.CreatedByTokenName,
		TokenPrefix: row.CreatedByTokenPrefix,
	}
}

func taskLinkActorColumns(row storage.TaskLink) actorColumns {
	return actorColumns{
		Type:        row.CreatedByActorType,
		UserID:      row.CreatedByUserID,
		TokenID:     row.CreatedByTokenID,
		TokenName:   row.CreatedByTokenName,
		TokenPrefix: row.CreatedByTokenPrefix,
	}
}

func notificationSinkActorColumns(row storage.NotificationSink) actorColumns {
	return actorColumns{
		Type:        row.CreatedByActorType,
		UserID:      row.CreatedByUserID,
		TokenID:     row.CreatedByTokenID,
		TokenName:   row.CreatedByTokenName,
		TokenPrefix: row.CreatedByTokenPrefix,
	}
}

func reminderRuleActorColumns(row storage.ReminderRule) actorColumns {
	return actorColumns{
		Type:        row.CreatedByActorType,
		UserID:      row.CreatedByUserID,
		TokenID:     row.CreatedByTokenID,
		TokenName:   row.CreatedByTokenName,
		TokenPrefix: row.CreatedByTokenPrefix,
	}
}

func eventNotificationRuleActorColumns(row storage.EventNotificationRule) actorColumns {
	return actorColumns{
		Type:        row.CreatedByActorType,
		UserID:      row.CreatedByUserID,
		TokenID:     row.CreatedByTokenID,
		TokenName:   row.CreatedByTokenName,
		TokenPrefix: row.CreatedByTokenPrefix,
	}
}

func hookDeliveryActorColumns(row storage.HookDelivery) actorColumns {
	return actorColumns{
		Type:        row.ActorType,
		UserID:      stringPtrOrNil(row.ActorUserID),
		TokenID:     row.ActorTokenID,
		TokenName:   row.ActorTokenName,
		TokenPrefix: row.ActorTokenPrefix,
	}
}

func notificationDeliveryActorColumns(row storage.NotificationDelivery) actorColumns {
	return actorColumns{
		Type:        row.ActorType,
		UserID:      row.ActorUserID,
		TokenID:     row.ActorTokenID,
		TokenName:   row.ActorTokenName,
		TokenPrefix: row.ActorTokenPrefix,
	}
}

func stringPtrOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
