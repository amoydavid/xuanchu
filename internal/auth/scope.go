package auth

import (
	"fmt"
	"sort"
	"strings"
)

var allowedScopes = map[string]struct{}{
	"task:read":      {},
	"task:write":     {},
	"project:read":   {},
	"project:write":  {},
	"context:read":   {},
	"context:write":  {},
	"config:read":    {},
	"config:write":   {},
	"workspace:read": {},
	"workspace:write": {},
	"audit:read":     {},
	"token:read":     {},
	"token:write":    {},
}

type ScopeSet map[string]struct{}

func ParseScopes(values []string) (ScopeSet, error) {
	out := ScopeSet{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			scope := strings.TrimSpace(part)
			if scope == "" {
				continue
			}
			if scope == "admin:*" {
				return nil, fmt.Errorf("invalid token scope %q", scope)
			}
			if _, ok := allowedScopes[scope]; !ok {
				return nil, fmt.Errorf("invalid token scope %q", scope)
			}
			out[scope] = struct{}{}
		}
	}
	return out, nil
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
