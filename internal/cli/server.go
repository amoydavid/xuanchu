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
	"github.com/dajee/taskg/internal/httpapi"
	"github.com/dajee/taskg/internal/storage/sqlite"
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
			env := runtimeEnv()
			cfg, err := config.Resolve(config.Options{
				DataDir: currentOpts.DataDir,
				DBPath:  currentOpts.DBPath,
				JSON:    currentOpts.JSON,
				NoColor: currentOpts.NoColor,
				Env:     env,
			})
			if err != nil {
				return err
			}
			store, err := sqlite.Open(cfg.DatabasePath)
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

			errCh := make(chan error, 1)
			go func() {
				if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
				}
				close(errCh)
			}()

			fmt.Fprintf(cmd.ErrOrStderr(), "taskg: server listening on http://%s\n", ln.Addr().String())

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				return err
			}
			if err := <-errCh; err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "HTTP listen address")
	cmd.Flags().DurationVar(&shutdownTimeout, "shutdown-timeout", 30*time.Second, "graceful shutdown timeout")
	return cmd
}
