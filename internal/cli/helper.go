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
				if field == "urgency" {
					explain, err := svc.ExplainUrgency(target)
					if err != nil {
						return err
					}
					urg = explain.Total
				} else if strings.HasPrefix(field, "tag.") {
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

func newUDAsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_udas",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			defs, err := svc.ListUDAs()
			if err != nil {
				return err
			}
			for _, def := range defs {
				fmt.Fprintln(cmd.OutOrStdout(), def.Name)
			}
			return nil
		},
	}
}

func newUniqueCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_unique <attr> [filters...]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			var expr query.Expr
			if len(args) > 1 {
				expr, err = query.ParseFilterExpr(args[1:])
				if err != nil {
					return err
				}
			}
			values, err := svc.UniqueValues(args[0], app.ListInput{Query: expr})
			if err != nil {
				return err
			}
			for _, value := range values {
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return nil
		},
	}
}

func newShowHelperCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_show [key...]",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			rt, err := runtimeFromOptions(currentOpts)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				for _, key := range rt.Keys() {
					value, _ := rt.Get(key)
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, value)
				}
				return nil
			}
			for _, key := range args {
				value, _ := rt.Get(key)
				fmt.Fprintln(cmd.OutOrStdout(), value)
			}
			return nil
		},
	}
}

func newVersionHelperCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "_version",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			version := opts.Version
			if version == "" {
				version = "dev"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "taskg %s\n", version)
			return nil
		},
	}
}
