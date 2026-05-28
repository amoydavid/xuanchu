package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dajee/taskg/internal/task"
)

func TaskList(w io.Writer, tasks []task.Task) {
	fmt.Fprintln(w, "ID  UUID      PRI  PROJECT  TAGS  DESCRIPTION")
	for i, tsk := range tasks {
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
			i+1, uuid, priority, project, strings.Join(tsk.Tags, ","), tsk.Description)
	}
}

func TaskInfo(w io.Writer, tsk task.Task) {
	fmt.Fprintf(w, "UUID: %s\n", tsk.UUID)
	fmt.Fprintf(w, "Status: %s\n", tsk.Status)
	fmt.Fprintf(w, "Description: %s\n", tsk.Description)
	fmt.Fprintf(w, "Tags: %s\n", strings.Join(tsk.Tags, ","))
}
