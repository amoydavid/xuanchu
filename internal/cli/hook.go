package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/remote"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newHookCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "管理 Webhook",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newHookListCommand(opts))
	cmd.AddCommand(newHookAddCommand(opts))
	cmd.AddCommand(newHookInfoCommand(opts))
	cmd.AddCommand(newHookModifyCommand(opts))
	cmd.AddCommand(newHookEnableCommand(opts))
	cmd.AddCommand(newHookDisableCommand(opts))
	cmd.AddCommand(newHookDeleteCommand(opts))
	cmd.AddCommand(newHookDeliveriesCommand(opts))
	cmd.AddCommand(newHookReplayCommand(opts))
	return cmd
}

func newHookListCommand(opts Options) *cobra.Command {
	var projectRef string
	var includeDisabled bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有 Webhook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				hooks, err := client.ListHooks(context.Background(), currentOpts.Workspace, projectRef)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookViewsForJSON(hooks))
				}
				renderHookList(cmd.OutOrStdout(), hooks)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			hooks, err := svc.ListHooks(projectRef)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), hookViewsForJSON(hooks))
			}
			renderHookList(cmd.OutOrStdout(), hooks)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectRef, "project", "", "按项目 slug 过滤")
	cmd.Flags().BoolVar(&includeDisabled, "all", false, "包含 disabled hook")
	_ = includeDisabled
	return cmd
}

func newHookAddCommand(opts Options) *cobra.Command {
	var (
		scopeType   string
		projectRef  string
		events      []string
		endpointURL string
		secret      string
		secretStdin bool
		secretFile  string
		timeout     int
		maxAttempts int
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "创建 Webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretVal, err := resolveSecret(secret, secretStdin, secretFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				hook, err := client.AddHook(context.Background(), currentOpts.Workspace, remote.HookCreateRequest{
					Name: args[0], ScopeType: scopeType, ProjectRef: projectRef, EventTypes: events, EndpointURL: endpointURL, Secret: secretVal, TimeoutSeconds: timeout, MaxAttempts: maxAttempts,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created hook %s (%s)\n", hook.Name, hook.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			hook, err := svc.AddHook(app.HookAddInput{
				Name:           args[0],
				ScopeType:      app.HookScopeType(scopeType),
				ProjectRef:     projectRef,
				EventTypes:     events,
				EndpointURL:    endpointURL,
				Secret:         secretVal,
				TimeoutSeconds: timeout,
				MaxAttempts:    maxAttempts,
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created hook %s (%s)\n", hook.Name, hook.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&scopeType, "scope", "workspace", "作用域类型: workspace 或 project")
	cmd.Flags().StringVar(&projectRef, "project", "", "项目 slug（project 作用域时必填）")
	cmd.Flags().StringArrayVar(&events, "event", nil, "事件类型（可重复指定）")
	cmd.Flags().StringVar(&endpointURL, "url", "", "webhook 接收端 URL")
	cmd.Flags().StringVar(&secret, "secret", "", "签名密钥")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "从 stdin 读取签名密钥")
	cmd.Flags().StringVar(&secretFile, "secret-file", "", "从文件读取签名密钥")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "超时秒数（默认 10）")
	cmd.Flags().IntVar(&maxAttempts, "max-attempts", 0, "最大重试次数（默认 5）")
	cmd.MarkFlagRequired("event")
	cmd.MarkFlagRequired("url")
	return cmd
}

func newHookInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <hook-id>",
		Short: "显示 Webhook 详情",
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
				hook, err := client.HookInfo(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
				}
				renderHookInfo(cmd.OutOrStdout(), hook)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			hook, err := svc.HookInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
			}
			renderHookInfo(cmd.OutOrStdout(), hook)
			return nil
		},
	}
}

func newHookModifyCommand(opts Options) *cobra.Command {
	var (
		events      []string
		name        string
		endpointURL string
		secret      string
		secretStdin bool
		secretFile  string
		timeout     int
		maxAttempts int
	)
	cmd := &cobra.Command{
		Use:   "modify <hook-id>",
		Short: "修改 Webhook 属性",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretVal, err := resolveSecret(secret, secretStdin, secretFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			currentOpts := optionsFromCmd(cmd, opts)
			input := buildHookModifyInput(name, events, endpointURL, secretVal, timeout, maxAttempts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				hook, err := client.ModifyHook(context.Background(), args[0], hookModifyInputToRemote(input))
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified hook %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			_, err = svc.ModifyHook(args[0], input)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				hook, infoErr := svc.HookInfo(args[0])
				if infoErr != nil {
					return infoErr
				}
				return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified hook %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "hook 名称")
	cmd.Flags().StringArrayVar(&events, "event", nil, "事件类型（可重复指定）")
	cmd.Flags().StringVar(&endpointURL, "url", "", "webhook 接收端 URL")
	cmd.Flags().StringVar(&secret, "secret", "", "签名密钥")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "从 stdin 读取签名密钥")
	cmd.Flags().StringVar(&secretFile, "secret-file", "", "从文件读取签名密钥")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "超时秒数")
	cmd.Flags().IntVar(&maxAttempts, "max-attempts", 0, "最大重试次数")
	return cmd
}

func newHookEnableCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <hook-id>",
		Short: "启用 Webhook",
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
				hook, err := client.EnableHook(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Enabled hook %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			_, err = svc.EnableHook(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Enabled hook %s\n", args[0])
			return nil
		},
	}
}

func newHookDisableCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <hook-id>",
		Short: "禁用 Webhook",
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
				hook, err := client.DisableHook(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookViewForJSON(hook))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Disabled hook %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			_, err = svc.DisableHook(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Disabled hook %s\n", args[0])
			return nil
		},
	}
}

func newHookDeleteCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <hook-id>",
		Short: "删除 Webhook",
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
				if err := client.DeleteHook(context.Background(), args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted hook %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.DeleteHook(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted hook %s\n", args[0])
			return nil
		},
	}
}

func newHookDeliveriesCommand(opts Options) *cobra.Command {
	var status string
	var limit int
	cmd := &cobra.Command{
		Use:   "deliveries <hook-id>",
		Short: "查看 Webhook 投递记录",
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
				deliveries, err := client.ListHookDeliveries(context.Background(), args[0], status, limit, 0)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookDeliveryViewsForJSON(deliveries))
				}
				renderHookDeliveryList(cmd.OutOrStdout(), deliveries)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			deliveries, err := svc.ListHookDeliveries(args[0], status, limit, 0)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), hookDeliveryViewsForJSON(deliveries))
			}
			renderHookDeliveryList(cmd.OutOrStdout(), deliveries)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "按状态过滤（pending/succeeded/failed/dead_lettered）")
	cmd.Flags().IntVar(&limit, "limit", 50, "最多显示投递记录数")
	return cmd
}

func newHookReplayCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "replay <delivery-id>",
		Short: "重放 Webhook 投递",
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
				delivery, err := client.ReplayHookDelivery(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), hookDeliveryViewForJSON(delivery))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Replayed delivery %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			_, err = svc.ReplayHookDelivery(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Replayed delivery %s\n", args[0])
			return nil
		},
	}
}

