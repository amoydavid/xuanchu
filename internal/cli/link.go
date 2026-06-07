package cli

import (
	"context"
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/spf13/cobra"
)

func newLinkCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "管理任务关联链接",
	}

	cmd.AddCommand(newLinkAddCommand(opts))
	cmd.AddCommand(newLinkListCommand(opts))
	cmd.AddCommand(newLinkRemoveCommand(opts))

	return cmd
}

func newLinkAddCommand(opts Options) *cobra.Command {
	var linkType string
	var linkURL string
	var linkTitle string

	cmd := &cobra.Command{
		Use:   "add <target>",
		Short: "添加任务关联链接",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runLinkAddRemote(cmd, currentOpts, args[0], linkType, linkURL, linkTitle)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			info, err := svc.TaskAddLink(args[0], linkType, linkURL, linkTitle)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), linkInfoForJSON(info))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added %s link to task %s\n", info.Type, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&linkType, "type", "", "link type (required)")
	cmd.Flags().StringVar(&linkURL, "url", "", "link URL (required)")
	cmd.Flags().StringVar(&linkTitle, "title", "", "link title")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("url")

	return cmd
}

func newLinkListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list <target>",
		Short: "列出任务关联链接",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runLinkListRemote(cmd, currentOpts, args[0])
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			tsk, err := svc.ResolveTarget(args[0])
			if err != nil {
				return err
			}
			return renderLinks(cmd, currentOpts.JSON, tsk.Links)
		},
	}
}

func newLinkRemoveCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <target> <link-id>",
		Short: "删除任务关联链接",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				return runLinkRemoveRemote(cmd, currentOpts, args[0], args[1])
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.TaskRemoveLink(args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed link from task %s\n", args[0])
			return nil
		},
	}
}

func linkInfoForJSON(info task.TaskLinkInfo) map[string]any {
	return map[string]any{
		"id":    info.ID,
		"type":  info.Type,
		"url":   info.URL,
		"title": info.Title,
	}
}

func renderLinks(cmd *cobra.Command, asJSON bool, links []task.TaskLinkInfo) error {
	if asJSON {
		return render.JSON(cmd.OutOrStdout(), links)
	}
	if len(links) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No links.")
		return nil
	}
	for _, l := range links {
		if l.Title != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s %s\n", l.Type, l.Title, l.URL)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", l.Type, l.URL)
		}
	}
	return nil
}

