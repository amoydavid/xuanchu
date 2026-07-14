package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth/directory"
	xuanchuOIDC "git.dajee.net/dajee/xuanchu/internal/auth/oidc"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/hookruntime"
	"git.dajee.net/dajee/xuanchu/internal/httpapi"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/notificationruntime"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/spf13/cobra"
)

func newServerCommand(opts Options) *cobra.Command {
	var listen string
	var shutdownTimeout time.Duration
	var shutdownForceTimeout time.Duration
	var reminderSchedulerInterval time.Duration
	var notificationDispatcherInterval time.Duration
	var hookDispatcherInterval time.Duration
	var automationSchedulerInterval time.Duration
	var automationDispatcherInterval time.Duration
	var taskSeriesSchedulerInterval time.Duration
	var notificationMaxConcurrency int
	var hookMaxConcurrency int
	var notificationBatchSize int
	var hookBatchSize int
	var notificationPrefetchFactor int
	var hookPrefetchFactor int
	var notificationClaimTTL time.Duration
	var hookClaimTTL time.Duration
	var mcpTrustedProxyHosts []string
	var consoleEnabled bool
	var consoleBasePath string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "启动 HTTP API 服务器",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if listen == "" {
				return app.RuntimeError{Code: "server_listen_required", Message: "server listen address is required"}
			}
			currentOpts := optionsFromCmd(cmd, opts)
			env := RuntimeEnv()
			cfg, err := config.Resolve(config.Options{
				DataDir:    currentOpts.DataDir,
				DBPath:     currentOpts.DBPath,
				DBURL:      currentOpts.DBURL,
				JSON:       currentOpts.JSON,
				NoColor:    currentOpts.NoColor,
				Env:        env,
				ConfigPath: currentOpts.Config,
			})
			if err != nil {
				return err
			}
			shutdownFlags := serverShutdownFlagOverrides{}
			if cmd.Flags().Changed("shutdown-timeout") {
				shutdownFlags.Timeout = &shutdownTimeout
			}
			if cmd.Flags().Changed("shutdown-force-timeout") {
				shutdownFlags.ForceTimeout = &shutdownForceTimeout
			}
			shutdownOptions, err := buildServerShutdownOptions(cfg, shutdownFlags)
			if err != nil {
				return err
			}
			shutdownTimeout = shutdownOptions.Timeout
			mcpFlagHosts := []string(nil)
			if cmd.Flags().Changed("mcp-trusted-proxy-host") {
				mcpFlagHosts = mcpTrustedProxyHosts
			}
			mcpOptions, err := buildServerMCPOptions(cfg, mcpFlagHosts)
			if err != nil {
				return err
			}
			consoleFlags := serverConsoleFlagOverrides{}
			if cmd.Flags().Changed("console") {
				consoleFlags.Enabled = &consoleEnabled
			}
			if cmd.Flags().Changed("console-base-path") {
				consoleFlags.BasePath = &consoleBasePath
			}
			consoleOptions, err := buildServerConsoleOptions(cfg, consoleFlags)
			if err != nil {
				return err
			}
			shutdown := runtimeutil.NewShutdownCoordinator()
			store, err := storage.Open(cfg.DatabaseTarget())
			if err != nil {
				return err
			}
			defer store.Close()

			logger, loggerClose, err := logging.Setup(cfg.Log, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer loggerClose()
			if opts.SetLogger != nil {
				opts.SetLogger(logger)
			}

			ln, err := net.Listen("tcp", listen)
			if err != nil {
				return err
			}
			defer ln.Close()

			handler := httpapi.NewServer(httpapi.Options{
				Store:                store,
				Stderr:               cmd.ErrOrStderr(),
				Logger:               logger,
				Admin:                cfg.ServerAdmin,
				Console:              consoleOptions,
				Shutdown:             shutdown,
				MCPTrustedProxyHosts: mcpOptions.TrustedProxyHosts,
				ConfigSecretKey:      cfg.SecretKey,
			})
			httpServer := &http.Server{
				Addr:              listen,
				Handler:           handler,
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				IdleTimeout:       120 * time.Second,
			}

			errCh := make(chan error, 4)
			go func() {
				if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
				}
			}()

			logger.Info("server listening",
				"component", "server",
				"operation", "server_listening",
				"addr", ln.Addr().String(),
				"url", "http://"+ln.Addr().String(),
			)
			if err := handler.WriteAdminSetupInstructions("http://" + ln.Addr().String()); err != nil {
				return err
			}

			flags := serverDispatcherFlagOverrides{}
			if cmd.Flags().Changed("notification-dispatcher-interval") {
				flags.LegacyNotificationInterval = &notificationDispatcherInterval
			}
			if cmd.Flags().Changed("hook-dispatcher-interval") {
				flags.HookInterval = &hookDispatcherInterval
			}
			if cmd.Flags().Changed("notification-dispatcher-max-concurrency") {
				flags.NotificationMaxConcurrency = &notificationMaxConcurrency
			}
			if cmd.Flags().Changed("hook-dispatcher-max-concurrency") {
				flags.HookMaxConcurrency = &hookMaxConcurrency
			}
			if cmd.Flags().Changed("notification-dispatcher-batch-size") {
				flags.NotificationBatchSize = &notificationBatchSize
			}
			if cmd.Flags().Changed("hook-dispatcher-batch-size") {
				flags.HookBatchSize = &hookBatchSize
			}
			if cmd.Flags().Changed("notification-dispatcher-prefetch-factor") {
				flags.NotificationPrefetchFactor = &notificationPrefetchFactor
			}
			if cmd.Flags().Changed("hook-dispatcher-prefetch-factor") {
				flags.HookPrefetchFactor = &hookPrefetchFactor
			}
			if cmd.Flags().Changed("notification-dispatcher-claim-ttl") {
				flags.NotificationClaimTTL = &notificationClaimTTL
			}
			if cmd.Flags().Changed("hook-dispatcher-claim-ttl") {
				flags.HookClaimTTL = &hookClaimTTL
			}
			runtimeOptions, err := buildServerDispatcherRuntimeOptions(cfg, flags)
			if err != nil {
				return err
			}

			hookOptions := runtimeOptions.Hook
			hookOptions.Store = store
			hookOptions.Clock = app.RealClock{}
			hookOptions.Version = "dev"
			hookOptions.Shutdown = shutdown
			hookOptions.Logger = logger
			hookDispatcher := hookruntime.NewDispatcher(hookOptions)
			notificationOptions := runtimeOptions.Notification
			notificationOptions.Store = store
			notificationOptions.Clock = app.RealClock{}
			notificationOptions.Version = "dev"
			notificationOptions.Shutdown = shutdown
			notificationOptions.Logger = logger
			notificationDispatcher := notificationruntime.NewDispatcher(notificationOptions)
			reminderScheduler := app.NewReminderScheduler(app.ReminderSchedulerOptions{
				Store:  store,
				Clock:  app.RealClock{},
				Logger: logger,
			})
			automationScheduler := app.NewProjectAutomationScheduler(app.ProjectAutomationSchedulerOptions{
				Store: store,
				Clock: app.RealClock{},
			})
			automationDispatcher := app.NewProjectAutomationDispatcher(app.ProjectAutomationDispatcherOptions{
				Store: store,
				Clock: app.RealClock{},
			})
			seriesScheduler := app.NewTaskSeriesScheduler(app.TaskSeriesSchedulerOptions{
				Store: store,
				Clock: app.RealClock{},
			})
			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stopSignals()

			var runtimeWG sync.WaitGroup
			runtimeWG.Add(8)
			// 解析 config secret key（TOML [security].config_secret_key）
			secretKey, err := app.ParseConfigSecretKey(cfg.SecretKey)
			if errors.Is(err, app.ErrConfigSecretKeyMissing) {
				secretKey = nil
			} else if err != nil {
				return fmt.Errorf("解析 config secret key 失败（[security].config_secret_key）: %w", err)
			}
			// 通讯录同步后台 dispatcher + scheduler
			directorySyncCfgSvc := app.NewOIDCConfigService(storage.NewConfigRepository(store.DB()), secretKey)
			directorySyncSvc := app.NewDirectorySyncService(store, directory.NewClient(xuanchuOIDC.NoProxyHTTPClient))
			directorySyncRuntime := app.NewDirectorySyncRuntime(store, directorySyncCfgSvc, directorySyncSvc)
			go func() {
				defer runtimeWG.Done()
				directorySyncRuntime.Run(runCtx)
			}()
			// browser session / OIDC auth flow 过期清理
			go func() {
				defer runtimeWG.Done()
				sessionRepo := storage.NewSessionRepository(store.DB())
				ticker := time.NewTicker(1 * time.Hour)
				defer ticker.Stop()
				for {
					select {
					case <-runCtx.Done():
						return
					case <-ticker.C:
						now := time.Now().Unix()
						_, _ = sessionRepo.PurgeExpiredSessions(now)
						_, _ = sessionRepo.PurgeExpiredAuthFlows(now)
					}
				}
			}()
			go func() {
				defer runtimeWG.Done()
				if err := hookDispatcher.Run(runCtx); err != nil {
					errCh <- fmt.Errorf("hook dispatcher: %w", err)
				}
			}()
			go func() {
				defer runtimeWG.Done()
				if err := notificationDispatcher.Run(runCtx); err != nil {
					errCh <- fmt.Errorf("notification dispatcher: %w", err)
				}
			}()
			go func() {
				defer runtimeWG.Done()
				if err := runReminderSchedulerLoop(runCtx, reminderScheduler, reminderSchedulerInterval); err != nil {
					errCh <- fmt.Errorf("reminder scheduler: %w", err)
				}
			}()
			// 项目自动化调度器：扫描 daily_at 规则并入队
			go func() {
				defer runtimeWG.Done()
				if err := automationScheduler.Run(runCtx, automationSchedulerInterval); err != nil {
					errCh <- fmt.Errorf("automation scheduler: %w", err)
				}
			}()
			// 项目自动化投递 dispatcher：认领到期投递并发送 OpenAI 兼容请求
			go func() {
				defer runtimeWG.Done()
				if err := automationDispatcher.Run(runCtx, automationDispatcherInterval); err != nil {
					errCh <- fmt.Errorf("automation dispatcher: %w", err)
				}
			}()
			// 循环任务系列调度器：每分钟按日历补齐所有 workspace 的 active series（spec §9）
			go func() {
				defer runtimeWG.Done()
				if err := seriesScheduler.Run(runCtx, taskSeriesSchedulerInterval); err != nil {
					errCh <- fmt.Errorf("task series scheduler: %w", err)
				}
			}()

			select {
			case err := <-errCh:
				stopSignals()
				cancelRun()
				shutdown.ForceCancel()
				return err
			case <-signalCtx.Done():
			}

			logger.Info("shutdown signal received",
				"component", "server",
				"operation", "shutdown_signal",
			)
			shutdown.StopAccepting()
			cancelRun()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownOptions.Timeout)
			defer cancel()
			httpDone := make(chan error, 1)
			go func() { httpDone <- httpServer.Shutdown(shutdownCtx) }()
			drainDone := make(chan error, 1)
			go func() { drainDone <- shutdown.Drain(shutdownCtx) }()
			runtimeDone := make(chan struct{})
			go func() {
				runtimeWG.Wait()
				close(runtimeDone)
			}()

			var httpErr error
			var drainErr error
			httpPending := true
			drainPending := true
			runtimePending := true
			for httpPending || drainPending || runtimePending {
				select {
				case err := <-httpDone:
					httpErr = err
					httpPending = false
					httpDone = nil
				case err := <-drainDone:
					drainErr = err
					drainPending = false
					drainDone = nil
				case <-runtimeDone:
					runtimePending = false
					runtimeDone = nil
				case <-shutdownCtx.Done():
					shutdown.ForceCancel()
					_ = httpServer.Close()
					logger.Warn("shutdown timeout",
						"component", "server",
						"operation", "shutdown_timeout",
						"timeout_ms", shutdownOptions.Timeout.Milliseconds(),
					)
					forceCtx, forceCancel := context.WithTimeout(context.Background(), shutdownOptions.ForceTimeout)
					defer forceCancel()
					for httpPending || drainPending || runtimePending {
						select {
						case err := <-httpDone:
							httpErr = err
							httpPending = false
							httpDone = nil
						case err := <-drainDone:
							drainErr = err
							drainPending = false
							drainDone = nil
						case <-runtimeDone:
							runtimePending = false
							runtimeDone = nil
						case <-forceCtx.Done():
							logger.Error("shutdown force timeout",
								"component", "server",
								"operation", "shutdown_force_timeout",
								"timeout_ms", shutdownOptions.ForceTimeout.Milliseconds(),
							)
							return fmt.Errorf("shutdown force timeout: %w", forceCtx.Err())
						}
					}
					return fmt.Errorf("shutdown timeout: %w", shutdownCtx.Err())
				}
			}
			if httpErr != nil {
				return httpErr
			}
			if drainErr != nil {
				return drainErr
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "HTTP listen address")
	cmd.Flags().DurationVar(&shutdownTimeout, "shutdown-timeout", 30*time.Second, "graceful shutdown timeout")
	cmd.Flags().DurationVar(&shutdownForceTimeout, "shutdown-force-timeout", 5*time.Second, "forced shutdown cleanup timeout")
	cmd.Flags().DurationVar(&reminderSchedulerInterval, "reminder-scheduler-interval", 60*time.Second, "reminder scheduler interval")
	cmd.Flags().DurationVar(&notificationDispatcherInterval, "notification-dispatcher-interval", 5*time.Second, "notification dispatcher interval")
	cmd.Flags().DurationVar(&hookDispatcherInterval, "hook-dispatcher-interval", 5*time.Second, "hook dispatcher interval")
	cmd.Flags().DurationVar(&automationSchedulerInterval, "automation-scheduler-interval", 60*time.Second, "project automation scheduler interval")
	cmd.Flags().DurationVar(&automationDispatcherInterval, "automation-dispatcher-interval", 5*time.Second, "project automation delivery dispatcher interval")
	cmd.Flags().DurationVar(&taskSeriesSchedulerInterval, "task-series-scheduler-interval", 60*time.Second, "task series recurrence scheduler interval")
	cmd.Flags().StringArrayVar(&mcpTrustedProxyHosts, "mcp-trusted-proxy-host", nil, "trusted external Host for HTTP MCP reverse proxy; repeatable")
	cmd.Flags().IntVar(&notificationMaxConcurrency, "notification-dispatcher-max-concurrency", 0, "notification dispatcher max concurrency")
	cmd.Flags().IntVar(&hookMaxConcurrency, "hook-dispatcher-max-concurrency", 0, "hook dispatcher max concurrency")
	cmd.Flags().IntVar(&notificationBatchSize, "notification-dispatcher-batch-size", 0, "notification dispatcher delivery claim batch size")
	cmd.Flags().IntVar(&hookBatchSize, "hook-dispatcher-batch-size", 0, "hook dispatcher delivery claim batch size")
	cmd.Flags().IntVar(&notificationPrefetchFactor, "notification-dispatcher-prefetch-factor", 0, "notification dispatcher claim prefetch factor")
	cmd.Flags().IntVar(&hookPrefetchFactor, "hook-dispatcher-prefetch-factor", 0, "hook dispatcher claim prefetch factor")
	cmd.Flags().DurationVar(&notificationClaimTTL, "notification-dispatcher-claim-ttl", 0, "notification dispatcher stale claim TTL")
	cmd.Flags().DurationVar(&hookClaimTTL, "hook-dispatcher-claim-ttl", 0, "hook dispatcher stale claim TTL")
	cmd.Flags().BoolVar(&consoleEnabled, "console", true, "enable embedded Web Admin Console")
	cmd.Flags().StringVar(&consoleBasePath, "console-base-path", "", "Web Admin Console base path")
	return cmd
}

