package cli

import (
	"fmt"
	"strings"

	calcexpr "git.dajee.net/dajee/xuanchu/internal/expr"
	"github.com/spf13/cobra"
)

func newCalcCommand(_ Options) *cobra.Command {
	return &cobra.Command{
		Use:   "calc <expression>",
		Short: "计算数学表达式",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := calcexpr.Calc(strings.Join(args, " "))
			if err != nil {
				return fmt.Errorf("calc: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), result)
			return nil
		},
	}
}
