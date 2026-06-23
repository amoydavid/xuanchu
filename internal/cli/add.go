package cli

import (
	"context"
	"fmt"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/spf13/cobra"
)

func newAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "add [title] [modifications...]",
		Short: "添加新任务",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			parsed, err := query.ParseAddArgs(args)
			if err != nil {
				return err
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				created, err := client.AddTask(context.Background(), currentOpts.Workspace, remote.AddTaskInput{
					Title:       parsed.Title,
					Description: parsed.Mod.Description,
					Project:     firstNonEmpty(stringValue(parsed.Mod.Project), currentOpts.Project),
					ProjectID:   currentOpts.ProjectID,
					Priority:    stringValue(parsed.Mod.Priority),
					Due:         parsed.Mod.Due,
					Assignees:   parsed.Mod.AddAssignees,
					Depends:     parsed.Mod.AddDepends,
					Wait:        parsed.Mod.Wait,
					Scheduled:   parsed.Mod.Scheduled,
					Until:       parsed.Mod.Until,
					Recur:       parsed.Mod.Recur,
					Tags:        parsed.Mod.AddTags,
					UDAs:        parsed.Mod.UDAs,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), task.ToJSON(created))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created task %s\n", created.UUID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			created, err := svc.Add(app.AddInput{
				Title:       parsed.Title,
				Description: parsed.Mod.Description,
				Project:     parsed.Mod.Project,
				Priority:    parsed.Mod.Priority,
				Due:         parsed.Mod.Due,
				Assignees:   parsed.Mod.AddAssignees,
				Depends:     parsed.Mod.AddDepends,
				Wait:        parsed.Mod.Wait,
				Scheduled:   parsed.Mod.Scheduled,
				Until:       parsed.Mod.Until,
				Recur:       parsed.Mod.Recur,
				Tags:        parsed.Mod.AddTags,
				UDAs:        parsed.Mod.UDAs,
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), task.ToJSON(created))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created task %s\n", created.UUID)
			return nil
		},
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
