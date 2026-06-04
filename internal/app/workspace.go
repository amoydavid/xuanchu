package app

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
)

var workspaceSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type UserView struct {
	ID                 string
	Name               string
	Email              *string
	DefaultWorkspaceID *string
	ExternalIDs        []task.ExternalIDInfo
	Active             bool
	CreatedAt          int64
	ModifiedAt         int64
}

type AddUserInput struct {
	Name  string
	Email string
}

type WorkspaceView struct {
	ID          string
	Slug        string
	Name        string
	Description string
	Visibility  string
	CreatedBy   *task.UserInfo
	ArchivedAt  *int64
	Role        Role
	Active      bool
	CreatedAt   int64
	ModifiedAt  int64
}

type AddWorkspaceInput struct {
	Slug        string
	Name        string
	Description string
	Visibility  string
}

type ModifyWorkspaceInput struct {
	Name        *string
	Description *string
	Visibility  *string
}

type MemberView struct {
	UserID     string
	Name       string
	Email      *string
	Role       Role
	JoinedAt   int64
	ModifiedAt int64
}

type AddMemberInput struct {
	WorkspaceRef string
	UserRef      string
	Role         Role
}

type ChangeMemberRoleInput struct {
	WorkspaceRef string
	UserRef      string
	Role         Role
}

func (s *Service) ListUsers() ([]UserView, error) {
	users, err := s.userRepo.List()
	if err != nil {
		return nil, err
	}
	userIDs := make([]string, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}
	extByUser, err := s.loadExternalIDsByUsers(userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]UserView, 0, len(users))
	for _, user := range users {
		out = append(out, userViewFromRow(user, user.ID == s.runtime.ActorUserID, extByUser[user.ID]))
	}
	return out, nil
}

func (s *Service) AddUser(input AddUserInput) (UserView, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return UserView{}, fmt.Errorf("user name is required")
	}
	email := strings.TrimSpace(input.Email)
	slug, err := normalizeWorkspaceSlug(name)
	if err != nil {
		return UserView{}, fmt.Errorf("user name %q is not a valid personal workspace slug: %w", name, err)
	}
	var created UserView
	err = s.withAuditEntries(func(tx *Service) ([]AuditEntry, error) {
		user, workspace, err := tx.addUserLocked(name, email, slug)
		if err != nil {
			return nil, err
		}
		created = userViewFromRow(user, false, nil)
		return []AuditEntry{
			{
				Action:     "user.add",
				TargetType: "user",
				TargetID:   user.ID,
			},
			{
				Action:      "workspace.add",
				WorkspaceID: &workspace.ID,
				TargetType:  "workspace",
				TargetID:    workspace.ID,
			},
		}, nil
	})
	return created, err
}

func (s *Service) addUserLocked(name, email, slug string) (sqlite.User, sqlite.Workspace, error) {
	now := s.clock.Unix()
	user := sqlite.User{
		ID:         uuid.NewString(),
		Name:       name,
		CreatedAt:  now,
		ModifiedAt: now,
	}
	if email != "" {
		user.Email = &email
	}
	createdUser, err := s.userRepo.Create(user)
	if err != nil {
		return sqlite.User{}, sqlite.Workspace{}, err
	}
	workspace := sqlite.Workspace{
		ID:              uuid.NewString(),
		Slug:            slug,
		Name:            name,
		CreatedByUserID: &s.runtime.ActorUserID,
		Visibility:      "private",
		SettingsJSON:    "{}",
		CreatedAt:       now,
		ModifiedAt:      now,
	}
	createdWorkspace, err := s.workspaceRepo.Create(workspace)
	if err != nil {
		return sqlite.User{}, sqlite.Workspace{}, err
	}
	if err := s.memberRepo.Upsert(sqlite.Membership{
		UserID:      createdUser.ID,
		WorkspaceID: createdWorkspace.ID,
		Role:        string(RoleOwner),
		JoinedAt:    now,
		ModifiedAt:  now,
	}); err != nil {
		return sqlite.User{}, sqlite.Workspace{}, err
	}
	if err := s.userRepo.UpdateDefaultWorkspace(createdUser.ID, createdWorkspace.ID, now); err != nil {
		return sqlite.User{}, sqlite.Workspace{}, err
	}
	createdUser.DefaultWorkspaceID = &createdWorkspace.ID
	createdUser.ModifiedAt = now
	return createdUser, createdWorkspace, nil
}

