package taskcontext

import "strings"

type Context struct {
	WorkspaceID  string
	Name         string
	FilterSource string
	CreatedAt    int64
	ModifiedAt   int64
}

func ValidateName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r == '-' || r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}
