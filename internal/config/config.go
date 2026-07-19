package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/logging"
)

type Config struct {
	DatabasePath           string
	DatabaseURL            string
	PublicBaseURL          string
	RemoteServer           string
	RemoteToken            string
	JSON                   bool
	Color                  bool
	Log                    logging.LogConfig
	ServerAdmin            AdminConfig
	ServerMCP              MCPConfig
	Console                ConsoleConfig
	NotificationDispatcher DispatcherConfig
	HookDispatcher         DispatcherConfig
	Shutdown               ShutdownConfig
	SecretKey              string
}

// ResourceBaseURL 返回 Web Console 资源 URL 的统一前缀。
// public base 未配置时返回空字符串，调用方不得退化为相对 URL。
func (c Config) ResourceBaseURL() string {
	if c.PublicBaseURL == "" {
		return ""
	}
	if c.Console.BasePath == "/" || c.Console.BasePath == "" {
		return c.PublicBaseURL
	}
	return c.PublicBaseURL + c.Console.BasePath
}

func (c Config) DatabaseTarget() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	return c.DatabasePath
}

type ConsoleConfig struct {
	Enabled     bool
	BasePath    string
	AssetsCache time.Duration
	AuthMode    string
}

type MCPConfig struct {
	TrustedProxyHosts []string
}

type DispatcherConfig struct {
	MaxConcurrency int
	BatchSize      int
	PrefetchFactor int
	PollInterval   time.Duration
	ClaimTTL       time.Duration
}

type ShutdownConfig struct {
	Timeout      time.Duration
	ForceTimeout time.Duration
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

	notificationDispatcher, err := parseDispatcherConfig(tomlValues, "notifications.dispatcher")
	if err != nil {
		return Config{}, err
	}
	hookDispatcher, err := parseDispatcherConfig(tomlValues, "hooks.dispatcher")
	if err != nil {
		return Config{}, err
	}
	serverMCP, err := parseMCPConfig(tomlValues)
	if err != nil {
		return Config{}, err
	}
	consoleCfg, err := parseConsoleConfig(tomlValues, env)
	if err != nil {
		return Config{}, err
	}
	publicBaseURL := ""
	if tomlValues != nil {
		publicBaseURL = tomlValues["server.public_base_url"]
	}
	if value := env["XUANCHU_PUBLIC_BASE_URL"]; value != "" {
		publicBaseURL = value
	}
	publicBaseURL, err = ValidatePublicBaseURL(publicBaseURL)
	if err != nil {
		return Config{}, err
	}
	shutdownCfg, err := parseShutdownConfig(tomlValues)
	if err != nil {
		return Config{}, err
	}

	secretKey := ""
	if tomlValues != nil {
		secretKey = tomlValues["security.config_secret_key"]
	}

	return Config{
		DatabasePath:           dbPath,
		DatabaseURL:            dbURL,
		PublicBaseURL:          publicBaseURL,
		RemoteServer:           server,
		RemoteToken:            token,
		JSON:                   opts.JSON,
		Color:                  !opts.NoColor,
		Log:                    logCfg,
		ServerAdmin:            adminCfg,
		ServerMCP:              serverMCP,
		Console:                consoleCfg,
		NotificationDispatcher: notificationDispatcher,
		HookDispatcher:         hookDispatcher,
		Shutdown:               shutdownCfg,
		SecretKey:              secretKey,
	}, nil
}

// ValidatePublicBaseURL 校验并规范化 Web Console 对外 origin。
// 这里只接受 origin；Console 部署前缀由 server.console.base_path 单独负责。
func ValidatePublicBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("server.public_base_url must be an absolute http(s) origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("server.public_base_url must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("server.public_base_url must not contain userinfo, path, query, or fragment")
	}
	return strings.TrimSuffix(value, "/"), nil
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
	if path, ok := firstValue(values, "log.file.path", "log.file"); ok {
		fc := &logging.FileConfig{Path: path}
		if v, ok := firstValue(values, "log.file.rotate", "log.rotate"); ok {
			fc.Rotate = v
		}
		if v, ok := firstValue(values, "log.file.max_size_mb", "log.max_size_mb"); ok {
			if n, err := strconv.Atoi(v); err == nil {
				fc.MaxSizeMB = n
			}
		}
		if v, ok := firstValue(values, "log.file.max_age_days", "log.max_age_days"); ok {
			if n, err := strconv.Atoi(v); err == nil {
				fc.MaxAgeDays = n
			}
		}
		cfg.File = fc
	}
	return cfg
}

