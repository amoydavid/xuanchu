package authz

import "testing"

func TestAllowedForRole(t *testing.T) {
	tests := []struct {
		name string
		role Role
		perm Permission
		want bool
	}{
		{name: "owner can archive workspace", role: RoleOwner, perm: PermissionWorkspaceArchive, want: true},
		{name: "owner can manage owner members", role: RoleOwner, perm: PermissionMemberManageOwner, want: true},
		{name: "admin can manage members", role: RoleAdmin, perm: PermissionMemberManage, want: true},
		{name: "admin cannot manage owner", role: RoleAdmin, perm: PermissionMemberManageOwner, want: false},
		{name: "admin cannot archive workspace", role: RoleAdmin, perm: PermissionWorkspaceArchive, want: false},
		{name: "member can write task", role: RoleMember, perm: PermissionTaskWrite, want: true},
		{name: "member cannot manage project", role: RoleMember, perm: PermissionProjectManage, want: false},
		{name: "member cannot manage members", role: RoleMember, perm: PermissionMemberManage, want: false},
		{name: "viewer can read task", role: RoleViewer, perm: PermissionTaskRead, want: true},
		{name: "viewer cannot write task", role: RoleViewer, perm: PermissionTaskWrite, want: false},
		{name: "member can read audit", role: RoleMember, perm: PermissionAuditRead, want: true},
		{name: "viewer can read audit", role: RoleViewer, perm: PermissionAuditRead, want: true},
		{name: "admin can read audit", role: RoleAdmin, perm: PermissionAuditRead, want: true},
		{name: "unknown role has no permission", role: Role("stranger"), perm: PermissionTaskRead, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowedForRole(tt.role, tt.perm); got != tt.want {
				t.Fatalf("AllowedForRole(%s,%s) = %v, want %v", tt.role, tt.perm, got, tt.want)
			}
		})
	}
}
