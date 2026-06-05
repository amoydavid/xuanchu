package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"github.com/spf13/cobra"
)

func newTokenCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "管理 API token（创建、列表、修改、吊销）",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newTokenCreateCommand(opts))
	cmd.AddCommand(newTokenListCommand(opts))
	cmd.AddCommand(newTokenModifyCommand(opts))
	cmd.AddCommand(newTokenRevokeCommand(opts))
	return cmd
}

func newTokenCreateCommand(opts Options) *cobra.Command {
	var userRef string
	var scopes []string
	var workspaceIDs []string
	var projectRefs []string
	var projectIDs []string
	var tokenType string
	var expiresIn string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "创建新的 API token",
		Long: "创建新的 API token 并返回令牌明文（仅显示一次）。\n" +
			"scope 支持通配符：*（全部）、resource:*（如 task:*）、*:action（如 *:read）。\n" +
			"PAT 类型 token 使用 * 通配符时会自动剔除 impersonate scope。",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			workspaceRefs := append([]string(nil), workspaceIDs...)
			if currentOpts.Workspace != "" {
				workspaceRefs = append([]string{currentOpts.Workspace}, workspaceRefs...)
			}
			projectScopeRefs := append([]string(nil), projectRefs...)
			projectScopeRefs = append(projectScopeRefs, projectIDs...)

			var ttl *time.Duration
			if strings.TrimSpace(expiresIn) != "" {
				value, err := time.ParseDuration(expiresIn)
				if err != nil {
					return err
				}
				ttl = &value
			}

			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				created, err := client.CreateToken(context.Background(), currentOpts.Workspace, remote.CreateTokenInput{
					Name:             args[0],
					Type:             tokenType,
					User:             userRef,
					Scopes:           scopes,
					WorkspaceIDs:     workspaceRefs,
					ProjectRefs:      projectRefs,
					ProjectIDs:       projectIDs,
					ExpiresInSeconds: remote.DurationSecondsPtr(ttl),
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					payload := tokenViewForJSON(created.View)
					payload["token"] = created.Token
					return render.JSON(cmd.OutOrStdout(), payload)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created token %s\n", created.View.ID)
				fmt.Fprintf(cmd.OutOrStdout(), "Token: %s\n", created.Token)
				fmt.Fprintln(cmd.OutOrStdout(), "This token is shown only once.")
				return nil
			}

			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			created, err := svc.CreateToken(app.CreateTokenInput{
				Name:          args[0],
				Type:          tokenType,
				UserRef:       userRef,
				Scopes:        scopes,
				WorkspaceRefs: workspaceRefs,
				ProjectRefs:   projectScopeRefs,
				ExpiresIn:     ttl,
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				payload := tokenViewForJSON(created.View)
				payload["token"] = created.RawToken
				return render.JSON(cmd.OutOrStdout(), payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created token %s\n", created.View.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "Token: %s\n", created.RawToken)
			fmt.Fprintln(cmd.OutOrStdout(), "This token is shown only once.")
			return nil
		},
	}
	cmd.Flags().StringVar(&userRef, "user", "", "target user name, id, or email")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "token scope (repeat or comma-separated)")
	cmd.Flags().StringSliceVar(&workspaceIDs, "workspace-id", nil, "workspace id allowlist (repeatable)")
	cmd.Flags().StringSliceVar(&projectRefs, "project", nil, "project slug allowlist in current or specified workspace (repeatable)")
	cmd.Flags().StringSliceVar(&projectIDs, "project-id", nil, "project id allowlist (repeatable)")
	cmd.Flags().StringVar(&tokenType, "type", "", "token type: pat or agent")
	cmd.Flags().StringVar(&expiresIn, "expires-in", "", "Go duration until expiry, for example 720h")
	return cmd
}

func newTokenListCommand(opts Options) *cobra.Command {
	var includeRevoked bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出当前用户的 token",
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
				rows, err := client.ListTokens(context.Background(), currentOpts.Workspace, includeRevoked)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), tokenViewsForJSON(rows))
				}
				fmt.Fprintln(cmd.OutOrStdout(), "ID\tPREFIX\tNAME\tTYPE\tWORKSPACES\tSCOPES\tEXPIRES_AT\tLAST_USED_AT")
				for _, row := range rows {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						row.ID,
						row.Prefix,
						row.Name,
						row.Type,
						strings.Join(row.WorkspaceIDs, ","),
						strings.Join(row.Scopes, ","),
						formatUnixPtr(row.ExpiresAt),
						formatUnixPtr(row.LastUsedAt),
					)
				}
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			rows, err := svc.ListTokens(app.ListTokensInput{IncludeRevoked: includeRevoked})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), tokenViewsForJSON(rows))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ID\tPREFIX\tNAME\tTYPE\tWORKSPACES\tSCOPES\tEXPIRES_AT\tLAST_USED_AT")
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					row.ID,
					row.Prefix,
					row.Name,
					row.Type,
					strings.Join(row.WorkspaceIDs, ","),
					strings.Join(row.Scopes, ","),
					formatUnixPtr(row.ExpiresAt),
					formatUnixPtr(row.LastUsedAt),
				)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeRevoked, "all", false, "include revoked tokens")
	return cmd
}

