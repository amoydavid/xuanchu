package projecttemplate

import (
	"fmt"
	"net/url"
	"sort"
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

type markdownReferenceOccurrence struct {
	markdownReference
	start int
	end   int
}

type markdownPatch struct {
	start       int
	end         int
	replacement string
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
	rewritten, err := rewriteMarkdownDestinations(markdown, replacements)
	return rewritten, issues, err
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
				return "", Error{Code: "project_template_dependency_missing", Message: "snapshot task content reference has no valid preallocation"}
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
	return rewriteMarkdownDestinations(markdown, replacements)
}

func validateMarkdownReferences(markdown string, taskRefs map[string]struct{}) error {
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
			if _, exists := taskRefs[ref.id]; !exists {
				return Error{Code: "project_template_dependency_missing", Message: "task content reference target is not selected"}
			}
		case "user":
			if !canonicalUUID(ref.id) {
				return invalid("snapshot user content reference must use a UUID")
			}
		default:
			return invalid("snapshot content reference kind is invalid")
		}
	}
	return nil
}

func markdownReferences(markdown string) ([]markdownReference, error) {
	occurrences, err := markdownReferenceOccurrences(markdown)
	if err != nil {
		return nil, err
	}
	refs := make([]markdownReference, 0, len(occurrences))
	for _, occurrence := range occurrences {
		refs = append(refs, occurrence.markdownReference)
	}
	return refs, nil
}

func markdownReferenceOccurrences(markdown string) ([]markdownReferenceOccurrence, error) {
	if markdown == "" {
		return nil, nil
	}
	source := []byte(markdown)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	var occurrences []markdownReferenceOccurrence
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination []byte
		var start, end int
		var err error
		switch typed := node.(type) {
		case *ast.Link:
			if typed.Reference != nil {
				return ast.WalkContinue, nil
			}
			destination = typed.Destination
			start, end, err = inlineDestinationSpan(source, typed.Pos(), destination)
		case *ast.Image:
			if typed.Reference != nil {
				return ast.WalkContinue, nil
			}
			destination = typed.Destination
			start, end, err = inlineDestinationSpan(source, typed.Pos(), destination)
		case *ast.LinkReferenceDefinition:
			destination = typed.Destination
			start, end, err = referenceDefinitionDestinationSpan(source, typed)
		default:
			return ast.WalkContinue, nil
		}
		if !strings.HasPrefix(string(destination), "ref://") {
			return ast.WalkContinue, nil
		}
		if err != nil {
			return ast.WalkStop, err
		}
		ref, err := parseMarkdownReference(string(destination))
		if err != nil {
			return ast.WalkStop, err
		}
		occurrences = append(occurrences, markdownReferenceOccurrence{markdownReference: ref, start: start, end: end})
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return occurrences, nil
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

// rewriteMarkdownDestinations 根据 Goldmark AST 节点的 source position 定位
// inline/reference definition destination，并生成不重叠的保字节 patch。
func rewriteMarkdownDestinations(markdown string, replacements map[string]string) (string, error) {
	if len(replacements) == 0 {
		return markdown, nil
	}
	occurrences, err := markdownReferenceOccurrences(markdown)
	if err != nil {
		return "", err
	}
	patches := make([]markdownPatch, 0, len(occurrences))
	for _, occurrence := range occurrences {
		original := markdown[occurrence.start:occurrence.end]
		if replacement, ok := replacements[original]; ok {
			patches = append(patches, markdownPatch{start: occurrence.start, end: occurrence.end, replacement: replacement})
		}
	}
	if len(patches) == 0 {
		return markdown, nil
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	var out strings.Builder
	previous := 0
	for _, patch := range patches {
		if patch.start < previous || patch.start > patch.end || patch.end > len(markdown) {
			return "", invalid("overlapping Markdown destination spans")
		}
		out.WriteString(markdown[previous:patch.start])
		out.WriteString(patch.replacement)
		previous = patch.end
	}
	out.WriteString(markdown[previous:])
	return out.String(), nil
}

func inlineDestinationSpan(source []byte, nodeStart int, destination []byte) (int, int, error) {
	if nodeStart < 0 || nodeStart >= len(source) {
		return 0, 0, invalid("Markdown link has no source position")
	}
	open := nodeStart
	if source[open] == '!' {
		open++
	}
	if open >= len(source) || source[open] != '[' {
		return 0, 0, invalid("Markdown link source span is invalid")
	}
	close, ok := matchingBracket(source, open)
	if !ok || close+1 >= len(source) || source[close+1] != '(' {
		return 0, 0, invalid("Markdown inline link source span is invalid")
	}
	return destinationSpan(source, close+2, len(source), destination)
}

func referenceDefinitionDestinationSpan(source []byte, definition *ast.LinkReferenceDefinition) (int, int, error) {
	if definition.Pos() < 0 || definition.Lines().Len() == 0 {
		return 0, 0, invalid("Markdown reference definition has no source span")
	}
	start := definition.Pos()
	end := start
	for i := 0; i < definition.Lines().Len(); i++ {
		segment := definition.Lines().At(i)
		if segment.Stop > end {
			end = segment.Stop
		}
	}
	open := start
	for open < end && (source[open] == ' ' || source[open] == '\t') {
		open++
	}
	if open >= end || source[open] != '[' {
		return 0, 0, invalid("Markdown reference definition source span is invalid")
	}
	close, ok := matchingBracket(source[:end], open)
	if !ok || close+1 >= end || source[close+1] != ':' {
		return 0, 0, invalid("Markdown reference definition source span is invalid")
	}
	return destinationSpan(source, close+2, end, definition.Destination)
}

func destinationSpan(source []byte, start, limit int, destination []byte) (int, int, error) {
	for start < limit && (source[start] == ' ' || source[start] == '\t' || source[start] == '\n' || source[start] == '\r') {
		start++
	}
	if start >= limit {
		return 0, 0, invalid("Markdown destination source span is empty")
	}
	if source[start] == '<' {
		start++
	}
	end := start + len(destination)
	if end > limit || string(source[start:end]) != string(destination) {
		return 0, 0, invalid("Markdown destination source span does not match AST")
	}
	return start, end, nil
}

func matchingBracket(source []byte, open int) (int, bool) {
	depth := 0
	for i := open; i < len(source); i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		switch source[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func referenceIssue(code, message string) Error {
	return Error{Code: code, Message: fmt.Sprintf("template content reference: %s", message)}
}