func parseMCPConfig(values map[string]string) (MCPConfig, error) {
	cfg := MCPConfig{}
	if values == nil {
		return cfg, nil
	}
	raw := values["server.mcp.trusted_proxy_hosts"]
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	seen := map[string]struct{}{}
	for _, item := range strings.Split(raw, ",") {
		host := normalizeTrustedProxyHost(item)
		if host == "" {
			continue
		}
		if strings.Contains(host, "*") {
			return MCPConfig{}, errors.New("server.mcp.trusted_proxy_hosts cannot contain wildcard host")
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		cfg.TrustedProxyHosts = append(cfg.TrustedProxyHosts, host)
	}
	return cfg, nil
}

func parseConsoleConfig(values map[string]string, env map[string]string) (ConsoleConfig, error) {
	cfg := ConsoleConfig{
		Enabled:     true,
		BasePath:    "/",
		AssetsCache: time.Hour,
		AuthMode:    "bearer",
	}
	var err error
	if values != nil {
		if v, ok := values["server.console.enabled"]; ok && v != "" {
			cfg.Enabled, err = strconv.ParseBool(v)
			if err != nil {
				return ConsoleConfig{}, fmt.Errorf("server.console.enabled must be a boolean")
			}
		}
		if v, ok := values["server.console.base_path"]; ok {
			cfg.BasePath = v
		}
		if v, ok := values["server.console.assets_cache"]; ok && v != "" {
			cfg.AssetsCache, err = time.ParseDuration(v)
			if err != nil || cfg.AssetsCache < 0 {
				return ConsoleConfig{}, fmt.Errorf("server.console.assets_cache must be a non-negative duration")
			}
		}
		if v, ok := values["server.console.auth_mode"]; ok && v != "" {
			cfg.AuthMode = v
		}
	}
	if v := env["XUANCHU_CONSOLE_ENABLED"]; v != "" {
		cfg.Enabled, err = strconv.ParseBool(v)
		if err != nil {
			return ConsoleConfig{}, fmt.Errorf("XUANCHU_CONSOLE_ENABLED must be a boolean")
		}
	}
	if v, ok := env["XUANCHU_CONSOLE_BASE_PATH"]; ok {
		cfg.BasePath = v
	}
	if err := ValidateConsoleBasePath(cfg.BasePath); err != nil {
		return ConsoleConfig{}, err
	}
	if cfg.AuthMode != "bearer" {
		return ConsoleConfig{}, fmt.Errorf("server.console.auth_mode must be bearer")
	}
	return cfg, nil
}

func ValidateConsoleBasePath(basePath string) error {
	if !strings.HasPrefix(basePath, "/") {
		return fmt.Errorf("server.console.base_path must be an absolute path")
	}
	if basePath != "/" && strings.HasSuffix(basePath, "/") {
		return fmt.Errorf("server.console.base_path must not end with /")
	}
	for _, reserved := range []string{"/api", "/api/v1", "/mcp", "/healthz"} {
		if basePath == reserved || strings.HasPrefix(basePath, reserved+"/") {
			return fmt.Errorf("server.console.base_path conflicts with reserved route %s", reserved)
		}
	}
	return nil
}

func normalizeTrustedProxyHost(value string) string {
	host := strings.ToLower(strings.TrimSpace(value))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.HasPrefix(host, "[") && strings.Contains(host, "]") {
		if end := strings.Index(host, "]"); end > 0 {
			host = host[1:end]
		}
	}
	host = strings.Trim(host, "[]")
	return host
}

func firstValue(values map[string]string, keys ...string) (string, bool) {
	for _, key := range keys {
		if v, ok := values[key]; ok {
			return v, true
		}
	}
	return "", false
}

func parseDispatcherConfig(values map[string]string, prefix string) (DispatcherConfig, error) {
	cfg := DispatcherConfig{
		MaxConcurrency: 1,
		BatchSize:      50,
		PrefetchFactor: 1,
		PollInterval:   5 * time.Second,
		ClaimTTL:       5 * time.Minute,
	}
	if values == nil {
		return cfg, nil
	}
	var err error
	if cfg.MaxConcurrency, err = parsePositiveIntField(values, prefix+".max_concurrency", cfg.MaxConcurrency); err != nil {
		return DispatcherConfig{}, err
	}
	if cfg.BatchSize, err = parsePositiveIntField(values, prefix+".batch_size", cfg.BatchSize); err != nil {
		return DispatcherConfig{}, err
	}
	if cfg.PrefetchFactor, err = parsePositiveIntField(values, prefix+".prefetch_factor", cfg.PrefetchFactor); err != nil {
		return DispatcherConfig{}, err
	}
	if cfg.PollInterval, err = parsePositiveDurationField(values, prefix+".poll_interval", cfg.PollInterval); err != nil {
		return DispatcherConfig{}, err
	}
	if cfg.ClaimTTL, err = parsePositiveDurationField(values, prefix+".claim_ttl", cfg.ClaimTTL); err != nil {
		return DispatcherConfig{}, err
	}
	return cfg, nil
}

func parseShutdownConfig(values map[string]string) (ShutdownConfig, error) {
	cfg := ShutdownConfig{
		Timeout:      30 * time.Second,
		ForceTimeout: 5 * time.Second,
	}
	if values == nil {
		return cfg, nil
	}
	var err error
	if cfg.Timeout, err = parsePositiveDurationField(values, "server.shutdown.timeout", cfg.Timeout); err != nil {
		return ShutdownConfig{}, err
	}
	if cfg.ForceTimeout, err = parsePositiveDurationField(values, "server.shutdown.force_timeout", cfg.ForceTimeout); err != nil {
		return ShutdownConfig{}, err
	}
	return cfg, nil
}

func parsePositiveIntField(values map[string]string, key string, fallback int) (int, error) {
	value, ok := values[key]
	if !ok || value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}

func parsePositiveDurationField(values map[string]string, key string, fallback time.Duration) (time.Duration, error) {
	value, ok := values[key]
	if !ok || value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return d, nil
}
