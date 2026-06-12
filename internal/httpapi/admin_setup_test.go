package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/config"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

func TestAdminStatusRequiresSetupWhenNoValidAdminTokenExists(t *testing.T) {
	srv := NewServer(Options{
		Store: openHTTPTestStore(t),
		Admin: config.AdminConfig{Enabled: true},
		AdminSetup: AdminSetupOptions{
			Code: "setup-code",
			TTL:  30 * time.Minute,
		},
	})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var status struct {
		Data struct {
			Status        string `json:"status"`
			SetupRequired bool   `json:"setup_required"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Data.Status != "setup_required" || !status.Data.SetupRequired {
		t.Fatalf("status payload = %#v body=%s", status.Data, rr.Body.String())
	}

	rr = requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/session", nil)
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_setup_required")
}

func TestAdminSetupRejectsInvalidCode(t *testing.T) {
	srv := NewServer(Options{
		Store: openHTTPTestStore(t),
		Admin: config.AdminConfig{Enabled: true},
		AdminSetup: AdminSetupOptions{
			Code: "setup-code",
			TTL:  30 * time.Minute,
		},
	})

	rr := requestHTTPBody(t, srv, http.MethodPost, "/api/v1/admin/setup", `{"setup_code":"wrong"}`, map[string]string{
		"Content-Type": "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusUnauthorized, "admin_setup_invalid")
}

func TestAdminSetupCreatesDatabaseAdminTokenAndAllowsImmediateLogin(t *testing.T) {
	store := openHTTPTestStore(t)
	srv := NewServer(Options{
		Store: store,
		Admin: config.AdminConfig{Enabled: true},
		AdminSetup: AdminSetupOptions{
			Code: "setup-code",
			TTL:  30 * time.Minute,
		},
	})

	rr := requestHTTPBody(t, srv, http.MethodPost, "/api/v1/admin/setup", `{"setup_code":"setup-code","name":"primary"}`, map[string]string{
		"Content-Type": "application/json",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		Data struct {
			Token       string `json:"token"`
			TokenName   string `json:"token_name"`
			TokenPrefix string `json:"token_prefix"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Data.Token, auth.AdminTokenPrefix) {
		t.Fatalf("token = %q", created.Data.Token)
	}
	if created.Data.TokenName != "primary" || created.Data.TokenPrefix == "" {
		t.Fatalf("created data = %#v", created.Data)
	}
	if strings.Contains(rr.Body.String(), "sha256:") {
		t.Fatalf("setup response leaked hash: %s", rr.Body.String())
	}

	var rows []storage.ServerAdminToken
	if err := store.DB().Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("server_admin_tokens = %#v", rows)
	}
	if rows[0].TokenHash == created.Data.Token {
		t.Fatalf("stored raw token in DB row: %#v", rows[0])
	}

	rr = requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/session", map[string]string{
		"Authorization": "Bearer " + created.Data.Token,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"token_name":"primary"`) {
		t.Fatalf("session body = %s", rr.Body.String())
	}

	rr = requestHTTPBody(t, srv, http.MethodPost, "/api/v1/admin/setup", `{"setup_code":"setup-code"}`, map[string]string{
		"Content-Type": "application/json",
	})
	assertHTTPErrorCode(t, rr, http.StatusConflict, "admin_setup_completed")
}

func TestAdminStatusUsesConfiguredTokenAsExistingVerifier(t *testing.T) {
	srv := NewServer(Options{
		Store: openHTTPTestStore(t),
		Admin: config.AdminConfig{
			Enabled: true,
			Tokens: []config.AdminTokenConfig{
				{Name: "config-token", Hash: auth.HashAdminToken("xuanchu_admin_config"), Enabled: true},
			},
		},
		AdminSetup: AdminSetupOptions{
			Code: "setup-code",
			TTL:  30 * time.Minute,
		},
	})

	rr := requestHTTP(t, srv, http.MethodGet, "/api/v1/admin/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"login_required"`) ||
		strings.Contains(rr.Body.String(), "setup-code") {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestAdminSetupInstructionsPrintOnlyWhenSetupRequired(t *testing.T) {
	var stderr bytes.Buffer
	store := openHTTPTestStore(t)
	srv := NewServer(Options{
		Store:  store,
		Stderr: &stderr,
		Admin:  config.AdminConfig{Enabled: true},
		AdminSetup: AdminSetupOptions{
			Code: "setup-code",
			TTL:  30 * time.Minute,
		},
	})
	if err := srv.WriteAdminSetupInstructions("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	out := stderr.String()
	if !strings.Contains(out, "setup-code: setup-code") || !strings.Contains(out, "/admin/setup") {
		t.Fatalf("stderr = %q", out)
	}

	stderr.Reset()
	repo := storage.NewServerAdminTokenRepository(store.DB())
	if err := repo.Create(storage.ServerAdminTokenEntry{
		ID:          "admin",
		Name:        "primary",
		TokenPrefix: "xuanchu_admin_a",
		TokenHash:   auth.HashAdminToken("xuanchu_admin_a-secret"),
		Enabled:     true,
		CreatedAt:   100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.WriteAdminSetupInstructions("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty after setup completed", stderr.String())
	}
}
