package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/spf13/cobra"
)

func newProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "管理项目",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newProjectListCommand(opts))
	cmd.AddCommand(newProjectAddCommand(opts))
	cmd.AddCommand(newProjectInfoCommand(opts))
	cmd.AddCommand(newProjectModifyCommand(opts))
	cmd.AddCommand(newProjectArchiveCommand(opts))
	cmd.AddCommand(newProjectTransitionCommand(opts))
	cmd.AddCommand(newProjectConfigCommand(opts))
	cmd.AddCommand(newProjectAnnotateCommand(opts))
	cmd.AddCommand(newProjectAnnotationsCommand(opts))
	cmd.AddCommand(newProjectDenotateCommand(opts))
	cmd.AddCommand(newProjectTimelineCommand(opts))
	cmd.AddCommand(newProjectTemplateCommand(opts))
	return cmd
}

const projectTemplateCLIInputLimitBytes = 1 << 20

type projectTemplateInstantiateFileInput struct {
	Description          *string            `json:"description,omitempty"`
	ConfigInputs         map[string]string  `json:"config_inputs,omitempty"`
	SecretInputs         map[string]string  `json:"secret_inputs,omitempty"`
	AssigneeReplacements map[string]*string `json:"assignee_replacements,omitempty"`
}

func newProjectTemplateCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use: "template", Short: "使用项目模板", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newProjectTemplateListCommand(opts))
	cmd.AddCommand(newProjectTemplateInstantiateCommand(opts))
	return cmd
}

func newProjectTemplateListCommand(opts Options) *cobra.Command {
	var q string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出可实例化的项目模板",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 100 {
				return fmt.Errorf("--limit must be from 1 to 100")
			}
			if offset < 0 {
				return fmt.Errorf("--offset must not be negative")
			}
			currentOpts := optionsFromCmd(cmd, opts)
			var page app.ProjectTemplatePage
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				page, err = client.ListProjectTemplatesForInstantiation(cmd.Context(), currentOpts.Workspace, q, limit, offset)
				if err != nil {
					return err
				}
			} else {
				svc, closeFn, err := buildServiceFromCmd(cmd, opts)
				if err != nil {
					return err
				}
				defer closeFn()
				page, err = svc.ListProjectTemplatesForInstantiation(app.TemplateInstantiationListInput{Q: q, Limit: limit, Offset: offset})
				if err != nil {
					return err
				}
			}
			return renderProjectTemplatePage(cmd.OutOrStdout(), currentOpts.JSON, page)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "按 key、名称或说明搜索")
	cmd.Flags().IntVar(&limit, "limit", 50, "返回数量（1-100）")
	cmd.Flags().IntVar(&offset, "offset", 0, "分页偏移")
	return cmd
}

