package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/dajee/taskg/internal/task"
	"github.com/olekukonko/tablewriter"
)

func TaskList(w io.Writer, tasks []task.Task) {
	TaskListWithIDs(w, tasks, nil)
}

func TaskListWithIDs(w io.Writer, tasks []task.Task, ids []int) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"ID", "UUID", "PRI", "PROJECT", "TAGS", "DESCRIPTION"})
	table.SetBorder(false)
	table.SetHeaderLine(true)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnSeparator(" ")
	table.SetCenterSeparator(" ")
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	for i, tsk := range tasks {
		idCell := strconv.Itoa(i + 1)
		if len(ids) == len(tasks) {
			if ids[i] > 0 {
				idCell = strconv.Itoa(ids[i])
			} else {
				idCell = "-"
			}
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
		table.Append([]string{
			idCell,
			uuid,
			priority,
			project,
			strings.Join(tsk.Tags, ","),
			tsk.Description,
		})
	}
	table.Render()
	fmt.Fprintf(w, "%d task(s)\n", len(tasks))
}

func TaskInfo(w io.Writer, tsk task.Task) {
	table := tablewriter.NewWriter(w)
	table.SetBorder(false)
	table.SetHeaderLine(false)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(false)
	table.SetColumnSeparator("")
	table.SetCenterSeparator("")
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnAlignment([]int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT})

	rows := [][]string{
		{"UUID:", tsk.UUID},
		{"Status:", string(tsk.Status)},
		{"Description:", tsk.Description},
		{"Entry:", formatUnix(tsk.Entry)},
		{"Modified:", formatUnix(tsk.Modified)},
		{"End:", formatUnixPtr(tsk.End)},
		{"Due:", formatUnixPtr(tsk.Due)},
		{"Project:", stringPtrValue(tsk.Project)},
		{"Priority:", stringPtrValue(tsk.Priority)},
		{"Tags:", strings.Join(tsk.Tags, ",")},
	}
	for _, row := range rows {
		table.Append(row)
	}
	table.Render()
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
