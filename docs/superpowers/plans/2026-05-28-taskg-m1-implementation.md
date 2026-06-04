# taskg M1 Implementation Plan

> **For agentic workers:** REQUIRED: Use `superpowers:subagent-driven-development` (if subagents available) or `superpowers:executing-plans` to implement this plan. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 实现 taskg M1：查询 AST、报表系统、urgency 计算、DOM/helper 基础和 `calc` 第一版，让 M0 CLI 具备 Taskwarrior 风格的查询与解释能力。

**Architecture:** 保持 M0 分层：CLI 解析命令形态，app service 承接用例，query/report/urgency/dom/expr 提供可复用领域能力，storage/sqlite 负责把 query AST 安全编译到 GORM 查询。日期相对值在 parser 阶段保持 lazy，到编译阶段使用 app 注入的 clock 求值；裸 token 不直接编译为 `uuid = ?`，而是由 CLI/app 按上下文解析为 target 或 description 子串查询。M1 不新增任务核心字段，不实现 M2 的 waiting/active/blocked 等能力，只基于 M0 字段交付真实可用功能。

**Tech Stack:** Go 1.22、Cobra、GORM、`github.com/glebarez/sqlite`、`github.com/expr-lang/expr`（用于 `calc`，纯 Go）、标准库 `time`/`strconv`/`strings`、Go test。

---

## Chunk 1: 查询 AST 与 Parser

### 文件职责

- Create: `internal/query/ast.go`
  - AST 节点、属性、操作符、typed value、lazy date value、bare token。
- Create: `internal/query/token.go`
  - 查询 tokenizer，处理引号、`/text/` 子串表达式、括号、布尔关键字。
- Create: `internal/query/parser_ast.go`
  - 递归下降 parser，输出 AST。
- Modify: `internal/query/date.go`
  - 复用 M0 日期解析，补 `ParseDateValue`/`ResolveDateValue`，parser 保存表达式，compiler 用 `Options.Now` 求值。
- Modify: `internal/query/parser.go`
  - 保留 M0 `ParseAddArgs` / `ParseModifyArgs`，将 `ParseFilters` 迁移到 AST 入口并兼容旧调用。
- Test: `internal/query/ast_test.go`
- Test: `internal/query/token_test.go`
- Test: `internal/query/parser_ast_test.go`

### Task 1: 定义 Query AST

**Files:**

- Create: `internal/query/ast.go`
- Create: `internal/query/ast_test.go`

- [x] **Step 1: 写失败测试**

创建 `internal/query/ast_test.go`：

```go
package query

import "testing"

func TestAndFlattensNilExpressions(t *testing.T) {
	left := Predicate{Attribute: AttrStatus, Operator: OpEqual, Value: StringValue("pending")}
	got := And(nil, left)
	if got == nil {
		t.Fatal("And() returned nil")
	}
	if got.String() != `status eq "pending"` {
		t.Fatalf("String() = %q", got.String())
	}
}

func TestPredicateStringForTag(t *testing.T) {
	p := Predicate{Attribute: AttrTag, Operator: OpHasTag, Value: StringValue("next")}
	if got := p.String(); got != `tag has "next"` {
		t.Fatalf("String() = %q", got)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/query -run 'TestAnd|TestPredicate' -v`  
Expected: FAIL，`Predicate`、`AttrStatus` 等尚不存在。

- [x] **Step 3: 实现 AST 类型**

创建 `internal/query/ast.go`：

```go
package query

import (
	"fmt"
	"strconv"
)

type Expr interface {
	queryExpr()
	String() string
}

type Attribute string

const (
	AttrUUID        Attribute = "uuid"
	AttrDescription Attribute = "description"
	AttrStatus      Attribute = "status"
	AttrEntry       Attribute = "entry"
	AttrModified    Attribute = "modified"
	AttrEnd         Attribute = "end"
	AttrDue         Attribute = "due"
	AttrProject     Attribute = "project"
	AttrPriority    Attribute = "priority"
	AttrTag         Attribute = "tag"
	AttrBare        Attribute = "bare"
)

type Operator string

const (
	OpEqual      Operator = "eq"
	OpBefore     Operator = "before"
	OpAfter      Operator = "after"
	OpContains   Operator = "contains"
	OpHasTag     Operator = "has"
	OpMissingTag Operator = "missing"
	OpIsNull     Operator = "is_null"
)

type Value struct {
	Raw  string
	Kind ValueKind
	Text string
}

// Raw keeps the user-facing expression for logs/String().
// Text is the normalized value used for comparisons and SQL parameters.
// They are equal in M1, but will diverge when escaped strings are supported.

type ValueKind string

const (
	ValueString ValueKind = "string"
	ValueDate   ValueKind = "date"
	ValueBare   ValueKind = "bare"
)

func StringValue(v string) Value {
	return Value{Raw: v, Kind: ValueString, Text: v}
}

func DateValue(raw string) Value {
	return Value{Raw: raw, Kind: ValueDate, Text: raw}
}

func BareValue(raw string) Value {
	return Value{Raw: raw, Kind: ValueBare, Text: raw}
}

type Predicate struct {
	Attribute Attribute
	Operator  Operator
	Value     Value
}

type Binary struct {
	Op          string
	Left, Right Expr
}

type Unary struct {
	Op   string
	Expr Expr
}

func (Predicate) queryExpr() {}
func (Binary) queryExpr()    {}
func (Unary) queryExpr()     {}

func And(left, right Expr) Expr {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return Binary{Op: "and", Left: left, Right: right}
}

func Or(left, right Expr) Expr {
	return Binary{Op: "or", Left: left, Right: right}
}

func Xor(left, right Expr) Expr {
	return Binary{Op: "xor", Left: left, Right: right}
}

func Not(expr Expr) Expr {
	return Unary{Op: "not", Expr: expr}
}

func (p Predicate) String() string {
	if p.Operator == OpIsNull {
		return fmt.Sprintf("%s %s", p.Attribute, p.Operator)
	}
	value := strconv.Quote(p.Value.Raw)
	return fmt.Sprintf("%s %s %s", p.Attribute, p.Operator, value)
}

func (b Binary) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left.String(), b.Op, b.Right.String())
}

func (u Unary) String() string {
	return fmt.Sprintf("(%s %s)", u.Op, u.Expr.String())
}
```

- [x] **Step 4: 运行测试确认通过**

Run: `go test ./internal/query -run 'TestAnd|TestPredicate' -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/query/ast.go internal/query/ast_test.go
git commit -m "feat: 定义 M1 查询 AST"
```

### Task 2: 实现 Query Tokenizer

**Files:**

- Create: `internal/query/token.go`
- Create: `internal/query/token_test.go`

- [x] **Step 1: 写 tokenizer 失败测试**

创建 `internal/query/token_test.go`：

```go
package query

import "testing"

func TestTokenizeQueryWithQuotesSlashTextAndParens(t *testing.T) {
	tokens, err := Tokenize(`(project:'Home & Garden' and /spec/) or +next`)
	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}
	got := tokenTexts(tokens)
	want := []string{"(", "project:Home & Garden", "and", "/spec/", ")", "or", "+next"}
	if !equalStrings(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

func TestTokenizeRejectsUnclosedQuote(t *testing.T) {
	if _, err := Tokenize(`project:'Home`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
}