func (s *Service) UseUser(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("user reference is required")
	}
	return s.withAudit("user.use", func(tx *Service) (AuditEntry, error) {
		user, err := tx.useUserLocked(ref)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "user",
			TargetID:   user.ID,
		}, nil
	})
}

func (s *Service) useUserLocked(ref string) (sqlite.User, error) {
	user, err := s.resolveUser(ref)
	if err != nil {
		return sqlite.User{}, err
	}
	// Avoid switching the active user into an unusable identity; each active
	// user must still resolve to a non-archived workspace.
	if _, err := ResolveRuntimeContext(s.store, s.userRepo, s.workspaceRepo, s.memberRepo, user.ID, ""); err != nil {
		return sqlite.User{}, err
	}
	if err := s.store.SetMeta("active_user_id", user.ID); err != nil {
		return sqlite.User{}, err
	}
	return user, nil
}

func (s *Service) UserInfo(ref string) (UserView, error) {
	user, err := s.resolveUser(ref)
	if err != nil {
		return UserView{}, err
	}
	extIDs, err := s.ListExternalIDs(user.ID)
	if err != nil {
		return UserView{}, err
	}
	return userViewFromRow(user, user.ID == s.runtime.ActorUserID, extIDs), nil
}

func (s *Service) ListWorkspaces(includeArchived bool) ([]WorkspaceView, error) {
	rows, err := s.workspaceRepo.ListVisibleForUser(s.runtime.ActorUserID, includeArchived)
	if err != nil {
		return nil, err
	}
	out := make([]WorkspaceView, 0, len(rows))
	for _, row := range rows {
		if s.requestScope != nil && !s.requestScope.AllowsWorkspace(row.Workspace.ID) {
			continue
		}
		out = append(out, workspaceViewFromRow(row.Workspace, Role(row.Role), row.Workspace.ID == s.runtime.WorkspaceID))
	}
	return out, nil
}

func (s *Service) AddWorkspace(input AddWorkspaceInput) (WorkspaceView, error) {
	var created WorkspaceView
	slug, err := normalizeWorkspaceSlug(input.Slug)
	if err != nil {
		return WorkspaceView{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = slug
	}
	visibility, err := normalizeWorkspaceVisibility(input.Visibility)
	if err != nil {
		return WorkspaceView{}, err
	}
	description := strings.TrimSpace(input.Description)
	err = s.withAudit("workspace.add", func(tx *Service) (AuditEntry, error) {
		workspace, err := tx.addWorkspaceLocked(slug, name, description, visibility)
		if err != nil {
			return AuditEntry{}, err
		}
		created = workspaceViewFromRow(workspace, RoleOwner, false)
		return AuditEntry{
			WorkspaceID: &workspace.ID,
			TargetType:  "workspace",
			TargetID:    workspace.ID,
		}, nil
	})
	return created, err
}

func (s *Service) addWorkspaceLocked(slug, name, description, visibility string) (sqlite.Workspace, error) {
	now := s.clock.Unix()
	workspace := sqlite.Workspace{
		ID:              uuid.NewString(),
		Slug:            slug,
		Name:            name,
		Description:     description,
		CreatedByUserID: &s.runtime.ActorUserID,
		Visibility:      visibility,
		SettingsJSON:    "{}",
		CreatedAt:       now,
		ModifiedAt:      now,
	}
	created, err := s.workspaceRepo.Create(workspace)
	if err != nil {
		return sqlite.Workspace{}, err
	}
	if err := s.memberRepo.Upsert(sqlite.Membership{
		UserID:      s.runtime.ActorUserID,
		WorkspaceID: created.ID,
		Role:        string(RoleOwner),
		JoinedAt:    now,
		ModifiedAt:  now,
	}); err != nil {
		return sqlite.Workspace{}, err
	}
	actor, err := s.userRepo.GetByID(s.runtime.ActorUserID)
	if err != nil {
		return sqlite.Workspace{}, err
	}
	if actor.DefaultWorkspaceID == nil || *actor.DefaultWorkspaceID == "" {
		if err := s.userRepo.UpdateDefaultWorkspace(actor.ID, created.ID, now); err != nil {
			return sqlite.Workspace{}, err
		}
	}
	return created, nil
}

func (s *Service) UseWorkspace(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("workspace reference is required")
	}
	return s.withAudit("workspace.use", func(tx *Service) (AuditEntry, error) {
		workspace, err := tx.useWorkspaceLocked(ref)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &workspace.ID,
			TargetType:  "workspace",
			TargetID:    workspace.ID,
		}, nil
	})
}

