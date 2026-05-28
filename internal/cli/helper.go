package cli

import (
	"fmt"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/dom"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/urgency"
	"github.com/spf13/cobra"
)

func newGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_get [expr...]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			for _, expr := range args {
				dot := strings.Index(expr, ".")
				if dot < 0 {
					return fmt.Errorf("invalid DOM expression %q", expr)
				}
				target := expr[:dot]
				field := expr[dot+1:]

				tsk, err := svc.ResolveTarget(target)
				if err != nil {
					return err
				}

				var urg float64
				if field == "urgency" || strings.HasPrefix(field, "tag.") {
					explain := urgency.Explain(tsk, urgency.Options{NowUnix: svc.Clock().Unix()})
					urg = explain.Total
				}

				value, err := dom.Resolve(tsk, field, urg)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return nil
		},
	}
}

func newIDsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_ids [filters...]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			var expr query.Expr
			if len(args) > 0 {
				expr, err = query.ParseFilterExpr(args)
				if err != nil {
					return err
				}
			}

			ids, err := svc.IDs(app.ListInput{Query: expr})
			if err != nil {
				return err
			}
			for _, id := range ids {
				fmt.Fprintln(cmd.OutOrStdout(), id)
			}
			return nil
		},
	}
}

func newUUIDsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_uuids [filters...]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			var expr query.Expr
			if len(args) > 0 {
				expr, err = query.ParseFilterExpr(args)
				if err != nil {
					return err
				}
			}

			uuids, err := svc.UUIDs(app.ListInput{Query: expr})
			if err != nil {
				return err
			}
			for _, uuid := range uuids {
				fmt.Fprintln(cmd.OutOrStdout(), uuid)
			}
			return nil
		},
	}
}

func newProjectsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_projects",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			projects, err := svc.Projects()
			if err != nil {
				return err
			}
			for _, p := range projects {
				fmt.Fprintln(cmd.OutOrStdout(), p)
			}
			return nil
		},
	}
}

func newTagsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_tags",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			tags, err := svc.Tags()
			if err != nil {
				return err
			}
			for _, tag := range tags {
				fmt.Fprintln(cmd.OutOrStdout(), tag)
			}
			return nil
		},
	}
}
