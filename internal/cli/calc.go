package cli

import (
	"fmt"
	"strings"

	calcexpr "github.com/dajee/taskg/internal/expr"
	"github.com/spf13/cobra"
)

func newCalcCommand(_ Options) *cobra.Command {
	return &cobra.Command{
		Use:  "calc <expression>",
		Args: cobra.MinimumNArgs(1),
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