func (s *Service) useWorkspaceLocked(ref string) (sqlite.Workspace, error) {
	workspace, err := lookupWorkspace(s.workspaceRepo, ref)
	if err != nil {
		return sqlite.Workspace{}, err
	}
	if workspace.ArchivedAt != nil {
		return sqlite.Workspace{}, RuntimeError{Code: "workspace_archived", Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}
	if _, err := s.memberRepo.Get(s.runtime.ActorUserID, workspace.ID); err != nil {
		if err == sqlite.ErrNotFound {
			return sqlite.Workspace{}, RuntimeError{Code: "membership_not_found", Message: fmt.Sprintf("user %q is not a member of workspace %q", s.runtime.ActorName, workspace.Slug)}
		}
		return sqlite.Workspace{}, err
	}
	if err := s.store.SetMeta(activeWorkspaceMetaKey(s.runtime.ActorUserID), workspace.ID); err != nil {
		return sqlite.Workspace{}, err
	}
	return workspace, nil
}

func (s *Service) WorkspaceInfo(ref string) (WorkspaceView, error) {
	workspace, role, err := s.resolveWorkspaceForActor(ref)
	if err != nil {
		return WorkspaceView{}, err
	}
	return workspaceViewFromRow(workspace, role, workspace.ID == s.runtime.WorkspaceID), nil
}

func (s *Service) ModifyWorkspace(ref string, input ModifyWorkspaceInput) error {
	workspace, role, err := s.resolveWorkspaceForActor(ref)
	if err != nil {
		return err
	}
	if workspace.ArchivedAt != nil {
		return RuntimeError{Code: "workspace_archived", Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}
	if err := requireRolePermission(role, PermissionWorkspaceModify); err != nil {
		return err
	}
	normalized, err := normalizeWorkspaceModifyInput(input)
	if err != nil {
		return err
	}
	return s.withAudit("workspace.modify", func(tx *Service) (AuditEntry, error) {
		if err := tx.modifyWorkspaceLocked(workspace.ID, normalized); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &workspace.ID,
			TargetType:  "workspace",
			TargetID:    workspace.ID,
		}, nil
	})
}

func (s *Service) modifyWorkspaceLocked(workspaceID string, input ModifyWorkspaceInput) error {
	return s.workspaceRepo.UpdateMetadata(workspaceID, sqlite.WorkspaceMetadataUpdate{
		Name:        input.Name,
		Description: input.Description,
		Visibility:  input.Visibility,
	}, s.clock.Unix())
}

func (s *Service) ArchiveWorkspace(ref string) error {
	workspace, role, err := s.resolveWorkspaceForActor(ref)
	if err != nil {
		return err
	}
	if workspace.ArchivedAt != nil {
		return RuntimeError{Code: "workspace_archived", Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}
	if err := requireRolePermission(role, PermissionWorkspaceArchive); err != nil {
		return err
	}
	return s.withAudit("workspace.archive", func(tx *Service) (AuditEntry, error) {
		payload, err := tx.archiveWorkspaceLocked(workspace)
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &workspace.ID,
			TargetType:  "workspace",
			TargetID:    workspace.ID,
			Payload:     payload,
		}, nil
	})
}

func (s *Service) archiveWorkspaceLocked(target sqlite.Workspace) (map[string]any, error) {
	users, err := s.userRepo.List()
	if err != nil {
		return nil, err
	}
	meta, err := s.store.ListMeta()
	if err != nil {
		return nil, err
	}
	type reassignment struct {
		userID        string
		workspaceID   string
		activeCurrent bool
	}
	affected := map[string]reassignment{}
	for _, user := range users {
		current := reassignment{userID: user.ID}
		if user.DefaultWorkspaceID != nil && *user.DefaultWorkspaceID == target.ID {
			current.workspaceID = target.ID
		}
		if active, ok := meta[activeWorkspaceMetaKey(user.ID)]; ok && active == target.ID {
			current.activeCurrent = true
		}
		if current.workspaceID != "" || current.activeCurrent {
			replacement, err := s.memberRepo.OtherUnarchivedWorkspaces(user.ID, target.ID)
			if err != nil {
				return nil, err
			}
			if len(replacement) == 0 {
				return nil, fmt.Errorf("cannot archive workspace %q: user %q has no other unarchived workspace", target.Slug, user.Name)
			}
			current.workspaceID = replacement[0].ID
			affected[user.ID] = current
		}
	}
	for _, user := range users {
		current, ok := affected[user.ID]
		if !ok {
			continue
		}
		if user.DefaultWorkspaceID != nil && *user.DefaultWorkspaceID == target.ID {
			if err := s.userRepo.UpdateDefaultWorkspace(user.ID, current.workspaceID, s.clock.Unix()); err != nil {
				return nil, err
			}
		}
		if current.activeCurrent {
			if err := s.store.SetMeta(activeWorkspaceMetaKey(user.ID), current.workspaceID); err != nil {
				return nil, err
			}
		}
	}
	if err := s.workspaceRepo.Archive(target.ID, s.clock.Unix()); err != nil {
		return nil, err
	}
	reassigned := make([]map[string]any, 0, len(affected))
	keys := make([]string, 0, len(affected))
	for userID := range affected {
		keys = append(keys, userID)
	}
	sort.Strings(keys)
	for _, userID := range keys {
		item := affected[userID]
		reassigned = append(reassigned, map[string]any{
			"user_id":                  item.userID,
			"replacement_workspace_id": item.workspaceID,
		})
	}
	return map[string]any{"reassigned": reassigned}, nil
}

func (s *Service) ListMembers(workspaceRef string) ([]MemberView, error) {
	workspace, _, err := s.resolveWorkspaceForActor(workspaceRef)
	if err != nil {
		return nil, err
	}
	if workspace.ArchivedAt != nil {
		return nil, RuntimeError{Code: "workspace_archived", Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}
	rows, err := s.memberRepo.List(workspace.ID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberView, 0, len(rows))
	for _, row := range rows {
		out = append(out, MemberView{
			UserID:     row.User.ID,
			Name:       row.User.Name,
			Email:      row.User.Email,
			Role:       Role(row.Membership.Role),
			JoinedAt:   row.Membership.JoinedAt,
			ModifiedAt: row.Membership.ModifiedAt,
		})
	}
	return out, nil
}

func (s *Service) AddMember(input AddMemberInput) error {
	workspace, role, err := s.resolveWorkspaceForActor(input.WorkspaceRef)
	if err != nil {
		return err
	}
	if workspace.ArchivedAt != nil {
		return RuntimeError{Code: "workspace_archived", Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}
	targetRole, err := normalizeRole(input.Role, RoleMember)
	if err != nil {
		return err
	}
	if err := requireMemberManagement(role, targetRole, ""); err != nil {
		return err
	}
	user, err := s.resolveUser(input.UserRef)
	if err != nil {
		return err
	}
	return s.withAudit("member.add", func(tx *Service) (AuditEntry, error) {
		if err := tx.addMemberLocked(workspace.ID, user.ID, targetRole); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &workspace.ID,
			TargetType:  "member",
			TargetID:    user.ID,
		}, nil
	})
}

func (s *Service) addMemberLocked(workspaceID, userID string, role Role) error {
	if _, err := s.memberRepo.Get(userID, workspaceID); err == nil {
		return fmt.Errorf("user is already a member")
	} else if err != sqlite.ErrNotFound {
		return err
	}
	now := s.clock.Unix()
	return s.memberRepo.Upsert(sqlite.Membership{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        string(role),
		JoinedAt:    now,
		ModifiedAt:  now,
	})
}

func (s *Service) ChangeMemberRole(input ChangeMemberRoleInput) error {
	workspace, role, err := s.resolveWorkspaceForActor(input.WorkspaceRef)
	if err != nil {
		return err
	}
	if workspace.ArchivedAt != nil {
		return RuntimeError{Code: "workspace_archived", Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}
	user, err := s.resolveUser(input.UserRef)
	if err != nil {
		return err
	}
	member, err := s.memberRepo.Get(user.ID, workspace.ID)
	if err != nil {
		return err
	}
	targetRole, err := normalizeRole(input.Role, Role(""))
	if err != nil {
		return err
	}
	if err := requireMemberManagement(role, targetRole, Role(member.Role)); err != nil {
		return err
	}
	if Role(member.Role) == RoleOwner && targetRole != RoleOwner {
		count, err := s.memberRepo.CountOwners(workspace.ID)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("cannot downgrade last owner")
		}
	}
	return s.withAudit("member.role", func(tx *Service) (AuditEntry, error) {
		if err := tx.changeMemberRoleLocked(workspace.ID, user.ID, targetRole); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			WorkspaceID: &workspace.ID,
			TargetType:  "member",
			TargetID:    user.ID,
		}, nil
	})
}

func (s *Service) changeMemberRoleLocked(workspaceID, userID string, role Role) error {
	return s.memberRepo.UpdateRole(userID, workspaceID, string(role), s.clock.Unix())
}

func (s *Service) resolveUser(ref string) (sqlite.User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return sqlite.User{}, fmt.Errorf("user reference is required")
	}
	if user, err := s.userRepo.GetByID(ref); err == nil {
		return user, nil
	}
	if idx := strings.Index(ref, ":"); idx > 0 {
		provider := ref[:idx]
		externalID := ref[idx+1:]
		if provider != "" && externalID != "" {
			if user, err := s.userRepo.GetByExternalID(provider, externalID); err == nil {
				return user, nil
			}
		}
	}
	if user, err := s.userRepo.GetByName(ref); err == nil {
		return user, nil
	}
	user, err := s.userRepo.GetByEmail(ref)
	if err == sqlite.ErrNotFound {
		return sqlite.User{}, RuntimeError{Code: "user_not_found", Message: fmt.Sprintf("user %q not found", ref)}
	}
	return user, err
}

