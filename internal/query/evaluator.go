// Package query 的 evaluator 对 merge 后的 TaskValue 执行 AST 求值（spec §17.2）。
package query

import (
	"fmt"
	"strconv"
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
	return MatchTaskValueAt(expr, tv, time.Now().Unix(), loc)
}

// MatchTaskValueAt 使用调用方注入的当前时间求值。App 的 SQL compiler 与
// merge evaluator 必须共享同一时钟，尤其是 today/tomorrow 等相对日期。
func MatchTaskValueAt(expr Expr, tv TaskValue, nowUnix int64, loc *time.Location) (bool, error) {
	if loc == nil {
		loc = time.Local
	}
	if expr == nil {
		return true, nil
	}
	switch e := expr.(type) {
	case Binary:
		leftOK, err := MatchTaskValueAt(e.Left, tv, nowUnix, loc)
		if err != nil {
			return false, err
		}
		rightOK, err := MatchTaskValueAt(e.Right, tv, nowUnix, loc)
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
		childOK, err := MatchTaskValueAt(e.Expr, tv, nowUnix, loc)
		if err != nil {
			return false, err
		}
		if e.Op == "not" {
			return !childOK, nil
		}
		return false, fmt.Errorf("query: unknown unary op %q", e.Op)
	case Predicate:
		return matchPredicate(e, tv, nowUnix, loc)
	}
	return false, fmt.Errorf("query: unsupported expr type %T", expr)
}

func matchPredicate(p Predicate, tv TaskValue, nowUnix int64, loc *time.Location) (bool, error) {
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
		return matchTextOp(p.Operator, tv.Title, p.Value.Raw), nil
	case AttrDescription:
		v := ""
		if tv.Description != nil {
			v = *tv.Description
		}
		return matchTextOp(p.Operator, v, p.Value.Raw), nil
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
		return matchDatePredicate(p, tv, nowUnix, loc)
	case AttrTag:
		return matchListPredicate(p.Operator, tv.Tags, p.Value.Raw), nil
	case AttrDepends:
		return matchListPredicate(p.Operator, tv.Depends, p.Value.Raw), nil
	case AttrAnnotations:
		return matchAnnotationPredicate(p.Operator, tv.AnnotationTexts, p.Value.Raw), nil
	case AttrAssignee:
		return matchListPredicate(p.Operator, tv.AssigneeIDs, p.Value.Raw), nil
	case AttrUDA:
		return matchUDAPredicate(p, tv.UDAs[p.Field], nowUnix, loc)
	}
	return false, fmt.Errorf("query: unsupported attribute %q in evaluator", p.Attribute)
}

func matchTextOp(op Operator, actual, expected string) bool {
	switch op {
	case OpEqual, OpContains:
		return strings.Contains(strings.ToLower(actual), strings.ToLower(expected))
	default:
		return matchStringOp(op, actual, expected)
	}
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

func matchListPredicate(op Operator, list []string, value string) bool {
	switch op {
	case OpEqual, OpHasTag:
		return matchContains(list, value)
	case OpMissingTag:
		return !matchContains(list, value)
	case OpIsNull:
		return len(list) == 0
	case OpNotNull:
		return len(list) > 0
	default:
		return false
	}
}

func matchAnnotationPredicate(op Operator, annotations []string, value string) bool {
	switch op {
	case OpContains, OpEqual:
		for _, annotation := range annotations {
			if strings.Contains(strings.ToLower(annotation), strings.ToLower(value)) {
				return true
			}
		}
		return false
	case OpIsNull:
		return len(annotations) == 0
	case OpNotNull:
		return len(annotations) > 0
	default:
		return false
	}
}

func matchUDAPredicate(p Predicate, actual string, nowUnix int64, loc *time.Location) (bool, error) {
	if p.Operator == OpIsNull {
		return actual == "", nil
	}
	if p.Operator == OpNotNull {
		return actual != "", nil
	}
	if actual == "" {
		return false, nil
	}
	if actualDate, err := time.Parse(time.RFC3339, actual); err == nil {
		if p.Operator == OpEqual {
			start, end, err := ResolveDateRange(p.Value, nowUnix, loc)
			if err != nil {
				return false, err
			}
			unix := actualDate.Unix()
			return unix >= start && unix < end, nil
		}
		threshold, err := ResolveDateValue(p.Value, nowUnix, loc)
		if err != nil {
			return false, err
		}
		if p.Operator == OpBefore {
			return actualDate.Unix() < threshold, nil
		}
		if p.Operator == OpAfter {
			return actualDate.Unix() > threshold, nil
		}
		return false, nil
	}
	actualNumber, actualErr := strconv.ParseFloat(actual, 64)
	expectedNumber, expectedErr := strconv.ParseFloat(p.Value.Raw, 64)
	if actualErr == nil && expectedErr == nil {
		switch p.Operator {
		case OpEqual:
			return actualNumber == expectedNumber, nil
		case OpBefore:
			return actualNumber < expectedNumber, nil
		case OpAfter:
			return actualNumber > expectedNumber, nil
		}
	}
	return matchTextOp(p.Operator, actual, p.Value.Raw), nil
}

func matchDatePredicate(p Predicate, tv TaskValue, nowUnix int64, loc *time.Location) (bool, error) {
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
	case OpEqual:
		start, end, err := ResolveDateRange(p.Value, nowUnix, loc)
		if err != nil {
			return false, err
		}
		if actual == nil {
			return false, nil
		}
		return *actual >= start && *actual < end, nil
	case OpBefore, OpAfter:
		threshold, err := ResolveDateValue(p.Value, nowUnix, loc)
		if err != nil {
			return false, err
		}
		if actual == nil {
			return false, nil
		}
		if p.Operator == OpBefore {
			return *actual < threshold, nil
		}
		return *actual > threshold, nil
	}
	return false, nil
}
