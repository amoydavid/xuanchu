package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"git.dajee.net/dajee/xuanchu/internal/task"
	"github.com/spf13/cobra"
)

func newNotificationCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notification",
		Short: "管理通知 sink 和投递记录",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newNotificationSinkCommand(opts))
	cmd.AddCommand(newNotificationRuleCommand(opts))
	cmd.AddCommand(newNotificationDeliveryCommand(opts))
	return cmd
}

func newNotificationSinkCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "sink", Short: "管理通知 sink", Args: cobra.NoArgs}
	cmd.AddCommand(newNotificationSinkAddCommand(opts))
	cmd.AddCommand(newNotificationSinkListCommand(opts))
	cmd.AddCommand(newNotificationSinkInfoCommand(opts))
	cmd.AddCommand(newNotificationSinkModifyCommand(opts))
	cmd.AddCommand(newNotificationSinkEnableCommand(opts))
	cmd.AddCommand(newNotificationSinkDisableCommand(opts))
	cmd.AddCommand(newNotificationSinkDeleteCommand(opts))
	return cmd
}

func newNotificationSinkAddCommand(opts Options) *cobra.Command {
	var input notificationSinkCLIInput
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "创建通知 sink",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			addInput, err := input.toApp(args[0])
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
				view, err := client.AddNotificationSink(context.Background(), currentOpts.Workspace, notificationSinkInputToRemote(addInput))
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created notification sink %s (%s)\n", view.Name, view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.AddNotificationSink(addInput)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created notification sink %s (%s)\n", view.Name, view.ID)
			return nil
		},
	}
	bindNotificationSinkFlags(cmd, &input)
	return cmd
}

type notificationSinkCLIInput struct {
	name             string
	typ              string
	endpointMode     string
	url              string
	urlTemplate      string
	configKey        string
	allowedHosts     []string
	headers          []string
	bodyTemplate     string
	bodyTemplateFile string
	bodyContentType  string
	secretRefs       []string
	secret           string
	timeoutSeconds   int
	maxAttempts      int
	maxConcurrency   int
}

func bindNotificationSinkFlags(cmd *cobra.Command, input *notificationSinkCLIInput) {
	cmd.Flags().StringVar(&input.typ, "type", "webhook", "sink 类型: webhook 或 http_template")
	cmd.Flags().StringVar(&input.endpointMode, "endpoint-mode", "static_url", "endpoint 模式: static_url、template、config_value")
	cmd.Flags().StringVar(&input.url, "url", "", "固定 endpoint URL")
	cmd.Flags().StringVar(&input.urlTemplate, "url-template", "", "受控 endpoint URL 模板")
	cmd.Flags().StringVar(&input.configKey, "config-key", "", "读取 endpoint 的共享配置 key")
	cmd.Flags().StringArrayVar(&input.allowedHosts, "allowed-host", nil, "允许的 endpoint host，可重复指定")
	cmd.Flags().StringArrayVar(&input.headers, "header", nil, "HTTP 模板 header，格式 Name=Value，可重复指定")
	cmd.Flags().StringVar(&input.bodyTemplate, "body-template", "", "HTTP 模板 body")
	cmd.Flags().StringVar(&input.bodyTemplateFile, "body-template-file", "", "从文件读取 HTTP 模板 body 并保存到数据库")
	cmd.Flags().StringVar(&input.bodyContentType, "body-content-type", "", "HTTP 模板 body content type")
	cmd.Flags().StringArrayVar(&input.secretRefs, "secret-ref", nil, "secret 模板引用，格式 alias=config.key，可重复指定")
	cmd.Flags().StringVar(&input.secret, "secret", "", "webhook 签名 secret")
	cmd.Flags().IntVar(&input.timeoutSeconds, "timeout", 0, "超时秒数（默认 10）")
	cmd.Flags().IntVar(&input.maxAttempts, "max-attempts", 0, "最大重试次数（默认 5）")
	cmd.Flags().IntVar(&input.maxConcurrency, "max-concurrency", 0, "最大并发投递数（0 表示继承默认值）")
}

func (input notificationSinkCLIInput) toApp(name string) (app.NotificationSinkAddInput, error) {
	body := input.bodyTemplate
	if input.bodyTemplateFile != "" {
		data, err := os.ReadFile(input.bodyTemplateFile)
		if err != nil {
			return app.NotificationSinkAddInput{}, err
		}
		body = string(data)
	}
	headers, err := parseHTTPHeaderTemplates(input.headers)
	if err != nil {
		return app.NotificationSinkAddInput{}, err
	}
	secretRefs, err := parseHTTPSecretRefs(input.secretRefs)
	if err != nil {
		return app.NotificationSinkAddInput{}, err
	}
	return app.NotificationSinkAddInput{
		Name:            name,
		Type:            input.typ,
		EndpointMode:    input.endpointMode,
		URL:             input.url,
		URLTemplate:     input.urlTemplate,
		ConfigKey:       input.configKey,
		AllowedHosts:    input.allowedHosts,
		HTTPMethod:      "POST",
		HeaderTemplates: headers,
		BodyTemplate:    body,
		BodyContentType: input.bodyContentType,
		SecretRefs:      secretRefs,
		Secret:          input.secret,
		TimeoutSeconds:  input.timeoutSeconds,
		MaxAttempts:     input.maxAttempts,
		MaxConcurrency:  input.maxConcurrency,
	}, nil
}

