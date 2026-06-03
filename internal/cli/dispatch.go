package cli

import (
	"fmt"
	"os"

	"github.com/dajee/taskg/internal/config"
	"github.com/dajee/taskg/internal/remote"
)

func resolveConfigFromOpts(opts Options) (config.Config, error) {
	return config.Resolve(config.Options{
		DataDir: opts.DataDir,
		DBPath:  opts.DBPath,
		Server:  opts.Server,
		Token:   opts.Token,
		JSON:    opts.JSON,
		NoColor: opts.NoColor,
		Env:     RuntimeEnv(),
	})
}

func isRemoteMode(opts Options) (bool, config.Config, error) {
	cfg, err := resolveConfigFromOpts(opts)
	if err != nil {
		return false, config.Config{}, err
	}
	return cfg.RemoteServer != "", cfg, nil
}

func buildRemoteClient(opts Options) (*remote.Client, error) {
	cfg, err := resolveConfigFromOpts(opts)
	if err != nil {
		return nil, err
	}
	if cfg.RemoteServer == "" {
		return nil, fmt.Errorf("remote server is required")
	}
	if warning, warnErr := remoteTokenWarning(RuntimeEnv()); warnErr == nil && warning != "" {
		fmt.Fprintln(opts.Stderr, "taskg:", warning)
	}
	return remote.NewClient(remote.Options{
		BaseURL: cfg.RemoteServer,
		Token:   cfg.RemoteToken,
		AsUser:  opts.As,
	})
}

func remoteTokenWarning(env map[string]string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return config.RemoteTokenPermissionWarning(config.ConfigDir(home, env))
}
