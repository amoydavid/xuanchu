package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newAnnotateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "annotate <target> <description...>",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.Annotate(args[0], strings.Join(args[1:], " ")); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", args[0])
			return nil
		},
	}
}

func newDenotateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "denotate <target> <index>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			index, err := strconv.Atoi(args[1])
			if err != nil {
				return err
			}
			if err := svc.Denotate(args[0], index); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Removed annotation from task", args[0])
			return nil
		},
	}
}
