package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dajee/taskg/internal/task"
)

func TaskList(w io.Writer, tasks []task.Task) {
	TaskListWithIDs(w, tasks, nil)
}

func TaskListWithIDs(w io.Writer, tasks []task.Task, ids []int) {
	fmt.Fprintln(w, "ID  UUID      PRI  PROJECT  TAGS  DESCRIPTION")
	for i, tsk := range tasks {
		id := i + 1
		if len(ids) == len(tasks) {
			id = ids[i]
		}
		priority := ""
		if tsk.Priority != nil {
			priority = *tsk.Priority
		}
		project := ""
		if tsk.Project != nil {
			project = *tsk.Project
		}
		uuid := tsk.UUID
		if len(uuid) > 8 {
			uuid = uuid[:8]
		}
		fmt.Fprintf(w, "%-3d %-8s %-4s %-8s %-5s %s\n",
			id, uuid, priority, project, strings.Join(tsk.Tags, ","), tsk.Description)
	}
}

func TaskInfo(w io.Writer, tsk task.Task) {
	fmt.Fprintf(w, "UUID: %s\n", tsk.UUID)
	fmt.Fprintf(w, "Status: %s\n", tsk.Status)
	fmt.Fprintf(w, "Description: %s\n", tsk.Description)
	fmt.Fprintf(w, "Entry: %s\n", formatUnix(tsk.Entry))
	fmt.Fprintf(w, "Modified: %s\n", formatUnix(tsk.Modified))
	fmt.Fprintf(w, "End: %s\n", formatUnixPtr(tsk.End))
	fmt.Fprintf(w, "Due: %s\n", formatUnixPtr(tsk.Due))
	fmt.Fprintf(w, "Project: %s\n", stringPtrValue(tsk.Project))
	fmt.Fprintf(w, "Priority: %s\n", stringPtrValue(tsk.Priority))
	fmt.Fprintf(w, "Tags: %s\n", strings.Join(tsk.Tags, ","))
}

func formatUnix(sec int64) string {
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func formatUnixPtr(sec *int64) string {
	if sec == nil {
		return ""
	}
	return formatUnix(*sec)
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
