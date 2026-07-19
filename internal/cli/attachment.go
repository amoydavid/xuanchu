package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/remote"
)

func newAttachmentCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attachment",
		Short: "管理任务附件",
	}
	cmd.AddCommand(newAttachmentAddCommand(opts))
	cmd.AddCommand(newAttachmentListCommand(opts))
	cmd.AddCommand(newAttachmentInfoCommand(opts))
	cmd.AddCommand(newAttachmentDownloadCommand(opts))
	cmd.AddCommand(newAttachmentRenameCommand(opts))
	cmd.AddCommand(newAttachmentRemoveCommand(opts))
	return cmd
}

// newAttachmentAddCommand 实现 `xuanchu attachment add <task-ref> <file> [--display-name <name>]`。
func newAttachmentAddCommand(opts Options) *cobra.Command {
	var displayName string
	cmd := &cobra.Command{
		Use:   "add <task-ref> <file>",
		Short: "上传附件到任务",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskRef := args[0]
			filePath := args[1]
			currentOpts := optionsFromCmd(cmd, opts)
			if currentOpts.JSON {
				// 下载/上传二进制不与 --json 混用；add 返回 metadata 可以用 JSON，
				// 但 download --output - 不允许与 --json 同时使用（见 download 命令）。
			}
			f, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer f.Close()
			stat, err := f.Stat()
			if err != nil {
				return err
			}

			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				resolved, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, taskRef)
				if err != nil {
					return err
				}
				dto, err := client.UploadTaskAttachment(context.Background(), currentOpts.Workspace, resolved, remote.UploadAttachmentInput{
					Reader:      f,
					Size:        stat.Size(),
					FileName:    filepath.Base(filePath),
					DisplayName: displayName,
					Mode:        "attachment",
				})
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					renderAttachmentJSON(cmd.OutOrStdout(), dto)
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Uploaded attachment %s\n", dto.ID)
				return nil
			}

			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.UploadAttachment(context.Background(), "task", taskRef, app.AttachmentUploadInput{
				Reader:       f,
				DeclaredSize: stat.Size(),
				OriginalName: filepath.Base(filePath),
				DisplayName:  displayName,
				Mode:         "attachment",
			})
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				renderAttachmentViewJSON(cmd.OutOrStdout(), view)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Uploaded attachment %s\n", view.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&displayName, "display-name", "", "附件展示名")
	return cmd
}

// newAttachmentListCommand 实现 `xuanchu attachment list <task-ref>`。
func newAttachmentListCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <task-ref>",
		Short: "列出任务附件",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskRef := args[0]
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				resolved, err := resolveRemoteTaskTarget(context.Background(), client, currentOpts, taskRef)
				if err != nil {
					return err
				}
				list, err := client.ListTaskAttachments(context.Background(), currentOpts.Workspace, resolved)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					renderAttachmentListJSON(cmd.OutOrStdout(), list)
					return nil
				}
				renderAttachmentListHuman(cmd.OutOrStdout(), list)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			list, err := svc.ListAttachments("task", taskRef, false)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				renderAttachmentViewListJSON(cmd.OutOrStdout(), list)
				return nil
			}
			renderAttachmentViewListHuman(cmd.OutOrStdout(), list)
			return nil
		},
	}
	return cmd
}

// newAttachmentInfoCommand 实现 `xuanchu attachment info <attachment-id>`。
func newAttachmentInfoCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <attachment-id>",
		Short: "查看附件 metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			attachmentID := args[0]
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				dto, err := client.GetAttachment(context.Background(), currentOpts.Workspace, attachmentID)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					renderAttachmentJSON(cmd.OutOrStdout(), dto)
					return nil
				}
				renderAttachmentHuman(cmd.OutOrStdout(), dto.ID, dto.DisplayName, dto.MediaType, dto.SizeBytes, dto.State)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.GetAttachment(attachmentID)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				renderAttachmentViewJSON(cmd.OutOrStdout(), view)
				return nil
			}
			renderAttachmentHuman(cmd.OutOrStdout(), view.ID, view.DisplayName, view.MediaType, view.SizeBytes, view.State)
			return nil
		},
	}
	return cmd
}

