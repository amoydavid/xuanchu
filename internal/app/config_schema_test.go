package app

import (
	"testing"
)

func TestBuiltinConfigDefinitionsIncludeProjectAutomationProvider(t *testing.T) {
	svc, cleanup := newTestService(t, 100)
	defer cleanup()
	rows, err := svc.ConfigSchemaList()
	if err != nil {
		t.Fatalf("ConfigSchemaList: %v", err)
	}
	keys := map[string]bool{}
	for _, row := range rows {
		keys[row.Key] = true
	}
	for _, key := range []string{"agent.provider.base_url", "agent.provider.api_key", "agent.provider.model", "agent.provider.protocol", "agent.provider.allowed_hosts", "feishu.chat_id"} {
		if !keys[key] {
			t.Fatalf("missing builtin config key %s", key)
		}
	}
}
