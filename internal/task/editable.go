package task

// EditableFields 是 external edit 可以整体替换的任务字段集合。
//
// 它刻意不包含 UUID、workspace、project_id/project_seq、series_id、
// recurrence_at、规则快照、override、创建/修改时间等身份和系统字段。
// projected occurrence 因此可以用这组字段表达编辑结果，而无需伪造成一个
// 不满足持久化 invariant 的 Task。
type EditableFields struct {
	Title       string
	Description *string
	Status      string
	Project     *string
	Priority    *string
	Due         *int64
	Wait        *int64
	Scheduled   *int64
	Until       *int64
	Tags        []string
	Annotations []Annotation
	Depends     []string
	Parent      *string
}

func EditableFieldsFromTask(t Task) EditableFields {
	return EditableFields{
		Title:       t.Title,
		Description: cloneEditableString(t.Description),
		Status:      t.Status,
		Project:     cloneEditableString(t.Project),
		Priority:    cloneEditableString(t.Priority),
		Due:         cloneEditableInt64(t.Due),
		Wait:        cloneEditableInt64(t.Wait),
		Scheduled:   cloneEditableInt64(t.Scheduled),
		Until:       cloneEditableInt64(t.Until),
		Tags:        append([]string(nil), t.Tags...),
		Annotations: append([]Annotation(nil), t.Annotations...),
		Depends:     append([]string(nil), t.Depends...),
		Parent:      cloneEditableString(t.Parent),
	}
}

// Validate 复用 Task 的字段校验，但不构造任何 occurrence 身份。
func (f EditableFields) Validate() error {
	return (Task{
		Title:       f.Title,
		Description: f.Description,
		Status:      f.Status,
		Priority:    f.Priority,
		Due:         f.Due,
		Wait:        f.Wait,
		Scheduled:   f.Scheduled,
		Until:       f.Until,
		Tags:        f.Tags,
		Annotations: f.Annotations,
		Depends:     f.Depends,
		Parent:      f.Parent,
	}).Validate()
}

func cloneEditableString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneEditableInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