func (s *Service) resolveWorkspaceForActor(ref string) (sqlite.Workspace, Role, error) {
	if strings.TrimSpace(ref) == "" {
		workspace, err := s.workspaceRepo.GetByID(s.runtime.WorkspaceID)
		return workspace, s.runtime.Role, err
	}
	workspace, err := lookupWorkspace(s.workspaceRepo, strings.TrimSpace(ref))
	if err != nil {
		return sqlite.Workspace{}, "", err
	}
	member, err := s.memberRepo.Get(s.runtime.ActorUserID, workspace.ID)
	if err == sqlite.ErrNotFound {
		return sqlite.Workspace{}, "", RuntimeError{Code: "membership_not_found", Message: fmt.Sprintf("user %q is not a member of workspace %q", s.runtime.ActorName, workspace.Slug)}
	}
	if err != nil {
		return sqlite.Workspace{}, "", err
	}
	return workspace, Role(member.Role), nil
}

func requireRolePermission(role Role, permission Permission) error {
	if allowedForRole(role, permission) {
		return nil
	}
	return PermissionError{Code: "permission_denied", Message: "permission denied"}
}

func allowedForRole(role Role, p Permission) bool {
	switch role {
	case RoleOwner:
		return true
	case RoleAdmin:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectManage, PermissionProjectConfigRead, PermissionProjectConfigWrite,
			PermissionContextUse, PermissionContextManage, PermissionUDAManage, PermissionWorkspaceRead, PermissionWorkspaceModify, PermissionMemberManage, PermissionAuditRead,
			PermissionTokenRead, PermissionTokenWrite,
			PermissionHookRead, PermissionHookWrite:
			return true
		}
	case RoleMember:
		switch p {
		case PermissionTaskRead, PermissionTaskWrite,
			PermissionProjectRead, PermissionProjectConfigRead,
			PermissionContextUse, PermissionContextManage, PermissionWorkspaceRead:
			return true
		}
	case RoleViewer:
		switch p {
		case PermissionTaskRead, PermissionProjectRead, PermissionProjectConfigRead, PermissionContextUse, PermissionWorkspaceRead:
			return true
		}
	}
	return false
}

