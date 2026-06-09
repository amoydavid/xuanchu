package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

func loadTomlConfig(dir string) (map[string]string, error) {
	_, values, err := loadTomlConfigWithPath(dir)
	return values, err
}

func loadTomlConfigWithPath(dir string) (string, map[string]string, error) {
	path, err := findTomlConfigPath(dir)
	if err != nil {
		return "", nil, err
	}
	values, err := loadTomlConfigFile(path)
	if err != nil {
		return "", nil, err
	}
	return path, values, nil
}

func findTomlConfigPath(dir string) (string, error) {
	for _, path := range []string{
		filepath.Join(dir, "xuanchu.toml"),
		filepath.Join(dir, "xuanchu", "xuanchu.toml"),
	} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", os.ErrNotExist
}

func loadTomlConfigFile(path string) (map[string]string, error) {
	raw := map[string]any{}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return flattenTomlMap(raw, ""), nil
}

func loadTomlAdminConfigFile(path string, env map[string]string) (AdminConfig, error) {
	var raw adminTomlRoot
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AdminConfig{}, os.ErrNotExist
		}
		return AdminConfig{}, err
	}
	return normalizeAdminConfig(raw, env)
}

func flattenTomlMap(values map[string]any, prefix string) map[string]string {
	out := map[string]string{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		fullKey = normalizeTomlKey(fullKey)
		for nestedKey, value := range flattenTomlValue(fullKey, values[key]) {
			out[nestedKey] = value
		}
	}
	return out
}

func flattenTomlValue(key string, value any) map[string]string {
	switch v := value.(type) {
	case map[string]any:
		return flattenTomlMap(v, key)
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, tomlScalarString(item))
		}
		return map[string]string{key: strings.Join(items, ",")}
	default:
		return map[string]string{key: tomlScalarString(v)}
	}
}

func normalizeTomlKey(key string) string {
	switch key {
	case "display.color":
		return "color"
	case "display.json":
		return "json"
	default:
		return key
	}
}

func tomlScalarString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}
