package auth

import (
	"fmt"
	"sort"
	"strings"
)

// Scope 常量。所有 token scope 字符串集中定义，避免散落各处的字面量拼写错误。
// 格式: resource:action（如 "task:read"），impersonate 为独立特殊 scope。
const (
	ScopeTaskRead          = "task:read"
	ScopeTaskWrite         = "task:write"
	ScopeProjectRead       = "project:read"
	ScopeProjectWrite      = "project:write"
	ScopeContextRead       = "context:read"
	ScopeContextWrite      = "context:write"
	ScopeConfigRead        = "config:read"
	ScopeConfigWrite       = "config:write"
	ScopeWorkspaceRead     = "workspace:read"
	ScopeWorkspaceWrite    = "workspace:write"
	ScopeAuditRead         = "audit:read"
	ScopeTokenRead         = "token:read"
	ScopeTokenWrite        = "token:write"
	ScopeHookRead          = "hook:read"
	ScopeHookWrite         = "hook:write"
	ScopeNotificationRead  = "notification:read"
	ScopeNotificationWrite = "notification:write"
	ScopeReminderRead      = "reminder:read"
	ScopeReminderWrite     = "reminder:write"
	ScopeImpersonate       = "impersonate"
)

var scopeRegistry = []string{
	ScopeTaskRead, ScopeTaskWrite,
	ScopeProjectRead, ScopeProjectWrite,
	ScopeContextRead, ScopeContextWrite,
	ScopeConfigRead, ScopeConfigWrite,
	ScopeWorkspaceRead, ScopeWorkspaceWrite,
	ScopeAuditRead,
	ScopeTokenRead, ScopeTokenWrite,
	ScopeHookRead, ScopeHookWrite,
	ScopeNotificationRead, ScopeNotificationWrite,
	ScopeReminderRead, ScopeReminderWrite,
	ScopeImpersonate,
}

var scopeLookup map[string]struct{}

var tenantAllowedScopes = map[string]struct{}{
	ScopeTaskRead:          {},
	ScopeTaskWrite:         {},
	ScopeProjectRead:       {},
	ScopeProjectWrite:      {},
	ScopeContextRead:       {},
	ScopeContextWrite:      {},
	ScopeConfigRead:        {},
	ScopeConfigWrite:       {},
	ScopeWorkspaceRead:     {},
	ScopeAuditRead:         {},
	ScopeHookRead:          {},
	ScopeHookWrite:         {},
	ScopeNotificationRead:  {},
	ScopeNotificationWrite: {},
	ScopeReminderRead:      {},
	ScopeReminderWrite:     {},
}

func init() {
	scopeLookup = make(map[string]struct{}, len(scopeRegistry))
	for _, s := range scopeRegistry {
		scopeLookup[s] = struct{}{}
	}
}

func ScopeRegistryValues() []string {
	out := make([]string, len(scopeRegistry))
	copy(out, scopeRegistry)
	return out
}

type ScopeSet map[string]struct{}

func ParseScopes(values []string) (ScopeSet, error) {
	expanded, err := expandWildcardScopes(values)
	if err != nil {
		return nil, err
	}
	out := ScopeSet{}
	for _, scope := range expanded {
		if _, ok := scopeLookup[scope]; !ok {
			return nil, fmt.Errorf("invalid token scope %q", scope)
		}
		out[scope] = struct{}{}
	}
	return out, nil
}

func ValidateTenantTokenScopes(values []string) (ScopeSet, error) {
	expanded, err := expandTenantWildcardScopes(values)
	if err != nil {
		return nil, err
	}
	out := ScopeSet{}
	for _, scope := range expanded {
		if _, ok := tenantAllowedScopes[scope]; !ok {
			return nil, fmt.Errorf("invalid tenant token scope %q", scope)
		}
		out[scope] = struct{}{}
	}
	return out, nil
}

func expandWildcardScopes(values []string) ([]string, error) {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			scope := strings.TrimSpace(part)
			if scope == "" {
				continue
			}
			if scope == "*" {
				out = append(out, scopeRegistry...)
				continue
			}
			if strings.HasSuffix(scope, ":*") {
				resource := strings.TrimSuffix(scope, ":*")
				matched := expandResourceWildcard(resource)
				if len(matched) == 0 {
					return nil, fmt.Errorf("invalid token scope %q", scope)
				}
				out = append(out, matched...)
				continue
			}
			if strings.HasPrefix(scope, "*:") {
				action := strings.TrimPrefix(scope, "*:")
				matched := expandActionWildcard(action)
				if len(matched) == 0 {
					return nil, fmt.Errorf("invalid token scope %q", scope)
				}
				out = append(out, matched...)
				continue
			}
			out = append(out, scope)
		}
	}
	return out, nil
}

func expandTenantWildcardScopes(values []string) ([]string, error) {
	var out []string
	tenantRegistry := tenantScopeRegistryValues()
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			scope := strings.TrimSpace(part)
			if scope == "" {
				continue
			}
			if scope == "*" {
				out = append(out, tenantRegistry...)
				continue
			}
			if strings.HasSuffix(scope, ":*") {
				resource := strings.TrimSuffix(scope, ":*")
				matched := expandTenantResourceWildcard(resource, tenantRegistry)
				if len(matched) == 0 {
					return nil, fmt.Errorf("invalid tenant token scope %q", scope)
				}
				out = append(out, matched...)
				continue
			}
			if strings.HasPrefix(scope, "*:") {
				action := strings.TrimPrefix(scope, "*:")
				matched := expandTenantActionWildcard(action, tenantRegistry)
				if len(matched) == 0 {
					return nil, fmt.Errorf("invalid tenant token scope %q", scope)
				}
				out = append(out, matched...)
				continue
			}
			out = append(out, scope)
		}
	}
	return out, nil
}

func expandResourceWildcard(resource string) []string {
	prefix := resource + ":"
	var out []string
	for _, s := range scopeRegistry {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func expandActionWildcard(action string) []string {
	suffix := ":" + action
	var out []string
	for _, s := range scopeRegistry {
		if strings.HasSuffix(s, suffix) {
			out = append(out, s)
		}
	}
	return out
}

func expandTenantResourceWildcard(resource string, registry []string) []string {
	prefix := resource + ":"
	var out []string
	for _, s := range registry {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func expandTenantActionWildcard(action string, registry []string) []string {
	suffix := ":" + action
	var out []string
	for _, s := range registry {
		if strings.HasSuffix(s, suffix) {
			out = append(out, s)
		}
	}
	return out
}

func tenantScopeRegistryValues() []string {
	out := make([]string, 0, len(tenantAllowedScopes))
	for _, scope := range scopeRegistry {
		if _, ok := tenantAllowedScopes[scope]; ok {
			out = append(out, scope)
		}
	}
	return out
}

func (s ScopeSet) Has(scope string) bool {
	_, ok := s[scope]
	return ok
}

func (s ScopeSet) Values() []string {
	out := make([]string, 0, len(s))
	for scope := range s {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}
