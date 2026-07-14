package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	apptedit "git.dajee.net/dajee/xuanchu/internal/edit"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/spf13/cobra"
)

func newEditCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <target>",
		Short: "在编辑器中修改任务",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return runEdit(cmd, svc, args[0])
		},
	}
}

func newAppendCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "append <target> <text...>",
		Short: "追加任务描述",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.AppendDescription(args[0], strings.Join(args[1:], " ")); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Appended description for task", args[0])
			return nil
		},
	}
}

func newPrependCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "prepend <target> <text...>",
		Short: "前置追加任务描述",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.PrependDescription(args[0], strings.Join(args[1:], " ")); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Prepended description for task", args[0])
			return nil
		},
	}
}

func runEdit(cmd *cobra.Command, svc interface {
	GetTaskView(string) (app.TaskOccurrenceView, error)
	ReplaceEditableTask(string, task.EditableFields) error
}, target string) error {
	view, err := svc.GetTaskView(target)
	if err != nil {
		return err
	}
	editable := editableTaskFromView(view)
	data, err := json.MarshalIndent(editable, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "xuanchu-edit-*.json")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	editCmd := exec.Command(editor, path)
	editCmd.Stdin = cmd.InOrStdin()
	editCmd.Stdout = cmd.OutOrStdout()
	editCmd.Stderr = cmd.ErrOrStderr()
	if err := editCmd.Run(); err != nil {
		return err
	}
	editedData, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(editedData) == string(data) {
		fmt.Fprintln(cmd.OutOrStdout(), "Edit unchanged", target)
		return nil
	}
	edited, err := apptedit.Parse(editedData, editable)
	if err != nil {
		return err
	}
	before, err := apptedit.Apply(editable)
	if err != nil {
		return err
	}
	after, err := apptedit.Apply(edited)
	if err != nil {
		return err
	}
	if editableFieldsEqual(before, after) {
		fmt.Fprintln(cmd.OutOrStdout(), "Edit unchanged", target)
		return nil
	}
	if err := svc.ReplaceEditableTask(target, after); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Edited task", target)
	return nil
}

func editableTaskFromView(view app.TaskOccurrenceView) apptedit.EditableTask {
	annotations := make([]task.JSONAnnotation, len(view.Annotations))
	for i, annotation := range view.Annotations {
		annotations[i] = task.JSONAnnotation{
			ID: annotation.ID, Entry: formatEditableUnix(annotation.Entry),
			Description: annotation.Description,
		}
	}
	return apptedit.EditableTask{
		ID:          view.ID,
		UUID:        cloneEditableString(view.UUID),
		Entry:       formatEditableUnixPtr(view.Entry),
		Title:       view.Title,
		Description: cloneEditableString(view.Description),
		Status:      view.Status,
		Project:     cloneEditableString(view.Project),
		Priority:    cloneEditableString(view.Priority),
		Due:         formatEditableUnixPtr(view.Due),
		Wait:        formatEditableUnixPtr(view.Wait),
		Scheduled:   formatEditableUnixPtr(view.Scheduled),
		Until:       formatEditableUnixPtr(view.Until),
		Tags:        append([]string(nil), view.Tags...),
		Annotations: annotations,
		Depends:     append([]string(nil), view.Depends...),
		Parent:      cloneEditableString(view.Parent),
	}
}

func editableFieldsEqual(a, b task.EditableFields) bool {
	return reflect.DeepEqual(a, b)
}

func formatEditableUnixPtr(value *int64) *string {
	if value == nil {
		return nil
	}
	formatted := formatEditableUnix(*value)
	return &formatted
}

func formatEditableUnix(value int64) string {
	return time.Unix(value, 0).UTC().Format(time.RFC3339)
}

func cloneEditableString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