func TestTokenizeRejectsUnclosedSlashText(t *testing.T) {
	if _, err := Tokenize(`/spec`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
}

func TestTokenizeRejectsEmptyQuotes(t *testing.T) {
	if _, err := Tokenize(`""`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
	if _, err := Tokenize(`project:''`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
}

func TestTokenizeHandlesUTF8Whitespace(t *testing.T) {
	tokens, err := Tokenize("description:中文　+next")
	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}
	got := tokenTexts(tokens)
	want := []string{"description:中文", "+next"}
	if !equalStrings(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

func tokenTexts(tokens []Token) []string {
	out := make([]string, len(tokens))
	for i, tok := range tokens {
		out[i] = tok.Text
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/query -run Tokenize -v`  
Expected: FAIL，`Tokenize` 未定义。

- [x] **Step 3: 实现 tokenizer**

创建 `internal/query/token.go`：

```go
package query

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Token struct {
	Text string
}

func Tokenize(input string) ([]Token, error) {
	var tokens []Token
	for i := 0; i < len(input); {
		r, size := utf8.DecodeRuneInString(input[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		switch input[i] {
		case '(', ')':
			tokens = append(tokens, Token{Text: string(input[i])})
			i++
		case '/':
			end := i + 1
			for end < len(input) && input[end] != '/' {
				end++
			}
			if end >= len(input) {
				return nil, fmt.Errorf("unclosed /text/ expression")
			}
			tokens = append(tokens, Token{Text: input[i : end+1]})
			i = end + 1
		default:
			var b strings.Builder
			for i < len(input) {
				r, size := utf8.DecodeRuneInString(input[i:])
				if unicode.IsSpace(r) || input[i] == '(' || input[i] == ')' {
					break
				}
				if input[i] == '\'' || input[i] == '"' {
					quote := input[i]
					i++
					segmentStart := i
					for i < len(input) && input[i] != quote {
						b.WriteByte(input[i])
						i++
					}
					if i >= len(input) {
						return nil, fmt.Errorf("unclosed quote")
					}
					if i == segmentStart {
						return nil, fmt.Errorf("empty quoted value")
					}
					i++
					continue
				}
				b.WriteString(input[i : i+size])
				i += size
			}
			if b.Len() == 0 {
				return nil, fmt.Errorf("empty token")
			}
			tokens = append(tokens, Token{Text: b.String()})
		}
	}
	return tokens, nil
}
```

- [x] **Step 4: 运行测试确认通过**

Run: `go test ./internal/query -run Tokenize -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/query/token.go internal/query/token_test.go
git commit -m "feat: 添加查询 tokenizer"
```

### Task 3: 实现递归下降 Query Parser

**Files:**

- Create: `internal/query/parser_ast.go`
- Create: `internal/query/parser_ast_test.go`
- Modify: `internal/query/parser.go`

- [x] **Step 1: 写 parser 失败测试**

创建 `internal/query/parser_ast_test.go`：

```go
package query

import "testing"

func TestParseQueryImplicitAnd(t *testing.T) {
	expr, err := ParseQuery(`+work status:pending`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if got := expr.String(); got != `(tag has "work" and status eq "pending")` {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseQueryBooleanPrecedence(t *testing.T) {
	expr, err := ParseQuery(`+next or due.before:tomorrow and priority:H`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	want := `(tag has "next" or (due before "tomorrow" and priority eq "H"))`
	if got := expr.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseQueryParentheses(t *testing.T) {
	expr, err := ParseQuery(`(project:work and +urgent) or priority:H`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	want := `((project eq "work" and tag has "urgent") or priority eq "H")`
	if got := expr.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseQueryRejectsUnknownAttribute(t *testing.T) {
	if _, err := ParseQuery(`foo:bar`); err == nil {
		t.Fatal("ParseQuery() error = nil, want error")
	}
}

func TestParseQueryBareTokenStaysBare(t *testing.T) {
	expr, err := ParseQuery(`abc123`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if got := expr.String(); got != `bare contains "abc123"` {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseQueryRejectsUnbalancedParens(t *testing.T) {
	if _, err := ParseQuery(`(+work or priority:H`); err == nil {
		t.Fatal("ParseQuery() error = nil, want error")
	}
}

func TestParseQueryNotExpression(t *testing.T) {
	expr, err := ParseQuery(`not +work`)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if got := expr.String(); got != `(not tag has "work")` {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseQueryEmptyInput(t *testing.T) {
	if expr, err := ParseQuery(`   `); err != nil || expr != nil {
		t.Fatalf("ParseQuery(empty) = %#v, %v; want nil, nil", expr, err)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/query -run ParseQuery -v`  
Expected: FAIL。

- [x] **Step 3: 实现 parser**

创建 `internal/query/parser_ast.go`：

```go
package query

import (
	"fmt"
	"strings"
)

func ParseQuery(input string) (Expr, error) {
	tokens, err := Tokenize(input)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	p := &queryParser{tokens: tokens}
	expr, err := p.parseOr()
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	if p.hasNext() {
		return nil, fmt.Errorf("invalid query: unexpected token %q", p.peek())
	}
	return expr, nil
}

type queryParser struct {
	tokens []Token
	pos    int
}

func (p *queryParser) parseOr() (Expr, error) {
	left, err := p.parseXor()
	if err != nil {
		return nil, err
	}
	for p.match("or") {
		right, err := p.parseXor()
		if err != nil {
			return nil, err
		}
		left = Or(left, right)
	}
	return left, nil
}

func (p *queryParser) parseXor() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.match("xor") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = Xor(left, right)
	}
	return left, nil
}

func (p *queryParser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		if p.match("and") {
			right, err := p.parseNot()
			if err != nil {
				return nil, err
			}
			left = And(left, right)
			continue
		}
		if p.startsExpression() {
			right, err := p.parseNot()
			if err != nil {
				return nil, err
			}
			left = And(left, right)
			continue
		}
		return left, nil
	}
}

func (p *queryParser) parseNot() (Expr, error) {
	if p.match("not") {
		expr, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return Not(expr), nil
	}
	return p.parsePrimary()
}

func (p *queryParser) parsePrimary() (Expr, error) {
	if !p.hasNext() {
		return nil, fmt.Errorf("expected expression")
	}
	if p.match("(") {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(")") {
			return nil, fmt.Errorf("expected )")
		}
		return expr, nil
	}
	tok := p.next()
	return parsePredicate(tok)
}

func parsePredicate(tok string) (Expr, error) {
	switch {
	case strings.HasPrefix(tok, "+") && len(tok) > 1:
		return Predicate{Attribute: AttrTag, Operator: OpHasTag, Value: StringValue(tok[1:])}, nil
	case strings.HasPrefix(tok, "-") && len(tok) > 1:
		return Predicate{Attribute: AttrTag, Operator: OpMissingTag, Value: StringValue(tok[1:])}, nil
	case strings.HasPrefix(tok, "/") && strings.HasSuffix(tok, "/") && len(tok) >= 2:
		return Predicate{Attribute: AttrDescription, Operator: OpContains, Value: StringValue(strings.Trim(tok, "/"))}, nil
	}

	name, value, ok := strings.Cut(tok, ":")
	if !ok {
		return Predicate{Attribute: AttrBare, Operator: OpContains, Value: BareValue(tok)}, nil
	}
	attr, op, err := parseAttributeOperator(name)
	if err != nil {
		return nil, err
	}
	switch attr {
	case AttrDue, AttrEntry, AttrModified, AttrEnd:
		if value == "" {
			return Predicate{Attribute: attr, Operator: OpIsNull, Value: StringValue("")}, nil
		}
		if op == OpEqual || op == OpBefore || op == OpAfter {
			return Predicate{Attribute: attr, Operator: op, Value: ParseDateValue(value)}, nil
		}
	}
	if attr == AttrDescription && strings.HasPrefix(value, "/") && strings.HasSuffix(value, "/") {
		return Predicate{Attribute: attr, Operator: OpContains, Value: StringValue(strings.Trim(value, "/"))}, nil
	}
	return Predicate{Attribute: attr, Operator: op, Value: StringValue(value)}, nil
}

func parseAttributeOperator(name string) (Attribute, Operator, error) {
	base, suffix, hasSuffix := strings.Cut(name, ".")
	attr := map[string]Attribute{
		"uuid": AttrUUID, "description": AttrDescription,
		"status": AttrStatus, "entry": AttrEntry, "modified": AttrModified,
		"end": AttrEnd, "due": AttrDue, "project": AttrProject,
		"priority": AttrPriority,
	}[base]
	if attr == "" {
		return "", "", fmt.Errorf("unknown attribute %q", base)
	}
	if !hasSuffix {
		return attr, OpEqual, nil
	}
	switch suffix {
	case "before":
		return attr, OpBefore, nil
	case "after":
		return attr, OpAfter, nil
	default:
		return "", "", fmt.Errorf("unknown modifier %q", suffix)
	}
}

func (p *queryParser) startsExpression() bool {
	if !p.hasNext() {
		return false
	}
	switch p.peek() {
	case ")", "or", "xor":
		return false
	default:
		return true
	}
}

func (p *queryParser) match(text string) bool {
	if p.hasNext() && p.peek() == text {
		p.pos++
		return true
	}
	return false
}

func (p *queryParser) hasNext() bool { return p.pos < len(p.tokens) }
func (p *queryParser) peek() string  { return p.tokens[p.pos].Text }
func (p *queryParser) next() string {
	text := p.peek()
	p.pos++
	return text
}
```

- [x] **Step 4: 兼容 `ParseFilters`**

修改 `internal/query/parser.go`：

- 保留 `ParseAddArgs` 和 `ParseModifyArgs`。
- `ParseFilters(args []string)` 可以继续返回旧 `Filter`，但新增 `ParseFilterExpr(args []string) (Expr, error)`。
- `ParseFilterExpr` 行为：
  - `args` 为空返回 `nil`。
  - 如果 `args` 中只有一个带空格的表达式，直接 `ParseQuery(args[0])`。
  - 否则 `strings.Join(args, " ")` 后 `ParseQuery`。

示例实现：

```go
func ParseFilterExpr(args []string) (Expr, error) {
	if len(args) == 0 {
		return nil, nil
	}
	return ParseQuery(strings.Join(args, " "))
}
```

同时在 `internal/query/date.go` 增加：

```go
func ParseDateValue(raw string) Value {
	return DateValue(raw)
}

func ResolveDateValue(value Value, nowUnix int64, loc *time.Location) (int64, error) {
	now := time.Unix(nowUnix, 0).In(loc)
	return ParseDate(value.Raw, now, loc)
}
```

parser 只调用 `ParseDateValue`，不得在 parse 阶段调用 `time.Now()`。

- [x] **Step 5: 运行测试**

Run: `go test ./internal/query -v`  
Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add internal/query/parser_ast.go internal/query/parser_ast_test.go internal/query/parser.go
git commit -m "feat: 实现查询 AST parser"
```

## Chunk 2: AST 到 GORM 查询编译

### 文件职责

- Create: `internal/storage/query_scope.go`
  - 将 query AST 编译成 GORM scopes。
- Modify: `internal/storage/task_repo.go`
  - 支持 `Query Expr` 与 sort 选项。
- Modify: `internal/app/service.go`
  - `ListInput` 增加 `Query query.Expr`。
- Test: `internal/storage/query_scope_test.go`
- Test: `internal/app/service_test.go`

### Task 4: 实现 AST 到 GORM scope

**Files:**

- Create: `internal/storage/query_scope.go`
- Create: `internal/storage/query_scope_test.go`
- Modify: `internal/storage/task_repo.go`

- [x] **Step 1: 写失败测试**

创建 `internal/storage/query_scope_test.go`：

```go
package sqlite

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dajee/taskg/internal/query"
	domain "github.com/dajee/taskg/internal/task"
)

func TestListWithQueryExprSupportsOrAndTags(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	mustCreate(t, repo, domain.Task{UUID: "1", WorkspaceID: ws.ID, Description: "work task", Status: domain.StatusPending, Entry: 1, Modified: 1, Tags: []string{"work"}})
	mustCreate(t, repo, domain.Task{UUID: "2", WorkspaceID: ws.ID, Description: "home task", Status: domain.StatusPending, Entry: 2, Modified: 2, Tags: []string{"home"}})
	t.Cleanup(func() { _ = store.Close() })

	expr, err := query.ParseQuery(`+work or /home/`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.List(ws.ID, ListOptions{Query: expr})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
}

func TestListWithQueryExprUsesBoundParameters(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	mustCreate(t, repo, domain.Task{UUID: "1", WorkspaceID: ws.ID, Description: "safe", Status: domain.StatusPending, Entry: 1, Modified: 1})
	t.Cleanup(func() { _ = store.Close() })

	expr, err := query.ParseQuery(`/x%' OR 1=1 --/`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.List(ws.ID, ListOptions{Query: expr})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("injection-like query matched tasks: %#v", tasks)
	}
}

func TestCompileQueryXorUsesBooleanCoalesce(t *testing.T) {
	expr, err := query.ParseQuery(`+work xor due:`)
	if err != nil {
		t.Fatal(err)
	}
	sql, _, err := CompileQuery(expr, QueryCompileOptions{NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	if !strings.Contains(sql, "COALESCE") || !strings.Contains(sql, "<>") {
		t.Fatalf("xor SQL = %s", sql)
	}
}

func TestCompileQueryDueEmptyIsNull(t *testing.T) {
	expr, err := query.ParseQuery(`due:`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := CompileQuery(expr, QueryCompileOptions{NowUnix: 100})
	if err != nil {
		t.Fatalf("CompileQuery() error = %v", err)
	}
	if sql != "due IS NULL" || len(args) != 0 {
		t.Fatalf("sql = %q args = %#v", sql, args)
	}
}

func TestQueryXorTruthTable(t *testing.T) {
	store, repo, ws := newQueryTestStore(t)
	now := int64(1000)
	due := now + 3600
	mustCreate(t, repo, domain.Task{UUID: "a", WorkspaceID: ws.ID, Description: "both true", Status: domain.StatusPending, Entry: 1, Modified: 1, Tags: []string{"work"}})
	mustCreate(t, repo, domain.Task{UUID: "b", WorkspaceID: ws.ID, Description: "tag only", Status: domain.StatusPending, Entry: 1, Modified: 1, Due: &due, Tags: []string{"work"}})
	mustCreate(t, repo, domain.Task{UUID: "c", WorkspaceID: ws.ID, Description: "null due only", Status: domain.StatusPending, Entry: 1, Modified: 1})
	mustCreate(t, repo, domain.Task{UUID: "d", WorkspaceID: ws.ID, Description: "both false", Status: domain.StatusPending, Entry: 1, Modified: 1, Due: &due})
	t.Cleanup(func() { _ = store.Close() })

	expr, err := query.ParseQuery(`+work xor due:`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.List(ws.ID, ListOptions{Query: expr, NowUnix: now})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := taskUUIDs(tasks)
	sort.Strings(got)
	want := []string{"b", "c"}
	if !slices.Equal(got, want) {
		t.Fatalf("uuids = %#v, want %#v", got, want)
	}
}

func taskUUIDs(tasks []domain.Task) []string {
	out := make([]string, len(tasks))
	for i, tsk := range tasks {
		out[i] = tsk.UUID
	}
	return out
}

func newQueryTestStore(t *testing.T) (*Store, *TaskRepository, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return store, NewTaskRepository(store.DB()), ws
}

func mustCreate(t *testing.T, repo *TaskRepository, task domain.Task) {
	t.Helper()
	if _, err := repo.Create(task); err != nil {
		t.Fatalf("Create(%s) error = %v", task.UUID, err)
	}
}

var _ = strings.Contains
```

如果 `strings` 没用到，实施时删除该 import 和占位。

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/storage -run QueryExpr -v`  
Expected: FAIL，`ListOptions.Query` 尚不存在。

- [x] **Step 3: 扩展 ListOptions**

修改 `internal/storage/task_repo.go`：

```go
import "github.com/dajee/taskg/internal/query"

type ListOptions struct {
	Status   string
	Project  *string
	Priority *string
	Tags     []string
	Text     *string
	Sort     string
	Query    query.Expr
	NowUnix  int64
}
```

在 `List` 中，在旧字段过滤之后添加：

```go
if opts.Query != nil {
	q = ApplyQuery(q, opts.Query, QueryCompileOptions{NowUnix: opts.NowUnix})
}
```

保留旧字段过滤，作为 M0 兼容层。后续 CLI 会优先使用 AST。

- [x] **Step 4: 实现 ApplyQuery**

创建 `internal/storage/query_scope.go`：

```go
package sqlite

import (
	"fmt"
	"time"

	"github.com/dajee/taskg/internal/query"
	"gorm.io/gorm"
)

type QueryCompileOptions struct {
	NowUnix int64
}

func ApplyQuery(db *gorm.DB, expr query.Expr, opts QueryCompileOptions) *gorm.DB {
	sql, args, err := CompileQuery(expr, opts)
	if err != nil {
		return db.AddError(err)
	}
	if sql == "" {
		return db
	}
	return db.Where(sql, args...)
}

func CompileQuery(expr query.Expr, opts QueryCompileOptions) (string, []any, error) {
	switch e := expr.(type) {
	case nil:
		return "", nil, nil
	case query.Predicate:
		return compilePredicate(e, opts)
	case query.Binary:
		leftSQL, leftArgs, err := CompileQuery(e.Left, opts)
		if err != nil {
			return "", nil, err
		}
		rightSQL, rightArgs, err := CompileQuery(e.Right, opts)
		if err != nil {
			return "", nil, err
		}
		if e.Op == "xor" {
			sql := "(COALESCE((" + leftSQL + "), 0) <> COALESCE((" + rightSQL + "), 0))"
			return sql, append(leftArgs, rightArgs...), nil
		}
		op := map[string]string{"and": "AND", "or": "OR"}[e.Op]
		if op == "" {
			return "", nil, fmt.Errorf("unknown binary op %q", e.Op)
		}
		return "(" + leftSQL + " " + op + " " + rightSQL + ")", append(leftArgs, rightArgs...), nil
	case query.Unary:
		sql, args, err := CompileQuery(e.Expr, opts)
		if err != nil {
			return "", nil, err
		}
		if e.Op != "not" {
			return "", nil, fmt.Errorf("unknown unary op %q", e.Op)
		}
		return "(NOT " + sql + ")", args, nil
	default:
		return "", nil, fmt.Errorf("unsupported query expr %T", expr)
	}
}

func compilePredicate(p query.Predicate, opts QueryCompileOptions) (string, []any, error) {
	value := p.Value.Text
	switch p.Attribute {
	case query.AttrStatus:
		return compareColumn("status", p.Operator, value, nil)
	case query.AttrProject:
		return compareColumn("project", p.Operator, value, nil)
	case query.AttrPriority:
		return compareColumn("priority", p.Operator, value, nil)
	case query.AttrUUID:
		return compareColumn("uuid", p.Operator, value, nil)
	case query.AttrBare:
		return "description LIKE ?", []any{"%" + value + "%"}, nil
	case query.AttrDescription:
		if p.Operator == query.OpContains {
			return "description LIKE ?", []any{"%" + value + "%"}, nil
		}
		return compareColumn("description", p.Operator, value, nil)
	case query.AttrDue:
		return compareDateColumn("due", p, opts)
	case query.AttrEntry:
		return compareDateColumn("entry", p, opts)
	case query.AttrModified:
		return compareDateColumn("modified", p, opts)
	case query.AttrEnd:
		return compareDateColumn("end_ts", p, opts)
	case query.AttrTag:
		if p.Operator == query.OpHasTag {
			return "uuid IN (SELECT task_uuid FROM task_tags WHERE tag = ?)", []any{value}, nil
		}
		if p.Operator == query.OpMissingTag {
			return "uuid NOT IN (SELECT task_uuid FROM task_tags WHERE tag = ?)", []any{value}, nil
		}
	}
	return "", nil, fmt.Errorf("unsupported predicate %s", p.String())
}

func compareColumn(column string, op query.Operator, value string, intValue *int64) (string, []any, error) {
	arg := any(value)
	if intValue != nil {
		arg = *intValue
	}
	switch op {
	case query.OpEqual:
		return column + " = ?", []any{arg}, nil
	case query.OpBefore:
		return column + " < ?", []any{arg}, nil
	case query.OpAfter:
		return column + " > ?", []any{arg}, nil
	case query.OpContains:
		return column + " LIKE ?", []any{"%" + value + "%"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator %q", op)
	}
}

func compareDateColumn(column string, p query.Predicate, opts QueryCompileOptions) (string, []any, error) {
	if p.Operator == query.OpIsNull {
		return column + " IS NULL", nil, nil
	}
	value, err := query.ResolveDateValue(p.Value, opts.NowUnix, time.Local)
	if err != nil {
		return "", nil, err
	}
	if p.Operator == query.OpEqual {
		end := value + 86400
		return column + " >= ? AND " + column + " < ?", []any{value, end}, nil
	}
	return compareColumn(column, p.Operator, "", &value)
}
```

实施时注意：`AttrID` 不进入 AST，不允许把裸 token 编译成 `uuid = ?`。数字 working-set ID 和 UUID 前缀只在 CLI/app 的 target 解析路径处理；查询路径中的裸 token 转为 description 子串。

- [x] **Step 5: 运行测试**

Run: `go test ./internal/storage -run QueryExpr -v`  
Expected: PASS。

- [x] **Step 6: 回归测试**

Run: `go test ./internal/storage ./internal/query -v`  
Expected: PASS。

- [x] **Step 7: 提交**

```bash
git add internal/storage/query_scope.go internal/storage/query_scope_test.go internal/storage/task_repo.go
git commit -m "feat: 编译查询 AST 到 GORM"
```

### Task 5: App List 接入 Query AST

**Files:**

- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/cli/list.go`

- [x] **Step 1: 写 app 层失败测试**

在 `internal/app/service_test.go` 增加：

```go
func TestServiceListWithQueryExpr(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "taskg.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc, _ := NewService(ServiceOptions{Store: store, Clock: fixedClock{NowUnix: 100}})

	_, _ = svc.Add(AddInput{Description: "work task", Tags: []string{"work"}})
	_, _ = svc.Add(AddInput{Description: "home task", Tags: []string{"home"}})

	expr, err := query.ParseQuery(`+work or /home/`)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := svc.List(ListInput{Query: expr})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
}
```

记得 import `github.com/dajee/taskg/internal/query`。

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app -run TestServiceListWithQueryExpr -v`  
Expected: FAIL，`ListInput.Query` 尚不存在。

- [x] **Step 3: 修改 app service**

在 `internal/app/service.go`：

```go
import "github.com/dajee/taskg/internal/query"

type ListInput struct {
	Target   *string
	Status   string
	Project  *string
	Priority *string
	Tags     []string
	Text     *string
	Sort     string
	Query    query.Expr
}
```

`repo.List` 调用传递 `Query: input.Query, NowUnix: s.clock.Unix()`，让 lazy date 在执行时使用 service clock。

- [x] **Step 4: CLI list 使用 AST**

修改 `internal/cli/list.go`：

- 如果 args 是单个纯整数、完整 36 字符 UUID 或完整 32 字符无连字符 UUID，先走 `input.Target`。
- 否则调用 `query.ParseFilterExpr(args)` 并设置 `input.Query`。
- 若 parser 返回 `AttrBare`，在查询语义中保留为 description 子串；不要再把它改写为 `AttrID`。

建议加 helper：

```go
func isPlainTargetArg(args []string) bool
```

该 helper 只用于单个 target 参数，不作为查询 token 白名单；复杂查询是否有效由 `ParseFilterExpr` 决定。M1 不支持 UUID 短前缀 target，`abcd list` 这类裸 token 会作为 description 子串查询，M2 再评估是否放宽。

在 `internal/cli/list_test.go` 或当前 CLI 测试文件中补 helper 测试：

```go
func TestIsPlainTargetArg(t *testing.T) {
	if !isPlainTargetArg([]string{"1"}) {
		t.Fatal("numeric id should be target")
	}
	if !isPlainTargetArg([]string{"550e8400-e29b-41d4-a716-446655440000"}) {
		t.Fatal("full UUID should be target")
	}
	if isPlainTargetArg([]string{"abcd"}) || isPlainTargetArg([]string{"foo"}) {
		t.Fatal("bare words and short UUID prefixes should be query text in M1")
	}
}
```

- [x] **Step 5: 运行测试**

Run:

```bash
go test ./internal/app -run TestServiceListWithQueryExpr -v
go test ./internal/cli ./tests/integration -v
```

Expected: PASS，M0 prefix filter 和 target filter 不退化。

- [x] **Step 6: 提交**

```bash
git add internal/app/service.go internal/app/service_test.go internal/cli/list.go
git commit -m "feat: 列表查询接入 AST"
```

## Chunk 3: Report 与 Urgency

### 文件职责

- Create: `internal/urgency/urgency.go`
- Create: `internal/urgency/explain.go`
- Test: `internal/urgency/urgency_test.go`
- Create: `internal/report/report.go`
- Create: `internal/report/registry.go`
- Test: `internal/report/report_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/cli/list.go`
- Create/Modify: `internal/cli/report.go`
- Create: `internal/cli/urgency.go`

### Task 6: 实现 Urgency 计算与 Explain

**Files:**

- Create: `internal/urgency/urgency.go`
- Create: `internal/urgency/explain.go`
- Create: `internal/urgency/urgency_test.go`

- [x] **Step 1: 写失败测试**

创建 `internal/urgency/urgency_test.go`：

```go
package urgency

import (
	"testing"

	"github.com/dajee/taskg/internal/task"
)

func TestExplainIncludesNextAndPriority(t *testing.T) {
	priority := "H"
	tsk := task.Task{
		UUID: "u1", Description: "important", Status: task.StatusPending,
		Entry: 0, Modified: 0, Priority: &priority, Tags: []string{"next"},
	}
	explain := Explain(tsk, Options{NowUnix: 86400})
	if explain.Total < 21 {
		t.Fatalf("Total = %f, want next + H contribution", explain.Total)
	}
	if !hasItem(explain, "tag.next") || !hasItem(explain, "priority.H") {
		t.Fatalf("items = %#v", explain.Items)
	}
}

func TestExplainTotalEqualsItemSum(t *testing.T) {
	tsk := task.Task{UUID: "u1", Description: "task", Status: task.StatusPending, Entry: 0, Modified: 0, Tags: []string{"a", "b"}}
	explain := Explain(tsk, Options{NowUnix: 86400})
	var sum float64
	for _, item := range explain.Items {
		sum += item.Contribution
	}
	if explain.Total != sum {
		t.Fatalf("Total = %f, sum = %f", explain.Total, sum)
	}
}

func TestDueContributionBoundaries(t *testing.T) {
	now := int64(100 * 86400)
	for _, tc := range []struct {
		name string
		days int64
		wantPositive bool
	}{
		{name: "today", days: 0, wantPositive: true},
		{name: "three days", days: 3, wantPositive: true},
		{name: "seven days", days: 7, wantPositive: false},
		{name: "thirty days", days: 30, wantPositive: false},
	} {
		due := now + tc.days*86400
		tsk := task.Task{UUID: "u1", Description: "task", Status: task.StatusPending, Entry: now - 86400, Modified: now - 86400, Due: &due}
		explain := Explain(tsk, Options{NowUnix: now})
		got := hasItem(explain, "due")
		if got != tc.wantPositive {
			t.Fatalf("%s due item = %v, want %v", tc.name, got, tc.wantPositive)
		}
	}
}

func TestPriorityAndMultipleTags(t *testing.T) {
	for _, priority := range []string{"L", "M", "H"} {
		tsk := task.Task{UUID: "u1", Description: "task", Status: task.StatusPending, Entry: 0, Modified: 0, Priority: &priority, Tags: []string{"a", "b", "c"}}
		explain := Explain(tsk, Options{NowUnix: 86400})
		if !hasItem(explain, "priority."+priority) || !hasItem(explain, "tags") {
			t.Fatalf("priority %s items = %#v", priority, explain.Items)
		}
	}
}

func hasItem(explain ExplainResult, name string) bool {
	for _, item := range explain.Items {
		if item.Name == name {
			return true
		}
	}
	return false
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/urgency -v`  
Expected: FAIL，package 不存在。

- [x] **Step 3: 实现 urgency**

创建 `internal/urgency/explain.go`：

```go
package urgency

type ExplainResult struct {
	UUID  string        `json:"uuid"`
	Total float64       `json:"total"`
	Items []ExplainItem `json:"items"`
}

type ExplainItem struct {
	Name         string  `json:"name"`
	Coefficient  float64 `json:"coefficient"`
	Raw          any     `json:"raw,omitempty"`
	Contribution float64 `json:"contribution"`
	Reason       string  `json:"reason"`
}

type Options struct {
	NowUnix int64
}
```

创建 `internal/urgency/urgency.go`：

```go
package urgency

import (
	"slices"

	"github.com/dajee/taskg/internal/task"
)

const (
	coefTagNext   = 15.0
	coefDue       = 12.0
	coefPriorityH = 6.0
	coefPriorityM = 3.9
	coefPriorityL = 1.8
	coefAge       = 2.0
	coefTags      = 1.0
	coefProject   = 1.0
	ageMaxDays    = 365.0
)

func Explain(tsk task.Task, opts Options) ExplainResult {
	var result ExplainResult
	result.UUID = tsk.UUID
	add := func(item ExplainItem) {
		result.Items = append(result.Items, item)
		result.Total += item.Contribution
	}
	if slices.Contains(tsk.Tags, "next") {
		add(ExplainItem{Name: "tag.next", Coefficient: coefTagNext, Raw: "next", Contribution: coefTagNext, Reason: "task has +next"})
	}
	if tsk.Due != nil {
		contribution := dueContribution(*tsk.Due, opts.NowUnix)
		if contribution > 0 {
			add(ExplainItem{Name: "due", Coefficient: coefDue, Raw: *tsk.Due, Contribution: contribution, Reason: "task has due date"})
		}
	}
	if tsk.Priority != nil {
		switch *tsk.Priority {
		case "H":
			add(ExplainItem{Name: "priority.H", Coefficient: coefPriorityH, Raw: "H", Contribution: coefPriorityH, Reason: "priority is H"})
		case "M":
			add(ExplainItem{Name: "priority.M", Coefficient: coefPriorityM, Raw: "M", Contribution: coefPriorityM, Reason: "priority is M"})
		case "L":
			add(ExplainItem{Name: "priority.L", Coefficient: coefPriorityL, Raw: "L", Contribution: coefPriorityL, Reason: "priority is L"})
		}
	}
	if opts.NowUnix > tsk.Entry {
		days := float64(opts.NowUnix-tsk.Entry) / 86400.0
		if days > ageMaxDays {
			days = ageMaxDays
		}
		contribution := coefAge * (days / ageMaxDays)
		if contribution > 0 {
			add(ExplainItem{Name: "age", Coefficient: coefAge, Raw: days, Contribution: contribution, Reason: "task age"})
		}
	}
	if len(tsk.Tags) > 0 {
		add(ExplainItem{Name: "tags", Coefficient: coefTags, Raw: len(tsk.Tags), Contribution: tagCountContribution(len(tsk.Tags)), Reason: "task has tags"})
	}
	if tsk.Project != nil && *tsk.Project != "" {
		add(ExplainItem{Name: "project", Coefficient: coefProject, Raw: *tsk.Project, Contribution: coefProject, Reason: "task has project"})
	}
	return result
}

func dueContribution(due, now int64) float64 {
	if due <= now {
		return coefDue
	}
	days := float64(due-now) / 86400.0
	if days > 7 {
		return 0
	}
	return coefDue * ((7 - days) / 7)
}

func tagCountContribution(n int) float64 {
	switch {
	case n <= 0:
		return 0
	case n == 1:
		return 0.8
	case n == 2:
		return 0.9
	default:
		return 1.0
	}
}
```

- [x] **Step 4: 运行测试**

Run: `go test ./internal/urgency -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/urgency/urgency.go internal/urgency/explain.go internal/urgency/urgency_test.go
git commit -m "feat: 添加 urgency explain"
```

### Task 7: 增加 Report Registry

**Files:**

- Create: `internal/report/report.go`
- Create: `internal/report/registry.go`
- Create: `internal/report/report_test.go`
- Modify: `internal/app/service.go`

- [x] **Step 1: 写失败测试**

创建 `internal/report/report_test.go`：

```go
package report

import "testing"

func TestDefaultRegistryHasM1Reports(t *testing.T) {
	reg := DefaultRegistry()
	for _, name := range []string{"list", "next", "all", "completed", "deleted", "overdue"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("missing report %s", name)
		}
	}
	for _, name := range []string{"waiting", "active", "ready", "blocked", "blocking"} {
		if _, ok := reg.Get(name); ok {
			t.Fatalf("M1 should not register report %s", name)
		}
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/report -v`  
Expected: FAIL，package 不存在。

- [x] **Step 3: 实现 report registry**

创建 `internal/report/report.go`：

```go
package report

import "github.com/dajee/taskg/internal/query"

type Definition struct {
	Name        string
	Description string
	FilterSource string
	Filter      query.Expr
	Sort        string
	Columns     []string
}
```

创建 `internal/report/registry.go`：

```go
package report

import "github.com/dajee/taskg/internal/query"

type Registry struct {
	defs map[string]Definition
}

func DefaultRegistry() Registry {
	defs := map[string]Definition{}
	add := func(def Definition) { defs[def.Name] = def }
	add(mustDefinition(Definition{Name: "list", Description: "Pending tasks", FilterSource: "status:pending", Sort: "entry", Columns: defaultColumns()}))
	add(mustDefinition(Definition{Name: "next", Description: "Next tasks", FilterSource: "status:pending", Sort: "urgency", Columns: defaultColumns()}))
	add(Definition{Name: "all", Description: "All tasks", Sort: "entry", Columns: defaultColumns()})
	add(mustDefinition(Definition{Name: "completed", Description: "Completed tasks", FilterSource: "status:completed", Sort: "completed", Columns: defaultColumns()}))
	add(mustDefinition(Definition{Name: "deleted", Description: "Deleted tasks", FilterSource: "status:deleted", Sort: "completed", Columns: defaultColumns()}))
	add(mustDefinition(Definition{Name: "overdue", Description: "Overdue tasks", FilterSource: "status:pending due.before:today", Sort: "due", Columns: defaultColumns()}))
	return Registry{defs: defs}
}

func (r Registry) Get(name string) (Definition, bool) {
	def, ok := r.defs[name]
	return def, ok
}

func mustDefinition(def Definition) Definition {
	expr, err := query.ParseQuery(def.FilterSource)
	if err != nil {
		panic(err)
	}
	def.Filter = expr
	return def
}

func defaultColumns() []string {
	return []string{"id", "uuid", "priority", "project", "tags", "description"}
}
```

这里保留 `FilterSource` 是为了调试和未来自定义 report 导出；实际避免日期冻结靠的是 query AST 中的 lazy date value，以及 storage 编译阶段的 `QueryCompileOptions.NowUnix`。

- [x] **Step 4: 运行测试**

Run: `go test ./internal/report -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/report/report.go internal/report/registry.go internal/report/report_test.go
git commit -m "feat: 添加 M1 内置报表定义"
```

### Task 8: Service 和 CLI 接入 Report + Urgency 排序

**Files:**

- Modify: `internal/app/service.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/cli/list.go`
- Create: `internal/cli/report.go`
- Create: `internal/cli/urgency.go`
- Modify: `internal/render/json.go`
  - `urgency --json` 输出 explain JSON，report JSON 可附加 urgency 字段。
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写 CLI 失败测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIReportsAndUrgency(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "normal", "task")
	run(t, bin, "--db", db, "add", "next", "task", "+next", "priority:H")
	nextUUID := strings.TrimSpace(run(t, bin, "--db", db, "_uuids", "+next"))
	run(t, bin, "--db", db, "1", "done")

	all := run(t, bin, "--db", db, "all")
	if !strings.Contains(all, "normal task") || !strings.Contains(all, "next task") {
		t.Fatalf("all output = %q", all)
	}
	completed := run(t, bin, "--db", db, "completed")
	if !strings.Contains(completed, "normal task") {
		t.Fatalf("completed output = %q", completed)
	}
	urg := run(t, bin, "--db", db, "urgency", nextUUID)
	if !strings.Contains(urg, "tag.next") || !strings.Contains(urg, "priority.H") {
		t.Fatalf("urgency output = %q", urg)
	}

	conflict := run(t, bin, "--db", db, "completed", "status:pending")
	if strings.Contains(conflict, "normal task") || strings.Contains(conflict, "next task") {
		t.Fatalf("conflicting report filter output = %q", conflict)
	}
}
```

注意：测试使用 `_uuids +next` 捕获稳定 target，避免完成任务后 working-set ID 重排导致误测。

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIReportsAndUrgency -v`  
Expected: FAIL，新命令尚不存在。

- [x] **Step 3: Service 增加 RunReport 和 ExplainUrgency**

在 `internal/app/service.go` 增加：

```go
type ReportInput struct {
	Name  string
	Query query.Expr
}

type ReportResult struct {
	Tasks []task.Task
}

func (s *Service) RunReport(input ReportInput) (ReportResult, error)
func (s *Service) ExplainUrgency(target string) (urgency.ExplainResult, error)
```

`RunReport` 行为：

- 从 `report.DefaultRegistry()` 查 definition。
- 合并 `definition.Filter` 与 `input.Query`：`query.And(def.Filter, input.Query)`。
- report 默认 filter 与用户 filter 始终 AND 合并；冲突时返回空结果。用户想绕开默认状态限制时使用 `all`。
- 调用 `List(ListInput{Query: merged, Sort: def.Sort})`。

`ExplainUrgency` 行为：

- ResolveTarget。
- 调用 `urgency.Explain(tsk, urgency.Options{NowUnix: s.clock.Unix()})`。

- [x] **Step 4: Storage 支持 sort**

修改 `TaskRepository.List`：

- `Sort == "entry"`：`entry ASC`。
- `Sort == "completed"`：`end_ts DESC, modified DESC`。
- `Sort == "due"`：`due ASC`。
- `Sort == "urgency"`：repo 不加 ORDER BY，由 app 层全量稳定排序；不要在 SQL 中复刻 urgency 公式。

推荐：M1 不在 SQL 中完整计算 urgency，避免复杂 SQL 与 Go 逻辑分叉。`app.RunReport` 对 `Sort == "urgency"` 的结果二次排序，并预计算 urgency，避免比较器内重复 `Explain`：

```go
type taskWithUrgency struct {
	Task  task.Task
	Total float64
}

withUrgency := make([]taskWithUrgency, len(tasks))
for i, tsk := range tasks {
	explain := urgency.Explain(tsk, urgency.Options{NowUnix: s.clock.Unix()})
	withUrgency[i] = taskWithUrgency{Task: tsk, Total: explain.Total}
}
sort.SliceStable(withUrgency, func(i, j int) bool {
	return withUrgency[i].Total > withUrgency[j].Total
})
```

- [x] **Step 5: CLI 增加 report 命令**

实现 `internal/cli/report.go`：

- 注册 `all`、`completed`、`deleted`、`overdue`。
- `list`、`next` 也可以改为调用 `RunReport`，减少分叉。
- prefix filter 继续通过 root reorder 传入。
- 命令 args 使用 `query.ParseFilterExpr(args)`。
- 增加 report 合并测试：`completed status:pending` 返回空结果，证明默认 filter 与用户 filter 通过 AND 合并。

- [x] **Step 6: CLI 增加 urgency 命令**

创建 `internal/cli/urgency.go`：

- `taskg urgency <target>` human 输出 explain。
- `taskg urgency --json <target>` 输出 explain JSON。
- `_urgency <target>` 只输出数字。

- [x] **Step 7: 运行测试**

Run:

```bash
go test ./internal/app ./internal/report ./internal/urgency -v
go test ./tests/integration -run TestCLIReportsAndUrgency -v
go test ./...
```

Expected: PASS。

- [x] **Step 8: 提交**

```bash
git add internal/app/service.go internal/storage/task_repo.go internal/cli/list.go internal/cli/report.go internal/cli/urgency.go internal/render/json.go tests/integration/cli_test.go
git commit -m "feat: 接入报表与 urgency"
```

## Chunk 4: DOM 与 Helper 命令

### 文件职责

- Create: `internal/dom/dom.go`
- Create: `internal/dom/dom_test.go`
- Create: `internal/cli/helper.go`
- Modify: `internal/app/service.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `internal/cli/root.go`
- Test: `tests/integration/cli_test.go`

### Task 9: 实现 DOM Field Resolver

**Files:**

- Create: `internal/dom/dom.go`
- Create: `internal/dom/dom_test.go`

- [x] **Step 1: 写失败测试**

创建 `internal/dom/dom_test.go`：

```go
package dom

import (
	"testing"

	"github.com/dajee/taskg/internal/task"
)

func TestResolveTaskField(t *testing.T) {
	project := "work"
	tsk := task.Task{UUID: "u1", Description: "write spec", Status: task.StatusPending, Project: &project, Tags: []string{"next"}}
	got, err := Resolve(tsk, "description", 12.5)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "write spec" {
		t.Fatalf("got %q", got)
	}
	tag, err := Resolve(tsk, "tag.next", 12.5)
	if err != nil {
		t.Fatalf("Resolve(tag.next) error = %v", err)
	}
	if tag != "next" {
		t.Fatalf("tag = %q", tag)
	}
}

func TestResolveUnknownField(t *testing.T) {
	if _, err := Resolve(task.Task{}, "foo", 0); err == nil {
		t.Fatal("Resolve() error = nil, want error")
	}
}

func TestResolveMissingVirtualTagReturnsEmpty(t *testing.T) {
	got, err := Resolve(task.Task{Tags: []string{"next"}}, "tag.urgent", 0)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/dom -v`  
Expected: FAIL。

- [x] **Step 3: 实现 Resolve**

创建 `internal/dom/dom.go`：

```go
package dom

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dajee/taskg/internal/task"
)

func Resolve(tsk task.Task, field string, urgency float64) (string, error) {
	switch field {
	case "uuid":
		return tsk.UUID, nil
	case "description":
		return tsk.Description, nil
	case "status":
		return tsk.Status, nil
	case "entry":
		return strconv.FormatInt(tsk.Entry, 10), nil
	case "modified":
		return strconv.FormatInt(tsk.Modified, 10), nil
	case "due":
		if tsk.Due == nil {
			return "", nil
		}
		return strconv.FormatInt(*tsk.Due, 10), nil
	case "project":
		if tsk.Project == nil {
			return "", nil
		}
		return *tsk.Project, nil
	case "priority":
		if tsk.Priority == nil {
			return "", nil
		}
		return *tsk.Priority, nil
	case "tags":
		return strings.Join(tsk.Tags, ","), nil
	case "urgency":
		return strconv.FormatFloat(urgency, 'f', 3, 64), nil
	}
	if strings.HasPrefix(field, "tag.") {
		tag := strings.TrimPrefix(field, "tag.")
		if slices.Contains(tsk.Tags, tag) {
			return tag, nil
		}
		return "", nil
	}
	return "", fmt.Errorf("unknown DOM field %q", field)
}
```

- [x] **Step 4: 运行测试**

Run: `go test ./internal/dom -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/dom/dom.go internal/dom/dom_test.go
git commit -m "feat: 添加 DOM 字段解析"
```

### Task 10: 实现 Helper CLI

**Files:**

- Create: `internal/cli/helper.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/app/service.go`
- Modify: `internal/storage/task_repo.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLIHelpers(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "work", "task", "project:work", "+next")
	run(t, bin, "--db", db, "add", "home", "task", "project:home", "+later")

	got := run(t, bin, "--db", db, "_get", "1.description", "1.tag.next", "1.tag.missing", "1.urgency")
	if !strings.Contains(got, "work task") || !strings.Contains(got, "next") {
		t.Fatalf("_get output = %q", got)
	}
	if !regexp.MustCompile(`(?m)^\d+\.\d{3}$`).MatchString(got) {
		t.Fatalf("_get output missing urgency line: %q", got)
	}
	ids := run(t, bin, "--db", db, "_ids", "+next")
	if strings.TrimSpace(ids) != "1" {
		t.Fatalf("_ids output = %q", ids)
	}
	projects := run(t, bin, "--db", db, "_projects")
	if !strings.Contains(projects, "home") || !strings.Contains(projects, "work") {
		t.Fatalf("_projects output = %q", projects)
	}
	tags := run(t, bin, "--db", db, "_tags")
	if !strings.Contains(tags, "next") || !strings.Contains(tags, "later") {
		t.Fatalf("_tags output = %q", tags)
	}
	emptyDB := filepath.Join(t.TempDir(), "empty.db")
	if projects := run(t, bin, "--db", emptyDB, "_projects"); strings.TrimSpace(projects) != "" {
		t.Fatalf("empty _projects output = %q", projects)
	}
	if tags := run(t, bin, "--db", emptyDB, "_tags"); strings.TrimSpace(tags) != "" {
		t.Fatalf("empty _tags output = %q", tags)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLIHelpers -v`  
Expected: FAIL。

- [x] **Step 3: app 增加 helper 方法**

在 `internal/app/service.go` 增加：

```go
func (s *Service) Projects() ([]string, error)
func (s *Service) Tags() ([]string, error)
func (s *Service) IDs(input ListInput) ([]int, error)
func (s *Service) UUIDs(input ListInput) ([]string, error)
```

实现建议：

- `IDs` 和 `UUIDs` 调用 `List(input)`。
- `Projects` 和 `Tags` 可通过 repo distinct 查询。

- [x] **Step 4: storage 增加 distinct 查询**

在 `internal/storage/task_repo.go`：

```go
func (r *TaskRepository) Projects(workspaceID string) ([]string, error)
func (r *TaskRepository) Tags(workspaceID string) ([]string, error)
```

要求：

- 去重。
- 排序。
- 空 project 不返回。

- [x] **Step 5: 实现 helper 命令**

创建 `internal/cli/helper.go`：

- `_get [expr...]`
- `_ids [filters...]`
- `_uuids [filters...]`
- `_projects`
- `_tags`

`_get` 解析：

- `1.description`：target = `1`，field = `description`。
- `<uuid>.description`：target = uuid，field = `description`。
- `1.tag.next`：target = `1`，field = `tag.next`。
- 当 field 为 `urgency` 时，CLI 先调用 `urgency.Explain(tsk, urgency.Options{NowUnix: s.clock.Unix()}).Total`，再传给 `dom.Resolve`。
- `tag.*` 不存在时输出空行并继续成功；未知 DOM 字段仍返回非零 exit code。

每个表达式一行输出。

- [x] **Step 6: root 注册 helper 命令**

修改 `internal/cli/root.go`：

- `cmd.AddCommand(newGetCommand(opts))`
- `cmd.AddCommand(newIDsCommand(opts))`
- `cmd.AddCommand(newUUIDsCommand(opts))`
- `cmd.AddCommand(newProjectsCommand(opts))`
- `cmd.AddCommand(newTagsCommand(opts))`
- knownSubcommands 增加 `_get`、`_ids`、`_uuids`、`_projects`、`_tags`、`_urgency`。

- [x] **Step 7: 运行测试**

Run:

```bash
go test ./internal/dom -v
go test ./tests/integration -run TestCLIHelpers -v
go test ./...
```

Expected: PASS。

- [x] **Step 8: 提交**

```bash
git add internal/cli/helper.go internal/cli/root.go internal/app/service.go internal/storage/task_repo.go tests/integration/cli_test.go
git commit -m "feat: 添加 DOM helper 命令"
```

## Chunk 5: Root 路由、Calc 与第三方表达式库

### 文件职责

- Modify: `internal/cli/root.go`
  - known subcommands 与 M1 query token reorder。
- Modify: `internal/cli/root_test.go`
  - 覆盖复杂 query、`/.../`、`±tag`、`attr:value` prefix filter 重排。
- Modify: `go.mod`
  - 新增 `github.com/expr-lang/expr`。
- Create: `internal/expr/calc.go`
- Create: `internal/expr/calc_test.go`
- Create: `internal/cli/calc.go`
- Modify: `tests/integration/cli_test.go`

### Task 11: 更新 root reorder 识别 M1 查询 token

**Files:**

- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`

- [x] **Step 1: 写 root reorder 失败测试**

在 `internal/cli/root_test.go` 增加：

```go
func TestRootReorderRecognizesM1QueryTokens(t *testing.T) {
	tests := []struct {
		name string
		args []string
		wantFirst string
	}{
		{name: "parentheses", args: []string{"(project:work and +urgent) or priority:H", "list"}, wantFirst: "list"},
		{name: "slash text", args: []string{"/spec/", "all"}, wantFirst: "all"},
		{name: "multi token bool", args: []string{"+next", "or", "due.before:tomorrow", "list"}, wantFirst: "list"},
		{name: "negative tag", args: []string{"-later", "completed"}, wantFirst: "completed"},
	}
	for _, tt := range tests {
		got := reorderArgs(tt.args, knownSubcommands())
		if got[0] != tt.wantFirst {
			t.Fatalf("%s: subcommand not at front: %#v", tt.name, got)
		}
		if len(got) != len(tt.args) {
			t.Fatalf("%s: token count changed: %#v -> %#v", tt.name, tt.args, got)
		}
	}
}
```

实施时按当前 `root.go` 的 helper 名称调整测试；核心断言是 query token 不能被误判成 target action，也不能阻止 prefix filter 重排。

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cli -run TestRootReorderRecognizesM1QueryTokens -v`  
Expected: FAIL，root reorder 尚未覆盖 M1 token。

- [x] **Step 3: 更新 root reorder**

修改 `internal/cli/root.go`：

- known subcommands 增加 `all`、`completed`、`deleted`、`overdue`、`urgency`、`calc`、`_get`、`_ids`、`_uuids`、`_projects`、`_tags`、`_urgency`。
- prefix query token 识别覆盖：
  - `(` 或 `)` 出现在 token 中。
  - token 以 `/` 开头并以 `/` 结尾。
  - token 以 `+` 或 `-` 开头且长度大于 1。
  - token 包含 `:`，例如 `project:work`、`due.before:tomorrow`。
  - token 为 `and`、`or`、`xor`、`not`。
- target action 仍优先识别 `<target> modify|done|delete|info`。

- [x] **Step 4: 运行测试**

Run: `go test ./internal/cli -v`  
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/cli/root.go internal/cli/root_test.go
git commit -m "feat: 支持 M1 查询参数重排"
```

### Task 12: 用 `expr-lang/expr` 实现 Calc

**Files:**

- Modify: `go.mod`
- Create: `internal/expr/calc.go`
- Create: `internal/expr/calc_test.go`

- [x] **Step 1: 添加依赖**

Run:

```bash
go get github.com/expr-lang/expr@v1.17.7
```

Expected: `go.mod` 和 `go.sum` 更新。  
注意：该库为纯 Go，不应影响 `CGO_ENABLED=0`。

- [x] **Step 2: 写失败测试**

创建 `internal/expr/calc_test.go`：

```go
package expr

import "testing"

func TestCalcArithmetic(t *testing.T) {
	got, err := Calc("1 + 2 * 3")
	if err != nil {
		t.Fatalf("Calc() error = %v", err)
	}
	if got != "7" {
		t.Fatalf("got %q, want 7", got)
	}
}

func TestCalcBoolean(t *testing.T) {
	got, err := Calc("3 > 2 and 1 < 2")
	if err != nil {
		t.Fatalf("Calc() error = %v", err)
	}
	if got != "true" {
		t.Fatalf("got %q, want true", got)
	}
}

func TestCalcRejectsInvalidExpression(t *testing.T) {
	if _, err := Calc("1 +"); err == nil {
		t.Fatal("Calc() error = nil, want error")
	}
}

```

- [x] **Step 3: 实现 Calc**

创建 `internal/expr/calc.go`：

```go
package expr

import (
	"fmt"
	"strconv"

	exprlib "github.com/expr-lang/expr"
)

func Calc(input string) (string, error) {
	program, err := exprlib.Compile(input, exprlib.AsAny())
	if err != nil {
		return "", err
	}
	out, err := exprlib.Run(program, nil)
	if err != nil {
		return "", err
	}
	switch v := out.(type) {
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return fmt.Sprint(v), nil
	}
}
```

实施时核对 `github.com/expr-lang/expr@v1.17.7` 的实际限制 API。若该版本支持运行步数或时间限制，加入小上限并补一个资源限制测试；若不支持，不要臆造 API，保留 `AsAny()` 并在 plan 执行记录中说明后续版本再补。

- [x] **Step 4: 运行测试**

Run: `go test ./internal/expr -v`  
Expected: PASS。

- [x] **Step 5: CGO-free 验证**

Run: `CGO_ENABLED=0 go test ./internal/expr -v`  
Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add go.mod go.sum internal/expr/calc.go internal/expr/calc_test.go
git commit -m "feat: 使用 expr 实现 calc"
```

### Task 13: 增加 Calc CLI

**Files:**

- Create: `internal/cli/calc.go`
- Modify: `internal/cli/root.go`
- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 写失败集成测试**

在 `tests/integration/cli_test.go` 增加：

```go
func TestCLICalc(t *testing.T) {
	bin := buildTaskg(t)
	out := run(t, bin, "calc", "1 + 2 * 3")
	if strings.TrimSpace(out) != "7" {
		t.Fatalf("calc output = %q", out)
	}
	out = run(t, bin, "calc", "3 > 2 and 1 < 2")
	if strings.TrimSpace(out) != "true" {
		t.Fatalf("calc bool output = %q", out)
	}
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./tests/integration -run TestCLICalc -v`  
Expected: FAIL，`calc` 命令尚不存在。

- [x] **Step 3: 实现 calc CLI**

创建 `internal/cli/calc.go`：

```go
package cli

import (
	"fmt"
	"strings"

	calcexpr "github.com/dajee/taskg/internal/expr"
	"github.com/spf13/cobra"
)

func newCalcCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:  "calc <expression>",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := calcexpr.Calc(strings.Join(args, " "))
			if err != nil {
				return fmt.Errorf("calc: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), result)
			return nil
		},
	}
}
```

修改 `internal/cli/root.go`：

- 注册 `newCalcCommand(opts)`。
- knownSubcommands 增加 `calc`。

- [x] **Step 4: 运行测试**

Run:

```bash
go test ./internal/expr ./internal/cli -v
go test ./tests/integration -run TestCLICalc -v
go test ./...
```

Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add internal/cli/calc.go internal/cli/root.go tests/integration/cli_test.go
git commit -m "feat: 添加 calc CLI"
```

## Chunk 6: 集成验收与文档更新

### 文件职责

- Modify: `README.md`
- Modify: `ROADMAP.md`
- Modify: `docs/superpowers/specs/2026-05-28-taskg-m1-design.md`
  - 如实现中产生范围修订，同步回 spec。
- Modify: `AGENTS.md`
  - 如新增约束，同步记录。
- Test: full suite。

### Task 14: 补齐 M1 CLI 集成测试

**Files:**

- Modify: `tests/integration/cli_test.go`

- [x] **Step 1: 增加 spec 中列出的集成测试**

在 `tests/integration/cli_test.go` 添加或确认存在：

```go
func TestCLIM1QueryExamples(t *testing.T) {
	bin := buildTaskg(t)
	db := filepath.Join(t.TempDir(), "taskg.db")
	run(t, bin, "--db", db, "add", "urgent", "work", "task", "project:work", "+urgent", "priority:H")
	run(t, bin, "--db", db, "add", "later", "task", "+later")
	run(t, bin, "--db", db, "add", "next", "task", "+next", "due:2030-01-01")

	out := run(t, bin, "--db", db, "(project:work and +urgent) or priority:H", "list")
	if !strings.Contains(out, "urgent work task") || strings.Contains(out, "later task") {
		t.Fatalf("complex query output = %q", out)
	}
	out = run(t, bin, "--db", db, "+next or due.before:tomorrow", "list")
	if !strings.Contains(out, "next task") {
		t.Fatalf("or query output = %q", out)
	}
}
```

实施时根据 root reorder 行为调整 argv 顺序，确保和 spec 保持一致。

- [x] **Step 2: 运行测试**

Run: `go test ./tests/integration -run 'TestCLIM1|TestCLIHelpers|TestCLICalc|TestCLIReports' -v`  
Expected: PASS。

- [x] **Step 3: 提交**

```bash
git add tests/integration/cli_test.go
git commit -m "test: 补齐 M1 CLI 集成覆盖"
```

### Task 15: 更新 README 和 ROADMAP

**Files:**

- Modify: `README.md`
- Modify: `ROADMAP.md`

- [x] **Step 1: 更新 README**

在 `README.md` 的 M0 用法后新增 M1 用法：

````markdown
## M1 查询与报表用法

```bash
./taskg '+next or due.before:tomorrow' list
./taskg '(project:work and +urgent) or priority:H' list
./taskg all
./taskg completed
./taskg overdue
./taskg urgency 1
./taskg _get 1.description 1.uuid
./taskg _ids +next
./taskg _projects
./taskg _tags
./taskg calc '1 + 2 * 3'
```

补充说明：

> 报表名等价于 `(默认 filter) AND (用户 filter)`。要绕过默认 status 限制，使用 `all`。
````

- [x] **Step 2: 更新 ROADMAP**

在 `ROADMAP.md`：

- 将 M1 状态改为“已完成”。
- 将“当前下一步”改为 M2。
- 如 M1 实际交付范围与 roadmap 有差异，写入简短说明。

- [x] **Step 3: 提交**

```bash
git add README.md ROADMAP.md
git commit -m "docs: 更新 M1 使用说明和路线图"
```

### Task 16: 最终验收

**Files:**

- No code changes expected.

- [x] **Step 1: 全量测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [x] **Step 2: CGO-free 测试**

Run:

```bash
CGO_ENABLED=0 go test ./...
```

Expected: PASS。

- [x] **Step 3: CGO-free build**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/taskg
```

Expected: PASS。

- [x] **Step 4: 确认没有引入 CGO SQLite driver**

Run:

```bash
go list -m all | grep -E 'gorm.io/driver/sqlite|mattn/go-sqlite3' || true
```

Expected: 无输出。

- [x] **Step 5: 手动冒烟**

Run:

```bash
tmp="$(mktemp -d)"
go run ./cmd/taskg --db "$tmp/taskg.db" add "urgent work task" project:work +urgent priority:H
go run ./cmd/taskg --db "$tmp/taskg.db" add "next task" +next due:tomorrow
go run ./cmd/taskg --db "$tmp/taskg.db" '(project:work and +urgent) or +next' list
go run ./cmd/taskg --db "$tmp/taskg.db" next
go run ./cmd/taskg --db "$tmp/taskg.db" urgency 1 --json
go run ./cmd/taskg --db "$tmp/taskg.db" _get 1.description 1.urgency
go run ./cmd/taskg --db "$tmp/taskg.db" calc '1 + 2 * 3'
```

Expected:

- complex query 输出两条任务。
- `next` 按 urgency 排序。
- `urgency --json` 输出 explain JSON。
- `_get` 输出 description 和 urgency。
- `calc` 输出 `7`。

- [x] **Step 6: 检查 git 状态**

Run:

```bash
git status --short --branch
```

Expected: 除用户刻意保留的文件外，没有未提交实现变更。

## 计划审阅说明

本计划根据 [docs/superpowers/specs/2026-05-28-taskg-m1-design.md](/Users/mac/code/projects/dajee/task/docs/superpowers/specs/2026-05-28-taskg-m1-design.md)、[AGENTS.md](/Users/mac/code/projects/dajee/task/AGENTS.md) 和当前 M0 代码编写。计划允许使用第三方 Go 库加快开发；M1 明确选择 `github.com/expr-lang/expr` 处理 `calc`，查询 DSL 仍自研以兼容 Taskwarrior 形态。

2026-05-28 人工评审反馈已处理：修订了 lazy 日期、`/text/` 子串语义、裸 token、root reorder、DOM 虚拟 tag、urgency 排序和测试计划。当前未执行 plan-document-reviewer subagent 审阅；本环境未暴露 subagent 工具，后续如需要严格执行 superpowers 审阅环节，请在实施前补充审阅。实施过程中若发现本计划与 spec 冲突，以 spec 为准并先更新计划。

2026-05-28 二轮人工评审反馈已处理：修正 `Predicate.String()` 引号策略对应测试、`_get` urgency 断言、空引号 tokenizer、target 判定规则、日期等值范围语义、root reorder 断言、xor 真表测试与 README 报表合并说明。本轮人工 reviewer 作为 plan-document-reviewer 的替代审阅记录。
