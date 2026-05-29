package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
	"github.com/dajee/taskg/internal/uda"
)

func (s *Service) DefineUDA(name, typ, label string, values []string, defaultValue string) error {
	def := uda.Definition{Name: strings.TrimSpace(name), Type: uda.Type(strings.TrimSpace(typ)), Label: label, Values: values, Default: defaultValue}
	if def.Type == "" {
		def.Type = uda.TypeString
	}
	if def.Default != "" {
		normalized, err := uda.NormalizeValue(def, def.Default)
		if err != nil {
			return err
		}
		def.Default = normalized
	}
	return s.udaRepo.UpsertDefinition(s.workspaceID, def, s.clock.Unix())
}

func (s *Service) DeleteUDA(name string) error {
	return s.udaRepo.DeleteDefinition(s.workspaceID, strings.TrimPrefix(name, "uda."))
}

func (s *Service) ListUDAs() ([]uda.Definition, error) {
	defs, err := s.udaRepo.ListDefinitions(s.workspaceID)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]uda.Definition, len(defs)+len(s.runtimeUDAs))
	for name, def := range s.runtimeUDAs {
		byName[name] = def
	}
	for _, def := range defs {
		byName[def.Name] = def
	}
	out := make([]uda.Definition, 0, len(byName))
	for _, def := range byName {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Service) udaDefinitionTypes() (map[string]string, error) {
	defs, err := s.ListUDAs()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(defs))
	for _, def := range defs {
		out[def.Name] = string(def.Type)
	}
	return out, nil
}

func (s *Service) SetConfig(key, value string) error {
	if strings.HasPrefix(key, "uda.") {
		return s.setUDAConfig(key, value)
	}
	if key == "database.path" {
		return fmt.Errorf("database.path is read-only; use --db or TASKG_DB")
	}
	if key == "context.active" {
		return fmt.Errorf("context.active is managed by context commands")
	}
	return s.store.SetMeta(key, value)
}

func (s *Service) GetConfig(key string) (string, bool, error) {
	if strings.HasPrefix(key, "uda.") {
		return s.getUDAConfig(key)
	}
	if value, ok := s.runtimeOverrides[key]; ok {
		return value, true, nil
	}
	value, ok, err := s.store.GetMeta(key)
	if err != nil {
		return "", false, err
	}
	if ok {
		return value, true, nil
	}
	if value, ok := s.runtimeConfig[key]; ok {
		return value, true, nil
	}
	return "", false, nil
}

func (s *Service) UnsetConfig(key string) error {
	if strings.HasPrefix(key, "uda.") {
		return s.unsetUDAConfig(key)
	}
	if key == "database.path" {
		return fmt.Errorf("database.path is read-only; use --db or TASKG_DB")
	}
	if key == "context.active" {
		return fmt.Errorf("context.active is managed by context commands")
	}
	return s.store.DeleteMeta(key)
}

func (s *Service) ConfigValues() (map[string]string, error) {
	values, err := s.mergedConfigValues()
	if err != nil {
		return nil, err
	}
	defs, err := s.ListUDAs()
	if err != nil {
		return nil, err
	}
	for _, def := range defs {
		prefix := "uda." + def.Name + "."
		values[prefix+"type"] = string(def.Type)
		if def.Label != "" {
			values[prefix+"label"] = def.Label
		}
		if len(def.Values) > 0 {
			values[prefix+"values"] = strings.Join(def.Values, ",")
		}
		if def.Default != "" {
			values[prefix+"default"] = def.Default
		}
	}
	return values, nil
}

