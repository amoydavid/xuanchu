package remote

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// AttachmentDTO 是远程返回的附件视图。
type AttachmentDTO struct {
	ID            string         `json:"id"`
	AttachedTo    map[string]any `json:"attached_to"`
	State         string         `json:"state"`
	OriginalName  string         `json:"original_name"`
	DisplayName   string         `json:"display_name"`
	MediaType     string         `json:"media_type"`
	Extension     string         `json:"extension"`
	SizeBytes     int64          `json:"size_bytes"`
	SHA256        string         `json:"sha256"`
	InlineCapable bool           `json:"inline_capable"`
	SourceType    string         `json:"source_type"`
	ContentURL    string         `json:"content_url"`
	CreatedBy     map[string]any `json:"created_by"`
	CreatedAt     int64          `json:"created_at"`
	ModifiedAt    int64          `json:"modified_at"`
}

// UploadAttachmentInput 描述远程上传输入。
type UploadAttachmentInput struct {
	Reader      io.Reader
	Size        int64
	FileName    string
	DisplayName string
	Mode        string
	Progress    func(int64)
}

// DownloadAttachmentResult 描述下载结果。
type DownloadAttachmentResult struct {
	DisplayName string
	MediaType   string
	SHA256      string
	SizeBytes   int64
}

// UploadTaskAttachment 通过 multipart 上传附件。
func (c *Client) UploadTaskAttachment(ctx context.Context, workspace, taskRef string, in UploadAttachmentInput) (AttachmentDTO, error) {
	r, w := io.Pipe()
	writer := multipart.NewWriter(w)
	go func() {
		defer w.Close()
		defer writer.Close()
		part, err := writer.CreateFormFile("file", in.FileName)
		if err != nil {
			w.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, in.Reader); err != nil {
			w.CloseWithError(err)
			return
		}
		if in.Mode != "" {
			_ = writer.WriteField("mode", in.Mode)
		}
		if in.DisplayName != "" {
			_ = writer.WriteField("display_name", in.DisplayName)
		}
	}()
	endpoint := c.baseURL + "/api/v1/tasks/" + taskRef + "/attachments?workspace=" + workspace
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, r)
	if err != nil {
		return AttachmentDTO{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	var out apiEnvelope[AttachmentDTO]
	if err := c.doStream(req, &out, in.Progress); err != nil {
		return AttachmentDTO{}, err
	}
	return out.Data, nil
}

// ListTaskAttachments 列出 task 的附件。
func (c *Client) ListTaskAttachments(ctx context.Context, workspace, taskRef string) ([]AttachmentDTO, error) {
	var out apiEnvelope[[]AttachmentDTO]
	if err := c.get(ctx, "/api/v1/tasks/"+taskRef+"/attachments", urlValues(workspace), &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// GetAttachment 返回单个附件 metadata。
func (c *Client) GetAttachment(ctx context.Context, workspace, id string) (AttachmentDTO, error) {
	var out apiEnvelope[AttachmentDTO]
	if err := c.get(ctx, "/api/v1/attachments/"+id, urlValues(workspace), &out); err != nil {
		return AttachmentDTO{}, err
	}
	return out.Data, nil
}

// urlValues 把 workspace 转成 url.Values。
func urlValues(workspace string) url.Values {
	values := url.Values{}
	if workspace != "" {
		values.Set("workspace", workspace)
	}
	return values
}

// DownloadAttachment 流式下载附件到 dst。
func (c *Client) DownloadAttachment(ctx context.Context, workspace, id string, dst io.Writer) (DownloadAttachmentResult, error) {
	endpoint := c.baseURL + "/api/v1/attachments/" + id + "/content"
	if enc := urlValues(workspace).Encode(); enc != "" {
		endpoint += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return DownloadAttachmentResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.asUser != "" {
		req.Header.Set("X-Xuanchu-As", c.asUser)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DownloadAttachmentResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var payload apiErrorEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return DownloadAttachmentResult{}, APIError{Status: resp.StatusCode, Message: resp.Status}
		}
		return DownloadAttachmentResult{}, APIError{Status: resp.StatusCode, Code: payload.Error.Code, Message: payload.Error.Message}
	}
	result := DownloadAttachmentResult{
		MediaType:   resp.Header.Get("Content-Type"),
		DisplayName: parseContentDispositionFilename(resp.Header.Get("Content-Disposition")),
	}
	if v := resp.Header.Get("Content-Length"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			result.SizeBytes = n
		}
	}
	if _, err := io.Copy(dst, resp.Body); err != nil {
		return result, err
	}
	return result, nil
}

// RenameAttachment 修改展示名。
func (c *Client) RenameAttachment(ctx context.Context, workspace, id, displayName string) (AttachmentDTO, error) {
	var out apiEnvelope[AttachmentDTO]
	body := map[string]string{"display_name": displayName}
	data, err := json.Marshal(body)
	if err != nil {
		return AttachmentDTO{}, err
	}
	endpoint := c.baseURL + "/api/v1/attachments/" + id
	if enc := urlValues(workspace).Encode(); enc != "" {
		endpoint += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return AttachmentDTO{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.do(req, &out); err != nil {
		return AttachmentDTO{}, err
	}
	return out.Data, nil
}

// RemoveAttachment 删除附件。
func (c *Client) RemoveAttachment(ctx context.Context, workspace, id string) error {
	endpoint := c.baseURL + "/api/v1/attachments/" + id
	if enc := urlValues(workspace).Encode(); enc != "" {
		endpoint += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// doStream 发送请求并把 JSON 响应解码到 out。
func (c *Client) doStream(req *http.Request, out any, progress func(int64)) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.asUser != "" {
		req.Header.Set("X-Xuanchu-As", c.asUser)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var payload apiErrorEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return APIError{Status: resp.StatusCode, Message: resp.Status}
		}
		return APIError{Status: resp.StatusCode, Code: payload.Error.Code, Message: payload.Error.Message}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// parseContentDispositionFilename 从 Content-Disposition 提取文件名。
func parseContentDispositionFilename(value string) string {
	parts := strings.Split(value, ";")
	for _, part := range parts {
		trim := strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(trim), "filename*=") {
			_, after, ok := strings.Cut(trim, "''")
			if ok {
				return after
			}
		}
		if strings.HasPrefix(strings.ToLower(trim), "filename=") {
			_, after, ok := strings.Cut(trim, "=")
			if ok {
				return strings.Trim(after, `"`)
			}
		}
	}
	return ""
}
