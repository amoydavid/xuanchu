package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/dajee/taskg/internal/render"
	"github.com/dajee/taskg/internal/task"
	"github.com/spf13/cobra"
)

func newExportCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "export",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			tasks, err := svc.Export()
			if err != nil {
				return err
			}
			dtos := make([]task.JSONTask, len(tasks))
			for i, tsk := range tasks {
				dtos[i] = task.ToJSON(tsk)
			}
			return render.JSON(cmd.OutOrStdout(), dtos)
		},
	}
}

func newImportCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "import [file]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()

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

			var dtos []task.JSONTask
			if err := json.NewDecoder(r).Decode(&dtos); err != nil {
				return err
			}
			count, err := svc.Import(dtos)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %d tasks\n", count)
			return nil
		},
	}
}