type serverConsoleFlagOverrides struct {
	Enabled  *bool
	BasePath *string
}

func buildServerConsoleOptions(cfg config.Config, flags serverConsoleFlagOverrides) (config.ConsoleConfig, error) {
	console := cfg.Console
	if flags.Enabled != nil {
		console.Enabled = *flags.Enabled
	}
	if flags.BasePath != nil {
		console.BasePath = *flags.BasePath
	}
	if err := config.ValidateConsoleBasePath(console.BasePath); err != nil {
		return config.ConsoleConfig{}, fmt.Errorf("%s", strings.Replace(err.Error(), "server.console.base_path", "console-base-path", 1))
	}
	if console.AuthMode == "" {
		console.AuthMode = "bearer"
	}
	if console.AuthMode != "bearer" {
		return config.ConsoleConfig{}, fmt.Errorf("console auth mode must be bearer")
	}
	return console, nil
}

type serverMCPOptions struct {
	TrustedProxyHosts []string
}

func buildServerMCPOptions(cfg config.Config, flagHosts []string) (serverMCPOptions, error) {
	hosts := cfg.ServerMCP.TrustedProxyHosts
	if flagHosts != nil {
		hosts = flagHosts
	}
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if strings.Contains(strings.TrimSpace(host), "*") {
			return serverMCPOptions{}, fmt.Errorf("mcp-trusted-proxy-host cannot contain wildcard host")
		}
		out = append(out, host)
	}
	return serverMCPOptions{TrustedProxyHosts: out}, nil
}

