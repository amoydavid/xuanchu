package authz

import "testing"

func TestAllowedForRole(t *testing.T) {
	allPermissions := []Permission{
		PermissionTaskWrite,
		PermissionTaskRead,
		PermissionProjectRead,
		PermissionProjectManage,
		PermissionProjectConfigRead,
		PermissionProjectConfigWrite,
		PermissionConfigSchemaRead,
		PermissionConfigSchemaWrite,
		PermissionContextUse,
		PermissionContextManage,
		PermissionUDAManage,
		PermissionWorkspaceRead,
		PermissionWorkspaceModify,
		PermissionWorkspaceArchive,
		PermissionMemberManage,
		PermissionMemberManageOwner,
		PermissionAuditRead,
		PermissionTokenRead,
		PermissionTokenWrite,
		PermissionHookRead,
		PermissionHookWrite,
		PermissionNotificationRead,
		PermissionNotificationWrite,
		PermissionReminderRead,
		PermissionReminderWrite,
	}
	if len(allPermissions) != 25 {
		t.Fatalf("allPermissions has %d entries, want 25; update the role matrix expectations when permissions change", len(allPermissions))
	}

	allowedByRole := map[Role]map[Permission]bool{
		RoleOwner: {
			PermissionTaskWrite:          true,
			PermissionTaskRead:           true,
			PermissionProjectRead:        true,
			PermissionProjectManage:      true,
			PermissionProjectConfigRead:  true,
			PermissionProjectConfigWrite: true,
			PermissionConfigSchemaRead:   true,
			PermissionConfigSchemaWrite:  true,
			PermissionContextUse:         true,
			PermissionContextManage:      true,
			PermissionUDAManage:          true,
			PermissionWorkspaceRead:      true,
			PermissionWorkspaceModify:    true,
			PermissionWorkspaceArchive:   true,
			PermissionMemberManage:       true,
			PermissionMemberManageOwner:  true,
			PermissionAuditRead:          true,
			PermissionTokenRead:          true,
			PermissionTokenWrite:         true,
			PermissionHookRead:           true,
			PermissionHookWrite:          true,
			PermissionNotificationRead:   true,
			PermissionNotificationWrite:  true,
			PermissionReminderRead:       true,
			PermissionReminderWrite:      true,
		},
		RoleAdmin: {
			PermissionTaskWrite:          true,
			PermissionTaskRead:           true,
			PermissionProjectRead:        true,
			PermissionProjectManage:      true,
			PermissionProjectConfigRead:  true,
			PermissionProjectConfigWrite: true,
			PermissionConfigSchemaRead:   true,
			PermissionConfigSchemaWrite:  true,
			PermissionContextUse:         true,
			PermissionContextManage:      true,
			PermissionUDAManage:          true,
			PermissionWorkspaceRead:      true,
			PermissionWorkspaceModify:    true,
			PermissionMemberManage:       true,
			PermissionAuditRead:          true,
			PermissionTokenRead:          true,
			PermissionTokenWrite:         true,
			PermissionHookRead:           true,
			PermissionHookWrite:          true,
			PermissionNotificationRead:   true,
			PermissionNotificationWrite:  true,
			PermissionReminderRead:       true,
			PermissionReminderWrite:      true,
		},
		RoleMember: {
			PermissionTaskWrite:         true,
			PermissionTaskRead:          true,
			PermissionProjectRead:       true,
			PermissionProjectConfigRead: true,
			PermissionConfigSchemaRead:  true,
			PermissionContextUse:        true,
			PermissionContextManage:     true,
			PermissionWorkspaceRead:     true,
			PermissionReminderRead:      true,
		},
		RoleViewer: {
			PermissionTaskRead:          true,
			PermissionProjectRead:       true,
			PermissionProjectConfigRead: true,
			PermissionConfigSchemaRead:  true,
			PermissionContextUse:        true,
			PermissionWorkspaceRead:     true,
		},
	}

	for _, role := range []Role{RoleOwner, RoleAdmin, RoleMember, RoleViewer} {
		t.Run(string(role), func(t *testing.T) {
			for _, perm := range allPermissions {
				want := allowedByRole[role][perm]
				if got := AllowedForRole(role, perm); got != want {
					t.Fatalf("AllowedForRole(%s,%s) = %v, want %v", role, perm, got, want)
				}
			}
		})
	}
}
