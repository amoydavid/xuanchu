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

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/hookruntime"
	"github.com/dajee/taskg/internal/httpapi"
	"github.com/dajee/taskg/internal/storage"
	"github.com/spf13/cobra"
)

func newServerCommand(opts Options) *cobra.Command {
	var listen string
	var shutdownTimeout time.Duration
	cmd := &cobra.Command{
		Use:  "server",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if listen == "" {
				return app.RuntimeError{Code: "server_listen_required", Message: "server listen address is required"}
			}
			currentOpts := optionsFromCmd(cmd, opts)
			env := RuntimeEnv()
			cfg, err := config.Resolve(config.Options{
				DataDir: currentOpts.DataDir,
				DBPath:  currentOpts.DBPath,
				DBURL:   currentOpts.DBURL,
				JSON:    currentOpts.JSON,
				NoColor: currentOpts.NoColor,
				Env:     env,
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

			ln, err := net.Listen("tcp", listen)
			if err != nil {
				return err
			}
			defer ln.Close()

			handler := httpapi.NewServer(httpapi.Options{
				Store:  store,
				Stderr: cmd.ErrOrStderr(),
			})
			httpServer := &http.Server{
				Addr:              listen,
				Handler:           handler,
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				IdleTimeout:       120 * time.Second,
			}

			errCh := make(chan error, 2)
			go func() {
				if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
				}
			}()

			fmt.Fprintf(cmd.ErrOrStderr(), "taskg: server listening on http://%s\n", ln.Addr().String())

			// 启动 webhook 投递调度器
			dispatcher := hookruntime.NewDispatcher(hookruntime.DispatcherOptions{
				Store:   store,
				Clock:   app.RealClock{},
				Version: "dev",
			})
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			go func() {
				if err := dispatcher.Run(ctx); err != nil {
					errCh <- fmt.Errorf("hook dispatcher: %w", err)
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
	return cmd
}
