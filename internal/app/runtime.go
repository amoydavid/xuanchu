package app

import (
	"fmt"

	"github.com/dajee/taskg/internal/storage/sqlite"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

type RuntimeContext struct {
	ActorUserID   string
	ActorName     string
	WorkspaceID   string
	WorkspaceSlug string
	Role          Role
}

type ServiceOptions struct {
	Store            *sqlite.Store
	Clock            Clock
	NoContext        bool
	RuntimeConfig    map[string]string
	RuntimeOverrides map[string]string
	ActorRef         string
	WorkspaceRef     string
	Runtime          *RuntimeContext
	RequestScope     *RequestScope
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

func ResolveRuntimeContext(store *sqlite.Store, userRepo *sqlite.UserRepository, workspaceRepo *sqlite.WorkspaceRepository, memberRepo *sqlite.MemberRepository, actorRef, workspaceRef string) (RuntimeContext, error) {
	user, err := resolveActor(store, userRepo, actorRef)
	if err != nil {
		return RuntimeContext{}, err
	}

	workspace, err := resolveWorkspace(store, workspaceRepo, user, workspaceRef)
	if err != nil {
		return RuntimeContext{}, err
	}
	if workspace.ArchivedAt != nil {
		return RuntimeContext{}, RuntimeError{Code: "workspace_archived", Message: fmt.Sprintf("workspace %q is archived", workspace.Slug)}
	}

	member, err := memberRepo.Get(user.ID, workspace.ID)
	if err == sqlite.ErrNotFound {
		return RuntimeContext{}, RuntimeError{Code: "membership_not_found", Message: fmt.Sprintf("user %q is not a member of workspace %q", user.Name, workspace.Slug)}
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

func resolveActor(store *sqlite.Store, userRepo *sqlite.UserRepository, actorRef string) (sqlite.User, error) {
	if actorRef != "" {
		if user, err := userRepo.GetByID(actorRef); err == nil {
			return user, nil
		}
		return userRepo.GetByName(actorRef)
	}
	if activeID, ok, err := store.GetMeta("active_user_id"); err != nil {
		return sqlite.User{}, err
	} else if ok && activeID != "" {
		return userRepo.GetByID(activeID)
	}
	return userRepo.GetByName("local")
}

func resolveWorkspace(store *sqlite.Store, workspaceRepo *sqlite.WorkspaceRepository, user sqlite.User, workspaceRef string) (sqlite.Workspace, error) {
	if workspaceRef != "" {
		return lookupWorkspace(workspaceRepo, workspaceRef)
	}
	if activeRef, ok, err := store.GetMeta(activeWorkspaceMetaKey(user.ID)); err != nil {
		return sqlite.Workspace{}, err
	} else if ok && activeRef != "" {
		return lookupWorkspace(workspaceRepo, activeRef)
	}
	if user.DefaultWorkspaceID != nil && *user.DefaultWorkspaceID != "" {
		return workspaceRepo.GetByID(*user.DefaultWorkspaceID)
	}
	return workspaceRepo.GetBySlug("local")
}

func lookupWorkspace(workspaceRepo *sqlite.WorkspaceRepository, ref string) (sqlite.Workspace, error) {
	if ws, err := workspaceRepo.GetByID(ref); err == nil {
		return ws, nil
	}
	ws, err := workspaceRepo.GetBySlug(ref)
	if err == sqlite.ErrNotFound {
		return sqlite.Workspace{}, RuntimeError{Code: "workspace_not_found", Message: fmt.Sprintf("workspace %q not found", ref)}
	}
	return ws, err
}
