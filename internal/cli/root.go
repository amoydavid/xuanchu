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
	cmd.AddCommand(newNextCommand(opts))
	cmd.AddCommand(newInfoCommand(opts))
	cmd.AddCommand(newExportCommand(opts))
	cmd.AddCommand(newImportCommand(opts))
	cmd.AddCommand(newShowCommand(opts))
	cmd.AddCommand(newConfigCommand(opts))
	cmd.AddCommand(newAllCommand(opts))
	cmd.AddCommand(newCompletedCommand(opts))
	cmd.AddCommand(newDeletedCommand(opts))
	cmd.AddCommand(newOverdueCommand(opts))
	cmd.AddCommand(newActiveCommand(opts))
	cmd.AddCommand(newWaitingCommand(opts))
	cmd.AddCommand(newReadyCommand(opts))
	cmd.AddCommand(newBlockedCommand(opts))
	cmd.AddCommand(newBlockingCommand(opts))
	cmd.AddCommand(newUrgencyCommand(opts))
	cmd.AddCommand(newUrgencyHelperCommand(opts))
	cmd.AddCommand(newGetCommand(opts))
	cmd.AddCommand(newIDsCommand(opts))
	cmd.AddCommand(newUUIDsCommand(opts))
	cmd.AddCommand(newProjectsCommand(opts))
	cmd.AddCommand(newTagsCommand(opts))
	cmd.AddCommand(newCalcCommand(opts))
	cmd.AddCommand(newStartCommand(opts))
	cmd.AddCommand(newStopCommand(opts))
	cmd.AddCommand(newAnnotateCommand(opts))
	cmd.AddCommand(newDenotateCommand(opts))
	cmd.AddCommand(newAppendCommand(opts))
	cmd.AddCommand(newPrependCommand(opts))
	cmd.AddCommand(newEditCommand(opts))

	return cmd
}

// Execute runs the root command, handling the taskg <target> <action> pattern
// by intercepting args before Cobra's subcommand matching.
func Execute(cmd *cobra.Command, opts Options, args []string) error {
	// Separate flags from positional args to detect target+action pattern.
	flags, positional := splitFlagsAndPositional(args)
	knownSubcommands := map[string]bool{"add": true, "list": true, "next": true, "info": true, "export": true, "import": true, "show": true, "config": true, "help": true, "version": true, "completion": true, "all": true, "completed": true, "deleted": true, "overdue": true, "active": true, "waiting": true, "ready": true, "blocked": true, "blocking": true, "urgency": true, "_urgency": true, "calc": true, "_get": true, "_ids": true, "_uuids": true, "_projects": true, "_tags": true, "start": true, "stop": true, "annotate": true, "denotate": true, "append": true, "prepend": true, "edit": true}

	knownActions := map[string]bool{"modify": true, "done": true, "delete": true, "start": true, "stop": true, "annotate": true, "denotate": true, "append": true, "prepend": true, "edit": true}

	if len(positional) >= 2 && !knownSubcommands[positional[0]] && knownActions[positional[1]] {
		// Pattern: taskg <target> <action> [args...]
		return handleTargetAction(cmd, opts, flags, positional)
	}
	if idx := commandIndex(positional, knownSubcommands); idx > 0 {
		// Pattern: taskg <filters...> <command> [args...]
		reordered := append([]string{positional[idx]}, positional[:idx]...)
		reordered = append(reordered, positional[idx+1:]...)
		positional = reordered
	}
	if len(positional) > 0 && positional[0] == "add" {
		positional = protectDashTagArgs(positional)
	}

	// Normal Cobra routing: set flags and let subcommand matching work.
	cmd.SetArgs(append(flags, positional...))
	return cmd.Execute()
}

func splitFlagsAndPositional(args []string) (flags []string, positional []string) {
	stringFlags := map[string]bool{"--data-dir": true, "--db": true}
	boolFlags := map[string]bool{"--json": true, "--no-color": true, "--help": true, "--version": true}
	for i := 0; i < len(args); i++ {
		name := args[i]
		if strings.HasPrefix(name, "--") && strings.Contains(name, "=") {
			name = strings.SplitN(name, "=", 2)[0]
		}
		switch {
		case stringFlags[name] && strings.Contains(args[i], "="):
			flags = append(flags, args[i])
		case stringFlags[name] && i+1 < len(args):
			flags = append(flags, args[i], args[i+1])
			i++
		case boolFlags[name]:
			flags = append(flags, args[i])
		default:
			positional = append(positional, args[i])
		}
	}
	return
}