func runLinkAddRemote(cmd *cobra.Command, opts Options, target, linkType, linkURL, linkTitle string) error {
	client, err := buildRemoteClient(opts)
	if err != nil {
		return err
	}
	resolved, err := resolveRemoteTaskTarget(context.Background(), client, opts, target)
	if err != nil {
		return err
	}
	dto, err := client.AddTaskLink(context.Background(), opts.Workspace, resolved, linkType, linkURL, linkTitle)
	if err != nil {
		return err
	}
	if opts.JSON {
		return render.JSON(cmd.OutOrStdout(), dto)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Added %s link to task %s\n", dto.Type, target)
	return nil
}

func runLinkListRemote(cmd *cobra.Command, opts Options, target string) error {
	client, err := buildRemoteClient(opts)
	if err != nil {
		return err
	}
	resolved, err := resolveRemoteTaskTarget(context.Background(), client, opts, target)
	if err != nil {
		return err
	}
	tsk, err := client.GetTask(context.Background(), opts.Workspace, resolved)
	if err != nil {
		return err
	}
	return renderLinks(cmd, opts.JSON, tsk.Links)
}

func runLinkRemoveRemote(cmd *cobra.Command, opts Options, target, linkID string) error {
	client, err := buildRemoteClient(opts)
	if err != nil {
		return err
	}
	resolved, err := resolveRemoteTaskTarget(context.Background(), client, opts, target)
	if err != nil {
		return err
	}
	if _, err := client.RemoveTaskLink(context.Background(), opts.Workspace, resolved, linkID); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed link from task %s\n", target)
	return nil
}

func parseLinkActionArgs(actionArgs []string) (subAction string, flags []string, positional []string) {
	for i := 0; i < len(actionArgs); i++ {
		arg := actionArgs[i]
		if subAction == "" && !strings.HasPrefix(arg, "-") {
			subAction = arg
			continue
		}
		if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
			flags = append(flags, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") {
			flags = append(flags, arg)
			if i+1 < len(actionArgs) && !strings.HasPrefix(actionArgs[i+1], "-") {
				flags = append(flags, actionArgs[i+1])
				i++
			}
			continue
		}
		positional = append(positional, arg)
	}
	return
}

func handleLinkAction(cmd *cobra.Command, opts Options, svc *app.Service, target string, actionArgs []string) error {
	if len(actionArgs) == 0 {
		return fmt.Errorf("link requires a subcommand: add, list, remove")
	}
	subAction, flags, positional := parseLinkActionArgs(actionArgs)
	switch subAction {
	case "add":
		linkType, linkURL, linkTitle, err := parseLinkAddFlags(flags)
		if err != nil {
			return err
		}
		info, err := svc.TaskAddLink(target, linkType, linkURL, linkTitle)
		if err != nil {
			return err
		}
		if opts.JSON {
			return render.JSON(cmd.OutOrStdout(), linkInfoForJSON(info))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Added %s link to task %s\n", info.Type, target)
	case "list":
		tsk, err := svc.ResolveTarget(target)
		if err != nil {
			return err
		}
		return renderLinks(cmd, opts.JSON, tsk.Links)
	case "remove":
		if len(positional) != 1 {
			return fmt.Errorf("link remove requires a link-id")
		}
		if err := svc.TaskRemoveLink(target, positional[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed link from task %s\n", target)
	default:
		return fmt.Errorf("unknown link subcommand %q (use: add, list, remove)", subAction)
	}
	return nil
}

func handleRemoteLinkAction(cmd *cobra.Command, opts Options, client *remote.Client, ctx context.Context, displayTarget, resolvedTarget string, actionArgs []string) error {
	if len(actionArgs) == 0 {
		return fmt.Errorf("link requires a subcommand: add, list, remove")
	}
	subAction, flags, positional := parseLinkActionArgs(actionArgs)
	switch subAction {
	case "add":
		linkType, linkURL, linkTitle, err := parseLinkAddFlags(flags)
		if err != nil {
			return err
		}
		dto, err := client.AddTaskLink(ctx, opts.Workspace, resolvedTarget, linkType, linkURL, linkTitle)
		if err != nil {
			return err
		}
		if opts.JSON {
			return render.JSON(cmd.OutOrStdout(), dto)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Added %s link to task %s\n", dto.Type, displayTarget)
	case "list":
		tsk, err := client.GetTask(ctx, opts.Workspace, resolvedTarget)
		if err != nil {
			return err
		}
		return renderLinks(cmd, opts.JSON, tsk.Links)
	case "remove":
		if len(positional) != 1 {
			return fmt.Errorf("link remove requires a link-id")
		}
		if _, err := client.RemoveTaskLink(ctx, opts.Workspace, resolvedTarget, positional[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed link from task %s\n", displayTarget)
	default:
		return fmt.Errorf("unknown link subcommand %q (use: add, list, remove)", subAction)
	}
	return nil
}

func parseLinkAddFlags(flags []string) (linkType, linkURL, linkTitle string, err error) {
	for i := 0; i < len(flags); i++ {
		switch {
		case strings.HasPrefix(flags[i], "--type="):
			linkType = strings.TrimPrefix(flags[i], "--type=")
		case flags[i] == "--type" && i+1 < len(flags):
			linkType = flags[i+1]
			i++
		case strings.HasPrefix(flags[i], "--url="):
			linkURL = strings.TrimPrefix(flags[i], "--url=")
		case flags[i] == "--url" && i+1 < len(flags):
			linkURL = flags[i+1]
			i++
		case strings.HasPrefix(flags[i], "--title="):
			linkTitle = strings.TrimPrefix(flags[i], "--title=")
		case flags[i] == "--title" && i+1 < len(flags):
			linkTitle = flags[i+1]
			i++
		}
	}
	if linkType == "" {
		return "", "", "", fmt.Errorf("--type is required")
	}
	if linkURL == "" {
		return "", "", "", fmt.Errorf("--url is required")
	}
	return
}
