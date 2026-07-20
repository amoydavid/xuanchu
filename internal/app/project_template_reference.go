package app

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"git.dajee.net/dajee/xuanchu/internal/projecttemplate"
)

// rewriteCaptureDescription 只改写 Goldmark 识别出的 Markdown destination。
// 对显式 drop 的正文 task ref 使用临时 local ref，再把该 destination 降级为普通锚点；
// code span/code block 中的同样文本不会被误改。
func rewriteCaptureDescription(description *string, localTaskRefs map[string]string, droppedTargets map[string]bool) (*string, []projecttemplate.ReferenceIssue, error) {
	if description == nil {
		return nil, nil, nil
	}
	refs := make(map[string]string, len(localTaskRefs)+len(droppedTargets))
	for source, local := range localTaskRefs {
		refs[source] = local
	}
	droppedCaptureTaskRef := unusedDroppedCaptureTaskRef(*description, localTaskRefs)
	for target := range droppedTargets {
		refs[target] = droppedCaptureTaskRef
	}
	rewritten, issues, err := projecttemplate.RewriteCaptureTaskReferences(*description, refs)
	if err != nil {
		return nil, nil, err
	}
	if len(droppedTargets) > 0 {
		rewritten = strings.ReplaceAll(rewritten, "ref://task/"+droppedCaptureTaskRef, "#")
	}
	return &rewritten, issues, nil
}

// unusedDroppedCaptureTaskRef 选择一个既未出现在源 Markdown、也未分配给真实
// blueprint 的合法 local ref。后续 ReplaceAll 因而只可能命中新插入的 destination，
// 不会污染 code block、inline code 或普通文本。
func unusedDroppedCaptureTaskRef(markdown string, localTaskRefs map[string]string) string {
	used := make(map[string]bool, len(localTaskRefs))
	for _, local := range localTaskRefs {
		used[local] = true
	}
	for index := 2147483647; ; index++ {
		candidate := fmt.Sprintf("task-%d", index)
		if !used[candidate] && !strings.Contains(markdown, "ref://task/"+candidate) {
			return candidate
		}
	}
}

func captureReferenceIssue(sourceKind, sourceRef string, issue projecttemplate.ReferenceIssue) CaptureIssue {
	return CaptureIssue{
		Code:       issue.Code,
		SourceKind: sourceKind,
		SourceRef:  sourceRef,
		TargetRef:  issue.TargetRef,
		Relation:   issue.Relation,
		Message:    fmt.Sprintf("%s %s references unselected task %s", sourceKind, sourceRef, issue.TargetRef),
	}
}

// captureMarkdownUserRefs 只读取 Markdown AST 中真实 link/image destination 的 user ref；
// code span 与 code block 中的字面量不会被当成身份引用。
func captureMarkdownUserRefs(markdown *string) ([]string, error) {
	if markdown == nil || *markdown == "" {
		return nil, nil
	}
	source := []byte(*markdown)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	refs := []string{}
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination []byte
		switch typed := node.(type) {
		case *ast.Link:
			if typed.Reference != nil {
				return ast.WalkContinue, nil
			}
			destination = typed.Destination
		case *ast.Image:
			if typed.Reference != nil {
				return ast.WalkContinue, nil
			}
			destination = typed.Destination
		case *ast.LinkReferenceDefinition:
			destination = typed.Destination
		default:
			return ast.WalkContinue, nil
		}
		if !strings.HasPrefix(string(destination), "ref://user/") {
			return ast.WalkContinue, nil
		}
		parsed, err := url.Parse(string(destination))
		if err != nil || parsed.Scheme != "ref" || strings.ToLower(parsed.Hostname()) != "user" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return ast.WalkStop, captureError("project_template_snapshot_invalid", "invalid user content reference")
		}
		id := strings.TrimPrefix(parsed.EscapedPath(), "/")
		parsedID, err := uuid.Parse(id)
		if err != nil || parsedID.String() != id {
			return ast.WalkStop, captureError("project_template_snapshot_invalid", "user content reference must use a canonical UUID")
		}
		refs = append(refs, id)
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return sortedUniqueCapture(refs), nil
}