func (input notificationSinkCLIInput) toModifyApp(cmd *cobra.Command) (app.NotificationSinkModifyInput, error) {
	var mod app.NotificationSinkModifyInput
	if cmd.Flags().Changed("name") {
		mod.Name = &input.name
	}
	if cmd.Flags().Changed("type") {
		mod.Type = &input.typ
	}
	if cmd.Flags().Changed("endpoint-mode") {
		mod.EndpointMode = &input.endpointMode
	}
	if cmd.Flags().Changed("url") {
		mod.URL = &input.url
	}
	if cmd.Flags().Changed("url-template") {
		mod.URLTemplate = &input.urlTemplate
	}
	if cmd.Flags().Changed("config-key") {
		mod.ConfigKey = &input.configKey
	}
	if cmd.Flags().Changed("allowed-host") {
		mod.AllowedHosts = &input.allowedHosts
	}
	if cmd.Flags().Changed("header") {
		headers, err := parseHTTPHeaderTemplates(input.headers)
		if err != nil {
			return app.NotificationSinkModifyInput{}, err
		}
		mod.HeaderTemplates = &headers
	}
	if cmd.Flags().Changed("body-template") || cmd.Flags().Changed("body-template-file") {
		body := input.bodyTemplate
		if input.bodyTemplateFile != "" {
			data, err := os.ReadFile(input.bodyTemplateFile)
			if err != nil {
				return app.NotificationSinkModifyInput{}, err
			}
			body = string(data)
		}
		mod.BodyTemplate = &body
	}
	if cmd.Flags().Changed("body-content-type") {
		mod.BodyContentType = &input.bodyContentType
	}
	if cmd.Flags().Changed("secret-ref") {
		secretRefs, err := parseHTTPSecretRefs(input.secretRefs)
		if err != nil {
			return app.NotificationSinkModifyInput{}, err
		}
		mod.SecretRefs = &secretRefs
	}
	if cmd.Flags().Changed("secret") {
		mod.Secret = &input.secret
	}
	if cmd.Flags().Changed("timeout") {
		mod.TimeoutSeconds = &input.timeoutSeconds
	}
	if cmd.Flags().Changed("max-attempts") {
		mod.MaxAttempts = &input.maxAttempts
	}
	if cmd.Flags().Changed("max-concurrency") {
		mod.MaxConcurrency = &input.maxConcurrency
	}
	return mod, nil
}

func parseHTTPHeaderTemplates(values []string) ([]app.HTTPHeaderTemplateInput, error) {
	out := make([]app.HTTPHeaderTemplateInput, 0, len(values))
	for _, value := range values {
		name, body, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, app.RuntimeError{Code: "notification_sink_invalid", Message: "header must use Name=Value"}
		}
		out = append(out, app.HTTPHeaderTemplateInput{Name: strings.TrimSpace(name), Value: body})
	}
	return out, nil
}

func parseHTTPSecretRefs(values []string) ([]app.HTTPTemplateSecretRefInput, error) {
	out := make([]app.HTTPTemplateSecretRefInput, 0, len(values))
	for _, value := range values {
		alias, key, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(alias) == "" || strings.TrimSpace(key) == "" {
			return nil, app.RuntimeError{Code: "notification_sink_invalid", Message: "secret-ref must use alias=config.key"}
		}
		out = append(out, app.HTTPTemplateSecretRefInput{Alias: strings.TrimSpace(alias), ConfigKey: strings.TrimSpace(key)})
	}
	return out, nil
}

func newNotificationSinkListCommand(opts Options) *cobra.Command {
	var includeDisabled bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出通知 sink",
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
				rows, err := client.ListNotificationSinks(context.Background(), currentOpts.Workspace, includeDisabled)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationSinkViewsForJSON(rows))
				}
				renderNotificationSinkList(cmd.OutOrStdout(), rows)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			rows, err := svc.ListNotificationSinks(includeDisabled)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationSinkViewsForJSON(rows))
			}
			renderNotificationSinkList(cmd.OutOrStdout(), rows)
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeDisabled, "all", false, "包含 disabled sink")
	return cmd
}

func newNotificationSinkInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <sink-id>",
		Short: "显示通知 sink 详情",
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
				view, err := client.NotificationSinkInfo(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
				}
				renderNotificationSinkInfo(cmd.OutOrStdout(), view)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.NotificationSinkInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
			}
			renderNotificationSinkInfo(cmd.OutOrStdout(), view)
			return nil
		},
	}
}

func newNotificationSinkModifyCommand(opts Options) *cobra.Command {
	var input notificationSinkCLIInput
	cmd := &cobra.Command{
		Use:   "modify <sink-id>",
		Short: "修改通知 sink",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			mod, err := input.toModifyApp(cmd)
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
				view, err := client.ModifyNotificationSink(context.Background(), args[0], notificationSinkModifyInputToRemote(mod))
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified notification sink %s (%s)\n", view.Name, view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.ModifyNotificationSink(args[0], mod)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified notification sink %s (%s)\n", view.Name, view.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&input.name, "name", "", "新的 sink 名称")
	bindNotificationSinkFlags(cmd, &input)
	return cmd
}

func newNotificationSinkEnableCommand(opts Options) *cobra.Command {
	return notificationSinkToggleCommand(opts, "enable", "启用通知 sink", true)
}

func newNotificationSinkDisableCommand(opts Options) *cobra.Command {
	return notificationSinkToggleCommand(opts, "disable", "禁用通知 sink", false)
}

