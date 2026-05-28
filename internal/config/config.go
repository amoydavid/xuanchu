package config

import (
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	DatabasePath string
	JSON         bool
	Color        bool
}

type Options struct {
	DataDir string
	DBPath  string
	JSON    bool
	NoColor bool
	Env     map[string]string
	HomeDir string
}

func Resolve(opts Options) (Config, error) {
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Config{}, err
		}
	}
	if home == "" {
		return Config{}, errors.New("home directory is required")
	}

	env := opts.Env
	if env == nil {
		env = environ()
	}

	dbPath := opts.DBPath
	if dbPath == "" {
		dbPath = env["TASKG_DB"]
	}
	if dbPath == "" && opts.DataDir != "" {
		dbPath = filepath.Join(opts.DataDir, "taskg.db")
	}
	if dbPath == "" {
		if values, err := loadTomlConfig(configDir(home, env)); err == nil {
			dbPath = values["database.path"]
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}
	if dbPath == "" && env["XDG_DATA_HOME"] != "" {
		dbPath = filepath.Join(env["XDG_DATA_HOME"], "taskg", "taskg.db")
	}
	if dbPath == "" {
		dbPath = filepath.Join(home, ".local", "share", "taskg", "taskg.db")
	}

	return Config{
		DatabasePath: dbPath,
		JSON:         opts.JSON,
		Color:        !opts.NoColor,
	}, nil
}

func environ() map[string]string {
	values := map[string]string{}
	for _, key := range []string{"TASKG_DB", "XDG_DATA_HOME", "XDG_CONFIG_HOME"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	return values
}

func configDir(home string, env map[string]string) string {
	if env != nil && env["XDG_CONFIG_HOME"] != "" {
		return filepath.Join(env["XDG_CONFIG_HOME"], "taskg")
	}
	return filepath.Join(home, ".config", "taskg")
}
