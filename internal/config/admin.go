package config

import (
	"fmt"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/auth"
)

type AdminConfig struct {
	Enabled bool
	Tokens  []AdminTokenConfig
}

type AdminTokenConfig struct {
	Name        string
	Hash        string
	HashEnv     string
	Enabled     bool
	Description string
}

type adminTomlRoot struct {
	Server struct {
		Admin struct {
			Enabled bool `toml:"enabled"`
			Tokens  []struct {
				Name        string `toml:"name"`
				Hash        string `toml:"hash"`
				HashEnv     string `toml:"hash_env"`
				Enabled     *bool  `toml:"enabled"`
				Description string `toml:"description"`
			} `toml:"tokens"`
		} `toml:"admin"`
	} `toml:"server"`
}

func normalizeAdminConfig(raw adminTomlRoot, env map[string]string) (AdminConfig, error) {
	cfg := AdminConfig{Enabled: raw.Server.Admin.Enabled}
	if !cfg.Enabled {
		return cfg, nil
	}
	seen := map[string]struct{}{}
	for _, token := range raw.Server.Admin.Tokens {
		name := strings.TrimSpace(token.Name)
		if name == "" {
			return AdminConfig{}, fmt.Errorf("admin token name is required")
		}
		if _, ok := seen[name]; ok {
			return AdminConfig{}, fmt.Errorf("duplicate admin token name %q", name)
		}
		seen[name] = struct{}{}
		enabled := true
		if token.Enabled != nil {
			enabled = *token.Enabled
		}
		hash := strings.TrimSpace(token.Hash)
		hashEnv := strings.TrimSpace(token.HashEnv)
		if enabled {
			switch {
			case hash != "" && hashEnv != "":
				return AdminConfig{}, fmt.Errorf("admin token %q must use either hash or hash_env", name)
			case hash == "" && hashEnv == "":
				return AdminConfig{}, fmt.Errorf("admin token %q requires hash or hash_env", name)
			case hashEnv != "":
				value := strings.TrimSpace(env[hashEnv])
				if value == "" {
					return AdminConfig{}, fmt.Errorf("admin token %q hash_env %s is empty", name, hashEnv)
				}
				hash = value
			}
			if !auth.ValidAdminTokenHash(hash) {
				return AdminConfig{}, fmt.Errorf("admin token %q has invalid hash", name)
			}
		}
		cfg.Tokens = append(cfg.Tokens, AdminTokenConfig{
			Name:        name,
			Hash:        hash,
			HashEnv:     hashEnv,
			Enabled:     enabled,
			Description: strings.TrimSpace(token.Description),
		})
	}
	return cfg, nil
}
