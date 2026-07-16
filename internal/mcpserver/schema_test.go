package mcpserver

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var update = flag.Bool("update", false, "update golden files")

// goldenGet 读取 golden 文件内容；当 -update 时写入 actual 并返回 actual。
func goldenGet(t *testing.T, name string, actual []byte) []byte {
	t.Helper()
	path := "testdata/" + name
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return actual
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return want
}

func TestListToolsDefaultServerHasTools(t *testing.T) {
	srv := NewServer(Options{Version: "test"})

	// 创建 in-memory transport 对
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// 启动服务端
	go func() {
		if err := srv.Run(context.Background(), serverTransport); err != nil {
			t.Logf("server run: %v", err)
		}
	}()

	// 连接客户端
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	// 调用 tools/list
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	if len(result.Tools) == 0 {
		t.Fatal("expected default MCP server to expose tools")
	}

	// golden 比较：序列化完整结果
	got, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	want := goldenGet(t, "list-tools-default.json", got)
	gotS := string(got)
	wantS := string(want)
	if gotS != wantS {
		t.Fatalf("golden mismatch:\n--- want\n%s\n--- got\n%s", wantS, gotS)
	}
}

// mustSchema 生成 input schema 并应用 patchInputSchema，用于断言工具参数契约。
func mustSchema[T any](t *testing.T) *jsonschema.Schema {
	t.Helper()
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		t.Fatalf("jsonschema.For[%T]: %v", *new(T), err)
	}
	patchInputSchema[T](schema)
	return schema
}

// scopeFieldDescriptionNonEmpty 断言作用域三件套字段在出现时都有描述，
// 让 LLM 在所有工具上看到一致的语义。
func scopeFieldDescriptionNonEmpty(t *testing.T, schema *jsonschema.Schema) {
	t.Helper()
	for _, name := range []string{"workspace", "project", "project_id"} {
		prop := schema.Properties[name]
		if prop == nil {
			continue
		}
		if prop.Description == "" {
			t.Fatalf("scope field %q has empty description", name)
		}
	}
}

// TestMCPScopeFieldsDocumented 防止作用域字段 description 回归为空。
func TestMCPScopeFieldsDocumented(t *testing.T) {
	checks := []func(t *testing.T) *jsonschema.Schema{
		func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectConfigListInput](t) },
		func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectGetInput](t) },
		func(t *testing.T) *jsonschema.Schema { return mustSchema[TaskAddInput](t) },
		func(t *testing.T) *jsonschema.Schema { return mustSchema[TaskQueryInput](t) },
		func(t *testing.T) *jsonschema.Schema { return mustSchema[ConfigGetInput](t) },
		func(t *testing.T) *jsonschema.Schema { return mustSchema[TokenListInput](t) },
	}
	for _, fn := range checks {
		schema := fn(t)
		scopeFieldDescriptionNonEmpty(t, schema)
	}
}

// projectRefRequiresAnyOf 断言 project-ref 工具声明了 project/project_id
// 至少传一个，否则 LLM 会误以为可省略导致 project_not_found。
func projectRefRequiresAnyOf(t *testing.T, schema *jsonschema.Schema, toolName string) {
	t.Helper()
	if len(schema.AnyOf) == 0 {
		t.Fatalf("%s: expected anyOf requiring project or project_id, got none", toolName)
	}
	hasProject, hasProjectID := false, false
	for _, branch := range schema.AnyOf {
		for _, req := range branch.Required {
			if req == "project" {
				hasProject = true
			}
			if req == "project_id" {
				hasProjectID = true
			}
		}
	}
	if !hasProject || !hasProjectID {
		t.Fatalf("%s: anyOf must require project and project_id branches, got project=%v project_id=%v", toolName, hasProject, hasProjectID)
	}
}

// TestMCPProjectRefToolsRequireProject 防止 project-ref 必填约束回归。
func TestMCPProjectRefToolsRequireProject(t *testing.T) {
	checks := []struct {
		name string
		fn   func(t *testing.T) *jsonschema.Schema
	}{
		{"project_get", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectGetInput](t) }},
		{"project_annotate", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectAnnotateInput](t) }},
		{"project_denotate", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectDenotateInput](t) }},
		{"project_list_annotations", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectAnnotationsInput](t) }},
		{"project_list_timeline", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectTimelineInput](t) }},
		{"project_modify", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectModifyInput](t) }},
		{"project_archive", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectArchiveInput](t) }},
		{"project_transition", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectTransitionInput](t) }},
		{"project_config_set", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectConfigSetInput](t) }},
		{"project_config_unset", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectConfigUnsetInput](t) }},
		{"project_config_list", func(t *testing.T) *jsonschema.Schema { return mustSchema[ProjectConfigListInput](t) }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			schema := c.fn(t)
			projectRefRequiresAnyOf(t, schema, c.name)
			scopeFieldDescriptionNonEmpty(t, schema)
		})
	}
}
