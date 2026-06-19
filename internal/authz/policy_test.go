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
		{name: "admin can manage members", role: RoleAdmin, perm: PermissionMemberManage, want: true},
		{name: "admin cannot manage owner", role: RoleAdmin, perm: PermissionMemberManageOwner, want: false},
		{name: "member can write task", role: RoleMember, perm: PermissionTaskWrite, want: true},
		{name: "member cannot manage project", role: RoleMember, perm: PermissionProjectManage, want: false},
		{name: "viewer can read task", role: RoleViewer, perm: PermissionTaskRead, want: true},
		{name: "viewer cannot write task", role: RoleViewer, perm: PermissionTaskWrite, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowedForRole(tt.role, tt.perm); got != tt.want {
				t.Fatalf("AllowedForRole(%s,%s) = %v, want %v", tt.role, tt.perm, got, tt.want)
			}
		})
	}
}
