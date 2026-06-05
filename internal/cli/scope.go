package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/dajee/taskg/internal/auth"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newScopeCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "scope",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newScopeListCommand(opts))
	return cmd
}

func newScopeListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			scopes := auth.ScopeRegistryValues()
			if currentOpts.JSON {
				entries := make([]map[string]string, len(scopes))
				for i, s := range scopes {
					entries[i] = map[string]string{"scope": s}
				}
				return render.JSON(cmd.OutOrStdout(), entries)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			for _, s := range scopes {
				fmt.Fprintf(w, "  %s\n", s)
			}
			w.Flush()
			return nil
		},
	}
}
