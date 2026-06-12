package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"

	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/auth"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

var (
	errAdminSetupInvalid = errors.New("admin setup code is invalid")
	errAdminSetupExpired = errors.New("admin setup code expired")
	errAdminSetupUsed    = errors.New("admin setup code already used")
)

type adminSetupState struct {
	mu        sync.Mutex
	code      string
	expiresAt int64
	used      bool
}

func newAdminSetupState(opts AdminSetupOptions, clock app.Clock) *adminSetupState {
	if opts.TTL <= 0 {
		opts.TTL = defaultAdminSetupTTL
	}
	code := strings.TrimSpace(opts.Code)
	if code == "" {
		code = generateAdminSetupCode()
	}
	if clock == nil {
		clock = systemClock{}
	}
	return &adminSetupState{
		code:      code,
		expiresAt: clock.Unix() + int64(opts.TTL.Seconds()),
	}
}

func (s *adminSetupState) Code() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code
}

func (s *adminSetupState) ExpiresAt() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expiresAt
}

func (s *adminSetupState) Validate(code string, now int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used {
		return errAdminSetupUsed
	}
	if s.expiresAt > 0 && now > s.expiresAt {
		return errAdminSetupExpired
	}
	if subtleStringEqual(strings.TrimSpace(code), s.code) {
		return nil
	}
	return errAdminSetupInvalid
}

func (s *adminSetupState) MarkUsed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.used = true
}

type adminStatusResponse struct {
	Status             string `json:"status"`
	Enabled            bool   `json:"enabled"`
	SetupRequired      bool   `json:"setup_required"`
	SetupCodeExpiresAt int64  `json:"setup_code_expires_at,omitempty"`
}

type adminSetupRequest struct {
	SetupCode string `json:"setup_code"`
	Name      string `json:"name,omitempty"`
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.adminStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	writeSuccess(w, http.StatusOK, status, nil)
}

func (s *Server) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	if !s.admin.Enabled {
		writeError(w, http.StatusNotFound, "route_not_found", "route not found", nil)
		return
	}
	required, err := s.adminSetupRequired()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	if !required {
		writeError(w, http.StatusConflict, "admin_setup_completed", "admin setup is already completed", nil)
		return
	}
	var req adminSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "api_bad_json", "invalid json body", nil)
		return
	}
	if err := s.adminSetup.Validate(req.SetupCode, s.effectiveClock().Unix()); err != nil {
		writeError(w, http.StatusUnauthorized, "admin_setup_invalid", "admin setup code is invalid", nil)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "primary"
	}
	raw, hash, err := auth.GenerateAdminToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	prefix := auth.AdminTokenDisplayPrefix(raw)
	repo := storage.NewServerAdminTokenRepository(s.store.DB())
	if err := repo.Create(storage.ServerAdminTokenEntry{
		ID:          uuid.NewString(),
		Name:        name,
		TokenPrefix: prefix,
		TokenHash:   hash,
		Enabled:     true,
		CreatedAt:   s.effectiveClock().Unix(),
		Description: "created by admin setup",
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "api_internal", "internal server error", nil)
		return
	}
	s.adminSetup.MarkUsed()
	writeSuccess(w, http.StatusCreated, map[string]any{
		"token":        raw,
		"token_name":   name,
		"token_prefix": prefix,
	}, nil)
}

func (s *Server) WriteAdminSetupInstructions(baseURL string) error {
	required, err := s.adminSetupRequired()
	if err != nil {
		return err
	}
	if !required {
		return nil
	}
	setupURL := "/admin/setup"
	if trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/"); trimmed != "" {
		setupURL = trimmed + setupURL
	}
	_, err = fmt.Fprintf(s.stderr, "xuanchu: server admin setup required\nsetup-code: %s\nopen: %s\n", s.adminSetup.Code(), setupURL)
	return err
}

func (s *Server) adminStatus() (adminStatusResponse, error) {
	if !s.admin.Enabled {
		return adminStatusResponse{Status: "disabled", Enabled: false}, nil
	}
	required, err := s.adminSetupRequired()
	if err != nil {
		return adminStatusResponse{}, err
	}
	if required {
		return adminStatusResponse{
			Status:             "setup_required",
			Enabled:            true,
			SetupRequired:      true,
			SetupCodeExpiresAt: s.adminSetup.ExpiresAt(),
		}, nil
	}
	return adminStatusResponse{Status: "login_required", Enabled: true}, nil
}

func (s *Server) adminSetupRequired() (bool, error) {
	if !s.admin.Enabled {
		return false, nil
	}
	if s.configuredAdminVerifierExists() {
		return false, nil
	}
	rows, err := storage.NewServerAdminTokenRepository(s.store.DB()).ListValid()
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if auth.ValidAdminTokenHash(row.TokenHash) {
			return false, nil
		}
	}
	return true, nil
}

func (s *Server) configuredAdminVerifierExists() bool {
	for _, token := range s.admin.Tokens {
		if token.Enabled && auth.ValidAdminTokenHash(token.Hash) {
			return true
		}
	}
	return false
}

func subtleStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
