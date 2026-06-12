package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

type AdminOwnerInput struct {
	Name  string
	Email string
}

type AdminCreateWorkspaceInput struct {
	AdminTokenName string
	Slug           string
	Name           string
	Description    string
	Visibility     string
	Owner          AdminOwnerInput
}

type AdminCreateWorkspaceResult struct {
	Workspace WorkspaceView
	Owner     UserView
}

type AdminCreateWorkspaceAdminInput struct {
	AdminTokenName string
	WorkspaceRef   string
	Name           string
	Email          string
	Role           Role
}

type AdminCreateWorkspaceAdminResult struct {
	Workspace  WorkspaceView
	Admin      UserView
	Membership MemberView
}

type AdminCreateAgentTokenInput struct {
	AdminTokenName string
	WorkspaceRef   string
	Name           string
	UserRef        string
	Scopes         []string
	ProjectRefs    []string
	ExpiresIn      *time.Duration
}

func (s *Service) AdminCreateWorkspace(input AdminCreateWorkspaceInput) (AdminCreateWorkspaceResult, error) {
	slug, err := normalizeWorkspaceSlug(input.Slug)
	if err != nil {
		return AdminCreateWorkspaceResult{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = slug
	}
	description := strings.TrimSpace(input.Description)
	visibility, err := normalizeWorkspaceVisibility(input.Visibility)
	if err != nil {
		return AdminCreateWorkspaceResult{}, err
	}
	ownerName := strings.TrimSpace(input.Owner.Name)
	if ownerName == "" {
		return AdminCreateWorkspaceResult{}, RuntimeError{Code: "admin_owner_required", Message: "owner name is required"}
	}
	ownerEmail := strings.TrimSpace(input.Owner.Email)

	var result AdminCreateWorkspaceResult
	err = s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		owner, err := txSvc.findOrCreateAdminOwner(ownerName, ownerEmail)
		if err != nil {
			return err
		}
		if _, err := txSvc.workspaceRepo.GetBySlug(slug); err == nil {
			return RuntimeError{Code: "admin_workspace_exists", Message: fmt.Sprintf("workspace %q already exists", slug)}
		} else if err != storage.ErrNotFound {
			return err
		}
		now := txSvc.clock.Unix()
		workspace := storage.Workspace{
			ID:              uuid.NewString(),
			Slug:            slug,
			Name:            name,
			Description:     description,
			CreatedByUserID: &owner.ID,
			Visibility:      visibility,
			SettingsJSON:    "{}",
			CreatedAt:       now,
			ModifiedAt:      now,
		}
		createdWorkspace, err := txSvc.workspaceRepo.Create(workspace)
		if err != nil {
			if storage.IsUniqueConstraintError(err) {
				return RuntimeError{Code: "admin_workspace_exists", Message: fmt.Sprintf("workspace %q already exists", slug)}
			}
			return err
		}
		if err := txSvc.memberRepo.Upsert(storage.Membership{
			UserID:      owner.ID,
			WorkspaceID: createdWorkspace.ID,
			Role:        string(RoleOwner),
			JoinedAt:    now,
			ModifiedAt:  now,
		}); err != nil {
			return err
		}
		if owner.DefaultWorkspaceID == nil || *owner.DefaultWorkspaceID == "" {
			if err := txSvc.userRepo.UpdateDefaultWorkspace(owner.ID, createdWorkspace.ID, now); err != nil {
				return err
			}
			owner.DefaultWorkspaceID = &createdWorkspace.ID
		}
		if err := txSvc.ensureBuiltinConfigDefinitions(createdWorkspace.ID); err != nil {
			return err
		}
		if err := txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
			Action:      "admin.workspace.create",
			WorkspaceID: &createdWorkspace.ID,
			TargetType:  "workspace",
			TargetID:    createdWorkspace.ID,
			Payload: map[string]any{
				"owner_user_id":  owner.ID,
				"workspace_slug": createdWorkspace.Slug,
			},
		}, input.AdminTokenName); err != nil {
			return err
		}
		result = AdminCreateWorkspaceResult{
			Workspace: workspaceViewFromRow(createdWorkspace, RoleOwner, false),
			Owner:     userViewFromRow(owner, false, nil),
		}
		return nil
	})
	return result, err
}

