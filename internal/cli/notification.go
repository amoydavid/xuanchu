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
	"github.com/spf13/cobra"
)

func newNotificationCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notification",
		Short: "管理定时通知",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newNotificationSinkCommand(opts))
	cmd.AddCommand(newNotificationDeliveryCommand(opts))
	return cmd
}

func newNotificationSinkCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sink",
		Short: "管理通知投递目标",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newNotificationSinkAddCommand(opts))
	cmd.AddCommand(newNotificationSinkListCommand(opts))
	cmd.AddCommand(newNotificationSinkInfoCommand(opts))
	cmd.AddCommand(newNotificationSinkModifyCommand(opts))
	cmd.AddCommand(newNotificationSinkEnableCommand(opts))
	cmd.AddCommand(newNotificationSinkDisableCommand(opts))
	cmd.AddCommand(newNotificationSinkDeleteCommand(opts))
	return cmd
}

func newNotificationDeliveryCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delivery",
		Short: "查看通知投递",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newNotificationDeliveryListCommand(opts))
	cmd.AddCommand(newNotificationDeliveryInfoCommand(opts))
	cmd.AddCommand(newNotificationDeliveryReplayCommand(opts))
	return cmd
}

func newReminderCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reminder",
		Short: "管理提醒规则",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newReminderRuleCommand(opts))
	return cmd
}

func newReminderRuleCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rule",
		Short: "管理提醒规则",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newReminderRuleAddCommand(opts))
	cmd.AddCommand(newReminderRuleListCommand(opts))
	cmd.AddCommand(newReminderRuleInfoCommand(opts))
	cmd.AddCommand(newReminderRuleModifyCommand(opts))
	cmd.AddCommand(newReminderRuleEnableCommand(opts))
	cmd.AddCommand(newReminderRuleDisableCommand(opts))
	cmd.AddCommand(newReminderRuleDeleteCommand(opts))
	return cmd
}

func newNotificationSinkAddCommand(opts Options) *cobra.Command {
	var (
		typ              string
		endpointMode     string
		url              string
		urlTemplate      string
		configKey        string
		allowedHosts     []string
		method           string
		headerTemplates  []string
		bodyTemplate     string
		bodyTemplateFile string
		bodyContentType  string
		secretRefs       []string
		secret           string
		secretStdin      bool
		timeout          int
		maxAttempts      int
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "创建通知投递目标",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretVal, err := resolveSecret(secret, secretStdin, "", cmd.InOrStdin())
			if err != nil {
				return err
			}
			template, err := resolveBodyTemplate(bodyTemplate, bodyTemplateFile)
			if err != nil {
				return err
			}
			headerRows, err := parseTemplatePairs(headerTemplates, "header-template")
			if err != nil {
				return err
			}
			secretRowValues, err := parseSecretRefPairs(secretRefs)
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
				view, err := client.AddNotificationSink(context.Background(), currentOpts.Workspace, remote.NotificationSinkCreateRequest{
					Name:            args[0],
					Type:            typ,
					EndpointMode:    endpointMode,
					URL:             url,
					URLTemplate:     urlTemplate,
					ConfigKey:       configKey,
					AllowedHosts:    allowedHosts,
					HTTPMethod:      method,
					HeaderTemplates: headerRows,
					BodyTemplate:    template,
					BodyContentType: bodyContentType,
					SecretRefs:      secretRowValues,
					Secret:          secretVal,
					TimeoutSeconds:  timeout,
					MaxAttempts:     maxAttempts,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), view)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created notification sink %s (%s)\n", view.Name, view.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.AddNotificationSink(app.NotificationSinkAddInput{
				Name:            args[0],
				Type:            typ,
				EndpointMode:    endpointMode,
				URL:             url,
				URLTemplate:     urlTemplate,
				ConfigKey:       configKey,
				AllowedHosts:    allowedHosts,
				HTTPMethod:      method,
				HeaderTemplates: headerRowsToApp(headerRows),
				BodyTemplate:    template,
				BodyContentType: bodyContentType,
				SecretRefs:      secretRowsToApp(secretRowValues),
				Secret:          secretVal,
				TimeoutSeconds:  timeout,
				MaxAttempts:     maxAttempts,
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), view)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created notification sink %s (%s)\n", view.Name, view.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "webhook", "sink 类型: webhook 或 http_template")
	cmd.Flags().StringVar(&endpointMode, "endpoint-mode", "static_url", "endpoint 解析模式")
	cmd.Flags().StringVar(&url, "url", "", "固定 URL")
	cmd.Flags().StringVar(&urlTemplate, "url-template", "", "URL 模板")
	cmd.Flags().StringVar(&configKey, "config-key", "", "配置键")
	cmd.Flags().StringArrayVar(&allowedHosts, "allowed-host", nil, "允许的 host（可重复）")
	cmd.Flags().StringVar(&method, "method", "", "HTTP 方法")
	cmd.Flags().StringArrayVar(&headerTemplates, "header-template", nil, "header 模板，格式 Name=template")
	cmd.Flags().StringVar(&bodyTemplate, "body-template", "", "body 模板")
	cmd.Flags().StringVar(&bodyTemplateFile, "body-template-file", "", "从文件读取 body 模板")
	cmd.Flags().StringVar(&bodyContentType, "body-content-type", "", "body content-type")
	cmd.Flags().StringArrayVar(&secretRefs, "secret-ref", nil, "secret 引用，格式 alias=config.key")
	cmd.Flags().StringVar(&secret, "secret", "", "webhook secret")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "从 stdin 读取 webhook secret")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "超时秒数")
	cmd.Flags().IntVar(&maxAttempts, "max-attempts", 0, "最大重试次数")
	return cmd
}

