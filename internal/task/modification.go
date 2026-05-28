package task

type Modification struct {
	Description    *string
	Project        *string
	Priority       *string
	Due            *int64
	ClearDue       bool
	Wait           *int64
	ClearWait      bool
	Scheduled      *int64
	ClearScheduled bool
	Until          *int64
	ClearUntil     bool
	AddDepends     []string
	ClearDepends   bool
	Recur          *string
	ClearRecur     bool
	AddTags        []string
	RemoveTags     []string
	UDAs           map[string]string
	ClearUDAs      []string
}

func (m Modification) Empty() bool {
	return m.Description == nil &&
		m.Project == nil &&
		m.Priority == nil &&
		m.Due == nil && !m.ClearDue &&
		m.Wait == nil && !m.ClearWait &&
		m.Scheduled == nil && !m.ClearScheduled &&
		m.Until == nil && !m.ClearUntil &&
		len(m.AddDepends) == 0 && !m.ClearDepends &&
		m.Recur == nil && !m.ClearRecur &&
		len(m.AddTags) == 0 &&
		len(m.RemoveTags) == 0 &&
		len(m.UDAs) == 0 &&
		len(m.ClearUDAs) == 0
}