func (s *Service) AdminCreateWorkspaceAdmin(input AdminCreateWorkspaceAdminInput) (AdminCreateWorkspaceAdminResult, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return AdminCreateWorkspaceAdminResult{}, RuntimeError{Code: "admin_owner_required", Message: "admin name is required"}
	}
	email := strings.TrimSpace(input.Email)
	role, err := normalizeAdminBootstrapRole(input.Role)
	if err != nil {
		return AdminCreateWorkspaceAdminResult{}, err
	}
	var result AdminCreateWorkspaceAdminResult
	err = s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		workspace, err := lookupWorkspace(txSvc.workspaceRepo, strings.TrimSpace(input.WorkspaceRef))
		if err != nil {
			return err
		}
		if workspace.ArchivedAt != nil {
			return RuntimeError{Code: "workspace_archived", Message: "workspace is archived"}
		}
		user, err := txSvc.findOrCreateAdminOwner(name, email)
		if err != nil {
			return err
		}
		now := txSvc.clock.Unix()
		member, err := txSvc.memberRepo.Get(user.ID, workspace.ID)
		action := ""
		switch {
		case err == storage.ErrNotFound:
			member = storage.Membership{
				UserID:      user.ID,
				WorkspaceID: workspace.ID,
				Role:        string(role),
				JoinedAt:    now,
				ModifiedAt:  now,
			}
			if err := txSvc.memberRepo.Upsert(member); err != nil {
				return err
			}
			action = "admin.workspace_admin.create"
		case err != nil:
			return err
		default:
			currentRole := Role(member.Role)
			if currentRole == RoleOwner && role == RoleAdmin {
				return RuntimeError{Code: "admin_role_invalid", Message: "cannot downgrade existing owner"}
			}
			if currentRole != role {
				if err := txSvc.memberRepo.UpdateRole(user.ID, workspace.ID, string(role), now); err != nil {
					return err
				}
				member.Role = string(role)
				member.ModifiedAt = now
				action = "admin.workspace_admin.promote"
			}
		}
		if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID == "" {
			if err := txSvc.userRepo.UpdateDefaultWorkspace(user.ID, workspace.ID, now); err != nil {
				return err
			}
			user.DefaultWorkspaceID = &workspace.ID
		}
		if action != "" {
			if err := txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
				Action:      action,
				WorkspaceID: &workspace.ID,
				TargetType:  "member",
				TargetID:    user.ID,
				Payload: map[string]any{
					"user_id":        user.ID,
					"workspace_id":   workspace.ID,
					"workspace_slug": workspace.Slug,
					"role":           role,
				},
			}, input.AdminTokenName); err != nil {
				return err
			}
		}
		extByUser, err := txSvc.loadExternalIDsByUsers([]string{user.ID})
		if err != nil {
			return err
		}
		result = AdminCreateWorkspaceAdminResult{
			Workspace: workspaceViewFromRow(workspace, role, false),
			Admin:     userViewFromRow(user, false, extByUser[user.ID]),
			Membership: MemberView{
				UserID:     user.ID,
				Name:       user.Name,
				Email:      user.Email,
				Role:       Role(member.Role),
				JoinedAt:   member.JoinedAt,
				ModifiedAt: member.ModifiedAt,
			},
		}
		return nil
	})
	return result, err
}

