// Package branding 持有平台品牌名，可通过 -ldflags 在打包时注入（OEM）。
//
// 注入示例：
//
//	go build -ldflags "\
//	  -X git.dajee.net/dajee/xuanchu/internal/branding.NameZh=ACME \
//	  -X git.dajee.net/dajee/xuanchu/internal/branding.NameEn=ACME" \
//	  ./cmd/xuanchu
//
// 留空（或不注入）时使用默认值 璇础 / Xuanchu。
package branding

// NameZh / NameEn 可被 ldflags 覆盖；默认值见初始化器。
var (
	NameZh = "璇础"
	NameEn = "Xuanchu"
)

// Info 是对外（API / 序列化）的品牌信息快照。
type Info struct {
	NameZh string `json:"name_zh"`
	NameEn string `json:"name_en"`
}

// Current 返回当前品牌信息，对空值做 fallback 保护
// （防止 ldflags 注入空字符串导致 UI 显示空白品牌）。
func Current() Info {
	zh, en := NameZh, NameEn
	if zh == "" {
		zh = "璇础"
	}
	if en == "" {
		en = "Xuanchu"
	}
	return Info{NameZh: zh, NameEn: en}
}
