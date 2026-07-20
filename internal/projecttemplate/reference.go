package projecttemplate

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type markdownReference struct {
	kind string
	id   string
}

// ReferenceIssue 是 Capture 时必须由调用方解决的正文引用问题。
type ReferenceIssue struct {
	Code      string
	TargetRef string
	Relation  string
}

// RewriteCaptureTaskReferences 把已选 task UUID 改为模板 local ref。
// 未选 task 与 attachment 不会被静默删除，而是作为 blocking issue 返回。
func RewriteCaptureTaskReferences(markdown string, localTaskRefs map[string]string) (string, []ReferenceIssue, error) {
	refs, err := markdownReferences(markdown)
	if err != nil {
		return "", nil, err
	}
	replacements := make(map[string]string)
	issues := make([]ReferenceIssue, 0)
	for _, ref := range refs {
		switch ref.kind {
		case "task":
			if !canonicalUUID(ref.id) {
				return "", nil, invalid("capture task content reference must use a UUID")
			}
			localRef, selected := localTaskRefs[ref.id]
			if !selected {
				issues = append(issues, ReferenceIssue{Code: "project_template_dependency_missing", TargetRef: ref.id, Relation: "content"})
				continue
			}
			if !validLocalRef(localRef, "task") {
				return "", nil, invalid("capture task local reference is invalid")
			}
			replacements["ref://task/"+ref.id] = "ref://task/" + localRef
		case "attachment":
			issues = append(issues, ReferenceIssue{Code: "project_template_attachment_unsupported", TargetRef: ref.id, Relation: "content"})
		case "user":
			if !canonicalUUID(ref.id) {
				return "", nil, invalid("capture user content reference must use a UUID")
			}
		default:
			return "", nil, invalid("capture content reference kind is invalid")
		}
	}
	return rewriteMarkdownDestinations(markdown, replacements), issues, nil
}

// RewriteInstantiateTaskReferences 把合法 local ref 还原为已预分配的新 task UUID。
func RewriteInstantiateTaskReferences(markdown string, taskIDs map[string]string) (string, error) {
	refs, err := markdownReferences(markdown)
	if err != nil {
		return "", err
	}
	replacements := make(map[string]string)
	for _, ref := range refs {
		switch ref.kind {
		case "task":
			if !validLocalRef(ref.id, "task") {
				return "", invalid("snapshot task content reference must use a local task ref")
			}
			taskID, allocated := taskIDs[ref.id]
			if !allocated || !canonicalUUID(taskID) {
				return "", invalid("snapshot task content reference has no valid preallocation")
			}
			replacements["ref://task/"+ref.id] = "ref://task/" + taskID
		case "attachment":
			return "", Error{Code: "project_template_attachment_unsupported", Message: "snapshot descriptions cannot reference attachments"}
		case "user":
			if !canonicalUUID(ref.id) {
				return "", invalid("snapshot user content reference must use a UUID")
			}
		default:
			return "", invalid("snapshot content reference kind is invalid")
		}
	}
	return rewriteMarkdownDestinations(markdown, replacements), nil
}

func validateMarkdownReferences(markdown string) error {
	refs, err := markdownReferences(markdown)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		switch ref.kind {
		case "attachment":
			return Error{Code: "project_template_attachment_unsupported", Message: "snapshot descriptions cannot reference attachments"}
		case "task":
			if !validLocalRef(ref.id, "task") {
				return invalid("snapshot task content reference must use a local task ref")
			}
		case "user":
			if _, err := uuid.Parse(ref.id); err != nil {
				return invalid("snapshot user content reference must use a UUID")
			}
		default:
			return invalid("snapshot content reference kind is invalid")
		}
	}
	return nil
}

func markdownReferences(markdown string) ([]markdownReference, error) {
	if markdown == "" {
		return nil, nil
	}
	source := []byte(markdown)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	var refs []markdownReference
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination string
		switch typed := node.(type) {
		case *ast.Link:
			destination = string(typed.Destination)
		case *ast.Image:
			destination = string(typed.Destination)
		default:
			return ast.WalkContinue, nil
		}
		if !strings.HasPrefix(destination, "ref://") {
			return ast.WalkContinue, nil
		}
		ref, err := parseMarkdownReference(destination)
		if err != nil {
			return ast.WalkStop, err
		}
		refs = append(refs, ref)
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func parseMarkdownReference(destination string) (markdownReference, error) {
	uri, err := url.Parse(destination)
	if err != nil || uri.Scheme != "ref" || uri.User != nil || uri.Port() != "" || uri.RawQuery != "" || uri.Fragment != "" {
		return markdownReference{}, invalid("invalid content reference")
	}
	path := strings.TrimPrefix(uri.EscapedPath(), "/")
	if uri.Host == "" || path == "" || strings.Contains(path, "/") || strings.Contains(path, "%") {
		return markdownReference{}, invalid("invalid content reference")
	}
	return markdownReference{kind: strings.ToLower(uri.Hostname()), id: path}, nil
}

func canonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

// rewriteMarkdownDestinations 只替换已由 Goldmark AST 识别过的 destination。
// 逐行扫描仅用于定位原文中的 destination，因此普通链接、inline code 与 fenced
// code block 会保留原始字节，不会被 renderer 重新格式化。
func rewriteMarkdownDestinations(markdown string, replacements map[string]string) string {
	if len(replacements) == 0 {
		return markdown
	}
	var out strings.Builder
	inFence := false
	for _, line := range strings.SplitAfter(markdown, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			out.WriteString(line)
			inFence = !inFence
			continue
		}
		if inFence {
			out.WriteString(line)
			continue
		}
		out.WriteString(rewriteMarkdownLine(line, replacements))
	}
	return out.String()
}

func rewriteMarkdownLine(line string, replacements map[string]string) string {
	var out strings.Builder
	codeDelimiterLength := 0
	for index := 0; index < len(line); {
		if line[index] == '`' {
			end := index
			for end < len(line) && line[end] == '`' {
				end++
			}
			length := end - index
			if codeDelimiterLength == 0 {
				codeDelimiterLength = length
			} else if codeDelimiterLength == length {
				codeDelimiterLength = 0
			}
			out.WriteString(line[index:end])
			index = end
			continue
		}
		if codeDelimiterLength == 0 && line[index] == ']' && index+1 < len(line) && line[index+1] == '(' {
			end := strings.IndexByte(line[index+2:], ')')
			if end >= 0 {
				end += index + 2
				destination := line[index+2 : end]
				if replacement, ok := replacements[destination]; ok {
					out.WriteString("](")
					out.WriteString(replacement)
					out.WriteByte(')')
					index = end + 1
					continue
				}
			}
		}
		out.WriteByte(line[index])
		index++
	}
	return out.String()
}

func referenceIssue(code, message string) Error {
	return Error{Code: code, Message: fmt.Sprintf("template content reference: %s", message)}
}