func (s *Service) AdminCreateWorkspaceAgentToken(input AdminCreateAgentTokenInput) (CreatedToken, error) {
	var created CreatedToken
	err := s.store.Transaction(func(txStore *storage.Store) error {
		txSvc, err := s.withStore(txStore)
		if err != nil {
			return err
		}
		workspace, err := lookupWorkspace(txSvc.workspaceRepo, strings.TrimSpace(input.WorkspaceRef))
		if err != nil {
			return err
		}
		if workspace.ArchivedAt != nil {
			return RuntimeError{Code: "workspace_archived", Message: "workspace is archived"}
		}
		user, err := txSvc.resolveAdminAgentTokenUser(workspace, input.UserRef)
		if err != nil {
			return err
		}
		if _, err := txSvc.memberRepo.Get(user.ID, workspace.ID); err == storage.ErrNotFound {
			return RuntimeError{Code: "membership_not_found", Message: fmt.Sprintf("user %q is not a member of workspace %q", user.Name, workspace.Slug)}
		} else if err != nil {
			return err
		}
		projectIDs, err := txSvc.resolveAdminTokenProjectIDs(workspace.ID, input.ProjectRefs)
		if err != nil {
			return err
		}
		storedToken, err := txSvc.createTokenStored(createTokenStoredInput{
			Name:         input.Name,
			TokenType:    auth.TokenTypeAgent,
			UserID:       user.ID,
			Scopes:       input.Scopes,
			WorkspaceIDs: []string{workspace.ID},
			ProjectIDs:   projectIDs,
			ExpiresIn:    input.ExpiresIn,
		})
		if err != nil {
			return err
		}
		if err := txSvc.appendAdminAuditInTx(txSvc, AuditEntry{
			Action:      "admin.agent_token.create",
			WorkspaceID: &workspace.ID,
			TargetType:  "token",
			TargetID:    storedToken.View.ID,
			Payload: map[string]any{
				"name":         storedToken.View.Name,
				"user_id":      user.ID,
				"workspace_id": workspace.ID,
				"project_ids":  projectIDs,
				"scopes":       storedToken.View.Scopes,
				"expires_at":   storedToken.View.ExpiresAt,
			},
		}, input.AdminTokenName); err != nil {
			return err
		}
		created = storedToken
		return nil
	})
	return created, err
}

func normalizeAdminBootstrapRole(role Role) (Role, error) {
	if role == "" {
		return RoleOwner, nil
	}
	switch role {
	case RoleOwner, RoleAdmin:
		return role, nil
	default:
		return "", RuntimeError{Code: "admin_role_invalid", Message: "admin role must be owner or admin"}
	}
}

func (s *Service) findOrCreateAdminOwner(name, email string) (storage.User, error) {
	var byEmail *storage.User
	if email != "" {
		if user, err := s.userRepo.GetByEmail(email); err == nil {
			byEmail = &user
		} else if err != storage.ErrNotFound {
			return storage.User{}, err
		}
	}
	if user, err := s.userRepo.GetByName(name); err == nil {
		if byEmail != nil && byEmail.ID != user.ID {
			return storage.User{}, RuntimeError{Code: "admin_owner_invalid", Message: "owner name and email refer to different users"}
		}
		return user, nil
	} else if err != storage.ErrNotFound {
		return storage.User{}, err
	}
	if byEmail != nil {
		return *byEmail, nil
	}
	now := s.clock.Unix()
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	return s.userRepo.Create(storage.User{
		ID:         uuid.NewString(),
		Name:       name,
		Email:      emailPtr,
		CreatedAt:  now,
		ModifiedAt: now,
	})
}

func (s *Service) resolveAdminAgentTokenUser(workspace storage.Workspace, userRef string) (storage.User, error) {
	userRef = strings.TrimSpace(userRef)
	if userRef != "" {
		return s.resolveUser(userRef)
	}
	if workspace.CreatedByUserID != nil && *workspace.CreatedByUserID != "" {
		user, err := s.userRepo.GetByID(*workspace.CreatedByUserID)
		if err == nil {
			if member, err := s.memberRepo.Get(user.ID, workspace.ID); err == nil && Role(member.Role) == RoleOwner {
				return user, nil
			}
		} else if err != storage.ErrNotFound {
			return storage.User{}, err
		}
	}
	members, err := s.memberRepo.List(workspace.ID)
	if err != nil {
		return storage.User{}, err
	}
	for _, member := range members {
		if Role(member.Membership.Role) == RoleOwner {
			return member.User, nil
		}
	}
	return storage.User{}, RuntimeError{Code: "admin_owner_required", Message: "workspace owner is required"}
}

func (s *Service) resolveAdminTokenProjectIDs(workspaceID string, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		project, err := s.projectRepo.ResolveInWorkspace(workspaceID, ref)
		if err != nil {
			return nil, RuntimeError{Code: "token_project_scope_invalid", Message: "invalid token project scope"}
		}
		if !slices.Contains(out, project.ID) {
			out = append(out, project.ID)
		}
	}
	return out, nil
}