func requireMemberManagement(actorRole, targetRole, currentRole Role) error {
	if targetRole == RoleOwner || currentRole == RoleOwner {
		return requireRolePermission(actorRole, PermissionMemberManageOwner)
	}
	return requireRolePermission(actorRole, PermissionMemberManage)
}

func normalizeRole(role Role, fallback Role) (Role, error) {
	if role == "" {
		role = fallback
	}
	switch role {
	case RoleOwner, RoleAdmin, RoleMember, RoleViewer:
		return role, nil
	default:
		return "", fmt.Errorf("invalid role %q", role)
	}
}

func normalizeWorkspaceSlug(slug string) (string, error) {
	slug = strings.TrimSpace(slug)
	if !workspaceSlugPattern.MatchString(slug) {
		return "", fmt.Errorf("invalid workspace slug %q", slug)
	}
	return slug, nil
}

func normalizeWorkspaceVisibility(visibility string) (string, error) {
	visibility = strings.TrimSpace(visibility)
	if visibility == "" {
		return "private", nil
	}
	switch visibility {
	case "private", "team", "public":
		return visibility, nil
	default:
		return "", fmt.Errorf("invalid workspace visibility %q", visibility)
	}
}

func normalizeWorkspaceModifyInput(input ModifyWorkspaceInput) (ModifyWorkspaceInput, error) {
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return ModifyWorkspaceInput{}, fmt.Errorf("workspace name cannot be empty")
		}
		input.Name = &name
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		input.Description = &description
	}
	if input.Visibility != nil {
		visibility, err := normalizeWorkspaceVisibility(*input.Visibility)
		if err != nil {
			return ModifyWorkspaceInput{}, err
		}
		input.Visibility = &visibility
	}
	return input, nil
}

