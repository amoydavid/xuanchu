package config

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type RuntimeOptions struct {
	ConfigDir   string
	Meta        map[string]string
	Env         map[string]string
	Flags       map[string]string
	RCOverrides map[string]*string
	Defaults    map[string]string
}

type Runtime struct {
	values map[string]string
}

func LoadRuntime(opts RuntimeOptions) (Runtime, error) {
	values := map[string]string{
		"color":       "true",
		"json":        "false",
		"date.format": "rfc3339",
	}
	for key, value := range opts.Defaults {
		values[key] = value
	}
	if opts.ConfigDir != "" {
		tomlValues, err := loadTomlConfig(opts.ConfigDir)
		if errors.Is(err, os.ErrNotExist) {
			tomlValues, err = loadTomlConfig(filepath.Join(opts.ConfigDir, "taskg"))
		}
		if errors.Is(err, os.ErrNotExist) {
			tomlValues = nil
		} else if err != nil {
			return Runtime{}, err
		}
		for key, value := range tomlValues {
			values[key] = value
		}
	}
	for key, value := range opts.Meta {
		values[key] = value
	}
	for key, value := range opts.Env {
		values[key] = value
	}
	for key, value := range opts.Flags {
		values[key] = value
	}
	for key, value := range opts.RCOverrides {
		if value == nil {
			values[key] = ""
			continue
		}
		values[key] = *value
	}
	return Runtime{values: values}, nil
}

func (r Runtime) Get(key string) (string, bool) {
	value, ok := r.values[key]
	return value, ok
}

func (r Runtime) Values() map[string]string {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out
}

func (r Runtime) Keys() []string {
	keys := make([]string, 0, len(r.values))
	for key := range r.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
