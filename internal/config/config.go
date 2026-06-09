package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"git.dajee.net/dajee/xuanchu/internal/logging"
)

type Config struct {
	DatabasePath string
	DatabaseURL  string
	RemoteServer string
	RemoteToken  string
	JSON         bool
	Color        bool
	Log          logging.LogConfig
	ServerAdmin  AdminConfig
}

type Options struct {
	DataDir    string
	DBPath     string
	DBURL      string
	Server     string
	Token      string
	JSON       bool
	NoColor    bool
	Env        map[string]string
	HomeDir    string
	ConfigPath string
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

	var tomlPath string
	var tomlValues map[string]string
	if opts.ConfigPath != "" {
		tomlPath = opts.ConfigPath
		values, err := loadTomlConfigFile(tomlPath)
		if err != nil {
			return Config{}, fmt.Errorf("--config: %w", err)
		}
		tomlValues = values
	} else if configPath := env["XUANCHU_CONFIG"]; configPath != "" {
		tomlPath = configPath
		values, err := loadTomlConfigFile(tomlPath)
		if err != nil {
			return Config{}, fmt.Errorf("XUANCHU_CONFIG: %w", err)
		}
		tomlValues = values
	} else if path, values, err := loadTomlConfigWithPath(configDir(home, env)); err == nil {
		tomlPath = path
		tomlValues = values
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}

	if opts.DBURL != "" && opts.DBPath != "" {
		return Config{}, errors.New("--db-url and --db are mutually exclusive")
	}

	dbURL := opts.DBURL
	if dbURL == "" {
		dbURL = env["XUANCHU_DB_URL"]
	}
	if dbURL == "" && tomlValues != nil {
		dbURL = tomlValues["database.url"]
	}

	var dbPath string
	if dbURL == "" {
		dbPath = opts.DBPath
		if dbPath == "" {
			dbPath = env["XUANCHU_DB"]
		}
		if dbPath != "" && strings.Contains(dbPath, "://") {
			return Config{}, errors.New("--db flag does not accept URLs; use --db-url instead")
		}
		if dbPath == "" && opts.DataDir != "" {
			dbPath = filepath.Join(opts.DataDir, "xuanchu.db")
		}
		if dbPath == "" && tomlValues != nil {
			dbPath = tomlValues["database.path"]
		}
		if dbPath == "" && env["XDG_DATA_HOME"] != "" {
			dbPath = filepath.Join(env["XDG_DATA_HOME"], "xuanchu", "xuanchu.db")
		}
		if dbPath == "" {
			dbPath = filepath.Join(home, ".local", "share", "xuanchu", "xuanchu.db")
		}
	}

	server := opts.Server
	if server == "" {
		server = env["XUANCHU_SERVER"]
	}
	if server == "" && tomlValues != nil {
		server = tomlValues["remote.server"]
	}
	token := opts.Token
	if token == "" {
		token = env["XUANCHU_TOKEN"]
	}
	if token == "" && tomlValues != nil {
		token = tomlValues["remote.token"]
	}

	logCfg := parseLogConfig(tomlValues)
	if v := env["XUANCHU_LOG_LEVEL"]; v != "" {
		logCfg.Level = v
	}
	if v := env["XUANCHU_LOG_FILE"]; v != "" {
		if logCfg.File == nil {
			logCfg.File = &logging.FileConfig{}
		}
		logCfg.File.Path = v
	}
	var adminCfg AdminConfig
	if tomlPath != "" {
		var err error
		adminCfg, err = loadTomlAdminConfigFile(tomlPath, env)
		if err != nil {
			return Config{}, err
		}
	}

	return Config{
		DatabasePath: dbPath,
		DatabaseURL:  dbURL,
		RemoteServer: server,
		RemoteToken:  token,
		JSON:         opts.JSON,
		Color:        !opts.NoColor,
		Log:          logCfg,
		ServerAdmin:  adminCfg,
	}, nil
}

func environ() map[string]string {
	values := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok && value != "" {
			values[key] = value
		}
	}
	return values
}

func ConfigDir(home string, env map[string]string) string {
	if env != nil && env["XDG_CONFIG_HOME"] != "" {
		return filepath.Join(env["XDG_CONFIG_HOME"], "xuanchu")
	}
	return filepath.Join(home, ".config", "xuanchu")
}

func configDir(home string, env map[string]string) string {
	return ConfigDir(home, env)
}

func parseLogConfig(values map[string]string) logging.LogConfig {
	cfg := logging.LogConfig{}
	if values == nil {
		return cfg
	}
	if v, ok := values["log.level"]; ok {
		cfg.Level = v
	}
	if v, ok := values["log.format"]; ok {
		cfg.Format = v
	}
	if _, ok := values["log.file.path"]; ok {
		fc := &logging.FileConfig{Path: values["log.file.path"]}
		if v, ok := values["log.file.rotate"]; ok {
			fc.Rotate = v
		}
		if v, ok := values["log.file.max_size_mb"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				fc.MaxSizeMB = n
			}
		}
		if v, ok := values["log.file.max_age_days"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				fc.MaxAgeDays = n
			}
		}
		cfg.File = fc
	}
	return cfg
}
