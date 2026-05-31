package cli

import (
	"context"
	"fmt"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/remote"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "add [description] [modifications...]",
		Args: cobra.MinimumNArgs(1),
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
					Description: parsed.Description,
					Project:     stringValue(parsed.Mod.Project),
					Priority:    stringValue(parsed.Mod.Priority),
					Tags:        parsed.Mod.AddTags,
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
				Description: parsed.Description,
				Project:     parsed.Mod.Project,
				Priority:    parsed.Mod.Priority,
				Due:         parsed.Mod.Due,
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
