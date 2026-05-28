package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/query"
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/spf13/cobra"
)

type Options struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Version string

	DataDir string
	DBPath  string
	JSON    bool
	NoColor bool
}

func NewRootCommand(opts Options) *cobra.Command {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}

	cmd := &cobra.Command{
		Use:           "taskg",
		Short:         "Taskwarrior-style task manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       opts.Version,
	}
	cmd.SetOut(opts.Stdout)
	cmd.SetErr(opts.Stderr)
	cmd.SetVersionTemplate(fmt.Sprintf("taskg %s\n", opts.Version))

	cmd.PersistentFlags().StringVar(&opts.DataDir, "data-dir", opts.DataDir, "data directory")
	cmd.PersistentFlags().StringVar(&opts.DBPath, "db", opts.DBPath, "SQLite database path")
	cmd.PersistentFlags().BoolVar(&opts.JSON, "json", opts.JSON, "render JSON output")
	cmd.PersistentFlags().BoolVar(&opts.NoColor, "no-color", opts.NoColor, "disable colored output")

	cmd.AddCommand(newAddCommand(opts))
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newInfoCommand(opts))
	cmd.AddCommand(newExportCommand(opts))
	cmd.AddCommand(newImportCommand(opts))
	cmd.AddCommand(newShowCommand(opts))
	cmd.AddCommand(newConfigCommand(opts))

	return cmd
}

// Execute runs the root command, handling the taskg <target> <action> pattern
// by intercepting args before Cobra's subcommand matching.
func Execute(cmd *cobra.Command, opts Options, args []string) error {
	// Separate flags from positional args to detect target+action pattern.
	flags, positional := splitFlagsAndPositional(args)
	knownSubcommands := map[string]bool{"add": true, "list": true, "info": true, "export": true, "import": true, "show": true, "config": true, "help": true, "version": true, "completion": true}

	knownActions := map[string]bool{"modify": true, "done": true, "delete": true}

	if len(positional) >= 2 && !knownSubcommands[positional[0]] && knownActions[positional[1]] {
		// Pattern: taskg <target> <action> [args...]
		return handleTargetAction(cmd, opts, flags, positional)
	}

	// Normal Cobra routing: set flags and let subcommand matching work.
	cmd.SetArgs(append(flags, positional...))
	return cmd.Execute()
}

func splitFlagsAndPositional(args []string) (flags []string, positional []string) {
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") && !strings.Contains(args[i], "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			flags = append(flags, args[i], args[i+1])
			i++
		} else if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i])
		} else {
			positional = append(positional, args[i])
		}
	}
	return
}

func handleTargetAction(cmd *cobra.Command, opts Options, flags []string, positional []string) error {
	// Apply flags to the root command's PersistentFlags.
	for i := 0; i < len(flags); i++ {
		if strings.HasPrefix(flags[i], "--") && strings.Contains(flags[i], "=") {
			parts := strings.SplitN(flags[i], "=", 2)
			_ = cmd.PersistentFlags().Set(parts[0][2:], parts[1])
		} else if strings.HasPrefix(flags[i], "--") && i+1 < len(flags) {
			_ = cmd.PersistentFlags().Set(flags[i][2:], flags[i+1])
			i++
		}
	}

	svc, closeFn, err := buildServiceFromCmd(cmd, opts)
	if err != nil {
		return err
	}
	defer closeFn()

	target := positional[0]
	action := positional[1]
	actionArgs := positional[2:]

	switch action {
	case "modify":
		mod, err := query.ParseModifyArgs(actionArgs)
		if err != nil {
			return err
		}
		if err := svc.Modify(target, app.ModifyInput{
			Description: mod.Description,
			Project:     mod.Project,
			Priority:    mod.Priority,
			Due:         mod.Due,
			AddTags:     mod.AddTags,
			RemoveTags:  mod.RemoveTags,
		}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Modified task", target)
	case "done":
		if err := svc.Done(target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Completed task", target)
	case "delete":
		if err := svc.Delete(target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Deleted task", target)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	return nil
}

func buildServiceFromCmd(cmd *cobra.Command, base Options) (*app.Service, func() error, error) {
	opts := base
	opts.DataDir = getCmdStringFlag(cmd, "data-dir", opts.DataDir)
	opts.DBPath = getCmdStringFlag(cmd, "db", opts.DBPath)
	opts.JSON = getCmdBoolFlag(cmd, "json", opts.JSON)
	opts.NoColor = getCmdBoolFlag(cmd, "no-color", opts.NoColor)
	return buildServiceFromOpts(opts)
}

// getCmdStringFlag reads a flag from cmd.Flags(), falling back to
// cmd.PersistentFlags() (needed when cmd is the root command).
func getCmdStringFlag(cmd *cobra.Command, name, fallback string) string {
	if v, err := cmd.Flags().GetString(name); err == nil {
		return v
	}
	if v, err := cmd.PersistentFlags().GetString(name); err == nil {
		return v
	}
	return fallback
}

func getCmdBoolFlag(cmd *cobra.Command, name string, fallback bool) bool {
	if v, err := cmd.Flags().GetBool(name); err == nil {
		return v
	}
	if v, err := cmd.PersistentFlags().GetBool(name); err == nil {
		return v
	}
	return fallback
}

func buildServiceFromOpts(opts Options) (*app.Service, func() error, error) {
	cfg, err := config.Resolve(config.Options{
		DataDir: opts.DataDir,
		DBPath:  opts.DBPath,
		JSON:    opts.JSON,
		NoColor: opts.NoColor,
	})
	if err != nil {
		return nil, nil, err
	}
	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return nil, nil, err
	}
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	return svc, store.Close, nil
}

func isNumericTarget(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