func userViewFromRow(user sqlite.User, active bool, externalIDs []task.ExternalIDInfo) UserView {
	return UserView{
		ID:                 user.ID,
		Name:               user.Name,
		Email:              user.Email,
		DefaultWorkspaceID: user.DefaultWorkspaceID,
		ExternalIDs:        externalIDs,
		Active:             active,
		CreatedAt:          user.CreatedAt,
		ModifiedAt:         user.ModifiedAt,
	}
}

func (s *Service) BindExternalID(userID, provider, externalID string) error {
	provider = strings.TrimSpace(provider)
	externalID = strings.TrimSpace(externalID)
	if provider == "" || externalID == "" {
		return RuntimeError{Code: "invalid_input", Message: "provider and external_id are required"}
	}
	if userID != s.runtime.ActorUserID {
		if err := requireRolePermission(s.runtime.Role, PermissionWorkspaceModify); err != nil {
			return RuntimeError{Code: "permission_denied", Message: "only admin/owner can bind external IDs for other users"}
		}
	}
	return s.withAudit("user.bind_external_id", func(tx *Service) (AuditEntry, error) {
		_, err := tx.extIDRepo.Create(sqlite.UserExternalID{
			ID:         uuid.NewString(),
			UserID:     userID,
			Provider:   provider,
			ExternalID: externalID,
			CreatedAt:  s.clock.Unix(),
		})
		if err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "user",
			TargetID:   userID,
			Payload:    map[string]any{"provider": provider, "external_id": externalID},
		}, nil
	})
}

