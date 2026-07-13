// Package query 的 evaluator 对 merge 后的 TaskValue 执行 AST 求值（spec §17.2）。
package query

import (
	"fmt"
	"strings"
	"time"
)

// TaskValue 是 evaluator 的输入，由 App 层从 TaskOccurrenceView 映射而来。
type TaskValue struct {
	ID              string
	UUID            *string
	Title           string
	Description     *string
	Status          string
	Entry           *int64
	Modified        *int64
	End             *int64
	Due             *int64
	Start           *int64
	Wait            *int64
	Scheduled       *int64
	Until           *int64
	Project         *string
	ProjectID       *string
	Priority        *string
	Parent          *string
	SeriesID        *string
	RecurrenceAt    *int64
	TaskType        *string
	Depends         []string
	AnnotationTexts []string
	AssigneeIDs     []string
	Tags            []string
	UDAs            map[string]string
}

// MatchTaskValue 对单个 TaskValue 求值 AST。
func MatchTaskValue(expr Expr, tv TaskValue, loc *time.Location) (bool, error) {
	if loc == nil {
		loc = time.Local
	}
	if expr == nil {
		return true, nil
	}
	switch e := expr.(type) {
	case Binary:
		leftOK, err := MatchTaskValue(e.Left, tv, loc)
		if err != nil {
			return false, err
		}
		rightOK, err := MatchTaskValue(e.Right, tv, loc)
		if err != nil {
			return false, err
		}
		switch e.Op {
		case "and":
			return leftOK && rightOK, nil
		case "or":
			return leftOK || rightOK, nil
		}
		return false, fmt.Errorf("query: unknown binary op %q", e.Op)
	case Unary:
		childOK, err := MatchTaskValue(e.Expr, tv, loc)
		if err != nil {
			return false, err
		}
		if e.Op == "not" {
			return !childOK, nil
		}
		return false, fmt.Errorf("query: unknown unary op %q", e.Op)
	case Predicate:
		return matchPredicate(e, tv, loc)
	}
	return false, fmt.Errorf("query: unsupported expr type %T", expr)
}

func matchPredicate(p Predicate, tv TaskValue, loc *time.Location) (bool, error) {
	if p.Attribute == AttrBare {
		return strings.Contains(strings.ToLower(tv.Title), strings.ToLower(p.Value.Raw)), nil
	}
	switch p.Attribute {
	case AttrUUID:
		v := ""
		if tv.UUID != nil {
			v = *tv.UUID
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrTitle:
		return matchStringOp(p.Operator, tv.Title, p.Value.Raw), nil
	case AttrDescription:
		v := ""
		if tv.Description != nil {
			v = *tv.Description
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrStatus:
		return matchStringOp(p.Operator, tv.Status, p.Value.Raw), nil
	case AttrProject:
		v := ""
		if tv.Project != nil {
			v = *tv.Project
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrProjectID:
		v := ""
		if tv.ProjectID != nil {
			v = *tv.ProjectID
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrPriority:
		v := ""
		if tv.Priority != nil {
			v = *tv.Priority
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrParent:
		v := ""
		if tv.Parent != nil {
			v = *tv.Parent
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrSeriesID:
		v := ""
		if tv.SeriesID != nil {
			v = *tv.SeriesID
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrTaskType:
		v := ""
		if tv.TaskType != nil {
			v = *tv.TaskType
		}
		return matchStringOp(p.Operator, v, p.Value.Raw), nil
	case AttrDue, AttrStart, AttrWait, AttrScheduled, AttrUntil, AttrEntry, AttrModified, AttrEnd, AttrRecurrenceAt:
		return matchDatePredicate(p, tv, loc)
	case AttrTag:
		return matchContains(tv.Tags, p.Value.Raw), nil
	case AttrDepends:
		return matchContains(tv.Depends, p.Value.Raw), nil
	case AttrAnnotations:
		return matchContains(tv.AnnotationTexts, p.Value.Raw), nil
	case AttrAssignee:
		return matchContains(tv.AssigneeIDs, p.Value.Raw), nil
	case AttrUDA:
		return matchStringOp(p.Operator, tv.UDAs[p.Field], p.Value.Raw), nil
	}
	return false, fmt.Errorf("query: unsupported attribute %q in evaluator", p.Attribute)
}

func matchStringOp(op Operator, actual, expected string) bool {
	switch op {
	case OpEqual:
		return actual == expected
	case OpNotEqual:
		return actual != expected
	case OpContains:
		return strings.Contains(strings.ToLower(actual), strings.ToLower(expected))
	case OpIsNull:
		return actual == ""
	case OpNotNull:
		return actual != ""
	}
	return false
}

func matchContains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func matchDatePredicate(p Predicate, tv TaskValue, loc *time.Location) (bool, error) {
	var actual *int64
	switch p.Attribute {
	case AttrDue:
		actual = tv.Due
	case AttrStart:
		actual = tv.Start
	case AttrWait:
		actual = tv.Wait
	case AttrScheduled:
		actual = tv.Scheduled
	case AttrUntil:
		actual = tv.Until
	case AttrEntry:
		actual = tv.Entry
	case AttrModified:
		actual = tv.Modified
	case AttrEnd:
		actual = tv.End
	case AttrRecurrenceAt:
		actual = tv.RecurrenceAt
	}
	switch p.Operator {
	case OpIsNull:
		return actual == nil, nil
	case OpNotNull:
		return actual != nil, nil
	case OpEqual, OpBefore, OpAfter:
		threshold, err := ResolveDeadlineDateValue(p.Value, time.Now().Unix(), loc)
		if err != nil {
			return false, err
		}
		if actual == nil {
			return false, nil
		}
		switch p.Operator {
		case OpEqual:
			return *actual == threshold, nil
		case OpBefore:
			return *actual < threshold, nil
		case OpAfter:
			return *actual > threshold, nil
		}
	}
	return false, nil
}
