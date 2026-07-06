package branding

import "testing"

// withVars 临时覆盖包变量，仅供测试使用。t.Cleanup 恢复原值。
func withVars(t *testing.T, zh, en string) {
	t.Helper()
	prevZh, prevEn := NameZh, NameEn
	NameZh, NameEn = zh, en
	t.Cleanup(func() { NameZh, NameEn = prevZh, prevEn })
}

func TestCurrentDefaultValues(t *testing.T) {
	withVars(t, "璇础", "Xuanchu")
	info := Current()
	if info.NameZh != "璇础" {
		t.Fatalf("NameZh = %q, want 璇础", info.NameZh)
	}
	if info.NameEn != "Xuanchu" {
		t.Fatalf("NameEn = %q, want Xuanchu", info.NameEn)
	}
}

func TestCurrentFallbacksEmptyToDefault(t *testing.T) {
	withVars(t, "", "")
	info := Current()
	if info.NameZh != "璇础" {
		t.Fatalf("NameZh = %q, want 璇础", info.NameZh)
	}
	if info.NameEn != "Xuanchu" {
		t.Fatalf("NameEn = %q, want Xuanchu", info.NameEn)
	}
}

func TestCurrentReflectsInjectedVars(t *testing.T) {
	withVars(t, "ACME平台", "ACME")
	info := Current()
	if info.NameZh != "ACME平台" {
		t.Fatalf("NameZh = %q, want ACME平台", info.NameZh)
	}
	if info.NameEn != "ACME" {
		t.Fatalf("NameEn = %q, want ACME", info.NameEn)
	}
}

// 仅 zh 为空 / 仅 en 为空 的边界。
func TestCurrentFallbacksPartialEmpty(t *testing.T) {
	withVars(t, "", "ACME")
	info := Current()
	if info.NameZh != "璇础" {
		t.Fatalf("NameZh = %q, want 璇础", info.NameZh)
	}
	if info.NameEn != "ACME" {
		t.Fatalf("NameEn = %q, want ACME", info.NameEn)
	}
}