type serverShutdownFlagOverrides struct {
	Timeout      *time.Duration
	ForceTimeout *time.Duration
}

type serverShutdownOptions struct {
	Timeout      time.Duration
	ForceTimeout time.Duration
}

func buildServerShutdownOptions(cfg config.Config, flags serverShutdownFlagOverrides) (serverShutdownOptions, error) {
	timeout := cfg.Shutdown.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	forceTimeout := cfg.Shutdown.ForceTimeout
	if forceTimeout == 0 {
		forceTimeout = 5 * time.Second
	}
	if flags.Timeout != nil {
		timeout = *flags.Timeout
	}
	if flags.ForceTimeout != nil {
		forceTimeout = *flags.ForceTimeout
	}
	if timeout <= 0 {
		return serverShutdownOptions{}, fmt.Errorf("shutdown-timeout must be positive")
	}
	if forceTimeout <= 0 {
		return serverShutdownOptions{}, fmt.Errorf("shutdown-force-timeout must be positive")
	}
	return serverShutdownOptions{Timeout: timeout, ForceTimeout: forceTimeout}, nil
}

type serverDispatcherFlagOverrides struct {
	NotificationMaxConcurrency *int
	HookMaxConcurrency         *int
	NotificationBatchSize      *int
	HookBatchSize              *int
	NotificationPrefetchFactor *int
	HookPrefetchFactor         *int
	NotificationClaimTTL       *time.Duration
	HookClaimTTL               *time.Duration
	LegacyNotificationInterval *time.Duration
	HookInterval               *time.Duration
}

