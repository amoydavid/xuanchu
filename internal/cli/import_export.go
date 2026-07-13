package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/render"
	"github.com/spf13/cobra"
)

func newExportCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "导出任务为 JSON",
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
				bundle, err := client.ExportTaskBundle(context.Background(), currentOpts.Workspace, currentOpts.Project, currentOpts.ProjectID)
				if err != nil {
					return err
				}
				return render.JSON(cmd.OutOrStdout(), bundle)
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			bundle, err := svc.ExportTaskBundle()
			if err != nil {
				return err
			}
			return render.JSON(cmd.OutOrStdout(), bundle)
		},
	}
}

func newImportCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "import [file]",
		Short: "从 JSON 导入任务",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			currentOpts := optionsFromCmd(cmd, opts)
			var r io.Reader
			if len(args) == 1 {
				f, err := os.Open(args[0])
				if err != nil {
					return err
				}
				defer f.Close()
				r = f
			} else {
				r = cmd.InOrStdin()
			}

			var bundle app.TaskBundleV1
			if err := json.NewDecoder(r).Decode(&bundle); err != nil {
				return err
			}
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				result, err := client.ImportTaskBundle(context.Background(), currentOpts.Workspace, bundle)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Imported %d tasks, %d series\n", result.TasksImported, result.SeriesImported)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			result, err := svc.ImportTaskBundle(bundle)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %d tasks, %d series\n", result.TasksImported, result.SeriesImported)
			return nil
		},
	}
}
