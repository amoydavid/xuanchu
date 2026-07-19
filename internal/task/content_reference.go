// Package task 中的 content_reference.go 负责解析 Markdown 正文中的
// 内部语义引用 URI（ref://user|task|attachment/{uuid}）。
//
// 本文件由两份实现计划共享：generic-attachments 计划先建立 attachment-only
// scaffold，content-references-mentions 计划在同一文件扩展 user/task。
// 不得新建第二套 parser。
package task

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ContentReferenceKind 是保留 URI 的 host 类型。
type ContentReferenceKind string

const (
	ContentReferenceUser       ContentReferenceKind = "user"
	ContentReferenceTask       ContentReferenceKind = "task"
	ContentReferenceAttachment ContentReferenceKind = "attachment"
)

// ContentReference 表示一个保留 URI 引用。
type ContentReference struct {
	Kind  ContentReferenceKind
	ID    string
	Label string
	Image bool
}

// ContentReferenceKey 是引用的稳定身份键，label 不参与身份。
type ContentReferenceKey struct {
	Kind ContentReferenceKind
	ID   string
}

// ErrDescriptionReferenceInvalid 描述保留 URI 格式/类型/目标非法。
//
// 调用方按需 wrap 成 RuntimeError{Code:"description_reference_invalid"}。
var ErrDescriptionReferenceInvalid = errors.New("description reference invalid")

// ParseContentReferences 解析 markdown，返回所有 ref:// 保留 URI 引用。
//
// 任何非法保留 URI（scheme/host/UUID 格式/未知类型/userinfo/port/query/fragment）
// 都会返回 ErrDescriptionReferenceInvalid；普通 http(s)/mailto 链接忽略。
// 同一节点多次出现会重复返回，调用方需要自己去重。
func ParseContentReferences(markdown string) ([]ContentReference, error) {
	if markdown == "" {
		return nil, nil
	}
	source := []byte(markdown)
	reader := text.NewReader(source)
	doc := goldmark.New().Parser().Parse(reader)
	var out []ContentReference
	return out, ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest string
		var isImage bool
		switch typed := n.(type) {
		case *ast.Link:
			dest = string(typed.Destination)
		case *ast.Image:
			dest = string(typed.Destination)
			isImage = true
		default:
			return ast.WalkContinue, nil
		}
		if !isReservedRefURI(dest) {
			return ast.WalkContinue, nil
		}
		kind, id, err := parseReferenceDestination(dest)
		if err != nil {
			return ast.WalkStop, err
		}
		out = append(out, ContentReference{Kind: kind, ID: id, Label: string(n.Text(source)), Image: isImage})
		return ast.WalkContinue, nil
	})
}

// isReservedRefURI 判断 destination 是否是 ref:// 保留协议。
func isReservedRefURI(dest string) bool {
	if dest == "" {
		return false
	}
	parsed, err := url.Parse(dest)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "ref")
}

// parseReferenceDestination 解析单个 ref:// URI。
func parseReferenceDestination(raw string) (ContentReferenceKind, string, error) {
	// 必须显式以小写 ref:// 开头；url.Parse 会把 scheme 归一化为小写，
	// 因此先在原始字符串上校验，拒绝 REF:// 等大小写漂移。
	if !strings.HasPrefix(raw, "ref://") {
		return "", "", fmt.Errorf("%w: scheme must be lowercase ref", ErrDescriptionReferenceInvalid)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return "", "", fmt.Errorf("%w: invalid url", ErrDescriptionReferenceInvalid)
	}
	if u.User != nil {
		return "", "", fmt.Errorf("%w: userinfo not allowed", ErrDescriptionReferenceInvalid)
	}
	if u.Port() != "" {
		return "", "", fmt.Errorf("%w: port not allowed", ErrDescriptionReferenceInvalid)
	}
	if u.RawQuery != "" {
		return "", "", fmt.Errorf("%w: query not allowed", ErrDescriptionReferenceInvalid)
	}
	if u.Fragment != "" {
		return "", "", fmt.Errorf("%w: fragment not allowed", ErrDescriptionReferenceInvalid)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", "", fmt.Errorf("%w: host required", ErrDescriptionReferenceInvalid)
	}
	kind := ContentReferenceKind(host)
	if kind != ContentReferenceAttachment && kind != ContentReferenceUser && kind != ContentReferenceTask {
		return "", "", fmt.Errorf("%w: unknown reference type %q", ErrDescriptionReferenceInvalid, host)
	}
	// path 必须是单一 UUID segment。
	path := strings.TrimPrefix(u.EscapedPath(), "/")
	if path == "" || strings.Contains(path, "/") {
		return "", "", fmt.Errorf("%w: id path required", ErrDescriptionReferenceInvalid)
	}
	if strings.Contains(path, "%") {
		return "", "", fmt.Errorf("%w: percent-encoded id not allowed", ErrDescriptionReferenceInvalid)
	}
	parsed, err := uuid.Parse(path)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid uuid", ErrDescriptionReferenceInvalid)
	}
	if parsed.String() != path {
		return "", "", fmt.Errorf("%w: non-canonical uuid", ErrDescriptionReferenceInvalid)
	}
	return kind, path, nil
}

// AttachmentReferenceIDs 返回 markdown 中所有 attachment 引用的 ID 去重列表。
func AttachmentReferenceIDs(markdown string) ([]string, error) {
	refs, err := ParseContentReferences(markdown)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var ids []string
	for _, r := range refs {
		if r.Kind != ContentReferenceAttachment {
			continue
		}
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		ids = append(ids, r.ID)
	}
	return ids, nil
}

// DiffContentReferenceKeys 计算两组引用的 added/removed。
//
// 身份只由 (kind, id) 决定，label 不参与。返回的两个切片按 (kind, id) 排序，去重。
func DiffContentReferenceKeys(before, after []ContentReference) (added, removed []ContentReferenceKey) {
	beforeSet := toKeySet(before)
	afterSet := toKeySet(after)
	for key := range afterSet {
		if _, ok := beforeSet[key]; !ok {
			added = append(added, key)
		}
	}
	for key := range beforeSet {
		if _, ok := afterSet[key]; !ok {
			removed = append(removed, key)
		}
	}
	sortKeys(added)
	sortKeys(removed)
	return added, removed
}

func toKeySet(refs []ContentReference) map[ContentReferenceKey]struct{} {
	set := make(map[ContentReferenceKey]struct{}, len(refs))
	for _, r := range refs {
		set[ContentReferenceKey{Kind: r.Kind, ID: r.ID}] = struct{}{}
	}
	return set
}

func sortKeys(keys []ContentReferenceKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Kind != keys[j].Kind {
			return keys[i].Kind < keys[j].Kind
		}
		return keys[i].ID < keys[j].ID
	})
}

// MentionedUserIDs 返回 markdown 中所有 user 引用的去重 user ID。
func MentionedUserIDs(markdown string) ([]string, error) {
	refs, err := ParseContentReferences(markdown)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var ids []string
	for _, r := range refs {
		if r.Kind != ContentReferenceUser {
			continue
		}
		if _, ok := seen[r.ID]; ok {
			continue
		}
		seen[r.ID] = struct{}{}
		ids = append(ids, r.ID)
	}
	return ids, nil
}

// 移除了 labelText 辅助：直接调用 ast.Node.Text(source) 获取 label 文本。
