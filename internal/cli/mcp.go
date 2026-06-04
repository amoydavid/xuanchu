package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/mcpserver"
	"github.com/dajee/taskg/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func newMCPCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server commands",
	}

	cmd.AddCommand(newMCPStdioCommand(opts))
	return cmd
}

func newMCPStdioCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stdio",
		Short: "Start MCP server on stdio transport",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			rt, err := runtimeFromResolvedConfig(currentOpts, cfg, store, env)
			if err != nil {
				return err
			}

			srv := mcpserver.NewServer(mcpserver.Options{
				Store:              store,
				Version:            opts.Version,
				Mode:               mcpserver.ModeStdio,
				Stderr:             cmd.ErrOrStderr(),
				LocalRuntimeValues: rt.Values(),
			})

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return srv.Run(ctx, &mcp.StdioTransport{})
		},
	}
	return cmd
}