func notificationSinkToggleCommand(opts Options, use string, short string, enabled bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <sink-id>",
		Short: short,
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
				var view app.NotificationSinkView
				if enabled {
					view, err = client.EnableNotificationSink(context.Background(), args[0])
				} else {
					view, err = client.DisableNotificationSink(context.Background(), args[0])
				}
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
				}
				if enabled {
					fmt.Fprintf(cmd.OutOrStdout(), "Enabled notification sink %s\n", args[0])
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Disabled notification sink %s\n", args[0])
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			var view app.NotificationSinkView
			if enabled {
				view, err = svc.EnableNotificationSink(args[0])
			} else {
				view, err = svc.DisableNotificationSink(args[0])
			}
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationSinkViewForJSON(view))
			}
			if enabled {
				fmt.Fprintf(cmd.OutOrStdout(), "Enabled notification sink %s\n", args[0])
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Disabled notification sink %s\n", args[0])
			}
			return nil
		},
	}
}

func newNotificationSinkDeleteCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <sink-id>",
		Short: "删除通知 sink",
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
				if err := client.DeleteNotificationSink(context.Background(), args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted notification sink %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.DeleteNotificationSink(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted notification sink %s\n", args[0])
			return nil
		},
	}
}

func newNotificationDeliveryCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "delivery", Short: "管理通知投递记录", Args: cobra.NoArgs}
	cmd.AddCommand(newNotificationDeliveryListCommand(opts))
	cmd.AddCommand(newNotificationDeliveryInfoCommand(opts))
	cmd.AddCommand(newNotificationDeliveryReplayCommand(opts))
	return cmd
}

func newNotificationDeliveryListCommand(opts Options) *cobra.Command {
	var status string
	var sinkID string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出通知投递记录",
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
				rows, err := client.ListNotificationDeliveries(context.Background(), currentOpts.Workspace, sinkID, status, limit, 0)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewsForJSON(rows))
				}
				renderNotificationDeliveryList(cmd.OutOrStdout(), rows)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			rows, err := svc.ListNotificationDeliveries(sinkID, status, limit, 0)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewsForJSON(rows))
			}
			renderNotificationDeliveryList(cmd.OutOrStdout(), rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "按状态过滤")
	cmd.Flags().StringVar(&sinkID, "sink", "", "按 sink ID 过滤")
	cmd.Flags().IntVar(&limit, "limit", 50, "最大返回条数")
	return cmd
}

func newNotificationDeliveryInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <delivery-id>",
		Short: "显示通知投递详情",
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
				view, err := client.NotificationDeliveryInfo(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewForJSON(view))
				}
				return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewForJSON(view))
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.NotificationDeliveryInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewForJSON(view))
			}
			return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewForJSON(view))
		},
	}
}

func newNotificationDeliveryReplayCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "replay <delivery-id>",
		Short: "重放通知投递",
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
				view, err := client.ReplayNotificationDelivery(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewForJSON(view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Replayed notification delivery %s\n", view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.ReplayNotificationDelivery(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationDeliveryViewForJSON(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Replayed notification delivery %s\n", view.ID)
			return nil
		},
	}
}

func newNotificationRuleCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "rule", Short: "管理事件通知规则", Args: cobra.NoArgs}
	cmd.AddCommand(newNotificationRuleAddCommand(opts))
	cmd.AddCommand(newNotificationRuleListCommand(opts))
	cmd.AddCommand(newNotificationRuleInfoCommand(opts))
	cmd.AddCommand(newNotificationRuleModifyCommand(opts))
	cmd.AddCommand(newNotificationRuleEnableCommand(opts))
	cmd.AddCommand(newNotificationRuleDisableCommand(opts))
	cmd.AddCommand(newNotificationRuleDeleteCommand(opts))
	return cmd
}

type notificationRuleCLIInput struct {
	projectRef      string
	eventType       string
	filterSource    string
	audience        string
	recipients      []string
	sinkRef         string
	templateSubject string
	templateBody    string
	name            string
}

func bindNotificationRuleFlags(cmd *cobra.Command, input *notificationRuleCLIInput) {
	cmd.Flags().StringVar(&input.projectRef, "project", "", "项目 slug")
	cmd.Flags().StringVar(&input.eventType, "event", "", "事件类型，例如 task.unblocked")
	cmd.Flags().StringVar(&input.filterSource, "filter", "", "任务过滤表达式，仅 task 事件支持")
	cmd.Flags().StringVar(&input.audience, "audience", "", "受众: actor、assignees、explicit_users、assignees_and_explicit_users")
	cmd.Flags().StringArrayVar(&input.recipients, "recipient", nil, "显式 recipient，可重复指定")
	cmd.Flags().StringVar(&input.sinkRef, "sink", "", "通知 sink 名称或 ID")
	cmd.Flags().StringVar(&input.templateSubject, "template-subject", "", "通知标题模板")
	cmd.Flags().StringVar(&input.templateBody, "template-body", "", "通知正文模板")
}

func (input notificationRuleCLIInput) toApp(name string) app.EventNotificationRuleAddInput {
	return app.EventNotificationRuleAddInput{
		Name:            name,
		ProjectRef:      input.projectRef,
		EventType:       input.eventType,
		FilterSource:    input.filterSource,
		AudienceType:    input.audience,
		Recipients:      input.recipients,
		SinkRef:         input.sinkRef,
		TemplateSubject: input.templateSubject,
		TemplateBody:    input.templateBody,
	}
}

