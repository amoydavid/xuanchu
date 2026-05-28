package query

import "github.com/dajee/taskg/internal/task"

type Filter struct {
	Target   *string
	Status   *string
	Project  *string
	Priority *string
	Tags     []string
	Text     *string
}

type ParsedAdd struct {
	Description string
	Mod         task.Modification
}
