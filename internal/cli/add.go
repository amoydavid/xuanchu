package cli

import (
	"fmt"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "add [description] [modifications...]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed, err := query.ParseAddArgs(args)
			if err != nil {
				return err
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
				Tags:        parsed.Mod.AddTags,
			})
			if err != nil {
				return err
			}
			if opts.JSON {
				return render.JSON(cmd.OutOrStdout(), created)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created task %s\n", created.UUID)
			return nil
		},
	}
}