func (input notificationRuleCLIInput) toModifyApp(cmd *cobra.Command) app.EventNotificationRuleModifyInput {
	var mod app.EventNotificationRuleModifyInput
	if cmd.Flags().Changed("name") {
		mod.Name = &input.name
	}
	if cmd.Flags().Changed("project") {
		mod.ProjectRef = &input.projectRef
	}
	if cmd.Flags().Changed("event") {
		mod.EventType = &input.eventType
	}
	if cmd.Flags().Changed("filter") {
		mod.FilterSource = &input.filterSource
	}
	if cmd.Flags().Changed("audience") {
		mod.AudienceType = &input.audience
	}
	if cmd.Flags().Changed("recipient") {
		mod.Recipients = &input.recipients
	}
	if cmd.Flags().Changed("sink") {
		mod.SinkRef = &input.sinkRef
	}
	if cmd.Flags().Changed("template-subject") {
		mod.TemplateSubject = &input.templateSubject
	}
	if cmd.Flags().Changed("template-body") {
		mod.TemplateBody = &input.templateBody
	}
	return mod
}

func newNotificationRuleAddCommand(opts Options) *cobra.Command {
	var input notificationRuleCLIInput
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "创建事件通知规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			addInput := input.toApp(args[0])
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				view, err := client.AddEventNotificationRule(context.Background(), currentOpts.Workspace, notificationRuleInputToRemote(addInput))
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationRuleViewForJSON(view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created notification rule %s (%s)\n", view.Name, view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.AddEventNotificationRule(addInput)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationRuleViewForJSON(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created notification rule %s (%s)\n", view.Name, view.ID)
			return nil
		},
	}
	bindNotificationRuleFlags(cmd, &input)
	return cmd
}

func newNotificationRuleListCommand(opts Options) *cobra.Command {
	var projectRef string
	var includeDisabled bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出事件通知规则",
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
				rows, err := client.ListEventNotificationRules(context.Background(), currentOpts.Workspace, projectRef, includeDisabled)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationRuleViewsForJSON(rows))
				}
				renderNotificationRuleList(cmd.OutOrStdout(), rows)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			rows, err := svc.ListEventNotificationRules(projectRef, includeDisabled)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationRuleViewsForJSON(rows))
			}
			renderNotificationRuleList(cmd.OutOrStdout(), rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectRef, "project", "", "按项目过滤")
	cmd.Flags().BoolVar(&includeDisabled, "all", false, "包含 disabled rule")
	return cmd
}

func newNotificationRuleInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <rule-id>",
		Short: "显示事件通知规则详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var view app.EventNotificationRuleView
			var err error
			if remoteMode, _, modeErr := isRemoteMode(currentOpts); modeErr != nil {
				return modeErr
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				view, err = client.EventNotificationRuleInfo(context.Background(), args[0])
			} else {
				var closeFn func() error
				var svc *app.Service
				svc, closeFn, err = buildServiceFromCmd(cmd, opts)
				if err == nil {
					defer func() { _ = closeFn() }()
					view, err = svc.EventNotificationRuleInfo(args[0])
				}
			}
			if err != nil {
				return err
			}
			return render.JSON(cmd.OutOrStdout(), notificationRuleViewForJSON(view))
		},
	}
}

func newNotificationRuleModifyCommand(opts Options) *cobra.Command {
	var input notificationRuleCLIInput
	cmd := &cobra.Command{
		Use:   "modify <rule-id>",
		Short: "修改事件通知规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			mod := input.toModifyApp(cmd)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				view, err := client.ModifyEventNotificationRule(context.Background(), args[0], notificationRuleModifyInputToRemote(mod))
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), notificationRuleViewForJSON(view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified notification rule %s (%s)\n", view.Name, view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.ModifyEventNotificationRule(args[0], mod)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationRuleViewForJSON(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified notification rule %s (%s)\n", view.Name, view.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&input.name, "name", "", "新的规则名称")
	bindNotificationRuleFlags(cmd, &input)
	return cmd
}

func newNotificationRuleEnableCommand(opts Options) *cobra.Command {
	return notificationRuleToggleCommand(opts, "enable", "启用事件通知规则", true)
}

func newNotificationRuleDisableCommand(opts Options) *cobra.Command {
	return notificationRuleToggleCommand(opts, "disable", "禁用事件通知规则", false)
}

func notificationRuleToggleCommand(opts Options, use string, short string, enabled bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <rule-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var view app.EventNotificationRuleView
			var err error
			if remoteMode, _, modeErr := isRemoteMode(currentOpts); modeErr != nil {
				return modeErr
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if enabled {
					view, err = client.EnableEventNotificationRule(context.Background(), args[0])
				} else {
					view, err = client.DisableEventNotificationRule(context.Background(), args[0])
				}
			} else {
				var closeFn func() error
				var svc *app.Service
				svc, closeFn, err = buildServiceFromCmd(cmd, opts)
				if err == nil {
					defer func() { _ = closeFn() }()
					if enabled {
						view, err = svc.EnableEventNotificationRule(args[0])
					} else {
						view, err = svc.DisableEventNotificationRule(args[0])
					}
				}
			}
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), notificationRuleViewForJSON(view))
			}
			if enabled {
				fmt.Fprintf(cmd.OutOrStdout(), "Enabled notification rule %s\n", args[0])
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Disabled notification rule %s\n", args[0])
			}
			return nil
		},
	}
}

func newNotificationRuleDeleteCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <rule-id>",
		Aliases: []string{"remove"},
		Short:   "删除事件通知规则",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if err := client.DeleteEventNotificationRule(context.Background(), args[0]); err != nil {
					return err
				}
			} else {
				svc, closeFn, err := buildServiceFromCmd(cmd, opts)
				if err != nil {
					return err
				}
				defer closeFn()
				if err := svc.DeleteEventNotificationRule(args[0]); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted notification rule %s\n", args[0])
			return nil
		},
	}
}

func newReminderCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "reminder", Short: "管理提醒规则", Args: cobra.NoArgs}
	cmd.AddCommand(newReminderRuleCommand(opts))
	return cmd
}

func newReminderRuleCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "rule", Short: "管理提醒规则", Args: cobra.NoArgs}
	cmd.AddCommand(newReminderRuleAddCommand(opts))
	cmd.AddCommand(newReminderRuleListCommand(opts))
	cmd.AddCommand(newReminderRuleInfoCommand(opts))
	cmd.AddCommand(newReminderRuleModifyCommand(opts))
	cmd.AddCommand(newReminderRuleEnableCommand(opts))
	cmd.AddCommand(newReminderRuleDisableCommand(opts))
	cmd.AddCommand(newReminderRuleDeleteCommand(opts))
	return cmd
}

func newReminderRuleAddCommand(opts Options) *cobra.Command {
	var input reminderRuleCLIInput
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "创建提醒规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			addInput, err := input.toApp(args[0])
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
				view, err := client.AddReminderRule(context.Background(), currentOpts.Workspace, reminderRuleInputToRemote(addInput))
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created reminder rule %s (%s)\n", view.Name, view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.AddReminderRule(addInput)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created reminder rule %s (%s)\n", view.Name, view.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&input.projectRef, "project", "", "项目 slug")
	cmd.Flags().StringVar(&input.trigger, "trigger", "", "触发类型: due_before 或 overdue")
	cmd.Flags().StringVar(&input.offset, "offset", "", "due_before 提前时间，例如 4h")
	cmd.Flags().StringVar(&input.after, "after", "", "overdue 延迟时间，例如 0h")
	cmd.Flags().StringVar(&input.repeat, "repeat", "", "重复策略: once 或 every:<duration>")
	cmd.Flags().StringVar(&input.schedule, "schedule", "", "规则级调度，例如 daily@08:50")
	cmd.Flags().StringVar(&input.filterSource, "filter", "", "任务过滤表达式，例如 due.before:now+24h")
	cmd.Flags().StringVar(&input.audience, "audience", "", "受众: assignees、explicit_users、assignees_and_explicit_users")
	cmd.Flags().StringArrayVar(&input.recipients, "recipient", nil, "显式 recipient，可重复指定")
	cmd.Flags().StringVar(&input.sinkRef, "sink", "", "通知 sink 名称或 ID")
	return cmd
}

type reminderRuleCLIInput struct {
	projectRef   string
	trigger      string
	offset       string
	after        string
	repeat       string
	schedule     string
	filterSource string
	audience     string
	recipients   []string
	sinkRef      string
	name         string
}

func (input reminderRuleCLIInput) toApp(name string) (app.ReminderRuleAddInput, error) {
	var offsetSeconds int64
	if input.offset != "" {
		d, err := time.ParseDuration(input.offset)
		if err != nil {
			return app.ReminderRuleAddInput{}, err
		}
		offsetSeconds = int64(d.Seconds())
	}
	var afterSeconds int64
	if input.after != "" {
		d, err := time.ParseDuration(input.after)
		if err != nil {
			return app.ReminderRuleAddInput{}, err
		}
		afterSeconds = int64(d.Seconds())
	}
	scheduleType, scheduleValue := parseReminderScheduleFlag(input.schedule)
	return app.ReminderRuleAddInput{
		Name:          name,
		ProjectRef:    input.projectRef,
		TriggerType:   input.trigger,
		OffsetSeconds: offsetSeconds,
		AfterSeconds:  afterSeconds,
		RepeatPolicy:  input.repeat,
		ScheduleType:  scheduleType,
		ScheduleValue: scheduleValue,
		FilterSource:  input.filterSource,
		AudienceType:  input.audience,
		Recipients:    input.recipients,
		SinkRef:       input.sinkRef,
	}, nil
}

func (input reminderRuleCLIInput) toModifyApp(cmd *cobra.Command) (app.ReminderRuleModifyInput, error) {
	var mod app.ReminderRuleModifyInput
	if cmd.Flags().Changed("name") {
		mod.Name = &input.name
	}
	if cmd.Flags().Changed("project") {
		mod.ProjectRef = &input.projectRef
	}
	if cmd.Flags().Changed("trigger") {
		mod.TriggerType = &input.trigger
	}
	if cmd.Flags().Changed("offset") {
		var offsetSeconds int64
		if input.offset != "" {
			d, err := time.ParseDuration(input.offset)
			if err != nil {
				return app.ReminderRuleModifyInput{}, err
			}
			offsetSeconds = int64(d.Seconds())
		}
		mod.OffsetSeconds = &offsetSeconds
	}
	if cmd.Flags().Changed("after") {
		var afterSeconds int64
		if input.after != "" {
			d, err := time.ParseDuration(input.after)
			if err != nil {
				return app.ReminderRuleModifyInput{}, err
			}
			afterSeconds = int64(d.Seconds())
		}
		mod.AfterSeconds = &afterSeconds
	}
	if cmd.Flags().Changed("repeat") {
		mod.RepeatPolicy = &input.repeat
	}
	if cmd.Flags().Changed("schedule") {
		scheduleType, scheduleValue := parseReminderScheduleFlag(input.schedule)
		mod.ScheduleType = &scheduleType
		mod.ScheduleValue = &scheduleValue
	}
	if cmd.Flags().Changed("filter") {
		mod.FilterSource = &input.filterSource
	}
	if cmd.Flags().Changed("audience") {
		mod.AudienceType = &input.audience
	}
	if cmd.Flags().Changed("recipient") {
		mod.Recipients = &input.recipients
	}
	if cmd.Flags().Changed("sink") {
		mod.SinkRef = &input.sinkRef
	}
	return mod, nil
}