func newNotificationSinkListCommand(opts Options) *cobra.Command {
	var includeDisabled bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出通知投递目标",
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
					return render.JSON(cmd.OutOrStdout(), rows)
				}
				for _, row := range rows {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", row.ID, row.Name, row.EndpointMode)
				}
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
				return render.JSON(cmd.OutOrStdout(), rows)
			}
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", row.ID, row.Name, row.EndpointMode)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeDisabled, "all", false, "包含禁用项")
	return cmd
}

func newNotificationSinkInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <sink>",
		Short: "显示通知投递目标详情",
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
				row, err := client.NotificationSinkInfo(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), row)
				}
				return render.JSON(cmd.OutOrStdout(), row)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			row, err := svc.NotificationSinkInfo(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), row)
			}
			return render.JSON(cmd.OutOrStdout(), row)
		},
	}
}

func newNotificationSinkModifyCommand(opts Options) *cobra.Command {
	var (
		typ              string
		endpointMode     string
		url              string
		urlTemplate      string
		configKey        string
		allowedHosts     []string
		method           string
		headerTemplates  []string
		bodyTemplate     string
		bodyTemplateFile string
		bodyContentType  string
		secretRefs       []string
		secret           string
		secretStdin      bool
		timeout          int
		maxAttempts      int
		enabled          bool
	)
	cmd := &cobra.Command{
		Use:   "modify <sink>",
		Short: "修改通知投递目标",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretVal, err := resolveSecret(secret, secretStdin, "", cmd.InOrStdin())
			if err != nil {
				return err
			}
			template, err := resolveBodyTemplate(bodyTemplate, bodyTemplateFile)
			if err != nil {
				return err
			}
			headerRows, err := parseTemplatePairs(headerTemplates, "header-template")
			if err != nil {
				return err
			}
			secretRowValues, err := parseSecretRefPairs(secretRefs)
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
				var enabledPtr *bool
				if cmd.Flags().Changed("enabled") {
					enabledPtr = &enabled
				}
				view, err := client.ModifyNotificationSink(context.Background(), args[0], remote.NotificationSinkModifyRequest{
					Type:            stringPtrIfSet(typ),
					EndpointMode:    stringPtrIfSet(endpointMode),
					URL:             stringPtrIfSet(url),
					URLTemplate:     stringPtrIfSet(urlTemplate),
					ConfigKey:       stringPtrIfSet(configKey),
					AllowedHosts:    slicePtrIfSet(allowedHosts),
					HTTPMethod:      stringPtrIfSet(method),
					HeaderTemplates: headerTemplatesPtrIfSet(headerRows),
					BodyTemplate:    stringPtrIfSet(template),
					BodyContentType: stringPtrIfSet(bodyContentType),
					SecretRefs:      secretRefsPtrIfSet(secretRowValues),
					Secret:          stringPtrIfSet(secretVal),
					TimeoutSeconds:  intPtrIfSet(timeout),
					MaxAttempts:     intPtrIfSet(maxAttempts),
					Enabled:         enabledPtr,
					Name:            nil,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), view)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified notification sink %s\n", view.Name)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			var enabledPtr *bool
			if cmd.Flags().Changed("enabled") {
				enabledPtr = &enabled
			}
			view, err := svc.ModifyNotificationSink(args[0], app.NotificationSinkModifyInput{
				Type:            stringPtrIfSet(typ),
				EndpointMode:    stringPtrIfSet(endpointMode),
				URL:             stringPtrIfSet(url),
				URLTemplate:     stringPtrIfSet(urlTemplate),
				ConfigKey:       stringPtrIfSet(configKey),
				AllowedHosts:    slicePtrIfSet(allowedHosts),
				HTTPMethod:      stringPtrIfSet(method),
				HeaderTemplates: headerTemplatesPtrToAppIfSet(headerRows),
				BodyTemplate:    stringPtrIfSet(template),
				BodyContentType: stringPtrIfSet(bodyContentType),
				SecretRefs:      secretRefsPtrToAppIfSet(secretRowValues),
				Secret:          stringPtrIfSet(secretVal),
				TimeoutSeconds:  intPtrIfSet(timeout),
				MaxAttempts:     intPtrIfSet(maxAttempts),
				Enabled:         enabledPtr,
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), view)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified notification sink %s\n", view.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "sink 类型")
	cmd.Flags().StringVar(&endpointMode, "endpoint-mode", "", "endpoint 模式")
	cmd.Flags().StringVar(&url, "url", "", "固定 URL")
	cmd.Flags().StringVar(&urlTemplate, "url-template", "", "URL 模板")
	cmd.Flags().StringVar(&configKey, "config-key", "", "配置键")
	cmd.Flags().StringArrayVar(&allowedHosts, "allowed-host", nil, "允许的 host")
	cmd.Flags().StringVar(&method, "method", "", "HTTP 方法")
	cmd.Flags().StringArrayVar(&headerTemplates, "header-template", nil, "header 模板")
	cmd.Flags().StringVar(&bodyTemplate, "body-template", "", "body 模板")
	cmd.Flags().StringVar(&bodyTemplateFile, "body-template-file", "", "body 模板文件")
	cmd.Flags().StringVar(&bodyContentType, "body-content-type", "", "body content-type")
	cmd.Flags().StringArrayVar(&secretRefs, "secret-ref", nil, "secret 引用")
	cmd.Flags().StringVar(&secret, "secret", "", "webhook secret")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "从 stdin 读取 webhook secret")
	cmd.Flags().IntVar(&timeout, "timeout", 0, "超时秒数")
	cmd.Flags().IntVar(&maxAttempts, "max-attempts", 0, "最大重试次数")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "启用状态")
	return cmd
}

func newNotificationSinkEnableCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <sink>",
		Short: "启用通知投递目标",
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
				view, err := client.EnableNotificationSink(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), view)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Enabled notification sink %s\n", view.Name)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.EnableNotificationSink(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), view)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Enabled notification sink %s\n", view.Name)
			return nil
		},
	}
}

func newNotificationSinkDisableCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <sink>",
		Short: "禁用通知投递目标",
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
				view, err := client.DisableNotificationSink(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), view)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Disabled notification sink %s\n", view.Name)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.DisableNotificationSink(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), view)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Disabled notification sink %s\n", view.Name)
			return nil
		},
	}
}

func newNotificationSinkDeleteCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <sink>",
		Short: "删除通知投递目标",
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

func newNotificationDeliveryListCommand(opts Options) *cobra.Command {
	var status string
	var limit int
	var offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出通知投递",
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
				rows, err := client.ListNotificationDeliveries(context.Background(), currentOpts.Workspace, status, limit, offset)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), rows)
				}
				for _, row := range rows {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", row.ID, row.EventType, row.Status)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			rows, err := svc.ListNotificationDeliveries(status, limit, offset)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), rows)
			}
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", row.ID, row.EventType, row.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "投递状态")
	cmd.Flags().IntVar(&limit, "limit", 50, "返回数量上限")
	cmd.Flags().IntVar(&offset, "offset", 0, "偏移量")
	return cmd
}

func newNotificationDeliveryInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <delivery>",
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
				row, err := client.NotificationDeliveryInfo(context.Background(), args[0])
				if err != nil {
					return err
				}
				return render.JSON(cmd.OutOrStdout(), row)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			row, err := svc.NotificationDeliveryInfo(args[0])
			if err != nil {
				return err
			}
			return render.JSON(cmd.OutOrStdout(), row)
		},
	}
}

func newNotificationDeliveryReplayCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "replay <delivery>",
		Short: "重试通知投递",
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
				row, err := client.ReplayNotificationDelivery(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), row)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Replayed notification delivery %s\n", row.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			row, err := svc.ReplayNotificationDelivery(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), row)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Replayed notification delivery %s\n", row.ID)
			return nil
		},
	}
}

func newReminderRuleAddCommand(opts Options) *cobra.Command {
	var (
		projectRef string
		trigger    string
		offset     string
		after      string
		repeat     string
		taskFilter string
		audience   string
		recipients []string
		sinkRef    string
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "创建提醒规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			offsetSeconds, err := parseDurationSeconds(offset)
			if err != nil {
				return err
			}
			afterSeconds, err := parseDurationSeconds(after)
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
				row, err := client.AddReminderRule(context.Background(), currentOpts.Workspace, remote.ReminderRuleCreateRequest{
					Name:          args[0],
					ProjectRef:    projectRef,
					TriggerType:   trigger,
					OffsetSeconds: offsetSeconds,
					AfterSeconds:  afterSeconds,
					RepeatPolicy:  repeat,
					TaskFilter:    taskFilter,
					AudienceType:  audience,
					Recipients:    recipients,
					SinkRef:       sinkRef,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), row)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created reminder rule %s (%s)\n", row.Name, row.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			row, err := svc.AddReminderRule(app.ReminderRuleAddInput{
				Name:          args[0],
				ProjectRef:    projectRef,
				TriggerType:   trigger,
				OffsetSeconds: offsetSeconds,
				AfterSeconds:  afterSeconds,
				RepeatPolicy:  repeat,
				TaskFilter:    taskFilter,
				AudienceType:  audience,
				Recipients:    recipients,
				SinkRef:       sinkRef,
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), row)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created reminder rule %s (%s)\n", row.Name, row.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectRef, "project", "", "项目 slug")
	cmd.Flags().StringVar(&trigger, "trigger", "due_before", "触发类型")
	cmd.Flags().StringVar(&offset, "offset", "0s", "提前时间")
	cmd.Flags().StringVar(&after, "after", "0s", "逾期后时间")
	cmd.Flags().StringVar(&repeat, "repeat", "once", "重复策略")
	cmd.Flags().StringVar(&taskFilter, "task-filter", "", "任务筛选条件，使用 task query 语法")
	cmd.Flags().StringVar(&audience, "audience", "assignees", "受众类型")
	cmd.Flags().StringArrayVar(&recipients, "recipient", nil, "显式收件人")
	cmd.Flags().StringVar(&sinkRef, "sink", "", "通知投递目标")
	_ = cmd.MarkFlagRequired("sink")
	return cmd
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
					return render.JSON(cmd.OutOrStdout(), rows)
				}
				for _, row := range rows {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", row.ID, row.Name, row.TriggerType)
				}
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
				return render.JSON(cmd.OutOrStdout(), rows)
			}
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", row.ID, row.Name, row.TriggerType)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectRef, "project", "", "项目 slug")
	cmd.Flags().BoolVar(&includeDisabled, "all", false, "包含禁用项")
	return cmd
}

func newReminderRuleInfoCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <rule>",
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
				row, err := client.ReminderRuleInfo(context.Background(), args[0])
				if err != nil {
					return err
				}
				return render.JSON(cmd.OutOrStdout(), row)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			row, err := svc.ReminderRuleInfo(args[0])
			if err != nil {
				return err
			}
			return render.JSON(cmd.OutOrStdout(), row)
		},
	}
}

func newReminderRuleModifyCommand(opts Options) *cobra.Command {
	var (
		projectRef string
		trigger    string
		offset     string
		after      string
		repeat     string
		taskFilter string
		audience   string
		recipients []string
		sinkRef    string
		enabled    bool
	)
	cmd := &cobra.Command{
		Use:   "modify <rule>",
		Short: "修改提醒规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			offsetSeconds, err := parseDurationSeconds(offset)
			if err != nil {
				return err
			}
			afterSeconds, err := parseDurationSeconds(after)
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
				var enabledPtr *bool
				if cmd.Flags().Changed("enabled") {
					enabledPtr = &enabled
				}
				row, err := client.ModifyReminderRule(context.Background(), args[0], remote.ReminderRuleModifyRequest{
					ProjectRef:    stringPtrIfSet(projectRef),
					TriggerType:   stringPtrIfSet(trigger),
					OffsetSeconds: int64PtrIfSet(offsetSeconds),
					AfterSeconds:  int64PtrIfSet(afterSeconds),
					RepeatPolicy:  stringPtrIfSet(repeat),
					TaskFilter:    stringPtrIfSet(taskFilter),
					AudienceType:  stringPtrIfSet(audience),
					Recipients:    slicePtrIfSet(recipients),
					SinkRef:       stringPtrIfSet(sinkRef),
					Enabled:       enabledPtr,
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), row)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified reminder rule %s\n", row.Name)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			var enabledPtr *bool
			if cmd.Flags().Changed("enabled") {
				enabledPtr = &enabled
			}
			row, err := svc.ModifyReminderRule(args[0], app.ReminderRuleModifyInput{
				ProjectRef:    stringPtrIfSet(projectRef),
				TriggerType:   stringPtrIfSet(trigger),
				OffsetSeconds: int64PtrIfSet(offsetSeconds),
				AfterSeconds:  int64PtrIfSet(afterSeconds),
				RepeatPolicy:  stringPtrIfSet(repeat),
				TaskFilter:    stringPtrIfSet(taskFilter),
				AudienceType:  stringPtrIfSet(audience),
				Recipients:    slicePtrIfSet(recipients),
				SinkRef:       stringPtrIfSet(sinkRef),
				Enabled:       enabledPtr,
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), row)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified reminder rule %s\n", row.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectRef, "project", "", "项目 slug")
	cmd.Flags().StringVar(&trigger, "trigger", "", "触发类型")
	cmd.Flags().StringVar(&offset, "offset", "0s", "提前时间")
	cmd.Flags().StringVar(&after, "after", "0s", "逾期后时间")
	cmd.Flags().StringVar(&repeat, "repeat", "", "重复策略")
	cmd.Flags().StringVar(&taskFilter, "task-filter", "", "任务筛选条件，使用 task query 语法")
	cmd.Flags().StringVar(&audience, "audience", "", "受众类型")
	cmd.Flags().StringArrayVar(&recipients, "recipient", nil, "显式收件人")
	cmd.Flags().StringVar(&sinkRef, "sink", "", "通知投递目标")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "启用状态")
	return cmd
}

func newReminderRuleEnableCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <rule>",
		Short: "启用提醒规则",
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
				row, err := client.EnableReminderRule(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), row)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Enabled reminder rule %s\n", row.Name)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			row, err := svc.EnableReminderRule(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), row)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Enabled reminder rule %s\n", row.Name)
			return nil
		},
	}
}

func newReminderRuleDisableCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <rule>",
		Short: "禁用提醒规则",
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
				row, err := client.DisableReminderRule(context.Background(), args[0])
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), row)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Disabled reminder rule %s\n", row.Name)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			row, err := svc.DisableReminderRule(args[0])
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), row)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Disabled reminder rule %s\n", row.Name)
			return nil
		},
	}
}

func newReminderRuleDeleteCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <rule>",
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

func resolveBodyTemplate(bodyTemplate, filePath string) (string, error) {
	if bodyTemplate != "" && filePath != "" {
		return "", fmt.Errorf("--body-template 和 --body-template-file 互斥，只能指定一个")
	}
	if filePath == "" {
		return bodyTemplate, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取 body template 文件 %s 失败: %w", filePath, err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func parseTemplatePairs(values []string, flagName string) ([]remote.HTTPHeaderTemplateRequest, error) {
	out := make([]remote.HTTPHeaderTemplateRequest, 0, len(values))
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("--%s 需要 Name=template 格式", flagName)
		}
		out = append(out, remote.HTTPHeaderTemplateRequest{Name: strings.TrimSpace(key), Value: value})
	}
	return out, nil
}

func parseSecretRefPairs(values []string) ([]remote.HTTPTemplateSecretRefRequest, error) {
	out := make([]remote.HTTPTemplateSecretRefRequest, 0, len(values))
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("--secret-ref 需要 alias=config.key 格式")
		}
		out = append(out, remote.HTTPTemplateSecretRefRequest{Alias: strings.TrimSpace(key), ConfigKey: strings.TrimSpace(value)})
	}
	return out, nil
}

func secretRowsToApp(rows []remote.HTTPTemplateSecretRefRequest) []app.HTTPTemplateSecretRefInput {
	out := make([]app.HTTPTemplateSecretRefInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPTemplateSecretRefInput{Alias: row.Alias, ConfigKey: row.ConfigKey})
	}
	return out
}

func headerRowsToApp(rows []remote.HTTPHeaderTemplateRequest) []app.HTTPHeaderTemplateInput {
	out := make([]app.HTTPHeaderTemplateInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPHeaderTemplateInput{Name: row.Name, Value: row.Value})
	}
	return out
}

func headerTemplatesPtrIfSet(rows []remote.HTTPHeaderTemplateRequest) *[]remote.HTTPHeaderTemplateRequest {
	if len(rows) == 0 {
		return nil
	}
	return &rows
}

func secretRefsPtrIfSet(rows []remote.HTTPTemplateSecretRefRequest) *[]remote.HTTPTemplateSecretRefRequest {
	if len(rows) == 0 {
		return nil
	}
	return &rows
}

func headerTemplatesPtrToAppIfSet(rows []remote.HTTPHeaderTemplateRequest) *[]app.HTTPHeaderTemplateInput {
	if len(rows) == 0 {
		return nil
	}
	out := make([]app.HTTPHeaderTemplateInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPHeaderTemplateInput{Name: row.Name, Value: row.Value})
	}
	return &out
}

func secretRefsPtrToAppIfSet(rows []remote.HTTPTemplateSecretRefRequest) *[]app.HTTPTemplateSecretRefInput {
	if len(rows) == 0 {
		return nil
	}
	out := make([]app.HTTPTemplateSecretRefInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, app.HTTPTemplateSecretRefInput{Alias: row.Alias, ConfigKey: row.ConfigKey})
	}
	return &out
}

func slicePtrIfSet(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func stringPtrIfSet(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func intPtrIfSet(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func int64PtrIfSet(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func parseDurationSeconds(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	return int64(dur.Seconds()), nil
}
