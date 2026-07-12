package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestSeriesCommandRegistered 验证 series 命令树包含所有子命令。
func TestSeriesCommandRegistered(t *testing.T) {
	root := &cobra.Command{}
	root.AddCommand(newSeriesCommand(Options{}))
	series, _, err := root.Find([]string{"series"})
	if err != nil {
		t.Fatalf("Find series: %v", err)
	}
	want := map[string]bool{
		"add": true, "list": true, "info": true, "modify": true,
		"occurrences": true, "stop": true, "skip": true,
	}
	for _, c := range series.Commands() {
		name := strings.Fields(c.Use)
		if len(name) > 0 {
			delete(want, name[0])
		}
	}
	if len(want) > 0 {
		t.Fatalf("缺少子命令: %v", want)
	}
}

// TestSeriesCommandHelpInChinese 验证 series 命令使用中文描述。
func TestSeriesCommandHelpInChinese(t *testing.T) {
	cmd := newSeriesCommand(Options{})
	if !strings.Contains(cmd.Short, "循环") {
		t.Fatalf("series Short 应含'循环': %q", cmd.Short)
	}
}

// TestParseSeriesDateFlag 验证日期 flag 解析。
func TestParseSeriesDateFlag(t *testing.T) {
	// unix 时间戳。
	ts, err := parseSeriesDateFlag("123")
	if err != nil || ts != 123 {
		t.Fatalf("unix: ts=%d err=%v", ts, err)
	}
	// YYYY-MM-DD。
	ts2, err := parseSeriesDateFlag("2030-01-15")
	if err != nil {
		t.Fatalf("date: %v", err)
	}
	if ts2 <= 0 {
		t.Fatalf("date ts = %d", ts2)
	}
	// 非法。
	if _, err := parseSeriesDateFlag("not-a-date"); err == nil {
		t.Fatal("非法日期应失败")
	}
}

// TestIsLikelyUUID 验证 UUID 判断。
func TestIsLikelyUUID(t *testing.T) {
	if !isLikelyUUID("11111111-1111-1111-1111-111111111111") {
		t.Fatal("标准 UUID 应识别为 UUID")
	}
	if isLikelyUUID("ops") {
		t.Fatal("slug 不应识别为 UUID")
	}
}

// 集成测试在 tests/integration 中通过二进制执行 series 全流程。