// resolveSecret 从三种来源解析密钥，互斥使用。
func resolveSecret(secret string, secretStdin bool, secretFile string, stdin interface {
	Read(p []byte) (n int, err error)
}) (string, error) {
	sources := 0
	if secret != "" {
		sources++
	}
	if secretStdin {
		sources++
	}
	if secretFile != "" {
		sources++
	}
	if sources > 1 {
		return "", fmt.Errorf("--secret, --secret-stdin 和 --secret-file 互斥，只能指定一个")
	}
	if secret != "" {
		return secret, nil
	}
	if secretStdin {
		reader := bufio.NewReader(stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			// 读取到 EOF 也是合法的（比如管道无换行）
			if line == "" {
				return "", fmt.Errorf("从 stdin 读取密钥失败")
			}
		}
		return strings.TrimRight(line, "\n"), nil
	}
	if secretFile != "" {
		data, err := os.ReadFile(secretFile)
		if err != nil {
			return "", fmt.Errorf("读取密钥文件 %s 失败: %w", secretFile, err)
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	return "", nil
}

// buildHookModifyInput 从 CLI flags 构建 app 层修改输入。
func buildHookModifyInput(name string, events []string, endpointURL, secret string, timeout, maxAttempts int) app.HookModifyInput {
	input := app.HookModifyInput{}
	if name != "" {
		input.Name = &name
	}
	if len(events) > 0 {
		input.EventTypes = &events
	}
	if endpointURL != "" {
		input.EndpointURL = &endpointURL
	}
	if secret != "" {
		input.Secret = &secret
	}
	if timeout != 0 {
		input.TimeoutSeconds = &timeout
	}
	if maxAttempts != 0 {
		input.MaxAttempts = &maxAttempts
	}
	return input
}

// hookModifyInputToRemote 将 app 层输入转换为 remote 层 DTO。
func hookModifyInputToRemote(input app.HookModifyInput) remote.HookModifyRequest {
	return remote.HookModifyRequest{
		Name:           input.Name,
		EventTypes:     input.EventTypes,
		EndpointURL:    input.EndpointURL,
		Secret:         input.Secret,
		TimeoutSeconds: input.TimeoutSeconds,
		MaxAttempts:    input.MaxAttempts,
	}
}

// renderHookList 渲染 hook 列表的可读表格。
func renderHookList(w interface {
	Write(p []byte) (n int, err error)
}, hooks []app.HookView) {
	if len(hooks) == 0 {
		return
	}
	for _, h := range hooks {
		enabled := "disabled"
		if h.Enabled {
			enabled = "enabled"
		}
		events := strings.Join(h.EventTypes, ",")
		scope := h.ScopeType
		fmt.Fprintf(w, "%s  %s  %s  [%s]  %s  %s\n", shortID(h.ID), h.Name, scope, events, enabled, h.EndpointURL)
	}
}

// renderHookInfo 渲染单个 hook 的详细信息。
func renderHookInfo(w interface {
	Write(p []byte) (n int, err error)
}, hook app.HookView) {
	fmt.Fprintf(w, "ID: %s\n", hook.ID)
	fmt.Fprintf(w, "Name: %s\n", hook.Name)
	fmt.Fprintf(w, "Scope: %s\n", hook.ScopeType)
	if hook.ProjectID != nil {
		fmt.Fprintf(w, "Project ID: %s\n", *hook.ProjectID)
	}
	fmt.Fprintf(w, "Events: %s\n", strings.Join(hook.EventTypes, ", "))
	fmt.Fprintf(w, "URL: %s\n", hook.EndpointURL)
	enabled := "disabled"
	if hook.Enabled {
		enabled = "enabled"
	}
	fmt.Fprintf(w, "Enabled: %s\n", enabled)
	fmt.Fprintf(w, "Timeout: %ds\n", hook.TimeoutSeconds)
	fmt.Fprintf(w, "Max Attempts: %d\n", hook.MaxAttempts)
}

// renderHookDeliveryList 渲染 hook 投递列表。
func renderHookDeliveryList(w interface {
	Write(p []byte) (n int, err error)
}, deliveries []app.HookDeliveryView) {
	if len(deliveries) == 0 {
		return
	}
	for _, d := range deliveries {
		errMsg := d.LastError
		if errMsg == "" {
			errMsg = "-"
		}
		fmt.Fprintf(w, "%s  %s  %s  %d  %s\n", shortID(d.ID), d.EventType, d.Status, d.AttemptCount, errMsg)
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// hookViewsForJSON 将 hook 视图列表转换为 JSON 输出格式。
func hookViewsForJSON(hooks []app.HookView) []map[string]any {
	out := make([]map[string]any, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, hookViewForJSON(h))
	}
	return out
}

// hookViewForJSON 将单个 hook 视图转换为 JSON 输出格式。
func hookViewForJSON(hook app.HookView) map[string]any {
	return map[string]any{
		"id":              hook.ID,
		"name":            hook.Name,
		"scope_type":      hook.ScopeType,
		"workspace_id":    hook.WorkspaceID,
		"project_id":      hook.ProjectID,
		"event_types":     hook.EventTypes,
		"endpoint_url":    hook.EndpointURL,
		"enabled":         hook.Enabled,
		"timeout_seconds": hook.TimeoutSeconds,
		"max_attempts":    hook.MaxAttempts,
		"created_at":      hook.CreatedAt,
		"modified_at":     hook.ModifiedAt,
	}
}

// hookDeliveryViewsForJSON 将投递视图列表转换为 JSON 输出格式。
func hookDeliveryViewsForJSON(deliveries []app.HookDeliveryView) []map[string]any {
	out := make([]map[string]any, 0, len(deliveries))
	for _, d := range deliveries {
		out = append(out, hookDeliveryViewForJSON(d))
	}
	return out
}

// hookDeliveryViewForJSON 将单个投递视图转换为 JSON 输出格式。
func hookDeliveryViewForJSON(delivery app.HookDeliveryView) map[string]any {
	return map[string]any{
		"id":               delivery.ID,
		"hook_id":          delivery.HookID,
		"event_id":         delivery.EventID,
		"event_type":       delivery.EventType,
		"workspace_id":     delivery.WorkspaceID,
		"project_id":       delivery.ProjectID,
		"actor":            userInfoToJSONMap(&delivery.Actor),
		"payload":          delivery.Payload,
		"headers":          delivery.Headers,
		"status":           delivery.Status,
		"attempt_count":    delivery.AttemptCount,
		"next_attempt_at":  delivery.NextAttemptAt,
		"claim_expires_at": delivery.ClaimExpiresAt,
		"last_attempt_at":  delivery.LastAttemptAt,
		"last_status_code": delivery.LastStatusCode,
		"last_error":       delivery.LastError,
		"created_at":       delivery.CreatedAt,
		"modified_at":      delivery.ModifiedAt,
	}
}
