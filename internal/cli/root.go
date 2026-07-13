package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/query"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/spf13/cobra"
)

type Options struct {
	Stdout    io.Writer
	Stderr    io.Writer
	Stdin     io.Reader
	Version   string
	SetLogger func(*logging.Logger)

	DataDir     string
	DBPath      string
	DBURL       string
	Server      string
	Token       string
	Workspace   string
	Project     string
	ProjectID   string
	As          string
	Config      string
	JSON        bool
	NoColor     bool
	NoContext   bool
	RCOverrides map[string]*string
}

var allowedRCKeys = map[string]bool{
	"color":          true,
	"json":           true,
	"date.format":    true,
	"context.active": true,
}

type effectiveOptionsContextKey struct{}

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
		Use:           "xuanchu",
		Short:         "Taskwarrior-style task manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       opts.Version,
	}
	cmd.SetOut(opts.Stdout)
	cmd.SetErr(opts.Stderr)
	if opts.Stdin != nil {
		cmd.SetIn(opts.Stdin)
	}
	cmd.SetVersionTemplate(fmt.Sprintf("xuanchu %s\n", opts.Version))

	cmd.PersistentFlags().StringVar(&opts.DataDir, "data-dir", opts.DataDir, "data directory")
	cmd.PersistentFlags().StringVar(&opts.DBPath, "db", opts.DBPath, "SQLite database path")
	cmd.PersistentFlags().StringVar(&opts.DBURL, "db-url", opts.DBURL, "Database URL (postgres://...); mutually exclusive with --db")
	cmd.PersistentFlags().StringVar(&opts.Config, "config", opts.Config, "TOML 配置文件路径")
	cmd.PersistentFlags().StringVar(&opts.Server, "server", opts.Server, "remote xuanchu server base URL")
	cmd.PersistentFlags().StringVar(&opts.Token, "token", opts.Token, "remote bearer token")
	cmd.PersistentFlags().StringVar(&opts.Workspace, "workspace", opts.Workspace, "workspace slug or UUID")
	cmd.PersistentFlags().StringVar(&opts.Project, "project", opts.Project, "remote project slug scope")
	cmd.PersistentFlags().StringVar(&opts.ProjectID, "project-id", opts.ProjectID, "remote project UUID scope")
	cmd.PersistentFlags().StringVar(&opts.As, "as", opts.As, "impersonate user by name, email or UUID (remote only)")
	cmd.PersistentFlags().BoolVar(&opts.JSON, "json", opts.JSON, "render JSON output")
	cmd.PersistentFlags().BoolVar(&opts.NoColor, "no-color", opts.NoColor, "disable colored output")
	cmd.PersistentFlags().BoolVar(&opts.NoContext, "no-context", opts.NoContext, "disable active context for this command")

	cmd.AddCommand(newAddCommand(opts))
	cmd.AddCommand(newListCommand(opts))
	cmd.AddCommand(newNextCommand(opts))
	cmd.AddCommand(newInfoCommand(opts))
	cmd.AddCommand(newExportCommand(opts))
	cmd.AddCommand(newImportCommand(opts))
	cmd.AddCommand(newShowCommand(opts))
	cmd.AddCommand(newConfigCommand(opts))
	cmd.AddCommand(newCompletionCommand(opts))
	cmd.AddCommand(newContextCommand(opts))
	cmd.AddCommand(newUserCommand(opts))
	cmd.AddCommand(newWorkspaceCommand(opts))
	cmd.AddCommand(newProjectCommand(opts))
	cmd.AddCommand(newSeriesCommand(opts))
	cmd.AddCommand(newMemberCommand(opts))
	cmd.AddCommand(newHookCommand(opts))
	cmd.AddCommand(newNotificationCommand(opts))
	cmd.AddCommand(newReminderCommand(opts))
	cmd.AddCommand(newAuditCommand(opts))
	cmd.AddCommand(newTokenCommand(opts))
	cmd.AddCommand(newAdminCommand(opts))
	cmd.AddCommand(newScopeCommand(opts))
	cmd.AddCommand(newMCPCommand(opts))
	cmd.AddCommand(newServerCommand(opts))
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
	cmd.AddCommand(newUDAsCommand(opts))
	cmd.AddCommand(newUniqueCommand(opts))
	cmd.AddCommand(newShowHelperCommand(opts))
	cmd.AddCommand(newVersionHelperCommand(opts))
	cmd.AddCommand(newCalcCommand(opts))
	cmd.AddCommand(newStartCommand(opts))
	cmd.AddCommand(newStopCommand(opts))
	cmd.AddCommand(newAnnotateCommand(opts))
	cmd.AddCommand(newDenotateCommand(opts))
	cmd.AddCommand(newAppendCommand(opts))
	cmd.AddCommand(newPrependCommand(opts))
	cmd.AddCommand(newEditCommand(opts))
	cmd.AddCommand(newLinkCommand(opts))

	return cmd
}

