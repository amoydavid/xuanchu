package cli

import (
	"fmt"
	"io"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"github.com/spf13/cobra"
)

func newAdminCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "管理 server admin bootstrap 工具"}
	tokenCmd := &cobra.Command{Use: "token", Short: "生成或计算 admin token hash"}
	tokenCmd.AddCommand(newAdminTokenGenerateCommand(opts))
	tokenCmd.AddCommand(newAdminTokenHashCommand(opts))
	cmd.AddCommand(tokenCmd)
	return cmd
}

func newAdminTokenGenerateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "generate",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, hash, err := auth.GenerateAdminToken()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "token: %s\nhash: %s\n", raw, hash)
			return nil
		},
	}
}

func newAdminTokenHashCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "hash",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			in := opts.Stdin
			if in == nil {
				in = cmd.InOrStdin()
			}
			data, err := io.ReadAll(in)
			if err != nil {
				return err
			}
			raw := strings.TrimSpace(string(data))
			if raw == "" {
				return fmt.Errorf("admin token is required on stdin")
			}
			fmt.Fprintln(cmd.OutOrStdout(), auth.HashAdminToken(raw))
			return nil
		},
	}
}
