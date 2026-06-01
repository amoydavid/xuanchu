package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/remote"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newTokenCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "token",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newTokenCreateCommand(opts))
	cmd.AddCommand(newTokenListCommand(opts))
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
		Use:  "create <name>",
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
		Use:  "list",
		Args: cobra.NoArgs,
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

func newTokenRevokeCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "revoke <id|prefix>",
		Args: cobra.ExactArgs(1),
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
		"user_id":       row.UserID,
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
