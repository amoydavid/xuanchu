package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/render"
	"github.com/spf13/cobra"
)

func newAuditCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "audit",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newAuditListCommand(opts))
	return cmd
}

func newAuditListCommand(opts Options) *cobra.Command {
	var limit int
	var projectRef string
	cmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var rows []app.AuditLogView
			var err error
			if remoteMode, _, modeErr := isRemoteMode(currentOpts); modeErr != nil {
				return modeErr
			} else if remoteMode {
				client, clientErr := buildRemoteClient(currentOpts)
				if clientErr != nil {
					return clientErr
				}
				rows, err = client.ListAudit(context.Background(), currentOpts.Workspace, projectRef, limit)
			} else {
				svc, closeFn, serviceErr := buildServiceFromCmd(cmd, opts)
				if serviceErr != nil {
					return serviceErr
				}
				defer closeFn()
				rows, err = svc.ListAudit(app.AuditListInput{
					WorkspaceRef: currentOpts.Workspace,
					ProjectRef:   projectRef,
					Limit:        limit,
				})
			}
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				return render.JSON(cmd.OutOrStdout(), auditRowsForJSON(rows))
			}
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s %s %s\n", time.Unix(row.CreatedAt, 0).UTC().Format(time.RFC3339), row.ActorName, row.Action, row.TargetType, row.TargetID)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "limit rows")
	cmd.Flags().StringVar(&projectRef, "project", "", "filter audit rows by project slug or UUID")
	return cmd
}

func auditRowsForJSON(rows []app.AuditLogView) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":            row.ID,
			"actor_user_id": row.ActorUserID,
			"actor_name":    row.ActorName,
			"workspace_id":  row.WorkspaceID,
			"project_id":    row.ProjectID,
			"action":        row.Action,
			"target_type":   row.TargetType,
			"target_id":     row.TargetID,
			"payload":       parseAuditPayload(row.PayloadJSON),
			"created_at":    row.CreatedAt,
		}
		out = append(out, item)
	}
	return out
}

func parseAuditPayload(raw string) any {
	if raw == "" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return value
}
