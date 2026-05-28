package report

import "github.com/dajee/taskg/internal/query"

type ScopeKind string

const (
	ScopeStatic           ScopeKind = ""
	ScopeReady            ScopeKind = "ready"
	ScopeBlocked          ScopeKind = "blocked"
	ScopeBlocking         ScopeKind = "blocking"
	ScopeHideUntilExpired ScopeKind = "hide_until_expired"
)

type Definition struct {
	Name         string
	Description  string
	FilterSource string
	Filter       query.Expr
	Sort         string
	Columns      []string
	Scope        ScopeKind
}
