package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dajee/taskg/internal/remote"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <target>",
		Short: "显示任务详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				target, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, args[0])
				if err != nil {
					return err
				}
				tsk, err := client.GetTask(context.Background(), currentOpts.Workspace, target)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), task.ToJSON(tsk))
				}
				render.TaskInfo(cmd.OutOrStdout(), tsk)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			tsk, err := svc.ResolveTarget(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), task.ToJSON(tsk))
			}
			render.TaskInfo(cmd.OutOrStdout(), tsk)
			return nil
		},
	}
}

func resolveRemoteTaskTarget(ctx context.Context, client *remote.Client, opts Options, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("task target is required")
	}
	if _, err := uuid.Parse(target); err == nil {
		if opts.Project != "" || opts.ProjectID != "" {
			tasks, err := client.ListTasks(ctx, remote.ListTasksInput{
				Workspace: opts.Workspace,
				Project:   opts.Project,
				ProjectID: opts.ProjectID,
				NoContext: true,
			})
			if err != nil {
				return "", err
			}
			for _, tsk := range tasks {
				if tsk.UUID == target {
					return target, nil
				}
			}
			return "", fmt.Errorf("task %q not found", target)
		}
		return target, nil
	}
	tasks, err := remoteDefaultWorkingSet(ctx, client, opts)
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(target); err == nil {
		if n < 1 || n > len(tasks) {
			return "", fmt.Errorf("task %q not found", target)
		}
		return tasks[n-1].UUID, nil
	}
	var matched string
	for _, tsk := range tasks {
		if strings.HasPrefix(tsk.UUID, target) {
			if matched != "" {
				return "", fmt.Errorf("task target %q is ambiguous", target)
			}
			matched = tsk.UUID
		}
	}
	if matched == "" {
		return "", fmt.Errorf("task %q not found", target)
	}
	return matched, nil
}

func remoteDefaultWorkingSet(ctx context.Context, client *remote.Client, opts Options) ([]task.Task, error) {
	return client.ListTasks(ctx, remote.ListTasksInput{
		Workspace: opts.Workspace,
		Project:   opts.Project,
		ProjectID: opts.ProjectID,
		Filters:   []string{"(status:pending or status:waiting)"},
		NoContext: true,
	})
}
