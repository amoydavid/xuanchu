package storage

import (
	"fmt"
	"strconv"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/query"
	"gorm.io/gorm"
)

type QueryCompileOptions struct {
	WorkspaceID    string
	NowUnix        int64
	Location       *time.Location
	UDADefinitions map[string]string
	Dialect        string
}

func likeOp(dialect string) string {
	if dialect == "postgres" {
		return "ILIKE"
	}
	return "LIKE"
}

func realCastType(dialect string) string {
	if dialect == "postgres" {
		return "DOUBLE PRECISION"
	}
	return "REAL"
}

func ApplyQuery(db *gorm.DB, expr query.Expr, opts QueryCompileOptions) *gorm.DB {
	sql, args, err := CompileQuery(expr, opts)
	if err != nil {
		out := db.Session(&gorm.Session{})
		out.Error = err
		return out
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
		return compareColumn("status", p.Operator, value, nil, opts.Dialect)
	case query.AttrProject:
		if p.Operator == query.OpIsNull {
			return "project_id IS NULL", nil, nil
		}
		return "", nil, query.ErrProjectPredicateUnresolved
	case query.AttrProjectID:
		return compareColumn("project_id", p.Operator, value, nil, opts.Dialect)
	case query.AttrPriority:
		return compareColumn("priority", p.Operator, value, nil, opts.Dialect)
	case query.AttrUUID:
		return compareColumn("uuid", p.Operator, value, nil, opts.Dialect)
	case query.AttrBare:
		return fmt.Sprintf("description %s ?", likeOp(opts.Dialect)), []any{"%" + value + "%"}, nil
	case query.AttrUDA:
		return compileUDAPredicate(p, opts)
	case query.AttrDescription:
		// description: 始终按子串匹配，无论是 description:abc、description:'abc def'
		// 还是 description:/abc/。这与 /abc/ 短语一致，避免 OpEqual 字面相等带来的反直觉。
		if p.Operator == query.OpEqual || p.Operator == query.OpContains {
			return fmt.Sprintf("description %s ?", likeOp(opts.Dialect)), []any{"%" + value + "%"}, nil
		}
		return compareColumn("description", p.Operator, value, nil, opts.Dialect)
	case query.AttrDue:
		return compareDateColumn("due", p, opts)
	case query.AttrEntry:
		return compareDateColumn("entry", p, opts)
	case query.AttrModified:
		return compareDateColumn("modified", p, opts)
	case query.AttrEnd:
		return compareDateColumn("end_ts", p, opts)
	case query.AttrStart:
		return compareDateColumn("start", p, opts)
	case query.AttrWait:
		return compareDateColumn("wait", p, opts)
	case query.AttrScheduled:
		return compareDateColumn("scheduled", p, opts)
	case query.AttrUntil:
		return compareDateColumn("until", p, opts)
	case query.AttrRecur:
		return compareColumn("recur", p.Operator, value, nil, opts.Dialect)
	case query.AttrParent:
		return compareColumn("parent", p.Operator, value, nil, opts.Dialect)
	case query.AttrAssignee:
		switch p.Operator {
		case query.OpEqual:
			return "EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid AND task_assignees.user_id = ?)", []any{value}, nil
		case query.OpIsNull:
			return "NOT EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid)", nil, nil
		case query.OpNotNull:
			return "EXISTS (SELECT 1 FROM task_assignees WHERE task_assignees.task_uuid = tasks.uuid)", nil, nil
		}
	case query.AttrDepends:
		switch p.Operator {
		case query.OpEqual:
			return "uuid IN (SELECT task_dependencies.task_uuid FROM task_dependencies JOIN tasks AS dep_tasks ON dep_tasks.uuid = task_dependencies.task_uuid WHERE dep_tasks.workspace_id = ? AND task_dependencies.depends_on = ?)", []any{opts.WorkspaceID, value}, nil
		case query.OpIsNull:
			return "uuid NOT IN (SELECT task_dependencies.task_uuid FROM task_dependencies JOIN tasks AS dep_tasks ON dep_tasks.uuid = task_dependencies.task_uuid WHERE dep_tasks.workspace_id = ?)", []any{opts.WorkspaceID}, nil
		case query.OpNotNull:
			return "uuid IN (SELECT task_dependencies.task_uuid FROM task_dependencies JOIN tasks AS dep_tasks ON dep_tasks.uuid = task_dependencies.task_uuid WHERE dep_tasks.workspace_id = ?)", []any{opts.WorkspaceID}, nil
		}
	case query.AttrAnnotations:
		switch p.Operator {
		case query.OpContains:
			return fmt.Sprintf("EXISTS (SELECT 1 FROM task_annotations JOIN tasks AS annotation_tasks ON annotation_tasks.uuid = task_annotations.task_uuid WHERE annotation_tasks.workspace_id = ? AND task_annotations.task_uuid = tasks.uuid AND task_annotations.description %s ?)", likeOp(opts.Dialect)), []any{opts.WorkspaceID, "%" + value + "%"}, nil
		case query.OpIsNull:
			return "NOT EXISTS (SELECT 1 FROM task_annotations JOIN tasks AS annotation_tasks ON annotation_tasks.uuid = task_annotations.task_uuid WHERE annotation_tasks.workspace_id = ? AND task_annotations.task_uuid = tasks.uuid)", []any{opts.WorkspaceID}, nil
		case query.OpNotNull:
			return "EXISTS (SELECT 1 FROM task_annotations JOIN tasks AS annotation_tasks ON annotation_tasks.uuid = task_annotations.task_uuid WHERE annotation_tasks.workspace_id = ? AND task_annotations.task_uuid = tasks.uuid)", []any{opts.WorkspaceID}, nil
		}
	case query.AttrTag:
		if p.Operator == query.OpHasTag {
			return "uuid IN (SELECT task_tags.task_uuid FROM task_tags JOIN tasks AS tag_tasks ON tag_tasks.uuid = task_tags.task_uuid WHERE tag_tasks.workspace_id = ? AND task_tags.tag = ?)", []any{opts.WorkspaceID, value}, nil
		}
		if p.Operator == query.OpMissingTag {
			return "uuid NOT IN (SELECT task_tags.task_uuid FROM task_tags JOIN tasks AS tag_tasks ON tag_tasks.uuid = task_tags.task_uuid WHERE tag_tasks.workspace_id = ? AND task_tags.tag = ?)", []any{opts.WorkspaceID, value}, nil
		}
	}
	return "", nil, fmt.Errorf("unsupported predicate %s", p.String())
}

