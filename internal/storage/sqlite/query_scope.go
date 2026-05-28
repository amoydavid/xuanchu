package sqlite

import (
	"fmt"
	"time"

	"github.com/dajee/taskg/internal/query"
	"gorm.io/gorm"
)

type QueryCompileOptions struct {
	WorkspaceID string
	NowUnix     int64
	Location    *time.Location
}

func ApplyQuery(db *gorm.DB, expr query.Expr, opts QueryCompileOptions) *gorm.DB {
	sql, args, err := CompileQuery(expr, opts)
	if err != nil {
		_ = db.AddError(err)
		return db
	}
	if sql == "" {
		return db
	}
	return db.Where(sql, args...)
}

func CompileQuery(expr query.Expr, opts QueryCompileOptions) (string, []any, error) {
	sql, args, err := compileExpr(expr, opts)
	if err != nil {
		return "", nil, err
	}
	if opts.WorkspaceID == "" {
		return sql, args, nil
	}
	if sql == "" {
		return "workspace_id = ?", []any{opts.WorkspaceID}, nil
	}
	return "(workspace_id = ? AND (" + sql + "))", append([]any{opts.WorkspaceID}, args...), nil
}

func compileExpr(expr query.Expr, opts QueryCompileOptions) (string, []any, error) {
	switch e := expr.(type) {
	case nil:
		return "", nil, nil
	case query.Predicate:
		return compilePredicate(e, opts)
	case query.Binary:
		leftSQL, leftArgs, err := compileExpr(e.Left, opts)
		if err != nil {
			return "", nil, err
		}
		rightSQL, rightArgs, err := compileExpr(e.Right, opts)
		if err != nil {
			return "", nil, err
		}
		if e.Op == "xor" {
			sql := "(COALESCE((" + leftSQL + "), 0) <> COALESCE((" + rightSQL + "), 0))"
			return sql, append(leftArgs, rightArgs...), nil
		}
		op := map[string]string{"and": "AND", "or": "OR"}[e.Op]
		if op == "" {
			return "", nil, fmt.Errorf("unknown binary op %q", e.Op)
		}
		return "(" + leftSQL + " " + op + " " + rightSQL + ")", append(leftArgs, rightArgs...), nil
	case query.Unary:
		sql, args, err := compileExpr(e.Expr, opts)
		if err != nil {
			return "", nil, err
		}
		if e.Op != "not" {
			return "", nil, fmt.Errorf("unknown unary op %q", e.Op)
		}
		return "(NOT " + sql + ")", args, nil
	default:
		return "", nil, fmt.Errorf("unsupported query expr %T", expr)
	}
}

func compilePredicate(p query.Predicate, opts QueryCompileOptions) (string, []any, error) {
	value := p.Value.Text
	switch p.Attribute {
	case query.AttrStatus:
		return compareColumn("status", p.Operator, value, nil)
	case query.AttrProject:
		return compareColumn("project", p.Operator, value, nil)
	case query.AttrPriority:
		return compareColumn("priority", p.Operator, value, nil)
	case query.AttrUUID:
		return compareColumn("uuid", p.Operator, value, nil)
	case query.AttrBare:
		return "description LIKE ?", []any{"%" + value + "%"}, nil
	case query.AttrDescription:
		// description: 始终按子串匹配，无论是 description:abc、description:'abc def'
		// 还是 description:/abc/。这与 /abc/ 短语一致，避免 OpEqual 字面相等带来的反直觉。
		if p.Operator == query.OpEqual || p.Operator == query.OpContains {
			return "description LIKE ?", []any{"%" + value + "%"}, nil
		}
		return compareColumn("description", p.Operator, value, nil)
	case query.AttrDue:
		return compareDateColumn("due", p, opts)
	case query.AttrEntry:
		return compareDateColumn("entry", p, opts)
	case query.AttrModified:
		return compareDateColumn("modified", p, opts)
	case query.AttrEnd:
		return compareDateColumn("end_ts", p, opts)
	case query.AttrTag:
		if p.Operator == query.OpHasTag {
			return "uuid IN (SELECT task_uuid FROM task_tags WHERE tag = ?)", []any{value}, nil
		}
		if p.Operator == query.OpMissingTag {
			return "uuid NOT IN (SELECT task_uuid FROM task_tags WHERE tag = ?)", []any{value}, nil
		}
	}
	return "", nil, fmt.Errorf("unsupported predicate %s", p.String())
}

func compareColumn(column string, op query.Operator, value string, intValue *int64) (string, []any, error) {
	arg := any(value)
	if intValue != nil {
		arg = *intValue
	}
	switch op {
	case query.OpEqual:
		return column + " = ?", []any{arg}, nil
	case query.OpBefore:
		return column + " < ?", []any{arg}, nil
	case query.OpAfter:
		return column + " > ?", []any{arg}, nil
	case query.OpContains:
		return column + " LIKE ?", []any{"%" + value + "%"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator %q", op)
	}
}

func compareDateColumn(column string, p query.Predicate, opts QueryCompileOptions) (string, []any, error) {
	if p.Operator == query.OpIsNull {
		return column + " IS NULL", nil, nil
	}
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	if p.Operator == query.OpEqual {
		start, end, err := query.ResolveDateRange(p.Value, opts.NowUnix, loc)
		if err != nil {
			return "", nil, err
		}
		return column + " >= ? AND " + column + " <= ?", []any{start, end}, nil
	}
	value, err := query.ResolveDateValue(p.Value, opts.NowUnix, loc)
	if err != nil {
		return "", nil, err
	}
	return compareColumn(column, p.Operator, "", &value)
}