func newProjectTemplateInstantiateCommand(opts Options) *cobra.Command {
	var snapshotID, snapshotHash, startDate, inputSource string
	cmd := &cobra.Command{
		Use:   "instantiate <template-ref> <new-project-slug> name:<name>",
		Short: "从模板当前版本创建项目",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			values, err := parseKeyValueArgs(args[2:], map[string]bool{"name": true})
			if err != nil {
				return err
			}
			projectName := strings.TrimSpace(values["name"])
			if projectName == "" {
				return fmt.Errorf("name:<name> is required")
			}
			if strings.TrimSpace(snapshotID) == "" {
				return fmt.Errorf("--snapshot is required")
			}
			if strings.TrimSpace(snapshotHash) == "" {
				return fmt.Errorf("--snapshot-hash is required")
			}
			if strings.TrimSpace(startDate) == "" {
				return fmt.Errorf("--start-date is required")
			}
			fileInput, err := readProjectTemplateInstantiateInput(cmd, inputSource)
			if err != nil {
				return err
			}
			input := app.CurrentSnapshotInstantiateInput{
				SnapshotID: strings.TrimSpace(snapshotID), ExpectedHash: strings.TrimSpace(snapshotHash),
				ProjectSlug: args[1], ProjectName: projectName, Description: fileInput.Description, StartDate: strings.TrimSpace(startDate),
				ConfigInputs: fileInput.ConfigInputs, SecretInputs: fileInput.SecretInputs, AssigneeReplacements: fileInput.AssigneeReplacements,
			}
			currentOpts := optionsFromCmd(cmd, opts)
			var result app.InstantiateResult
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				result, err = client.InstantiateCurrentProjectTemplate(cmd.Context(), currentOpts.Workspace, args[0], input)
				if err != nil {
					return err
				}
			} else {
				svc, closeFn, err := buildServiceFromCmd(cmd, opts)
				if err != nil {
					return err
				}
				defer closeFn()
				result, err = svc.InstantiateCurrentProjectTemplate(args[0], input)
				if err != nil {
					return err
				}
			}
			return renderProjectTemplateInstantiateResult(cmd.OutOrStdout(), currentOpts.JSON, result)
		},
	}
	cmd.Flags().StringVar(&snapshotID, "snapshot", "", "current Snapshot UUID（必填）")
	cmd.Flags().StringVar(&snapshotHash, "snapshot-hash", "", "current Snapshot hash（必填）")
	cmd.Flags().StringVar(&startDate, "start-date", "", "项目开始日期 YYYY-MM-DD（必填）")
	cmd.Flags().StringVar(&inputSource, "input", "", "补充 JSON 文件路径，或 - 从标准输入读取（最大 1 MiB）")
	return cmd
}