// Execute runs the root command, handling the xuanchu <target> <action> pattern
// by intercepting args before Cobra's subcommand matching.
func Execute(cmd *cobra.Command, opts Options, args []string) error {
	// Separate flags from positional args to detect target+action pattern.
	flags, positional, rcOverrides := splitFlagsRcAndPositional(args)
	opts = mergeRCOverrides(opts, rcOverrides)
	if err := validateRCOverrides(opts.RCOverrides); err != nil {
		return err
	}
	if disablesContext(opts.RCOverrides) {
		opts.NoContext = true
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetContext(context.WithValue(ctx, effectiveOptionsContextKey{}, opts))
	knownSubcommands := knownCommandNames(cmd)
	knownActions := knownTargetActions()

	if len(positional) >= 2 && !knownSubcommands[positional[0]] && knownActions[positional[1]] {
		// Pattern: xuanchu <target> <action> [args...]
		return handleTargetAction(cmd, opts, flags, positional)
	}
	if idx := commandIndex(positional, knownSubcommands); idx > 0 {
		// Pattern: xuanchu <filters...> <command> [args...]
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

func knownCommandNames(root *cobra.Command) map[string]bool {
	known := map[string]bool{
		"help":    true,
		"version": true,
	}
	for _, child := range root.Commands() {
		if child.Name() != "" {
			known[child.Name()] = true
		}
	}
	return known
}

func knownTargetActions() map[string]bool {
	return map[string]bool{
		"modify":      true,
		"done":        true,
		"delete":      true,
		"start":       true,
		"stop":        true,
		"reopen":      true,
		"annotate":    true,
		"denotate":    true,
		"append":      true,
		"prepend":     true,
		"edit":        true,
		"link":        true,
		"annotations": true,
		"timeline":    true,
	}
}

func splitFlagsAndPositional(args []string) (flags []string, positional []string) {
	flags, positional, _ = splitFlagsRcAndPositional(args)
	return
}

func splitFlagsRcAndPositional(args []string) (flags []string, positional []string, rc map[string]*string) {
	rc = map[string]*string{}
	stringFlags := map[string]bool{"--data-dir": true, "--db": true, "--db-url": true, "--server": true, "--token": true, "--workspace": true, "--project": true, "--project-id": true, "--as": true, "--config": true}
	boolFlags := map[string]bool{"--json": true, "--no-color": true, "--no-context": true, "--help": true, "--version": true}
	for i := 0; i < len(args); i++ {
		if key, value, ok := parseRCOverride(args[i]); ok {
			rc[key] = value
			continue
		}
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

func parseRCOverride(arg string) (string, *string, bool) {
	if !strings.HasPrefix(arg, "rc.") {
		return "", nil, false
	}
	body := strings.TrimPrefix(arg, "rc.")
	var key, value string
	switch {
	case strings.Contains(body, "="):
		parts := strings.SplitN(body, "=", 2)
		key, value = parts[0], parts[1]
	case strings.HasSuffix(body, ":"):
		key = strings.TrimSuffix(body, ":")
		value = ""
	default:
		return "", nil, false
	}
	key = normalizeRCKey(key)
	if value == "" || (key == "context.active" && value == "none") {
		return key, nil, true
	}
	return key, &value, true
}

func normalizeRCKey(key string) string {
	if key == "context" {
		return "context.active"
	}
	return key
}

func mergeRCOverrides(opts Options, overrides map[string]*string) Options {
	if len(overrides) == 0 {
		return opts
	}
	merged := map[string]*string{}
	for key, value := range opts.RCOverrides {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	opts.RCOverrides = merged
	return opts
}

func validateRCOverrides(overrides map[string]*string) error {
	for key := range overrides {
		if !allowedRCKeys[key] && !strings.HasPrefix(key, "urgency.") {
			return fmt.Errorf("unknown rc key %q", key)
		}
	}
	return nil
}

func disablesContext(overrides map[string]*string) bool {
	value, ok := overrides["context.active"]
	return ok && value == nil
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
	stringFlags := map[string]bool{"--data-dir": true, "--db": true, "--db-url": true, "--server": true, "--token": true, "--workspace": true, "--project": true, "--project-id": true, "--as": true, "--config": true}
	boolFlags := map[string]bool{"--json": true, "--no-color": true, "--no-context": true, "--help": true, "--version": true}
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

	currentOpts := optionsFromCmd(cmd, opts)
	if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
		return err
	} else if remoteMode {
		return handleRemoteTargetAction(cmd, currentOpts, positional)
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
			Title:           mod.Title,
			Description:     mod.Description,
			Project:         mod.Project,
			Priority:        mod.Priority,
			Due:             mod.Due,
			ClearDue:        mod.ClearDue,
			Wait:            mod.Wait,
			ClearWait:       mod.ClearWait,
			Scheduled:       mod.Scheduled,
			ClearScheduled:  mod.ClearScheduled,
			Until:           mod.Until,
			ClearUntil:      mod.ClearUntil,
			AddDepends:      mod.AddDepends,
			ClearDepends:    mod.ClearDepends,
			AddAssignees:    mod.AddAssignees,
			RemoveAssignees: mod.RemoveAssignees,
			ClearAssignees:  mod.ClearAssignees,
			AddTags:         mod.AddTags,
			RemoveTags:      mod.RemoveTags,
			UDAs:            mod.UDAs,
			ClearUDAs:       mod.ClearUDAs,
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
	case "reopen":
		if err := svc.Reopen(target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Reopened task", target)
	case "annotate":
		if len(actionArgs) == 0 {
			return fmt.Errorf("annotate requires a description")
		}
		content := strings.Join(actionArgs, " ")
		err := svc.Annotate(target, content)
		if err != nil {
			if isPossibleProjectSlug(target) {
				if _, projectErr := svc.ProjectAnnotate(target, content); projectErr == nil {
					fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", target)
					return nil
				}
			}
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", target)
	case "denotate":
		if len(actionArgs) != 1 {
			return fmt.Errorf("denotate requires an annotation id")
		}
		if err := svc.Denotate(target, actionArgs[0]); err != nil {
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
	case "annotations":
		tsk, err := svc.Info(target)
		if err == nil {
			return renderAnnotations(cmd, currentOpts.JSON, tsk.Annotations)
		}
		if isPossibleProjectSlug(target) {
			annotations, err := svc.ProjectAnnotations(target)
			if err == nil {
				return renderProjectAnnotationInfos(cmd, currentOpts.JSON, annotations)
			}
		}
		return fmt.Errorf("target %q not found", target)
	case "timeline":
		if !isPossibleProjectSlug(target) {
			return fmt.Errorf("timeline is only available for projects")
		}
		entries, err := svc.ProjectTimeline(target, app.TimelineOptions{Limit: 50})
		if err != nil {
			return err
		}
		return renderTimelineEntriesApp(cmd, currentOpts.JSON, entries)
	case "link":
		return handleLinkAction(cmd, opts, svc, target, actionArgs)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	return nil
}

func handleRemoteTargetAction(cmd *cobra.Command, opts Options, positional []string) error {
	client, err := buildRemoteClient(opts)
	if err != nil {
		return err
	}
	ctx := context.Background()
	target, err := resolveRemoteTaskTarget(ctx, client, opts, positional[0])
	if err != nil {
		return err
	}
	action := positional[1]
	actionArgs := positional[2:]
	switch action {
	case "modify":
		mod, err := query.ParseModifyArgs(actionArgs)
		if err != nil {
			return err
		}
		_, err = client.ModifyTask(ctx, opts.Workspace, target, remote.ModifyTaskInput{
			Title:           mod.Title,
			Description:     mod.Description,
			Project:         mod.Project,
			Priority:        mod.Priority,
			Due:             mod.Due,
			ClearDue:        mod.ClearDue,
			Wait:            mod.Wait,
			ClearWait:       mod.ClearWait,
			Scheduled:       mod.Scheduled,
			ClearScheduled:  mod.ClearScheduled,
			Until:           mod.Until,
			ClearUntil:      mod.ClearUntil,
			Depends:         mod.AddDepends,
			ClearDepends:    mod.ClearDepends,
			Assignees:       mod.AddAssignees,
			RemoveAssignees: mod.RemoveAssignees,
			ClearAssignees:  mod.ClearAssignees,
			Tags:            mod.AddTags,
			RemoveTags:      mod.RemoveTags,
			UDAs:            mod.UDAs,
			ClearUDAs:       mod.ClearUDAs,
		})
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Modified task", positional[0])
	case "done":
		if _, err := client.DoneTask(ctx, opts.Workspace, target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Completed task", positional[0])
	case "delete":
		if _, err := client.DeleteTask(ctx, opts.Workspace, target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Deleted task", positional[0])
	case "start":
		if _, err := client.StartTask(ctx, opts.Workspace, target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Started task", positional[0])
	case "stop":
		if _, err := client.StopTask(ctx, opts.Workspace, target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Stopped task", positional[0])
	case "reopen":
		if _, err := client.ReopenTask(ctx, opts.Workspace, target); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Reopened task", positional[0])
	case "annotate":
		if len(actionArgs) == 0 {
			return fmt.Errorf("annotate requires a description")
		}
		content := strings.Join(actionArgs, " ")
		_, err := client.AnnotateTask(ctx, opts.Workspace, target, content)
		if err != nil {
			if isPossibleProjectSlug(positional[0]) {
				if _, projectErr := client.AnnotateProject(ctx, opts.Workspace, target, content); projectErr == nil {
					fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", positional[0])
					return nil
				}
			}
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Annotated task", positional[0])
	case "denotate":
		if len(actionArgs) != 1 {
			return fmt.Errorf("denotate requires an annotation id")
		}
		if _, err := client.DenotateTask(ctx, opts.Workspace, target, actionArgs[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Removed annotation from task", positional[0])
	case "append", "prepend":
		if len(actionArgs) == 0 {
			return fmt.Errorf("%s requires text", action)
		}
		dto, err := client.GetTaskView(ctx, opts.Workspace, target)
		if err != nil {
			return err
		}
		tsk := remoteDTOToTask(dto)
		text := strings.Join(actionArgs, " ")
		title := strings.TrimSpace(tsk.Title + " " + text)
		if action == "prepend" {
			title = strings.TrimSpace(text + " " + tsk.Title)
		}
		if _, err := client.ModifyTask(ctx, opts.Workspace, target, remote.ModifyTaskInput{Title: &title}); err != nil {
			return err
		}
		if action == "append" {
			fmt.Fprintln(cmd.OutOrStdout(), "Appended description for task", positional[0])
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Prepended description for task", positional[0])
		}
	case "edit":
		return app.RuntimeError{Code: "remote_unsupported_command", Message: `command "edit" is not supported in remote mode`}
	case "annotations":
		dto, tskErr := client.GetTaskView(ctx, opts.Workspace, target)
		if tskErr == nil {
			return renderAnnotations(cmd, opts.JSON, remoteDTOToTask(dto).Annotations)
		}
		if isPossibleProjectSlug(positional[0]) {
			annotations, annErr := client.ListProjectAnnotations(ctx, opts.Workspace, target)
			if annErr == nil {
				return renderRemoteProjectAnnotations(cmd, opts.JSON, annotations)
			}
		}
		return fmt.Errorf("target %q not found", positional[0])
	case "timeline":
		if !isPossibleProjectSlug(positional[0]) {
			return fmt.Errorf("timeline is only available for projects")
		}
		entries, err := client.ProjectTimeline(ctx, opts.Workspace, target, 50)
		if err != nil {
			return err
		}
		return renderRemoteTimelineEntries(cmd, opts.JSON, entries)
	case "link":
		return handleRemoteLinkAction(cmd, opts, client, ctx, positional[0], target, actionArgs)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
	return nil
}

func buildServiceFromCmd(cmd *cobra.Command, base Options) (*app.Service, func() error, error) {
	return buildServiceFromOpts(optionsFromCmd(cmd, base))
}

func optionsFromCmd(cmd *cobra.Command, base Options) Options {
	if root := cmd.Root(); root != nil && root.Context() != nil {
		if effective, ok := root.Context().Value(effectiveOptionsContextKey{}).(Options); ok {
			base = effective
		}
	}
	opts := base
	opts.DataDir = getCmdStringFlag(cmd, "data-dir", opts.DataDir)
	opts.DBPath = getCmdStringFlag(cmd, "db", opts.DBPath)
	opts.DBURL = getCmdStringFlag(cmd, "db-url", opts.DBURL)
	opts.Server = getCmdStringFlag(cmd, "server", opts.Server)
	opts.Token = getCmdStringFlag(cmd, "token", opts.Token)
	opts.Workspace = getCmdStringFlag(cmd, "workspace", opts.Workspace)
	opts.Project = getCmdStringFlag(cmd, "project", opts.Project)
	opts.ProjectID = getCmdStringFlag(cmd, "project-id", opts.ProjectID)
	opts.As = getCmdStringFlag(cmd, "as", opts.As)
	opts.Config = getCmdStringFlag(cmd, "config", opts.Config)
	opts.JSON = getCmdBoolFlag(cmd, "json", opts.JSON)
	opts.NoColor = getCmdBoolFlag(cmd, "no-color", opts.NoColor)
	opts.NoContext = getCmdBoolFlag(cmd, "no-context", opts.NoContext)
	return opts
}

// getCmdStringFlag reads a flag from cmd.Flags(), falling back to
// cmd.PersistentFlags() (needed when cmd is the root command).
func getCmdStringFlag(cmd *cobra.Command, name, fallback string) string {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		if v, err := cmd.Flags().GetString(name); err == nil {
			return v
		}
	}
	if v, err := cmd.InheritedFlags().GetString(name); err == nil {
		return v
	}
	if v, err := cmd.PersistentFlags().GetString(name); err == nil {
		return v
	}
	if root := cmd.Root(); root != nil {
		if v, err := root.PersistentFlags().GetString(name); err == nil {
			return v
		}
	}
	if v, err := cmd.Flags().GetString(name); err == nil {
		return v
	}
	return fallback
}

func getCmdBoolFlag(cmd *cobra.Command, name string, fallback bool) bool {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		if v, err := cmd.Flags().GetBool(name); err == nil {
			return v
		}
	}
	if v, err := cmd.InheritedFlags().GetBool(name); err == nil {
		return v
	}
	if v, err := cmd.PersistentFlags().GetBool(name); err == nil {
		return v
	}
	if root := cmd.Root(); root != nil {
		if v, err := root.PersistentFlags().GetBool(name); err == nil {
			return v
		}
	}
	if v, err := cmd.Flags().GetBool(name); err == nil {
		return v
	}
	return fallback
}

func buildServiceFromOpts(opts Options) (*app.Service, func() error, error) {
	if opts.As != "" {
		fmt.Fprintln(opts.Stderr, "xuanchu: --as 仅在远程模式下生效，本地模式已忽略")
	}
	env := RuntimeEnv()
	cfg, err := config.Resolve(config.Options{
		DataDir:    opts.DataDir,
		DBPath:     opts.DBPath,
		DBURL:      opts.DBURL,
		Server:     opts.Server,
		Token:      opts.Token,
		JSON:       opts.JSON,
		NoColor:    opts.NoColor,
		Env:        env,
		ConfigPath: opts.Config,
	})
	if err != nil {
		return nil, nil, err
	}
	logger, loggerClose, err := logging.Setup(cfg.Log, opts.Stderr)
	if err != nil {
		return nil, nil, err
	}
	if opts.SetLogger != nil {
		opts.SetLogger(logger)
	}
	store, err := storage.Open(cfg.DatabaseTarget())
	if err != nil {
		return nil, nil, err
	}
	runtimeOpts := opts
	runtimeOpts.RCOverrides = nil
	rt, err := runtimeFromResolvedConfig(runtimeOpts, cfg, store, env)
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	svc, err := app.NewService(app.ServiceOptions{
		Store:            store,
		NoContext:        opts.NoContext,
		RuntimeConfig:    rt.Values(),
		RuntimeOverrides: rcOverridesAsStrings(opts.RCOverrides),
		WorkspaceRef:     opts.Workspace,
	})
	if err != nil {
		_ = store.Close()
		return nil, nil, err
	}
	if value, ok := opts.RCOverrides["context.active"]; ok {
		if value == nil {
			svc.OverrideActiveContext("")
		} else {
			svc.OverrideActiveContext(*value)
		}
	}
	// 本地 CLI 命令前补齐当前 workspace 的循环任务 occurrence（spec §9.3）。
	// 忽略错误：reconcile 失败不阻断用户命令。
	_, _ = svc.ReconcileWorkspaceTaskSeries(time.Now().Unix())
	return svc, func() error {
		loggerClose()
		return store.Close()
	}, nil
}

func rcOverridesAsStrings(overrides map[string]*string) map[string]string {
	if len(overrides) == 0 {
		return nil
	}
	values := map[string]string{}
	for key, value := range overrides {
		if value == nil {
			values[key] = ""
			continue
		}
		values[key] = *value
	}
	return values
}

func isPossibleProjectSlug(ref string) bool {
	if len(ref) == 0 {
		return false
	}
	if ref[0] >= '0' && ref[0] <= '9' {
		return false
	}
	for _, ch := range ref {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return false
	}
	return true
}
