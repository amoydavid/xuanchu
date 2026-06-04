package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DatabasePath string
	DatabaseURL  string
	RemoteServer string
	RemoteToken  string
	JSON         bool
	Color        bool
}

type Options struct {
	DataDir string
	DBPath  string
	DBURL   string
	Server  string
	Token   string
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

	var tomlValues map[string]string
	if values, err := loadTomlConfig(configDir(home, env)); err == nil {
		tomlValues = values
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}

	if opts.DBURL != "" && opts.DBPath != "" {
		return Config{}, errors.New("--db-url and --db are mutually exclusive")
	}

	dbURL := opts.DBURL
	if dbURL == "" {
		dbURL = env["TASKG_DB_URL"]
	}
	if dbURL == "" && tomlValues != nil {
		dbURL = tomlValues["database.url"]
	}

	var dbPath string
	if dbURL == "" {
		dbPath = opts.DBPath
		if dbPath == "" {
			dbPath = env["TASKG_DB"]
		}
		if dbPath != "" && strings.Contains(dbPath, "://") {
			return Config{}, errors.New("--db flag does not accept URLs; use --db-url instead")
		}
		if dbPath == "" && opts.DataDir != "" {
			dbPath = filepath.Join(opts.DataDir, "taskg.db")
		}
		if dbPath == "" && tomlValues != nil {
			dbPath = tomlValues["database.path"]
		}
		if dbPath == "" && env["XDG_DATA_HOME"] != "" {
			dbPath = filepath.Join(env["XDG_DATA_HOME"], "taskg", "taskg.db")
		}
		if dbPath == "" {
			dbPath = filepath.Join(home, ".local", "share", "taskg", "taskg.db")
		}
	}

	server := opts.Server
	if server == "" {
		server = env["TASKG_SERVER"]
	}
	if server == "" && tomlValues != nil {
		server = tomlValues["remote.server"]
	}
	token := opts.Token
	if token == "" {
		token = env["TASKG_TOKEN"]
	}
	if token == "" && tomlValues != nil {
		token = tomlValues["remote.token"]
	}

	return Config{
		DatabasePath: dbPath,
		DatabaseURL:  dbURL,
		RemoteServer: server,
		RemoteToken:  token,
		JSON:         opts.JSON,
		Color:        !opts.NoColor,
	}, nil
}

func environ() map[string]string {
	values := map[string]string{}
	for _, key := range []string{"TASKG_DB", "TASKG_DB_URL", "TASKG_SERVER", "TASKG_TOKEN", "XDG_DATA_HOME", "XDG_CONFIG_HOME"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	return values
}

func ConfigDir(home string, env map[string]string) string {
	if env != nil && env["XDG_CONFIG_HOME"] != "" {
		return filepath.Join(env["XDG_CONFIG_HOME"], "taskg")
	}
	return filepath.Join(home, ".config", "taskg")
}

func configDir(home string, env map[string]string) string {
	return ConfigDir(home, env)
}
