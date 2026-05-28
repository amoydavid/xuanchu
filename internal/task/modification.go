package task

type Modification struct {
	Description *string
	Project     *string
	Priority    *string
	Due         *int64
	AddTags     []string
	RemoveTags  []string
}

func (m Modification) Empty() bool {
	return m.Description == nil &&
		m.Project == nil &&
		m.Priority == nil &&
		m.Due == nil &&
		len(m.AddTags) == 0 &&
		len(m.RemoveTags) == 0
}
