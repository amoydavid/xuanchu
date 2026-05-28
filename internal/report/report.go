package report

import "github.com/dajee/taskg/internal/query"

type Definition struct {
	Name         string
	Description  string
	FilterSource string
	Filter       query.Expr
	Sort         string
	Columns      []string
}