func (s *Service) UnbindExternalID(userID, provider, externalID string) error {
	provider = strings.TrimSpace(provider)
	externalID = strings.TrimSpace(externalID)
	if provider == "" || externalID == "" {
		return RuntimeError{Code: "invalid_input", Message: "provider and external_id are required"}
	}
	if userID != s.runtime.ActorUserID {
		if err := requireRolePermission(s.runtime.Role, PermissionWorkspaceModify); err != nil {
			return RuntimeError{Code: "permission_denied", Message: "only admin/owner can unbind external IDs for other users"}
		}
	}
	return s.withAudit("user.unbind_external_id", func(tx *Service) (AuditEntry, error) {
		if err := tx.extIDRepo.Delete(userID, provider, externalID); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{
			TargetType: "user",
			TargetID:   userID,
			Payload:    map[string]any{"provider": provider, "external_id": externalID},
		}, nil
	})
}

func (s *Service) ListExternalIDs(userID string) ([]task.ExternalIDInfo, error) {
	rows, err := s.extIDRepo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	out := make([]task.ExternalIDInfo, len(rows))
	for i, row := range rows {
		out[i] = task.ExternalIDInfo{Provider: row.Provider, ExternalID: row.ExternalID}
	}
	return out, nil
}

func (s *Service) loadExternalIDsByUsers(userIDs []string) (map[string][]task.ExternalIDInfo, error) {
	rows, err := s.extIDRepo.ListByUsers(userIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]task.ExternalIDInfo)
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], task.ExternalIDInfo{
			Provider:   row.Provider,
			ExternalID: row.ExternalID,
		})
	}
	return result, nil
}

func workspaceViewFromRow(workspace sqlite.Workspace, role Role, active bool) WorkspaceView {
	var createdBy *task.UserInfo
	if workspace.CreatedByUserID != nil {
		createdBy = &task.UserInfo{ID: *workspace.CreatedByUserID}
	}
	return WorkspaceView{
		ID:          workspace.ID,
		Slug:        workspace.Slug,
		Name:        workspace.Name,
		Description: workspace.Description,
		Visibility:  workspace.Visibility,
		CreatedBy:   createdBy,
		ArchivedAt:  workspace.ArchivedAt,
		Role:        role,
		Active:      active,
		CreatedAt:   workspace.CreatedAt,
		ModifiedAt:  workspace.ModifiedAt,
	}
}

func (s *Service) TaskAddLink(taskRef, linkType, url, title string) (task.TaskLinkInfo, error) {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return task.TaskLinkInfo{}, err
	}
	var result task.TaskLinkInfo
	if err := s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		created, updated, err := tx.addLinkLocked(taskRef, linkType, url, title)
		if err != nil {
			return nil, nil, err
		}
		result = created
		entry := AuditEntry{
			Action:     "task.link.add",
			TargetType: "task",
			TargetID:   updated.UUID,
			Payload:    map[string]any{"link_id": created.ID, "type": linkType, "url": url},
		}
		event := buildTaskHookEvent("task.modified", updated, tx.runtime, tx.clock.Unix())
		return []AuditEntry{entry}, []HookEvent{event}, nil
	}); err != nil {
		return task.TaskLinkInfo{}, err
	}
	return result, nil
}