func parseReminderScheduleFlag(value string) (string, string) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "daily@") {
		return "daily_at", strings.TrimPrefix(value, "daily@")
	}
	return value, ""
}

func newReminderRuleListCommand(opts Options) *cobra.Command {
	var projectRef string
	var includeDisabled bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出提醒规则",
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
				rows, err := client.ListReminderRules(context.Background(), currentOpts.Workspace, projectRef, includeDisabled)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), reminderRuleViewsForJSON(rows))
				}
				renderReminderRuleList(cmd.OutOrStdout(), rows)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			rows, err := svc.ListReminderRules(projectRef, includeDisabled)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), reminderRuleViewsForJSON(rows))
			}
			renderReminderRuleList(cmd.OutOrStdout(), rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectRef, "project", "", "按项目过滤")
	cmd.Flags().BoolVar(&includeDisabled, "all", false, "包含 disabled rule")
	return cmd
}

func newReminderRuleInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <rule-id>",
		Short: "显示提醒规则详情",
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
				view, err := client.ReminderRuleInfo(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
				}
				return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.ReminderRuleInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
			}
			return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
		},
	}
}

func newReminderRuleModifyCommand(opts Options) *cobra.Command {
	var input reminderRuleCLIInput
	cmd := &cobra.Command{
		Use:   "modify <rule-id>",
		Short: "修改提醒规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			mod, err := input.toModifyApp(cmd)
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
				view, err := client.ModifyReminderRule(context.Background(), args[0], reminderRuleModifyInputToRemote(mod))
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified reminder rule %s (%s)\n", view.Name, view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.ModifyReminderRule(args[0], mod)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified reminder rule %s (%s)\n", view.Name, view.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&input.name, "name", "", "新的规则名称")
	cmd.Flags().StringVar(&input.projectRef, "project", "", "项目 slug，传空字符串可清除项目范围")
	cmd.Flags().StringVar(&input.trigger, "trigger", "", "触发类型: due_before 或 overdue")
	cmd.Flags().StringVar(&input.offset, "offset", "", "due_before 提前时间，例如 4h")
	cmd.Flags().StringVar(&input.after, "after", "", "overdue 延迟时间，例如 0h")
	cmd.Flags().StringVar(&input.repeat, "repeat", "", "重复策略: once 或 every:<duration>")
	cmd.Flags().StringVar(&input.schedule, "schedule", "", "规则级调度，例如 daily@08:50")
	cmd.Flags().StringVar(&input.filterSource, "filter", "", "任务过滤表达式，例如 due.before:now+24h")
	cmd.Flags().StringVar(&input.audience, "audience", "", "受众: assignees、explicit_users、assignees_and_explicit_users")
	cmd.Flags().StringArrayVar(&input.recipients, "recipient", nil, "显式 recipient，可重复指定")
	cmd.Flags().StringVar(&input.sinkRef, "sink", "", "通知 sink 名称或 ID")
	return cmd
}

func newReminderRuleEnableCommand(opts Options) *cobra.Command {
	return reminderRuleToggleCommand(opts, "enable", "启用提醒规则", true)
}

func newReminderRuleDisableCommand(opts Options) *cobra.Command {
	return reminderRuleToggleCommand(opts, "disable", "禁用提醒规则", false)
}

func reminderRuleToggleCommand(opts Options, use string, short string, enabled bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <rule-id>",
		Short: short,
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
				var view app.ReminderRuleView
				if enabled {
					view, err = client.EnableReminderRule(context.Background(), args[0])
				} else {
					view, err = client.DisableReminderRule(context.Background(), args[0])
				}
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
				}
				if enabled {
					fmt.Fprintf(cmd.OutOrStdout(), "Enabled reminder rule %s\n", args[0])
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Disabled reminder rule %s\n", args[0])
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			var view app.ReminderRuleView
			if enabled {
				view, err = svc.EnableReminderRule(args[0])
			} else {
				view, err = svc.DisableReminderRule(args[0])
			}
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), reminderRuleViewForJSON(view))
			}
			if enabled {
				fmt.Fprintf(cmd.OutOrStdout(), "Enabled reminder rule %s\n", args[0])
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Disabled reminder rule %s\n", args[0])
			}
			return nil
		},
	}
}

func newReminderRuleDeleteCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <rule-id>",
		Short: "删除提醒规则",
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
				if err := client.DeleteReminderRule(context.Background(), args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deleted reminder rule %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.DeleteReminderRule(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted reminder rule %s\n", args[0])
			return nil
		},
	}
}

func renderNotificationSinkList(w interface{ Write([]byte) (int, error) }, rows []app.NotificationSinkView) {
	for _, row := range rows {
		enabled := "disabled"
		if row.Enabled {
			enabled = "enabled"
		}
		fmt.Fprintf(w, "%s  %s  %s  %s  %s\n", shortID(row.ID), row.Name, row.Type, enabled, notificationSinkEndpoint(row))
	}
}

func renderNotificationSinkInfo(w interface{ Write([]byte) (int, error) }, row app.NotificationSinkView) {
	_ = render.JSON(w, notificationSinkViewForJSON(row))
}