// newAttachmentDownloadCommand 实现 `xuanchu attachment download <attachment-id> [--output <path>]`。
func newAttachmentDownloadCommand(opts Options) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "download <attachment-id>",
		Short: "下载附件内容",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			attachmentID := args[0]
			currentOpts := optionsFromCmd(cmd, opts)
			if output == "-" && currentOpts.JSON {
				return errors.New("attachment_binary_json_conflict: --json cannot be combined with --output -")
			}

			var dst io.Writer
			var closeDst func() error
			var createdPath string
			if output == "-" {
				dst = cmd.OutOrStdout()
			} else if output == "" {
				// 默认写到当前目录的 display_name basename，已存在则报错。
				var displayName string
				if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
					return err
				} else if remoteMode {
					client, err := buildRemoteClient(currentOpts)
					if err != nil {
						return err
					}
					dto, err := client.GetAttachment(context.Background(), currentOpts.Workspace, attachmentID)
					if err != nil {
						return err
					}
					displayName = dto.DisplayName
				} else {
					svc, closeFn, err := buildServiceFromCmd(cmd, opts)
					if err != nil {
						return err
					}
					defer closeFn()
					view, err := svc.GetAttachment(attachmentID)
					if err != nil {
						return err
					}
					displayName = view.DisplayName
				}
				base := safeBasename(displayName)
				if base == "" {
					base = "attachment"
				}
				createdPath = base
				f, err := os.OpenFile(createdPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
				if err != nil {
					if errors.Is(err, os.ErrExist) {
						return fmt.Errorf("attachment_output_exists: %s already exists", createdPath)
					}
					return err
				}
				dst = f
				closeDst = f.Close
			} else {
				f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
				if err != nil {
					if errors.Is(err, os.ErrExist) {
						return fmt.Errorf("attachment_output_exists: %s already exists", output)
					}
					return err
				}
				createdPath = output
				dst = f
				closeDst = f.Close
			}

			// 失败时清理本次创建的文件；不清理调用前已存在的文件。
			cleanupOnError := func() {
				if createdPath != "" {
					_ = os.Remove(createdPath)
				}
			}

			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				cleanupOnError()
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					cleanupOnError()
					return err
				}
				result, err := client.DownloadAttachment(context.Background(), currentOpts.Workspace, attachmentID, dst)
				if closeDst != nil {
					_ = closeDst()
				}
				if err != nil {
					cleanupOnError()
					return err
				}
				if output != "-" {
					fmt.Fprintf(cmd.ErrOrStderr(), "Downloaded %s (%d bytes)\n", result.DisplayName, result.SizeBytes)
				}
				return nil
			}

			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				cleanupOnError()
				return err
			}
			defer closeFn()
			content, err := svc.OpenAttachmentContent(context.Background(), attachmentID)
			if err != nil {
				cleanupOnError()
				return err
			}
			defer content.Reader.Close()
			if _, err := io.Copy(dst, content.Reader); err != nil {
				if closeDst != nil {
					_ = closeDst()
				}
				cleanupOnError()
				return err
			}
			if closeDst != nil {
				_ = closeDst()
			}
			if output != "-" {
				fmt.Fprintf(cmd.ErrOrStderr(), "Downloaded %s (%d bytes)\n", content.View.DisplayName, content.Blob.Size)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "输出路径；- 表示 stdout")
	return cmd
}

// newAttachmentRenameCommand 实现 `xuanchu attachment rename <attachment-id> <display-name>`。
func newAttachmentRenameCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <attachment-id> <display-name>",
		Short: "修改附件展示名",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			attachmentID := args[0]
			displayName := args[1]
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				dto, err := client.RenameAttachment(context.Background(), currentOpts.Workspace, attachmentID, displayName)
				if err != nil {
					return err
				}
				if currentOpts.JSON {
					renderAttachmentJSON(cmd.OutOrStdout(), dto)
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Renamed attachment %s\n", dto.ID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			view, err := svc.RenameAttachment(attachmentID, displayName)
			if err != nil {
				return err
			}
			if currentOpts.JSON {
				renderAttachmentViewJSON(cmd.OutOrStdout(), view)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Renamed attachment %s\n", view.ID)
			return nil
		},
	}
	return cmd
}

// newAttachmentRemoveCommand 实现 `xuanchu attachment remove <attachment-id>`。
func newAttachmentRemoveCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <attachment-id>",
		Short: "删除附件",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			attachmentID := args[0]
			currentOpts := optionsFromCmd(cmd, opts)
			if remoteMode, _, err := isRemoteMode(currentOpts); err != nil {
				return err
			} else if remoteMode {
				client, err := buildRemoteClient(currentOpts)
				if err != nil {
					return err
				}
				if err := client.RemoveAttachment(context.Background(), currentOpts.Workspace, attachmentID); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed attachment %s\n", attachmentID)
				return nil
			}
			svc, closeFn, err := buildServiceFromCmd(cmd, opts)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.RemoveAttachment(context.Background(), attachmentID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed attachment %s\n", attachmentID)
			return nil
		},
	}
	return cmd
}

// --- 渲染 helpers ---

func renderAttachmentJSON(w io.Writer, dto remote.AttachmentDTO) {
	_ = json.NewEncoder(w).Encode(dto)
}

func renderAttachmentListJSON(w io.Writer, list []remote.AttachmentDTO) {
	_ = json.NewEncoder(w).Encode(list)
}

func renderAttachmentViewJSON(w io.Writer, view app.AttachmentView) {
	_ = json.NewEncoder(w).Encode(view)
}

func renderAttachmentViewListJSON(w io.Writer, list []app.AttachmentView) {
	_ = json.NewEncoder(w).Encode(list)
}

func renderAttachmentHuman(w io.Writer, id, displayName, mediaType string, sizeBytes int64, state string) {
	fmt.Fprintf(w, "ID: %s\nDisplay Name: %s\nMedia Type: %s\nSize: %d\nState: %s\n", id, displayName, mediaType, sizeBytes, state)
}

func renderAttachmentListHuman(w io.Writer, list []remote.AttachmentDTO) {
	for _, dto := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", dto.ID, dto.DisplayName, dto.MediaType, dto.SizeBytes, dto.State)
	}
}

func renderAttachmentViewListHuman(w io.Writer, list []app.AttachmentView) {
	for _, view := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", view.ID, view.DisplayName, view.MediaType, view.SizeBytes, view.State)
	}
}

// safeBasename 剥离路径分隔符和换行，避免写出到意外位置。
func safeBasename(name string) string {
	if name == "" {
		return ""
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	return name
}