func newTokenModifyCommand(opts Options) *cobra.Command {
	var name string
	var scopes []string
	var expiresIn string
	cmd := &cobra.Command{
		Use:   "modify <id|prefix>",
		Short: "修改 token 属性（名称、scope、过期时间）",
		Long: "修改已有 token 的属性。可以同时指定多个修改项。\n" +
			"  --name        修改 token 名称\n" +
			"  --scope       替换 scope 列表（支持通配符）\n" +
			"  --expires-in  设置新的过期时间（Go duration，如 720h），0 表示移除过期限制",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)

			var ttl *time.Duration
			if cmd.Flags().Changed("expires-in") && strings.TrimSpace(expiresIn) != "" {
				d, err := time.ParseDuration(expiresIn)
				if err != nil {
					return err
				}
				ttl = &d
			}

			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				input := remote.ModifyTokenInput{
					TokenID: args[0],
				}
				if cmd.Flags().Changed("name") {
					input.Name = &name
				}
				if cmd.Flags().Changed("scope") {
					input.Scopes = scopes
				}
				input.ExpiresInSeconds = remote.DurationSecondsPtr(ttl)
				view, err := client.ModifyToken(context.Background(), currentOpts.Workspace, input)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					return render.JSON(cmd.OutOrStdout(), tokenViewForJSON(*view))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Modified token %s\n", view.ID)
				return nil
			}

			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

			input := app.ModifyTokenInput{
				TokenID: args[0],
			}
			if cmd.Flags().Changed("name") {
				input.Name = &name
			}
			if cmd.Flags().Changed("scope") {
				input.Scopes = scopes
			}
			if ttl != nil {
				input.ExpiresIn = ttl
			} else if cmd.Flags().Changed("expires-in") {
				zero := time.Duration(0)
				input.ExpiresIn = &zero
			}

			view, err := svc.ModifyToken(input)
			if err != nil {
				return err
			}

			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), tokenViewForJSON(*view))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Modified token %s\n", view.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "新名称")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "新的 scope 列表")
	cmd.Flags().StringVar(&expiresIn, "expires-in", "", "新的过期时间（Go duration，如 720h），0 表示永不过期")
	return cmd
}

func newTokenRevokeCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id|prefix>",
		Short: "吊销 token，使其立即失效",
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
				if err := client.RevokeToken(context.Background(), currentOpts.Workspace, args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Revoked token %s\n", args[0])
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.RevokeToken(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Revoked token %s\n", args[0])
			return nil
		},
	}
}

func tokenViewsForJSON(rows []app.TokenView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, tokenViewForJSON(row))
	}
	return out
}

func tokenViewForJSON(row app.TokenView) map[string]any {
	return map[string]any{
		"id":            row.ID,
		"prefix":        row.Prefix,
		"name":          row.Name,
		"type":          row.Type,
		"user":          userInfoToJSONMap(&row.User),
		"workspace_ids": row.WorkspaceIDs,
		"project_ids":   row.ProjectIDs,
		"scopes":        row.Scopes,
		"created_at":    formatUnixPtr(&row.CreatedAt),
		"expires_at":    formatUnixPtr(row.ExpiresAt),
		"revoked_at":    formatUnixPtr(row.RevokedAt),
		"last_used_at":  formatUnixPtr(row.LastUsedAt),
	}
}

func formatUnixPtr(ts *int64) any {
	if ts == nil {
		return nil
	}
	return time.Unix(*ts, 0).UTC().Format(time.RFC3339)
}