func renderReminderRuleList(w interface{ Write([]byte) (int, error) }, rows []app.ReminderRuleView) {
	for _, row := range rows {
		enabled := "disabled"
		if row.Enabled {
			enabled = "enabled"
		}
		trigger := row.TriggerType
		if row.ScheduleType == "daily_at" && row.ScheduleValue != "" {
			trigger = "daily@" + row.ScheduleValue
		}
		fmt.Fprintf(w, "%s  %s  %s  %s  %s\n", shortID(row.ID), row.Name, trigger, row.AudienceType, enabled)
	}
}

func renderNotificationRuleList(w interface{ Write([]byte) (int, error) }, rows []app.EventNotificationRuleView) {
	for _, row := range rows {
		enabled := "disabled"
		if row.Enabled {
			enabled = "enabled"
		}
		fmt.Fprintf(w, "%s  %s  %s  %s  %s\n", shortID(row.ID), row.Name, row.EventType, row.AudienceType, enabled)
	}
}

func renderNotificationDeliveryList(w interface{ Write([]byte) (int, error) }, rows []app.NotificationDeliveryView) {
	for _, row := range rows {
		fmt.Fprintf(w, "%s  %s  %s  %d  %s\n", shortID(row.ID), row.EventType, row.Status, row.AttemptCount, row.LastError)
	}
}

func notificationSinkEndpoint(row app.NotificationSinkView) string {
	switch row.EndpointMode {
	case app.NotificationEndpointTemplate:
		return row.URLTemplate
	case app.NotificationEndpointConfigValue:
		return "config:" + row.ConfigKey
	default:
		return row.URL
	}
}

func notificationSinkViewsForJSON(rows []app.NotificationSinkView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationSinkViewForJSON(row))
	}
	return out
}

func notificationSinkViewForJSON(row app.NotificationSinkView) map[string]any {
	return map[string]any{
		"id":                row.ID,
		"workspace_id":      row.WorkspaceID,
		"name":              row.Name,
		"type":              row.Type,
		"endpoint_mode":     row.EndpointMode,
		"url":               row.URL,
		"url_template":      row.URLTemplate,
		"config_key":        row.ConfigKey,
		"allowed_hosts":     row.AllowedHosts,
		"http_method":       row.HTTPMethod,
		"header_templates":  row.HeaderTemplates,
		"body_template":     row.BodyTemplate,
		"body_content_type": row.BodyContentType,
		"secret_refs":       row.SecretRefs,
		"enabled":           row.Enabled,
		"timeout_seconds":   row.TimeoutSeconds,
		"max_attempts":      row.MaxAttempts,
		"max_concurrency":   row.MaxConcurrency,
		"created_by":        task.ActorInfoToJSON(row.CreatedBy),
		"created_at":        row.CreatedAt,
		"modified_at":       row.ModifiedAt,
	}
}

func notificationSinkInputToRemote(input app.NotificationSinkAddInput) remote.NotificationSinkRequest {
	return remote.NotificationSinkRequest{
		Name:            input.Name,
		Type:            input.Type,
		EndpointMode:    input.EndpointMode,
		URL:             input.URL,
		URLTemplate:     input.URLTemplate,
		ConfigKey:       input.ConfigKey,
		AllowedHosts:    input.AllowedHosts,
		HeaderTemplates: input.HeaderTemplates,
		BodyTemplate:    input.BodyTemplate,
		BodyContentType: input.BodyContentType,
		SecretRefs:      input.SecretRefs,
		Secret:          input.Secret,
		TimeoutSeconds:  input.TimeoutSeconds,
		MaxAttempts:     input.MaxAttempts,
		MaxConcurrency:  input.MaxConcurrency,
	}
}

func notificationSinkModifyInputToRemote(input app.NotificationSinkModifyInput) remote.NotificationSinkModifyRequest {
	return remote.NotificationSinkModifyRequest{
		Name:            input.Name,
		Type:            input.Type,
		EndpointMode:    input.EndpointMode,
		URL:             input.URL,
		URLTemplate:     input.URLTemplate,
		ConfigKey:       input.ConfigKey,
		AllowedHosts:    input.AllowedHosts,
		HeaderTemplates: input.HeaderTemplates,
		BodyTemplate:    input.BodyTemplate,
		BodyContentType: input.BodyContentType,
		SecretRefs:      input.SecretRefs,
		Secret:          input.Secret,
		TimeoutSeconds:  input.TimeoutSeconds,
		MaxAttempts:     input.MaxAttempts,
		MaxConcurrency:  input.MaxConcurrency,
	}
}

func reminderRuleInputToRemote(input app.ReminderRuleAddInput) remote.ReminderRuleRequest {
	return remote.ReminderRuleRequest{
		Name:          input.Name,
		ProjectRef:    input.ProjectRef,
		TriggerType:   input.TriggerType,
		OffsetSeconds: input.OffsetSeconds,
		AfterSeconds:  input.AfterSeconds,
		RepeatPolicy:  input.RepeatPolicy,
		ScheduleType:  input.ScheduleType,
		ScheduleValue: input.ScheduleValue,
		FilterSource:  input.FilterSource,
		AudienceType:  input.AudienceType,
		Recipients:    input.Recipients,
		SinkRef:       input.SinkRef,
	}
}

