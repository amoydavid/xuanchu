package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/auth"
)

// newHTTPAttachmentFixture 构造一个带附件运行时和 task:* scope 的 fixture。
func newHTTPAttachmentFixture(t *testing.T) httpTokenFixture {
	t.Helper()
	store := openHTTPTestStore(t)
	svc, err := app.NewService(app.ServiceOptions{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	cfg := attachments.DefaultConfig(filepath.Join(t.TempDir(), "attachments"))
	cfg.MaxFileSizeBytes = 256 * 1024
	cfg.MaxResourceTotalSizeBytes = 1 << 20
	cfg.MaxWorkspaceTotalSizeBytes = 2 << 20
	cfg.MaxAttachmentsPerResource = 20
	rt, err := app.NewAttachmentRuntime(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateToken(app.CreateTokenInput{
		Name:          "http-attachment",
		Scopes:        []string{auth.ScopeTaskRead, auth.ScopeTaskWrite},
		WorkspaceRefs: []string{"local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{
		Store:           store,
		ResourceBaseURL: httpTestResourceBaseURL,
		ConfigSecretKey: httpTestSecretKeyBase64(),
		Attachments:     rt,
	})
	return httpTokenFixture{server: server, token: created.RawToken, id: created.View.ID}
}

// createTaskViaHTTP 走 HTTP 创建 task，避免依赖 server 私有字段。
func createTaskViaHTTP(t *testing.T, fixture httpTokenFixture) string {
	t.Helper()
	body := `{"title":"attachment-task"}`
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks?workspace=local", body, authHeader(fixture.token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create task: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			UUID string `json:"uuid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	return resp.Data.UUID
}

func authHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func pngPayload() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// multipartUploadBody 构造 multipart/form-data body。
func multipartUploadBody(t *testing.T, fieldName, fileName string, content []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}

func TestAttachmentUploadRequiresMode(t *testing.T) {
	fixture := newHTTPAttachmentFixture(t)
	taskRef := createTaskViaHTTP(t, fixture)
	payload := pngPayload()
	body, contentType := multipartUploadBody(t, "file", "a.png", payload, map[string]string{})
	headers := authHeader(fixture.token)
	headers["Content-Type"] = contentType
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+taskRef+"/attachments?workspace=local", body.String(), headers)
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 400 or 409 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "attachment_state_invalid") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestTaskCreationDraftAttachmentIsAtomicallyBoundAndActivated(t *testing.T) {
	fixture := newHTTPAttachmentFixture(t)
	draftTarget := uuid.NewString()
	body, contentType := multipartUploadBody(t, "file", "screenshot.png", pngPayload(), nil)
	headers := authHeader(fixture.token)
	headers["Content-Type"] = contentType
	upload := requestHTTPBody(t, fixture.server, http.MethodPost,
		"/api/v1/task-drafts/"+draftTarget+"/attachments?workspace=local", body.String(), headers)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload draft: status=%d body=%s", upload.Code, upload.Body.String())
	}
	var uploaded struct {
		Data struct {
			ID         string `json:"id"`
			State      string `json:"state"`
			AttachedTo struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"attached_to"`
		} `json:"data"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("decode draft upload: %v", err)
	}
	if uploaded.Data.State != "draft" || uploaded.Data.AttachedTo.Type != "task_draft" || uploaded.Data.AttachedTo.ID != draftTarget {
		t.Fatalf("unexpected draft attachment: %#v", uploaded.Data)
	}

	createBody := `{"title":"atomic screenshot","attachment_draft_target":"` + draftTarget + `","description":"![截图](ref://attachment/` + uploaded.Data.ID + `)"}`
	created := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks?workspace=local", createBody, authHeader(fixture.token))
	if created.Code != http.StatusCreated {
		t.Fatalf("create task: status=%d body=%s", created.Code, created.Body.String())
	}
	var createdResponse struct {
		Data struct {
			UUID string `json:"uuid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdResponse); err != nil {
		t.Fatalf("decode created task: %v", err)
	}
	metadata := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/attachments/"+uploaded.Data.ID+"?workspace=local", authHeader(fixture.token))
	if metadata.Code != http.StatusOK {
		t.Fatalf("get attachment: status=%d body=%s", metadata.Code, metadata.Body.String())
	}
	var bound struct {
		Data struct {
			State      string `json:"state"`
			AttachedTo struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"attached_to"`
		} `json:"data"`
	}
	if err := json.Unmarshal(metadata.Body.Bytes(), &bound); err != nil {
		t.Fatalf("decode bound attachment: %v", err)
	}
	if bound.Data.State != "active" || bound.Data.AttachedTo.Type != "task" || bound.Data.AttachedTo.ID != createdResponse.Data.UUID {
		t.Fatalf("attachment was not atomically bound and activated: %#v", bound.Data)
	}
}

func TestAttachmentUploadAndDownloadRoundTrip(t *testing.T) {
	fixture := newHTTPAttachmentFixture(t)
	taskRef := createTaskViaHTTP(t, fixture)
	payload := pngPayload()
	body, contentType := multipartUploadBody(t, "file", "diagram.png", payload, map[string]string{"mode": "attachment"})
	headers := authHeader(fixture.token)
	headers["Content-Type"] = contentType
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+taskRef+"/attachments?workspace=local", body.String(), headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID            string `json:"id"`
			State         string `json:"state"`
			MediaType     string `json:"media_type"`
			ContentURL    string `json:"content_url"`
			InlineCapable bool   `json:"inline_capable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if resp.Data.State != "active" || resp.Data.MediaType != "image/png" || !resp.Data.InlineCapable {
		t.Fatalf("data = %#v", resp.Data)
	}
	if !strings.HasSuffix(resp.Data.ContentURL, "/api/v1/attachments/"+resp.Data.ID+"/content") {
		t.Fatalf("content_url = %q", resp.Data.ContentURL)
	}

	// 下载 content。
	rr2 := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/attachments/"+resp.Data.ID+"/content?workspace=local", authHeader(fixture.token))
	if rr2.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if rr2.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff header missing")
	}
	if rr2.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache-control = %q", rr2.Header().Get("Cache-Control"))
	}
	if !bytes.Equal(rr2.Body.Bytes(), payload) {
		t.Fatalf("downloaded bytes mismatch")
	}
}

func TestAttachmentListAndRenameAndRemove(t *testing.T) {
	fixture := newHTTPAttachmentFixture(t)
	taskRef := createTaskViaHTTP(t, fixture)
	payload := pngPayload()
	body, contentType := multipartUploadBody(t, "file", "a.png", payload, map[string]string{"mode": "attachment"})
	headers := authHeader(fixture.token)
	headers["Content-Type"] = contentType
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+taskRef+"/attachments?workspace=local", body.String(), headers)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	// list
	rrList := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/tasks/"+taskRef+"/attachments?workspace=local", authHeader(fixture.token))
	if rrList.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rrList.Code, rrList.Body.String())
	}

	// rename
	renameBody := `{"display_name":"new-name.png"}`
	rrRename := requestHTTPBody(t, fixture.server, http.MethodPatch, "/api/v1/attachments/"+resp.Data.ID+"?workspace=local", renameBody, authHeader(fixture.token))
	if rrRename.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rrRename.Code, rrRename.Body.String())
	}

	// remove
	rrRemove := requestHTTP(t, fixture.server, http.MethodDelete, "/api/v1/attachments/"+resp.Data.ID+"?workspace=local", authHeader(fixture.token))
	if rrRemove.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rrRemove.Code, rrRemove.Body.String())
	}
}

func TestAttachmentContentMissingReturns404(t *testing.T) {
	fixture := newHTTPAttachmentFixture(t)
	rr := requestHTTP(t, fixture.server, http.MethodGet, "/api/v1/attachments/nonexistent-id/content?workspace=local", authHeader(fixture.token))
	assertHTTPErrorCode(t, rr, http.StatusNotFound, "attachment_not_found")
}

func TestAttachmentUploadRejectsBlockedType(t *testing.T) {
	fixture := newHTTPAttachmentFixture(t)
	taskRef := createTaskViaHTTP(t, fixture)
	body, contentType := multipartUploadBody(t, "file", "evil.exe", []byte("MZ"), map[string]string{"mode": "attachment"})
	headers := authHeader(fixture.token)
	headers["Content-Type"] = contentType
	rr := requestHTTPBody(t, fixture.server, http.MethodPost, "/api/v1/tasks/"+taskRef+"/attachments?workspace=local", body.String(), headers)
	assertHTTPErrorCode(t, rr, http.StatusUnsupportedMediaType, "attachment_type_not_allowed")
}