func (s *Service) addLinkLocked(taskRef, linkType, url, title string) (task.TaskLinkInfo, task.Task, error) {
	tsk, err := s.resolveTargetForWrite(taskRef)
	if err != nil {
		return task.TaskLinkInfo{}, task.Task{}, err
	}
	if tsk.Status == task.StatusCompleted || tsk.Status == task.StatusDeleted {
		return task.TaskLinkInfo{}, task.Task{}, RuntimeError{Code: "task_not_writable", Message: fmt.Sprintf("cannot add link to %s task", tsk.Status)}
	}
	linkType = strings.TrimSpace(linkType)
	if linkType == "" {
		return task.TaskLinkInfo{}, task.Task{}, RuntimeError{Code: "link_type_required", Message: "link type is required"}
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return task.TaskLinkInfo{}, task.Task{}, RuntimeError{Code: "link_url_required", Message: "link url is required"}
	}
	now := s.clock.Unix()
	link := sqlite.TaskLink{
		ID:        uuid.NewString(),
		TaskUUID:  tsk.UUID,
		Type:      linkType,
		URL:       url,
		Title:     strings.TrimSpace(title),
		CreatedAt: now,
		CreatedBy: s.runtime.ActorUserID,
	}
	created, err := sqlite.NewTaskLinkRepository(s.store.DB()).Create(link)
	if err != nil {
		if sqlite.IsUniqueConstraintError(err) {
			return task.TaskLinkInfo{}, task.Task{}, RuntimeError{Code: "link_duplicate", Message: "this URL is already linked to the task"}
		}
		return task.TaskLinkInfo{}, task.Task{}, err
	}
	tsk.Modified = now
	if err := s.repo.Update(tsk); err != nil {
		return task.TaskLinkInfo{}, task.Task{}, err
	}
	updated, err := s.repo.GetByUUID(s.workspaceID, tsk.UUID)
	if err != nil {
		return task.TaskLinkInfo{}, task.Task{}, err
	}
	return task.TaskLinkInfo{
		ID: created.ID, Type: created.Type, URL: created.URL,
		Title: created.Title, CreatedAt: created.CreatedAt,
		CreatedBy: task.UserInfo{ID: created.CreatedBy},
	}, updated, nil
}

func (s *Service) TaskRemoveLink(taskRef, linkID string) error {
	if err := s.Require(PermissionTaskWrite); err != nil {
		return err
	}
	return s.withAuditEntriesAndEvents(func(tx *Service) ([]AuditEntry, []HookEvent, error) {
		updated, err := tx.removeLinkLocked(taskRef, linkID)
		if err != nil {
			return nil, nil, err
		}
		entry := AuditEntry{
			Action:     "task.link.remove",
			TargetType: "task",
			TargetID:   updated.UUID,
			Payload:    map[string]any{"link_id": linkID},
		}
		event := buildTaskHookEvent("task.modified", updated, tx.runtime, tx.clock.Unix())
		return []AuditEntry{entry}, []HookEvent{event}, nil
	})
}

func (s *Service) removeLinkLocked(taskRef, linkID string) (task.Task, error) {
	tsk, err := s.resolveTargetForWrite(taskRef)
	if err != nil {
		return task.Task{}, err
	}
	linkRepo := sqlite.NewTaskLinkRepository(s.store.DB())
	link, err := linkRepo.GetByID(linkID)
	if err != nil {
		return task.Task{}, RuntimeError{Code: "link_not_found", Message: fmt.Sprintf("link %q not found", linkID)}
	}
	if link.TaskUUID != tsk.UUID {
		return task.Task{}, RuntimeError{Code: "link_not_found", Message: fmt.Sprintf("link %q not found", linkID)}
	}
	if err := linkRepo.Delete(linkID); err != nil {
		return task.Task{}, err
	}
	tsk.Modified = s.clock.Unix()
	if err := s.repo.Update(tsk); err != nil {
		return task.Task{}, err
	}
	updated, err := s.repo.GetByUUID(s.workspaceID, tsk.UUID)
	if err != nil {
		return task.Task{}, err
	}
	return updated, nil
}
