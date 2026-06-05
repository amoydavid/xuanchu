package cli

import (
	"fmt"
	"text/tabwriter"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"github.com/spf13/cobra"
)

func newScopeCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scope",
		Short: "查看和管理 token scope",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newScopeListCommand(opts))
	return cmd
}

func newScopeListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "列出所有可用的 token scope",
		Long:    "列出系统注册的所有 token scope，可用于创建或修改 token 时指定 --scope 参数。\n支持通配符：*（全部）、resource:*（如 task:*）、*:action（如 *:read）。",
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