func commandIndex(args []string, commands map[string]bool) int {
	for i, arg := range args {
		if commands[arg] {
			return i
		}
	}
	return -1
}

func protectDashTagArgs(args []string) []string {
	for i := 1; i < len(args); i++ {
		if isDashTag(args[i]) {
			out := make([]string, 0, len(args)+1)
			out = append(out, args[:i]...)
			out = append(out, "--")
			out = append(out, args[i:]...)
			return out
		}
	}
	return args
}

func isDashTag(arg string) bool {
	return strings.HasPrefix(arg, "-") && len(arg) > 1 && !strings.HasPrefix(arg, "--")
}

func handleTargetAction(cmd *cobra.Command, opts Options, flags []string, positional []string) error {
	// Apply flags to the root command's PersistentFlags.
	stringFlags := map[string]bool{"--data-dir": true, "--db": true}
	boolFlags := map[string]bool{"--json": true, "--no-color": true, "--help": true, "--version": true}
	for i := 0; i < len(flags); i++ {
		if strings.HasPrefix(flags[i], "--") && strings.Contains(flags[i], "=") {
			parts := strings.SplitN(flags[i], "=", 2)
			_ = cmd.PersistentFlags().Set(parts[0][2:], parts[1])
		} else if stringFlags[flags[i]] && i+1 < len(flags) {
			_ = cmd.PersistentFlags().Set(flags[i][2:], flags[i+1])
			i++
		} else if boolFlags[flags[i]] {
			_ = cmd.PersistentFlags().Set(flags[i][2:], "true")
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
			Description:    mod.Description,
			Project:        mod.Project,
			Priority:       mod.Priority,
			Due:            mod.Due,
			ClearDue:       mod.ClearDue,
			Wait:           mod.Wait,
			ClearWait:      mod.ClearWait,
			Scheduled:      mod.Scheduled,
			ClearScheduled: mod.ClearScheduled,
			Until:          mod.Until,
			ClearUntil:     mod.ClearUntil,
			AddDepends:     mod.AddDepends,
			ClearDepends:   mod.ClearDepends,
			Recur:          mod.Recur,
			ClearRecur:     mod.ClearRecur,
			AddTags:        mod.AddTags,
			RemoveTags:     mod.RemoveTags,
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
	case "start":
		if err := svc.Start(target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Started task", target)
	case "stop":
		if err := svc.Stop(target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Stopped task", target)
	case "annotate":
		if len(actionArgs) == 0 {
			return fmt.Errorf("annotate requires a description")
		}
		if err := svc.Annotate(target, strings.Join(actionArgs, " ")); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", target)
	case "denotate":
		if len(actionArgs) != 1 {
			return fmt.Errorf("denotate requires an index")
		}
		index, err := strconv.Atoi(actionArgs[0])
		if err != nil {
			return err
		}
		if err := svc.Denotate(target, index); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Removed annotation from task", target)
	case "append":
		if len(actionArgs) == 0 {
			return fmt.Errorf("append requires text")
		}
		if err := svc.AppendDescription(target, strings.Join(actionArgs, " ")); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Appended description for task", target)
	case "prepend":
		if len(actionArgs) == 0 {
			return fmt.Errorf("prepend requires text")
		}
		if err := svc.PrependDescription(target, strings.Join(actionArgs, " ")); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Prepended description for task", target)
	case "edit":
		if len(actionArgs) != 0 {
			return fmt.Errorf("edit does not take inline arguments")
		}
		return runEdit(cmd, svc, target)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	return nil
}

func buildServiceFromCmd(cmd *cobra.Command, base Options) (*app.Service, func() error, error) {
	return buildServiceFromOpts(optionsFromCmd(cmd, base))
}

func optionsFromCmd(cmd *cobra.Command, base Options) Options {
	opts := base
	opts.DataDir = getCmdStringFlag(cmd, "data-dir", opts.DataDir)
	opts.DBPath = getCmdStringFlag(cmd, "db", opts.DBPath)
	opts.JSON = getCmdBoolFlag(cmd, "json", opts.JSON)
	opts.NoColor = getCmdBoolFlag(cmd, "no-color", opts.NoColor)
	return opts
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