type serverDispatcherRuntimeOptions struct {
	Notification notificationruntime.DispatcherOptions
	Hook         hookruntime.DispatcherOptions
	SinkLimiter  *runtimeutil.SinkLimiter
}

func buildServerDispatcherRuntimeOptions(cfg config.Config, flags serverDispatcherFlagOverrides) (serverDispatcherRuntimeOptions, error) {
	notificationCfg := cfg.NotificationDispatcher
	hookCfg := cfg.HookDispatcher
	if flags.NotificationMaxConcurrency != nil {
		if *flags.NotificationMaxConcurrency <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("notification-dispatcher-max-concurrency must be a positive integer")
		}
		notificationCfg.MaxConcurrency = *flags.NotificationMaxConcurrency
	}
	if flags.HookMaxConcurrency != nil {
		if *flags.HookMaxConcurrency <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("hook-dispatcher-max-concurrency must be a positive integer")
		}
		hookCfg.MaxConcurrency = *flags.HookMaxConcurrency
	}
	if flags.NotificationBatchSize != nil {
		if *flags.NotificationBatchSize <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("notification-dispatcher-batch-size must be a positive integer")
		}
		notificationCfg.BatchSize = *flags.NotificationBatchSize
	}
	if flags.HookBatchSize != nil {
		if *flags.HookBatchSize <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("hook-dispatcher-batch-size must be a positive integer")
		}
		hookCfg.BatchSize = *flags.HookBatchSize
	}
	if flags.NotificationPrefetchFactor != nil {
		if *flags.NotificationPrefetchFactor <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("notification-dispatcher-prefetch-factor must be a positive integer")
		}
		notificationCfg.PrefetchFactor = *flags.NotificationPrefetchFactor
	}
	if flags.HookPrefetchFactor != nil {
		if *flags.HookPrefetchFactor <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("hook-dispatcher-prefetch-factor must be a positive integer")
		}
		hookCfg.PrefetchFactor = *flags.HookPrefetchFactor
	}
	if flags.NotificationClaimTTL != nil {
		if *flags.NotificationClaimTTL <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("notification-dispatcher-claim-ttl must be a positive duration")
		}
		notificationCfg.ClaimTTL = *flags.NotificationClaimTTL
	}
	if flags.HookClaimTTL != nil {
		if *flags.HookClaimTTL <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("hook-dispatcher-claim-ttl must be a positive duration")
		}
		hookCfg.ClaimTTL = *flags.HookClaimTTL
	}
	if flags.LegacyNotificationInterval != nil {
		if *flags.LegacyNotificationInterval <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("notification-dispatcher-interval must be a positive duration")
		}
		notificationCfg.PollInterval = *flags.LegacyNotificationInterval
		if flags.HookInterval == nil {
			hookCfg.PollInterval = *flags.LegacyNotificationInterval
		}
	}
	if flags.HookInterval != nil {
		if *flags.HookInterval <= 0 {
			return serverDispatcherRuntimeOptions{}, fmt.Errorf("hook-dispatcher-interval must be a positive duration")
		}
		hookCfg.PollInterval = *flags.HookInterval
	}
	notificationMax := runtimeutil.EffectiveConcurrency(notificationCfg.MaxConcurrency)
	hookMax := runtimeutil.EffectiveConcurrency(hookCfg.MaxConcurrency)
	defaultSinkConcurrency := notificationMax
	if hookMax < defaultSinkConcurrency {
		defaultSinkConcurrency = hookMax
	}
	if defaultSinkConcurrency < 1 {
		defaultSinkConcurrency = 1
	}
	sharedSinkLimiter := runtimeutil.NewSinkLimiter()
	return serverDispatcherRuntimeOptions{
		Notification: notificationruntime.DispatcherOptions{
			BatchSize:              notificationCfg.BatchSize,
			PollInterval:           notificationCfg.PollInterval,
			ClaimTTL:               notificationCfg.ClaimTTL,
			MaxConcurrency:         notificationCfg.MaxConcurrency,
			PrefetchFactor:         notificationCfg.PrefetchFactor,
			DefaultSinkConcurrency: defaultSinkConcurrency,
			SinkLimiter:            sharedSinkLimiter,
		},
		Hook: hookruntime.DispatcherOptions{
			BatchSize:              hookCfg.BatchSize,
			PollInterval:           hookCfg.PollInterval,
			ClaimTTL:               hookCfg.ClaimTTL,
			MaxConcurrency:         hookCfg.MaxConcurrency,
			PrefetchFactor:         hookCfg.PrefetchFactor,
			DefaultSinkConcurrency: defaultSinkConcurrency,
			SinkLimiter:            sharedSinkLimiter,
		},
		SinkLimiter: sharedSinkLimiter,
	}, nil
}

func runReminderSchedulerLoop(ctx context.Context, scheduler *app.ReminderScheduler, interval time.Duration) error {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if _, err := scheduler.RunOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := scheduler.RunOnce(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}
