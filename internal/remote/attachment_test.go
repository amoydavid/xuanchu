package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAttachmentFixtureServer(t *testing.T, uploadPayload []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tasks/task-1/attachments", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "missing file", http.StatusBadRequest)
				return
			}
			defer file.Close()
			got, _ := io.ReadAll(file)
			if !bytes.Equal(got, uploadPayload) {
				http.Error(w, "payload mismatch", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"data":{"id":"att-1","state":"active","display_name":%q,"media_type":"image/png","size_bytes":%d,"inline_capable":true,"content_url":"/api/v1/attachments/att-1/content"}}`, header.Filename, len(uploadPayload))
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"data":[{"id":"att-1","state":"active"}]}`)
		}
	})
	mux.HandleFunc("/api/v1/attachments/att-1/content", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''test.png`)
		w.WriteHeader(http.StatusOK)
		w.Write(uploadPayload)
	})
	mux.HandleFunc("/api/v1/attachments/att-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"data":{"id":"att-1","display_name":"renamed"}}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"data":{"id":"att-1","state":"active"}}`)
		}
	})
	return httptest.NewServer(mux)
}

func TestUploadAndDownloadAttachmentStreamsBytes(t *testing.T) {
	payload := []byte("payload")
	srv := newAttachmentFixtureServer(t, payload)
	defer srv.Close()

	client, err := NewClient(Options{BaseURL: srv.URL, Token: "token", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.UploadTaskAttachment(context.Background(), "local", "task-1", UploadAttachmentInput{
		Reader:   bytes.NewReader(payload),
		Size:     int64(len(payload)),
		FileName: "需求.txt",
		Mode:     "attachment",
	})
	if err != nil {
		t.Fatalf("UploadTaskAttachment: %v", err)
	}
	if got.State != "active" || got.MediaType != "image/png" {
		t.Fatalf("got = %#v", got)
	}

	var out bytes.Buffer
	result, err := client.DownloadAttachment(context.Background(), "local", got.ID, &out)
	if err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	if out.String() != "payload" {
		t.Fatalf("downloaded = %q", out.String())
	}
	if result.MediaType != "image/png" || result.DisplayName != "test.png" {
		t.Fatalf("download result = %#v", result)
	}
}

func TestListAndRemoveAttachment(t *testing.T) {
	srv := newAttachmentFixtureServer(t, []byte("x"))
	defer srv.Close()
	client, _ := NewClient(Options{BaseURL: srv.URL, Token: "token", HTTPClient: srv.Client()})

	list, err := client.ListTaskAttachments(context.Background(), "local", "task-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != "att-1" {
		t.Fatalf("list = %#v", list)
	}

	renamed, err := client.RenameAttachment(context.Background(), "local", "att-1", "renamed")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.DisplayName != "renamed" {
		t.Fatalf("renamed = %#v", renamed)
	}

	if err := client.RemoveAttachment(context.Background(), "local", "att-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

// 确保 multipart writer 写错误能传播到调用方。
func TestUploadAttachmentPropagatesReaderError(t *testing.T) {
	srv := newAttachmentFixtureServer(t, []byte("x"))
	defer srv.Close()
	client, _ := NewClient(Options{BaseURL: srv.URL, Token: "token", HTTPClient: srv.Client()})
	errReader := &errorReader{}
	_, err := client.UploadTaskAttachment(context.Background(), "local", "task-1", UploadAttachmentInput{
		Reader:   errReader,
		Size:     1,
		FileName: "a.png",
		Mode:     "attachment",
	})
	if err == nil {
		t.Fatal("expected reader error to propagate")
	}
	_ = multipart.NewWriter
}

type errorReader struct{}

func (*errorReader) Read(p []byte) (int, error) { return 0, fmt.Errorf("boom") }

var _ = strings.TrimSpace