func reminderRuleModifyInputToRemote(input app.ReminderRuleModifyInput) remote.ReminderRuleModifyRequest {
	return remote.ReminderRuleModifyRequest{
		Name:          input.Name,
		ProjectRef:    input.ProjectRef,
		TriggerType:   input.TriggerType,
		OffsetSeconds: input.OffsetSeconds,
		AfterSeconds:  input.AfterSeconds,
		RepeatPolicy:  input.RepeatPolicy,
		ScheduleType:  input.ScheduleType,
		ScheduleValue: input.ScheduleValue,
		FilterSource:  input.FilterSource,
		AudienceType:  input.AudienceType,
		Recipients:    input.Recipients,
		SinkRef:       input.SinkRef,
	}
}

func notificationRuleInputToRemote(input app.EventNotificationRuleAddInput) remote.EventNotificationRuleRequest {
	return remote.EventNotificationRuleRequest{
		Name:            input.Name,
		ProjectRef:      input.ProjectRef,
		EventType:       input.EventType,
		FilterSource:    input.FilterSource,
		AudienceType:    input.AudienceType,
		Recipients:      input.Recipients,
		Sink:            input.SinkRef,
		TemplateSubject: input.TemplateSubject,
		TemplateBody:    input.TemplateBody,
	}
}

func notificationRuleModifyInputToRemote(input app.EventNotificationRuleModifyInput) remote.EventNotificationRuleModifyRequest {
	return remote.EventNotificationRuleModifyRequest{
		Name:            input.Name,
		ProjectRef:      input.ProjectRef,
		EventType:       input.EventType,
		FilterSource:    input.FilterSource,
		AudienceType:    input.AudienceType,
		Recipients:      input.Recipients,
		Sink:            input.SinkRef,
		TemplateSubject: input.TemplateSubject,
		TemplateBody:    input.TemplateBody,
	}
}

func reminderRuleViewsForJSON(rows []app.ReminderRuleView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, reminderRuleViewForJSON(row))
	}
	return out
}

func reminderRuleViewForJSON(row app.ReminderRuleView) map[string]any {
	return map[string]any{
		"id":              row.ID,
		"workspace_id":    row.WorkspaceID,
		"project_id":      row.ProjectID,
		"name":            row.Name,
		"enabled":         row.Enabled,
		"trigger_type":    row.TriggerType,
		"offset_seconds":  row.OffsetSeconds,
		"after_seconds":   row.AfterSeconds,
		"repeat_policy":   row.RepeatPolicy,
		"schedule_type":   row.ScheduleType,
		"schedule_value":  row.ScheduleValue,
		"filter_source":   row.FilterSource,
		"audience_type":   row.AudienceType,
		"recipient_users": notificationUserInfosForJSON(row.RecipientUsers),
		"sink_id":         row.SinkID,
		"created_by":      task.ActorInfoToJSON(row.CreatedBy),
		"created_at":      row.CreatedAt,
		"modified_at":     row.ModifiedAt,
	}
}

func notificationRuleViewsForJSON(rows []app.EventNotificationRuleView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationRuleViewForJSON(row))
	}
	return out
}

func notificationRuleViewForJSON(row app.EventNotificationRuleView) map[string]any {
	return map[string]any{
		"id":               row.ID,
		"workspace_id":     row.WorkspaceID,
		"project_id":       row.ProjectID,
		"name":             row.Name,
		"enabled":          row.Enabled,
		"event_type":       row.EventType,
		"filter_source":    row.FilterSource,
		"audience_type":    row.AudienceType,
		"recipient_users":  notificationUserInfosForJSON(row.RecipientUsers),
		"sink_id":          row.SinkID,
		"template_subject": row.TemplateSubject,
		"template_body":    row.TemplateBody,
		"created_by":       task.ActorInfoToJSON(row.CreatedBy),
		"created_at":       row.CreatedAt,
		"modified_at":      row.ModifiedAt,
	}
}

func notificationDeliveryViewsForJSON(rows []app.NotificationDeliveryView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationDeliveryViewForJSON(row))
	}
	return out
}

func notificationDeliveryViewForJSON(row app.NotificationDeliveryView) map[string]any {
	return map[string]any{
		"id":                            row.ID,
		"workspace_id":                  row.WorkspaceID,
		"project_id":                    row.ProjectID,
		"rule_id":                       row.RuleID,
		"sink_id":                       row.SinkID,
		"task_uuid":                     row.TaskUUID,
		"object_kind":                   row.ObjectKind,
		"object_id":                     row.ObjectID,
		"recipient":                     task.UserInfoToJSON(row.Recipient),
		"event_id":                      row.EventID,
		"event_type":                    row.EventType,
		"resolved_url":                  row.ResolvedURL,
		"resolved_endpoint_source":      row.ResolvedEndpointSource,
		"resolved_endpoint_fingerprint": row.ResolvedEndpointFingerprint,
		"rendered_method":               row.RenderedMethod,
		"rendered_headers":              row.RenderedHeaders,
		"rendered_body":                 row.RenderedBody,
		"rendered_content_type":         row.RenderedContentType,
		"payload":                       row.Payload,
		"status":                        row.Status,
		"attempt_count":                 row.AttemptCount,
		"next_attempt_at":               row.NextAttemptAt,
		"claim_expires_at":              row.ClaimExpiresAt,
		"last_attempt_at":               row.LastAttemptAt,
		"last_status_code":              row.LastStatusCode,
		"last_error":                    row.LastError,
		"created_at":                    row.CreatedAt,
		"modified_at":                   row.ModifiedAt,
	}
}

func notificationUserInfosForJSON(rows []task.UserInfo) []task.JSONUserInfo {
	out := make([]task.JSONUserInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, task.UserInfoToJSON(row))
	}
	return out
}

var _ = context.Background