func readProjectTemplateInstantiateInput(cmd *cobra.Command, source string) (projectTemplateInstantiateFileInput, error) {
	if strings.TrimSpace(source) == "" {
		return projectTemplateInstantiateFileInput{}, nil
	}
	var reader io.Reader
	var closeFn func() error
	if source == "-" {
		reader = cmd.InOrStdin()
	} else {
		file, err := os.Open(source)
		if err != nil {
			return projectTemplateInstantiateFileInput{}, fmt.Errorf("read project template input: %w", err)
		}
		reader, closeFn = file, file.Close
	}
	if closeFn != nil {
		defer closeFn()
	}
	raw, err := io.ReadAll(io.LimitReader(reader, projectTemplateCLIInputLimitBytes+1))
	if err != nil {
		return projectTemplateInstantiateFileInput{}, fmt.Errorf("read project template input: %w", err)
	}
	if len(raw) > projectTemplateCLIInputLimitBytes {
		return projectTemplateInstantiateFileInput{}, fmt.Errorf("project template input exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input projectTemplateInstantiateFileInput
	if err := decoder.Decode(&input); err != nil {
		return projectTemplateInstantiateFileInput{}, fmt.Errorf("invalid project template input JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return projectTemplateInstantiateFileInput{}, fmt.Errorf("invalid project template input JSON")
	}
	return input, nil
}

func renderProjectTemplatePage(w io.Writer, asJSON bool, page app.ProjectTemplatePage) error {
	if asJSON {
		items := make([]map[string]any, 0, len(page.Items))
		for _, item := range page.Items {
			row := map[string]any{
				"id": item.ID, "key": item.Key, "name": item.Name, "description": item.Description, "status": item.Status,
				"created_by": task.ActorInfoToJSON(item.CreatedBy), "created_at": item.CreatedAt, "modified_at": item.ModifiedAt,
			}
			if item.ArchivedAt != nil {
				row["archived_at"] = item.ArchivedAt
			}
			if item.CurrentSnapshot != nil {
				row["current_snapshot"] = map[string]any{
					"id": item.CurrentSnapshot.ID, "version": item.CurrentSnapshot.Version, "hash": item.CurrentSnapshot.Hash,
					"source_project_id": item.CurrentSnapshot.SourceProjectID, "counts": item.CurrentSnapshot.Counts,
					"required_secret_keys": append([]string{}, item.CurrentSnapshot.RequiredSecretKeys...),
					"config_inputs":        append([]app.ProjectTemplateConfigInputView{}, item.CurrentSnapshot.ConfigInputs...),
					"created_by":           task.ActorInfoToJSON(item.CurrentSnapshot.CreatedBy), "created_at": item.CurrentSnapshot.CreatedAt,
				}
			}
			items = append(items, row)
		}
		return render.JSON(w, map[string]any{"items": items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
	}
	for _, item := range page.Items {
		if item.CurrentSnapshot == nil {
			fmt.Fprintf(w, "%s\t%s\tno-current-snapshot\n", item.Key, item.Name)
			continue
		}
		current := item.CurrentSnapshot
		secrets := "-"
		if len(current.RequiredSecretKeys) > 0 {
			secrets = strings.Join(current.RequiredSecretKeys, ",")
		}
		inputs := "-"
		if len(current.ConfigInputs) > 0 {
			parts := make([]string, 0, len(current.ConfigInputs))
			for _, input := range current.ConfigInputs {
				requirement := "optional"
				if input.Required {
					requirement = "required"
				}
				parts = append(parts, fmt.Sprintf("%s(%s)", input.Key, requirement))
			}
			inputs = strings.Join(parts, ",")
		}
		fmt.Fprintf(w, "%s\t%s\tv%d\t%s\t%s\tconfigs=%d tasks=%d series=%d automations=%d\tsecrets=%s\tinputs=%s\n",
			item.Key, item.Name, current.Version, current.ID, current.Hash,
			current.Counts.Configs, current.Counts.Tasks, current.Counts.Series, current.Counts.Automations, secrets, inputs)
	}
	return nil
}

func renderProjectTemplateInstantiateResult(w io.Writer, asJSON bool, result app.InstantiateResult) error {
	if asJSON {
		return render.JSON(w, map[string]any{"project": projectViewForJSON(result.Project), "counts": result.Counts})
	}
	fmt.Fprintf(w, "Created project %s\nURL: %s\nConfigs: %d Tasks: %d Series: %d Automations: %d\n",
		result.Project.Slug, result.Project.URL, result.Counts.Configs, result.Counts.Tasks, result.Counts.Series, result.Counts.Automations)
	return nil
}

func newProjectListCommand(opts Options) *cobra.Command {
	var includeArchived bool
	var statusFilter string
	renderProjects := func(cmd *cobra.Command, asJSON bool, projects []app.ProjectView) {
		if asJSON {
			_ = render.JSON(cmd.OutOrStdout(), projectViewsForJSON(projects))
			return
		}
		for _, project := range projects {
			suffix := ""
			if project.Status != "" && project.Status != "active" {
				suffix = " " + project.Status
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s%s %s\n", project.Slug, project.Name, suffix, project.URL)
		}
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有项目",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			effective := statusFilter
			if !cmd.Flags().Changed("status") {
				if includeArchived {
					effective = "all"
				} else {
					effective = "open"
				}
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				projects, err := client.ListProjectsByStatus(context.Background(), currentOpts.Workspace, effective)
				if err != nil {
					return err
				}
				renderProjects(cmd, currentOpts.JSON, projects)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			projects, err := svc.ListProjectsByStatus(effective)
			if err != nil {
				return err
			}
			renderProjects(cmd, currentOpts.JSON, projects)
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeArchived, "all", false, "include archived projects")
	cmd.Flags().StringVar(&statusFilter, "status", "open", "filter by status: open|planning|active|archived|cancelled|all")
	return cmd
}

func newProjectAddCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "add <slug> [name:<name>] [description:<text>]",
		Short: "创建项目",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			input, err := parseProjectAddArgs(args)
			if err != nil {
				return err
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				project, err := client.AddProject(context.Background(), currentOpts.Workspace, remote.AddProjectInput{
					Slug:        input.Slug,
					Name:        input.Name,
					Description: input.Description,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created project %s\nURL: %s\n", project.Slug, project.URL)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			project, err := svc.AddProject(input)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created project %s\nURL: %s\n", project.Slug, project.URL)
			return nil
		},
	}
}

func newProjectInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <slug|uuid>",
		Short: "显示项目详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				project, err := client.GetProject(context.Background(), currentOpts.Workspace, args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Slug: %s\nName: %s\nURL: %s\n", project.Slug, project.Name, project.URL)
				if project.Description != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", project.Description)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\nTask Count: %d\n", project.Status, project.TaskCount)
				if len(project.RecentAnnotations) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Recent Annotations:")
					for _, a := range project.RecentAnnotations {
						preview := a.Content
						if len(preview) > 100 {
							preview = preview[:100] + "..."
						}
						fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s\n", formatUnixTime(a.Entry), preview)
					}
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			project, err := svc.ProjectInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Slug: %s\nName: %s\nURL: %s\n", project.Slug, project.Name, project.URL)
			if project.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", project.Description)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\nTask Count: %d\n", project.Status, project.TaskCount)
			if len(project.RecentAnnotations) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Recent Annotations:")
				for _, a := range project.RecentAnnotations {
					preview := a.Content
					if len(preview) > 100 {
						preview = preview[:100] + "..."
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s\n", formatUnixTime(a.Entry), preview)
				}
			}
			return nil
		},
	}
}

func newProjectModifyCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "modify <slug|uuid> [name:<name>] [description:<text>]",
		Short: "修改项目属性",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			input, err := parseProjectModifyArgs(args[1:])
			if err != nil {
				return err
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				project, err := client.ModifyProject(context.Background(), currentOpts.Workspace, args[0], remote.ModifyProjectInput{
					Slug:        input.Slug,
					Name:        input.Name,
					Description: input.Description,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified project %s\nURL: %s\n", args[0], project.URL)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.ModifyProject(args[0], input); err != nil {
				return err
			}
			project, err := svc.ProjectInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified project %s\nURL: %s\n", args[0], project.URL)
			return nil
		},
	}
}

func newProjectArchiveCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <slug|uuid>",
		Short: "归档项目",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				project, err := client.ArchiveProject(context.Background(), currentOpts.Workspace, args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				if project.TaskCount > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "xuanchu: warning: archived project %s still has %d non-deleted task(s)\n", project.Slug, project.TaskCount)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Archived project %s\nURL: %s\n", args[0], project.URL)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			project, err := svc.ArchiveProject(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			if project.TaskCount > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "xuanchu: warning: archived project %s still has %d non-deleted task(s)\n", project.Slug, project.TaskCount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Archived project %s\nURL: %s\n", args[0], project.URL)
			return nil
		},
	}
}

func newProjectTransitionCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "transition <slug|uuid> <planning|active|archived|cancelled>",
		Short: "转移项目状态",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				project, err := client.TransitionProject(context.Background(), currentOpts.Workspace, args[0], args[1])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Transitioned project %s to %s\nURL: %s\n", project.Slug, args[1], project.URL)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			project, err := svc.TransitionProject(args[0], args[1])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), projectViewForJSON(project))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Transitioned project %s to %s\nURL: %s\n", project.Slug, args[1], project.URL)
			return nil
		},
	}
}

func newProjectConfigCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "查看或修改项目配置",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newProjectConfigGetCommand(opts))
	cmd.AddCommand(newProjectConfigSetCommand(opts))
	cmd.AddCommand(newProjectConfigUnsetCommand(opts))
	cmd.AddCommand(newProjectConfigListCommand(opts))
	return cmd
}

func newProjectConfigGetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <project> <key>",
		Short: "获取项目配置项的值",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				value, err := client.ProjectConfigGet(context.Background(), currentOpts.Workspace, args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), value)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			value, ok, err := svc.ProjectConfigGet(args[0], args[1])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("unknown project config key %q", args[1])
			}
			fmt.Fprintln(cmd.OutOrStdout(), value)
			return nil
		},
	}
}

func newProjectConfigSetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "set <project> <key> <value>",
		Short: "设置项目配置项",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.ProjectConfigSet(context.Background(), currentOpts.Workspace, args[0], args[1], args[2])
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ProjectConfigSet(args[0], args[1], args[2])
		},
	}
}

func newProjectConfigUnsetCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "unset <project> <key>",
		Short: "删除项目配置项",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				return client.ProjectConfigUnset(context.Background(), currentOpts.Workspace, args[0], args[1])
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			return svc.ProjectConfigUnset(args[0], args[1])
		},
	}
}

func newProjectConfigListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list <project>",
		Short: "列出项目所有配置项",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var values map[string]string
			var err error
			if remoteMode, _, modeErr := isRemoteMode(currentOpts); modeErr != nil {
				return modeErr
			} else if remoteMode {
				client, clientErr := buildRemoteClient(currentOpts)
				if clientErr != nil {
					return clientErr
				}
				values, err = client.ProjectConfigList(context.Background(), currentOpts.Workspace, args[0])
			} else {
				svc, closeFn, serviceErr := buildServiceFromCmd(cmd, opts)
				if serviceErr != nil {
					return serviceErr
				}
				defer closeFn()
				values, err = svc.ProjectConfigList(args[0])
			}
			if err != nil {
				return err
			}
			keys := make([]string, 0, len(values))
			for key := range values {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, values[key])
			}
			return nil
		},
	}
}

func parseProjectAddArgs(args []string) (app.AddProjectInput, error) {
	input := app.AddProjectInput{Slug: args[0]}
	values, err := parseKeyValueArgs(args[1:], map[string]bool{"name": true, "description": true})
	if err != nil {
		return app.AddProjectInput{}, err
	}
	input.Name = values["name"]
	input.Description = values["description"]
	return input, nil
}

func parseProjectModifyArgs(args []string) (app.ModifyProjectInput, error) {
	input := app.ModifyProjectInput{}
	values, err := parseKeyValueArgs(args, map[string]bool{"name": true, "description": true})
	if err != nil {
		return app.ModifyProjectInput{}, err
	}
	if value, ok := values["name"]; ok {
		v := value
		input.Name = &v
	}
	if value, ok := values["description"]; ok {
		v := value
		input.Description = &v
	}
	return input, nil
}

func projectViewsForJSON(projects []app.ProjectView) []map[string]any {
	out := make([]map[string]any, 0, len(projects))
	for _, project := range projects {
		out = append(out, projectViewForJSON(project))
	}
	return out
}

func projectViewForJSON(project app.ProjectView) map[string]any {
	m := map[string]any{
		"url":          project.URL,
		"id":           project.ID,
		"workspace_id": project.WorkspaceID,
		"slug":         project.Slug,
		"name":         project.Name,
		"description":  project.Description,
		"status":       project.Status,
		"task_count":   project.TaskCount,
		"created_at":   project.CreatedAt,
		"modified_at":  project.ModifiedAt,
		"archived_at":  project.ArchivedAt,
	}
	if len(project.RecentAnnotations) > 0 {
		anns := make([]map[string]any, len(project.RecentAnnotations))
		for i, a := range project.RecentAnnotations {
			anns[i] = map[string]any{
				"id":         a.ID,
				"project_id": a.ProjectID,
				"entry":      a.Entry,
				"content":    a.Content,
				"created_by": actorInfoToJSONMap(a.CreatedBy),
				"created_at": a.CreatedAt,
			}
		}
		m["recent_annotations"] = anns
	}
	return m
}

func newProjectAnnotateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "annotate <project-ref> <content...>",
		Short: "为项目添加备注",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			content := strings.Join(args[1:], " ")
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if _, err := client.AnnotateProject(context.Background(), currentOpts.Workspace, args[0], content); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if _, err := svc.ProjectAnnotate(args[0], content); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Annotated project", args[0])
			return nil
		},
	}
}

func newProjectAnnotationsCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "annotations <project-ref>",
		Short: "列出项目备注",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				annotations, err := client.ListProjectAnnotations(context.Background(), currentOpts.Workspace, args[0])
				if err != nil {
					return err
				}
				return renderRemoteProjectAnnotations(cmd, currentOpts.JSON, annotations)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			annotations, err := svc.ProjectAnnotations(args[0])
			if err != nil {
				return err
			}
			return renderProjectAnnotationInfos(cmd, currentOpts.JSON, annotations)
		},
	}
}

func newProjectTimelineCommand(opts Options) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "timeline <project-ref>",
		Short: "查看项目时间线",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				entries, err := client.ProjectTimeline(context.Background(), currentOpts.Workspace, args[0], limit)
				if err != nil {
					return err
				}
				return renderRemoteTimelineEntries(cmd, currentOpts.JSON, entries)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			entries, err := svc.ProjectTimeline(args[0], app.TimelineOptions{Limit: limit})
			if err != nil {
				return err
			}
			return renderTimelineEntriesApp(cmd, currentOpts.JSON, entries)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of timeline entries")
	return cmd
}

func newProjectDenotateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "denotate <project-ref> <annotation-id>",
		Short: "删除项目备注",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if err := client.DenotateProject(context.Background(), currentOpts.Workspace, args[0], args[1]); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Removed annotation from project", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.ProjectDenotate(args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Removed annotation from project", args[0])
			return nil
		},
	}
}

func formatUnixTime(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}

func renderProjectAnnotationInfos(cmd *cobra.Command, asJSON bool, annotations []app.ProjectAnnotationInfo) error {
	if asJSON {
		return render.JSON(cmd.OutOrStdout(), annotations)
	}
	if len(annotations) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No annotations.")
		return nil
	}
	for i, a := range annotations {
		fmt.Fprintf(cmd.OutOrStdout(), "%d [%s] %s\n", i+1, formatUnixTime(a.Entry), a.Content)
	}
	return nil
}

func renderRemoteProjectAnnotations(cmd *cobra.Command, asJSON bool, annotations []remote.ProjectAnnotationDTO) error {
	if asJSON {
		return render.JSON(cmd.OutOrStdout(), annotations)
	}
	if len(annotations) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No annotations.")
		return nil
	}
	for i, a := range annotations {
		fmt.Fprintf(cmd.OutOrStdout(), "%d [%s] %s\n", i+1, formatUnixTime(a.Entry), a.Content)
	}
	return nil
}

func renderTimelineEntriesApp(cmd *cobra.Command, asJSON bool, entries []app.TimelineEntry) error {
	if asJSON {
		return render.JSON(cmd.OutOrStdout(), entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No timeline entries.")
		return nil
	}
	for _, e := range entries {
		label := e.SourceLabel
		if len(label) > 40 {
			label = label[:40] + "..."
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] <%s> %s: %s\n", formatUnixTime(e.Entry), e.SourceType, label, e.Content)
	}
	return nil
}

func renderRemoteTimelineEntries(cmd *cobra.Command, asJSON bool, entries []remote.TimelineEntryDTO) error {
	if asJSON {
		return render.JSON(cmd.OutOrStdout(), entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No timeline entries.")
		return nil
	}
	for _, e := range entries {
		label := e.SourceLabel
		if len(label) > 40 {
			label = label[:40] + "..."
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] <%s> %s: %s\n", formatUnixTime(e.Entry), e.SourceType, label, e.Content)
	}
	return nil
}

func renderAnnotations(cmd *cobra.Command, asJSON bool, annotations []task.Annotation) error {
	if asJSON {
		if annotations == nil {
			annotations = []task.Annotation{}
		}
		return render.JSON(cmd.OutOrStdout(), annotations)
	}
	if len(annotations) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No annotations.")
		return nil
	}
	for _, a := range annotations {
		fmt.Fprintf(cmd.OutOrStdout(), "%s [%s] %s\n", a.ID, formatUnixTime(a.Entry), a.Description)
	}
	return nil
}
