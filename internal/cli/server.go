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
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/spf13/cobra"
)

func newServerCommand(opts Options) *cobra.Command {
	var listen string
	var shutdownTimeout time.Duration
	var reminderSchedulerInterval time.Duration
	var notificationDispatcherInterval time.Duration
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

			hookDispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
				Store:        store,
				Clock:        app.RealClock{},
				Version:      "dev",
				PollInterval: notificationDispatcherInterval,
			})
			notificationDispatcher := notificationruntime.NewDispatcher(notificationruntime.DispatcherOptions{
				Store:        store,
				Clock:        app.RealClock{},
				Version:      "dev",
				PollInterval: notificationDispatcherInterval,
			})
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
	return cmd
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
