package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/task"
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
	if isTaskSlugRef(target) {
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

func isTaskSlugRef(target string) bool {
	dash := strings.LastIndex(target, "-")
	if dash <= 0 || dash == len(target)-1 {
		return false
	}
	if seq, err := strconv.ParseInt(target[dash+1:], 10, 64); err != nil || seq < 1 {
		return false
	}
	project := target[:dash]
	if len(project) < 3 || len(project) > 10 {
		return false
	}
	first := project[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}
	for i := 0; i < len(project); i++ {
		ch := project[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
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
