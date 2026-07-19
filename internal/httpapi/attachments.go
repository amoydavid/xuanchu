package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

// handleAttachmentUpload 处理 multipart/form-data 附件上传。
//
// file=<binary>（必填），mode=attachment|description_draft（必填），display_name=<text>（可选）。
func (s *Server) handleAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	// bodyLimitMiddleware 已经按附件上限放宽了 r.Body。
	if err := r.ParseMultipartForm(s.attachmentUploadLimit()); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment_too_large", "multipart payload too large", nil)
		return
	}
	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode == "" {
		writeError(w, http.StatusBadRequest, "attachment_state_invalid", "mode is required", nil)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_upload_incomplete", "file field is required", nil)
		return
	}
	defer file.Close()

	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.UploadAttachment(r.Context(), "task", taskRef, app.AttachmentUploadInput{
		Reader:       file,
		DeclaredSize: header.Size,
		OriginalName: header.Filename,
		DisplayName:  r.FormValue("display_name"),
		Mode:         mode,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, view, nil)
}

// handleAttachmentImportURL 处理远程图片转存。
func (s *Server) handleAttachmentImportURL(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	var req struct {
		SourceURL   string `json:"source_url"`
		Mode        string `json:"mode"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if strings.TrimSpace(req.SourceURL) == "" {
		writeError(w, http.StatusBadRequest, "content_reference_query_invalid", "source_url is required", nil)
		return
	}
	if strings.TrimSpace(req.Mode) == "" {
		writeError(w, http.StatusBadRequest, "attachment_state_invalid", "mode is required", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.ImportAttachmentURL(r.Context(), "task", taskRef, app.AttachmentImportURLInput{
		SourceURL:   req.SourceURL,
		DisplayName: req.DisplayName,
		Mode:        req.Mode,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, view, nil)
}

// handleAttachmentList 列出 task 的附件。
func (s *Server) handleAttachmentList(w http.ResponseWriter, r *http.Request) {
	taskRef, ok := requireTaskRef(w, r)
	if !ok {
		return
	}
	includeDrafts := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_drafts")), "true")
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	list, err := scoped.ListAttachments("task", taskRef, includeDrafts)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, list, nil)
}

// handleAttachmentGet 返回单个附件 metadata。
func (s *Server) handleAttachmentGet(w http.ResponseWriter, r *http.Request) {
	attachmentID, ok := requireAttachmentID(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.GetAttachment(attachmentID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

// handleAttachmentContent 流式返回附件二进制内容。
func (s *Server) handleAttachmentContent(w http.ResponseWriter, r *http.Request) {
	attachmentID, ok := requireAttachmentID(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskRead, app.PermissionTaskRead, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	content, err := scoped.OpenAttachmentContent(r.Context(), attachmentID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	defer content.Reader.Close()

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if content.Blob.ETag != "" {
		w.Header().Set("ETag", content.Blob.ETag)
	}
	if content.Blob.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(content.Blob.Size, 10))
	}
	if content.View.MediaType != "" {
		w.Header().Set("Content-Type", content.View.MediaType)
	}
	filename := sanitizeContentDispositionFilename(content.View.DisplayName)
	if content.View.InlineCapable {
		w.Header().Set("Content-Disposition", `inline; filename*=UTF-8''`+filename)
	} else {
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+filename)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, content.Reader)
}

// handleAttachmentRename 修改附件展示名。
func (s *Server) handleAttachmentRename(w http.ResponseWriter, r *http.Request) {
	attachmentID, ok := requireAttachmentID(w, r)
	if !ok {
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	view, err := scoped.RenameAttachment(attachmentID, req.DisplayName)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, view, nil)
}

// handleAttachmentRemove 删除附件。
func (s *Server) handleAttachmentRemove(w http.ResponseWriter, r *http.Request) {
	attachmentID, ok := requireAttachmentID(w, r)
	if !ok {
		return
	}
	scoped, _, err := s.scopedService(r, auth.ScopeTaskWrite, app.PermissionTaskWrite, "")
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := scoped.RemoveAttachment(r.Context(), attachmentID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireAttachmentID 解析并校验 attachmentID path 参数。
func requireAttachmentID(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, "attachmentID"))
	if raw == "" || strings.ContainsAny(raw, "/\x00\r\n") {
		writeError(w, http.StatusBadRequest, "attachment_not_found", "invalid attachment id", nil)
		return "", false
	}
	return raw, true
}

// sanitizeContentDispositionFilename 剥离控制字符、路径分隔符和换行，
// 并做 RFC 5987 percent-encoding，避免 header 注入。
func sanitizeContentDispositionFilename(name string) string {
	if name == "" {
		return "attachment"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			// 按 RFC 5987 percent-encode 不安全字符。
			s := string(r)
			for _, c := range []byte(s) {
				fmtHexByte(&b, c)
			}
		}
	}
	out := b.String()
	if out == "" {
		return "attachment"
	}
	return out
}

func fmtHexByte(b *strings.Builder, c byte) {
	const hex = "0123456789ABCDEF"
	b.WriteByte('%')
	b.WriteByte(hex[c>>4])
	b.WriteByte(hex[c&0x0f])
}