func (s *Service) UniqueValues(field string, input ListInput) ([]string, error) {
	field = strings.TrimSpace(field)
	tasks, err := s.List(input)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, tsk := range tasks {
		switch field {
		case "project":
			if tsk.Project != nil && *tsk.Project != "" {
				seen[*tsk.Project] = true
			}
		case "priority":
			if tsk.Priority != nil && *tsk.Priority != "" {
				seen[*tsk.Priority] = true
			}
		case "tags":
			for _, tag := range tsk.Tags {
				if tag != "" {
					seen[tag] = true
				}
			}
		default:
			if value, ok := tsk.UDAs[strings.TrimPrefix(field, "uda.")]; ok && value.Raw != "" {
				seen[value.Raw] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Service) setUDAConfig(key, value string) error {
	name, field, err := splitUDAConfigKey(key)
	if err != nil {
		return err
	}
	def, err := s.udaRepo.GetDefinition(s.workspaceID, name)
	if err == sqlite.ErrNotFound {
		def = uda.Definition{Name: name, Type: uda.TypeString}
	} else if err != nil {
		return err
	}
	switch field {
	case "type":
		def.Type = uda.Type(value)
	case "label":
		def.Label = value
	case "values":
		def.Values = uda.ParseValuesCSV(value)
	case "default":
		def.Default = value
	default:
		return fmt.Errorf("unknown UDA config field %q", field)
	}
	return s.udaRepo.UpsertDefinition(s.workspaceID, def, s.clock.Unix())
}

func (s *Service) getUDAConfig(key string) (string, bool, error) {
	name, field, err := splitUDAConfigKey(key)
	if err != nil {
		return "", false, err
	}
	def, err := s.udaDefinition(name)
	if err == sqlite.ErrNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	switch field {
	case "type":
		return string(def.Type), true, nil
	case "label":
		if def.Label == "" {
			return "", false, nil
		}
		return def.Label, true, nil
	case "values":
		if len(def.Values) == 0 {
			return "", false, nil
		}
		return strings.Join(def.Values, ","), true, nil
	case "default":
		if def.Default == "" {
			return "", false, nil
		}
		return def.Default, true, nil
	default:
		return "", false, fmt.Errorf("unknown UDA config field %q", field)
	}
}

func (s *Service) unsetUDAConfig(key string) error {
	name, field, err := splitUDAConfigKey(key)
	if err != nil {
		return err
	}
	if field == "type" {
		return s.DeleteUDA(name)
	}
	def, err := s.udaRepo.GetDefinition(s.workspaceID, name)
	if err == sqlite.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	switch field {
	case "label":
		def.Label = ""
	case "values":
		def.Values = nil
	case "default":
		def.Default = ""
	default:
		return fmt.Errorf("unknown UDA config field %q", field)
	}
	return s.udaRepo.UpsertDefinition(s.workspaceID, def, s.clock.Unix())
}

func splitUDAConfigKey(key string) (string, string, error) {
	body := strings.TrimPrefix(key, "uda.")
	parts := strings.Split(body, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid UDA config key %q", key)
	}
	field := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-1], ".")
	if name == "" || field == "" {
		return "", "", fmt.Errorf("invalid UDA config key %q", key)
	}
	return name, field, nil
}

func (s *Service) normalizeUDAModifications(existing map[string]task.UDAValue, set map[string]string, clear []string, allowOrphan bool) (map[string]task.UDAValue, error) {
	out := cloneUDAs(existing)
	for _, name := range clear {
		name = strings.TrimPrefix(strings.TrimSpace(name), "uda.")
		if current, ok := out[name]; ok && current.Orphan && !allowOrphan {
			return nil, fmt.Errorf("modifying orphan UDA %q is not allowed", name)
		}
		if _, err := s.udaDefinition(name); err == sqlite.ErrNotFound && !allowOrphan {
			return nil, fmt.Errorf("UDA %q is not defined", name)
		} else if err != nil {
			return nil, err
		}
		delete(out, name)
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := set[name]
		name = strings.TrimPrefix(strings.TrimSpace(name), "uda.")
		if current, ok := out[name]; ok && current.Orphan && !allowOrphan {
			return nil, fmt.Errorf("modifying orphan UDA %q is not allowed", name)
		}
		def, err := s.udaDefinition(name)
		if err == sqlite.ErrNotFound {
			if !allowOrphan {
				return nil, fmt.Errorf("UDA %q is not defined", name)
			}
			out[name] = task.UDAValue{Name: name, Raw: raw, Orphan: true}
			continue
		}
		if err != nil {
			return nil, err
		}
		normalized, err := uda.NormalizeValue(def, raw)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			delete(out, name)
			continue
		}
		out[name] = task.UDAValue{Name: name, Raw: normalized, Type: string(def.Type)}
	}
	return out, nil
}

func (s *Service) normalizeImportedUDAs(values map[string]task.UDAValue) (map[string]task.UDAValue, error) {
	out := map[string]task.UDAValue{}
	for name, value := range values {
		name = strings.TrimPrefix(strings.TrimSpace(name), "uda.")
		def, err := s.udaDefinition(name)
		if err == sqlite.ErrNotFound {
			if value.Raw != "" {
				out[name] = task.UDAValue{Name: name, Raw: value.Raw, Type: value.Type, Orphan: true}
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		normalized, err := uda.NormalizeValue(def, value.Raw)
		if err != nil {
			return nil, err
		}
		if normalized != "" {
			out[name] = task.UDAValue{Name: name, Raw: normalized, Type: string(def.Type)}
		}
	}
	return out, nil
}

func (s *Service) udaDefinition(name string) (uda.Definition, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "uda.")
	def, err := s.udaRepo.GetDefinition(s.workspaceID, name)
	if err == nil {
		return def, nil
	}
	if err != sqlite.ErrNotFound {
		return uda.Definition{}, err
	}
	if def, ok := s.runtimeUDAs[name]; ok {
		return def, nil
	}
	return uda.Definition{}, sqlite.ErrNotFound
}

func (s *Service) mergedConfigValues() (map[string]string, error) {
	values := cloneStringMap(s.runtimeConfig)
	if values == nil {
		values = map[string]string{}
	}
	meta, err := s.store.ListMeta()
	if err != nil {
		return nil, err
	}
	for key, value := range meta {
		values[key] = value
	}
	for key, value := range s.runtimeOverrides {
		values[key] = value
	}
	return values, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func udaDefinitionsFromConfig(values map[string]string) (map[string]uda.Definition, error) {
	defs := map[string]uda.Definition{}
	for key, value := range values {
		if !strings.HasPrefix(key, "uda.") {
			continue
		}
		name, field, err := splitUDAConfigKey(key)
		if err != nil {
			return nil, err
		}
		def := defs[name]
		if def.Name == "" {
			def = uda.Definition{Name: name, Type: uda.TypeString}
		}
		switch field {
		case "type":
			def.Type = uda.Type(value)
		case "label":
			def.Label = value
		case "values":
			def.Values = uda.ParseValuesCSV(value)
		case "default":
			def.Default = value
		default:
			return nil, fmt.Errorf("unknown UDA config field %q", field)
		}
		defs[name] = def
	}
	for name, def := range defs {
		if def.Type == "" {
			def.Type = uda.TypeString
		}
		if err := uda.ValidateDefinition(def); err != nil {
			return nil, err
		}
		defs[name] = def
	}
	return defs, nil
}

func cloneUDAs(values map[string]task.UDAValue) map[string]task.UDAValue {
	if len(values) == 0 {
		return map[string]task.UDAValue{}
	}
	out := make(map[string]task.UDAValue, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}
