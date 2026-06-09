package auth

import (
	"fmt"
	"sort"
	"strings"
)

var scopeRegistry = []string{
	"task:read", "task:write",
	"project:read", "project:write",
	"context:read", "context:write",
	"config:read", "config:write",
	"workspace:read", "workspace:write",
	"audit:read",
	"token:read", "token:write",
	"hook:read", "hook:write",
	"notification:read", "notification:write",
	"reminder:read", "reminder:write",
	"impersonate",
}

var scopeLookup map[string]struct{}

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
