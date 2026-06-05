package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dajee/taskg/internal/app"
	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newAuditCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "查看审计日志",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newAuditListCommand(opts))
	return cmd
}

func newAuditListCommand(opts Options) *cobra.Command {
	var limit int
	var projectRef string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出审计日志",
		Args:  cobra.NoArgs,
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
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s %s %s\n", time.Unix(row.CreatedAt, 0).UTC().Format(time.RFC3339), actorDisplayName(row.Actor), row.Action, row.TargetType, row.TargetID)
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
			"id":           row.ID,
			"actor":        userInfoToJSONMap(row.Actor),
			"workspace_id": row.WorkspaceID,
			"project_id":   row.ProjectID,
			"action":       row.Action,
			"target_type":  row.TargetType,
			"target_id":    row.TargetID,
			"payload":      parseAuditPayload(row.PayloadJSON),
			"created_at":   row.CreatedAt,
		}
		if row.DelegatorUser != nil {
			item["delegator_user"] = userInfoToJSONMap(row.DelegatorUser)
		}
		if row.DelegatorTokenID != nil {
			item["delegator_token_id"] = *row.DelegatorTokenID
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

func userInfoToJSONMap(ui *task.UserInfo) map[string]any {
	if ui == nil {
		return nil
	}
	m := map[string]any{
		"id":   ui.ID,
		"name": ui.Name,
	}
	if ui.Email != nil {
		m["email"] = *ui.Email
	}
	if len(ui.ExternalIDs) > 0 {
		extIDs := make([]map[string]string, len(ui.ExternalIDs))
		for i, eid := range ui.ExternalIDs {
			extIDs[i] = map[string]string{"provider": eid.Provider, "external_id": eid.ExternalID}
		}
		m["external_ids"] = extIDs
	}
	return m
}

func actorDisplayName(actor *task.UserInfo) string {
	if actor == nil {
		return ""
	}
	return actor.Name
}
