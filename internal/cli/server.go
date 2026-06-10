package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
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
	var reminderSchedulerInterval time.Duration
	var notificationDispatcherInterval time.Duration
	var hookDispatcherInterval time.Duration
	var notificationMaxConcurrency int
	var hookMaxConcurrency int
	var notificationBatchSize int
	var hookBatchSize int
	var notificationPrefetchFactor int
	var hookPrefetchFactor int
	var notificationClaimTTL time.Duration
	var hookClaimTTL time.Duration
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
			dbTarget := cfg.DatabaseURL
			if dbTarget == "" {
				dbTarget = cfg.DatabasePath
			}
			store, err := storage.Open(dbTarget)
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
				Store:  store,
				Stderr: cmd.ErrOrStderr(),
				Logger: logger,
				Admin:  cfg.ServerAdmin,
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

			fmt.Fprintf(cmd.ErrOrStderr(), "xuanchu: server listening on http://%s\n", ln.Addr().String())

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
			hookDispatcher := hookruntime.NewDispatcher(hookOptions)
			notificationOptions := runtimeOptions.Notification
			notificationOptions.Store = store
			notificationOptions.Clock = app.RealClock{}
			notificationOptions.Version = "dev"
			notificationDispatcher := notificationruntime.NewDispatcher(notificationOptions)
			reminderScheduler := app.NewReminderScheduler(app.ReminderSchedulerOptions{
				Store: store,
				Clock: app.RealClock{},
			})
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			go func() {
				if err := hookDispatcher.Run(ctx); err != nil {
					errCh <- fmt.Errorf("hook dispatcher: %w", err)
				}
			}()
			go func() {
				if err := notificationDispatcher.Run(ctx); err != nil {
					errCh <- fmt.Errorf("notification dispatcher: %w", err)
				}
			}()
			go func() {
				if err := runReminderSchedulerLoop(ctx, reminderScheduler, reminderSchedulerInterval); err != nil {
					errCh <- fmt.Errorf("reminder scheduler: %w", err)
				}
			}()

			select {
			case err := <-errCh:
				stop()
				return err
			case <-ctx.Done():
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "HTTP listen address")
	cmd.Flags().DurationVar(&shutdownTimeout, "shutdown-timeout", 30*time.Second, "graceful shutdown timeout")
	cmd.Flags().DurationVar(&reminderSchedulerInterval, "reminder-scheduler-interval", 60*time.Second, "reminder scheduler interval")
	cmd.Flags().DurationVar(&notificationDispatcherInterval, "notification-dispatcher-interval", 5*time.Second, "notification dispatcher interval")
	cmd.Flags().DurationVar(&hookDispatcherInterval, "hook-dispatcher-interval", 5*time.Second, "hook dispatcher interval")
	cmd.Flags().IntVar(&notificationMaxConcurrency, "notification-dispatcher-max-concurrency", 0, "notification dispatcher max concurrency")
	cmd.Flags().IntVar(&hookMaxConcurrency, "hook-dispatcher-max-concurrency", 0, "hook dispatcher max concurrency")
	cmd.Flags().IntVar(&notificationBatchSize, "notification-dispatcher-batch-size", 0, "notification dispatcher delivery claim batch size")
	cmd.Flags().IntVar(&hookBatchSize, "hook-dispatcher-batch-size", 0, "hook dispatcher delivery claim batch size")
	cmd.Flags().IntVar(&notificationPrefetchFactor, "notification-dispatcher-prefetch-factor", 0, "notification dispatcher claim prefetch factor")
	cmd.Flags().IntVar(&hookPrefetchFactor, "hook-dispatcher-prefetch-factor", 0, "hook dispatcher claim prefetch factor")
	cmd.Flags().DurationVar(&notificationClaimTTL, "notification-dispatcher-claim-ttl", 0, "notification dispatcher stale claim TTL")
	cmd.Flags().DurationVar(&hookClaimTTL, "hook-dispatcher-claim-ttl", 0, "hook dispatcher stale claim TTL")
	return cmd
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
