package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/mcpserver"
	"git.dajee.net/dajee/xuanchu/internal/runtimeutil"
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func newMCPCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP 服务器",
	}

	cmd.AddCommand(newMCPStdioCommand(opts))
	return cmd
}

func newMCPStdioCommand(opts Options) *cobra.Command {
	var shutdownTimeout time.Duration
	var shutdownForceTimeout time.Duration
	cmd := &cobra.Command{
		Use:   "stdio",
		Short: "在 stdio 上启动 MCP 服务器",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			store, err := storage.Open(cfg.DatabaseTarget())
			if err != nil {
				return err
			}
			defer store.Close()
			rt, err := runtimeFromResolvedConfig(currentOpts, cfg, store, env)
			if err != nil {
				return err
			}

			logger, loggerClose, err := logging.Setup(cfg.Log, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer loggerClose()
			if opts.SetLogger != nil {
				opts.SetLogger(logger)
			}

			shutdown := runtimeutil.NewShutdownCoordinator()
			srv := mcpserver.NewServer(mcpserver.Options{
				Store:              store,
				Version:            opts.Version,
				Mode:               mcpserver.ModeStdio,
				Stderr:             cmd.ErrOrStderr(),
				LocalRuntimeValues: rt.Values(),
				Logger:             logger,
				Shutdown:           shutdown,
			})

			signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return runMCPServerWithShutdown(signalCtx, srv, &mcp.StdioTransport{}, shutdown, shutdownOptions, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().DurationVar(&shutdownTimeout, "shutdown-timeout", 30*time.Second, "graceful shutdown timeout")
	cmd.Flags().DurationVar(&shutdownForceTimeout, "shutdown-force-timeout", 5*time.Second, "forced shutdown cleanup timeout")
	return cmd
}

func runMCPServerWithShutdown(signalCtx context.Context, srv *mcp.Server, transport mcp.Transport, shutdown *runtimeutil.ShutdownCoordinator, opts serverShutdownOptions, stderr io.Writer) error {
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(runCtx, transport)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-signalCtx.Done():
	}

	if stderr != nil {
		fmt.Fprintln(stderr, "xuanchu: shutdown: signal received")
	}
	shutdown.StopAccepting()
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancelDrain()
	if err := shutdown.Drain(drainCtx); err != nil {
		shutdown.ForceCancel()
		forceCtx, cancelForce := context.WithTimeout(context.Background(), opts.ForceTimeout)
		defer cancelForce()
		cancelRun()
		select {
		case err := <-errCh:
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-forceCtx.Done():
			return fmt.Errorf("shutdown force timeout: %w", forceCtx.Err())
		}
	}
	cancelRun()
	forceCtx, cancelForce := context.WithTimeout(context.Background(), opts.ForceTimeout)
	defer cancelForce()
	select {
	case err := <-errCh:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-forceCtx.Done():
		shutdown.ForceCancel()
		return fmt.Errorf("shutdown force timeout: %w", forceCtx.Err())
	}
}