func compareColumn(column string, op query.Operator, value string, intValue *int64, dialect string) (string, []any, error) {
	arg := any(value)
	if intValue != nil {
		arg = *intValue
	}
	switch op {
	case query.OpEqual:
		return column + " = ?", []any{arg}, nil
	case query.OpNotEqual:
		return column + " != ?", []any{arg}, nil
	case query.OpBefore:
		return column + " < ?", []any{arg}, nil
	case query.OpAfter:
		return column + " > ?", []any{arg}, nil
	case query.OpContains:
		return fmt.Sprintf("%s %s ?", column, likeOp(dialect)), []any{"%" + value + "%"}, nil
	case query.OpIsNull:
		return column + " IS NULL", nil, nil
	case query.OpNotNull:
		return column + " IS NOT NULL", nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator %q", op)
	}
}

func compareDateColumn(column string, p query.Predicate, opts QueryCompileOptions) (string, []any, error) {
	if p.Operator == query.OpIsNull {
		return column + " IS NULL", nil, nil
	}
	if p.Operator == query.OpNotNull {
		return column + " IS NOT NULL", nil, nil
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
		return column + " >= ? AND " + column + " < ?", []any{start, end}, nil
	}
	value, err := query.ResolveDateValue(p.Value, opts.NowUnix, loc)
	if err != nil {
		return "", nil, err
	}
	return compareColumn(column, p.Operator, "", &value, opts.Dialect)
}

func compileUDAPredicate(p query.Predicate, opts QueryCompileOptions) (string, []any, error) {
	name := p.Field
	base := "task_uda_values.workspace_id = ? AND task_uda_values.task_uuid = tasks.uuid AND task_uda_values.name = ?"
	baseArgs := []any{opts.WorkspaceID, name}
	switch p.Operator {
	case query.OpIsNull:
		return "NOT EXISTS (SELECT 1 FROM task_uda_values WHERE " + base + ")", baseArgs, nil
	case query.OpNotNull:
		return "EXISTS (SELECT 1 FROM task_uda_values WHERE " + base + ")", baseArgs, nil
	}
	typ, ok := opts.UDADefinitions[name]
	if !ok {
		return "", nil, fmt.Errorf("unknown UDA %q", name)
	}
	value := p.Value.Text
	compareSQL := ""
	compareArgs := []any{}
	switch typ {
	case "numeric", "duration":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return "", nil, err
		}
		rt := realCastType(opts.Dialect)
		switch p.Operator {
		case query.OpEqual:
			compareSQL = fmt.Sprintf("CAST(task_uda_values.value AS %s) = CAST(? AS %s)", rt, rt)
		case query.OpBefore:
			compareSQL = fmt.Sprintf("CAST(task_uda_values.value AS %s) < CAST(? AS %s)", rt, rt)
		case query.OpAfter:
			compareSQL = fmt.Sprintf("CAST(task_uda_values.value AS %s) > CAST(? AS %s)", rt, rt)
		default:
			return "", nil, fmt.Errorf("unsupported UDA operator %s", p.Operator)
		}
		compareArgs = append(compareArgs, value)
	case "date":
		loc := opts.Location
		if loc == nil {
			loc = time.UTC
		}
		if p.Operator == query.OpEqual {
			start, end, err := query.ResolveDateRange(query.ParseDateValue(value), opts.NowUnix, loc)
			if err != nil {
				return "", nil, err
			}
			compareSQL = "task_uda_values.value >= ? AND task_uda_values.value < ?"
			compareArgs = append(compareArgs, time.Unix(start, 0).UTC().Format(time.RFC3339), time.Unix(end, 0).UTC().Format(time.RFC3339))
		} else {
			ts, err := query.ResolveDateValue(query.ParseDateValue(value), opts.NowUnix, loc)
			if err != nil {
				return "", nil, err
			}
			if p.Operator == query.OpBefore {
				compareSQL = "task_uda_values.value < ?"
			} else if p.Operator == query.OpAfter {
				compareSQL = "task_uda_values.value > ?"
			} else {
				return "", nil, fmt.Errorf("unsupported UDA operator %s", p.Operator)
			}
			compareArgs = append(compareArgs, time.Unix(ts, 0).UTC().Format(time.RFC3339))
		}
	default:
		switch p.Operator {
		case query.OpEqual, query.OpContains:
			compareSQL = fmt.Sprintf("task_uda_values.value %s ?", likeOp(opts.Dialect))
			compareArgs = append(compareArgs, "%"+value+"%")
		default:
			return "", nil, fmt.Errorf("unsupported UDA operator %s", p.Operator)
		}
	}
	args := append(baseArgs, compareArgs...)
	return "EXISTS (SELECT 1 FROM task_uda_values WHERE " + base + " AND " + compareSQL + ")", args, nil
}
