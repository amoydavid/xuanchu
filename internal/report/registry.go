package report

import "github.com/dajee/taskg/internal/query"

type Registry struct {
	defs map[string]Definition
}

func DefaultRegistry() Registry {
	defs := map[string]Definition{}
	add := func(def Definition) { defs[def.Name] = def }
	add(mustDefinition(Definition{Name: "list", Description: "Pending tasks", FilterSource: "status:pending", Sort: "entry", Columns: defaultColumns()}))
	add(mustDefinition(Definition{Name: "next", Description: "Next tasks", FilterSource: "status:pending", Sort: "urgency", Columns: defaultColumns()}))
	add(Definition{Name: "all", Description: "All tasks", Sort: "entry", Columns: defaultColumns()})
	add(mustDefinition(Definition{Name: "completed", Description: "Completed tasks", FilterSource: "status:completed", Sort: "completed", Columns: defaultColumns()}))
	add(mustDefinition(Definition{Name: "deleted", Description: "Deleted tasks", FilterSource: "status:deleted", Sort: "completed", Columns: defaultColumns()}))
	add(mustDefinition(Definition{Name: "overdue", Description: "Overdue tasks", FilterSource: "status:pending due.before:today", Sort: "due", Columns: defaultColumns()}))
	return Registry{defs: defs}
}

func (r Registry) Get(name string) (Definition, bool) {
	def, ok := r.defs[name]
	return def, ok
}

func mustDefinition(def Definition) Definition {
	expr, err := query.ParseQuery(def.FilterSource)
	if err != nil {
		panic(err)
	}
	def.Filter = expr
	return def
}

func defaultColumns() []string {
	return []string{"id", "uuid", "priority", "project", "tags", "description"}
}
