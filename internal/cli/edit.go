package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	apptedit "github.com/dajee/taskg/internal/edit"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newEditCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "edit <target>",
		Args: cobra.ExactArgs(1),
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
		Use:  "append <target> <text...>",
		Args: cobra.MinimumNArgs(2),
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
		Use:  "prepend <target> <text...>",
		Args: cobra.MinimumNArgs(2),
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
	ResolveTarget(string) (task.Task, error)
	ReplaceEditableTask(string, task.Task) error
}, target string) error {
	tsk, err := svc.ResolveTarget(target)
	if err != nil {
		return err
	}
	editable := apptedit.FromTask(tsk)
	data, err := json.MarshalIndent(editable, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "taskg-edit-*.json")
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
	applied, err := apptedit.Apply(tsk, edited)
	if err != nil {
		return err
	}
	if err := svc.ReplaceEditableTask(target, applied); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Edited task", target)
	return nil
}
