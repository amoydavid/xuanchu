package app

import (
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/authz"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// Role 复用 authz.Role，保持 app 层现有 API 稳定。
type Role = authz.Role

const (
	RoleOwner  = authz.RoleOwner
	RoleAdmin  = authz.RoleAdmin
	RoleMember = authz.RoleMember
	RoleViewer = authz.RoleViewer
)

type RuntimeContext struct {
	ActorUserID      string
	ActorName        string
	WorkspaceID      string
	WorkspaceSlug    string
	Role             Role
	DelegatorTokenID string
	DelegatorUserID  string
}

type ServiceOptions struct {
	Store                 *storage.Store
	Clock                 Clock
	NoContext             bool
	DisableScopeBootstrap bool
	RuntimeConfig         map[string]string
	RuntimeOverrides      map[string]string
	ActorRef              string
	WorkspaceRef          string
	Runtime               *RuntimeContext
	RequestScope          *RequestScope
}

type RuntimeError struct {
	Code    string
	Message string
}

func (e RuntimeError) Error() string {
	return e.Message
}

func activeWorkspaceMetaKey(userID string) string {
	return "active_workspace." + userID
}

func activeContextMetaKey(userID, workspaceID string) string {
	return "active_context." + userID + "." + workspaceID
}

func ResolveRuntimeContext(store *storage.Store, userRepo *storage.UserRepository, workspaceRepo *storage.WorkspaceRepository, memberRepo *storage.MemberRepository, actorRef, workspaceRef string) (RuntimeContext, error) {
	user, err := resolveActor(store, userRepo, actorRef)
	if err != nil {
		return RuntimeContext{}, err
	}

	workspace, err := resolveWorkspace(store, workspaceRepo, user, workspaceRef)
	if err != nil {
		return RuntimeContext{}, err
	}
	if workspace.ArchivedAt != nil {
		return RuntimeContext{}, RuntimeError{Code: authz.CodeWorkspaceArchived, Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}

	member, err := memberRepo.Get(user.ID, workspace.ID)
	if err == storage.ErrNotFound {
		return RuntimeContext{}, RuntimeError{Code: authz.CodeMembershipNotFound, Message: fmt.Sprintf("user %q is not a member of workspace %q", user.Name, workspace.Slug)}
	}
	if err != nil {
		return RuntimeContext{}, err
	}

	return RuntimeContext{
		ActorUserID:   user.ID,
		ActorName:     user.Name,
		WorkspaceID:   workspace.ID,
		WorkspaceSlug: workspace.Slug,
		Role:          Role(member.Role),
	}, nil
}

func resolveActor(store *storage.Store, userRepo *storage.UserRepository, actorRef string) (storage.User, error) {
	if actorRef != "" {
		if user, err := userRepo.GetByID(actorRef); err == nil {
			return user, nil
		}
		return userRepo.GetByName(actorRef)
	}
	if activeID, ok, err := store.GetMeta("active_user_id"); err != nil {
		return storage.User{}, err
	} else if ok && activeID != "" {
		return userRepo.GetByID(activeID)
	}
	return userRepo.GetByName("local")
}

func resolveWorkspace(store *storage.Store, workspaceRepo *storage.WorkspaceRepository, user storage.User, workspaceRef string) (storage.Workspace, error) {
	if workspaceRef != "" {
		return lookupWorkspace(workspaceRepo, workspaceRef)
	}
	if activeRef, ok, err := store.GetMeta(activeWorkspaceMetaKey(user.ID)); err != nil {
		return storage.Workspace{}, err
	} else if ok && activeRef != "" {
		return lookupWorkspace(workspaceRepo, activeRef)
	}
	if user.DefaultWorkspaceID != nil && *user.DefaultWorkspaceID != "" {
		return workspaceRepo.GetByID(*user.DefaultWorkspaceID)
	}
	return workspaceRepo.GetBySlug("local")
}

func lookupWorkspace(workspaceRepo *storage.WorkspaceRepository, ref string) (storage.Workspace, error) {
	if ws, err := workspaceRepo.GetByID(ref); err == nil {
		return ws, nil
	}
	ws, err := workspaceRepo.GetBySlug(ref)
	if err == storage.ErrNotFound {
		return storage.Workspace{}, RuntimeError{Code: "workspace_not_found", Message: fmt.Sprintf("workspace %q not found", ref)}
	}
	return ws, err
}
