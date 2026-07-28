package mcpserver

// TaskQueryHelpInput 是 task_query_help 的空入参。
type TaskQueryHelpInput struct{}

// taskFilterSyntaxDoc 是 task filter 表达式的完整语法手册。
// 所有语法点依据 internal/query 包源码（parser_ast.go / evaluator.go / date.go）。
//
// 文档内的表达式用纯文本书写（不使用 markdown 反引号），方便 LLM 直接引用。
const taskFilterSyntaxDoc = `# Task filter 表达式语法

task_query 与 report_run 的 query 参数、CLI 的 list/next/report 位置参数都使用同一套 filter 表达式。

## 1. 高频场景速查

| 场景 | 写法 | 说明 |
|------|------|------|
| 某个用户的待办任务 | assignee:<user ref> status:pending | user ref 支持四种形式：用户 ID（UUID）/ 用户名 / email / provider:external_id，详见下方 assignee 字段说明 |
| 飞书用户的任务 | assignee:feishu:ou_xxx status:pending | provider:external_id 可直接写，无需先查 ID；provider 名以入库为准（如 feishu / wecom / dingtalk） |
| 高优先级待办 | status:pending priority:H | priority 取值 H / M / L |
| 今天到期 | due:today | 不自动排除已完成；要"今天到期且未完成"写 due:today status:pending |
| 本周到期 | due.before:eow | eow = 本周日 23:59:59 |
| 已逾期 | due.before:today status:pending | 注意 due.before:today 不含今天本身（严格在今天之前） |
| 推迟中的任务 | status:waiting | waiting 由 wait 字段驱动：设置了未来 wait 的 pending 任务自动变为 waiting，wait 到期自动转回 pending |
| 某项目的任务 | project:<slug> | 也可用 task_query 顶层参数 project / project_id |
| 含某标签 | +urgent | 必须用 + / -，不能写 tag:xxx |
| 排除某标签 | -blocked | 同上 |
| 循环任务实例 | task_type:occurrence | normal = 普通任务，occurrence = 循环实例；仅支持相等 |
| 没有负责人的任务 | assignee: | 空值即 isnull（列表为空） |
| 有负责人的任务 | assignee.notnull | — |

## 2. 逻辑组合

- 关键字 and / or / not，**必须小写**（大写会被当成普通词）。
- 括号 ( ) 强制改变结合顺序。
- 空格分隔 = 隐式 AND：+work status:pending 等价于 +work and status:pending。
- 优先级：not > and > or。
- xor 不建议使用（语法能解析，但内存求值器不支持，会报错）。

示例：+next or due.before:tomorrow and priority:H 解析为 (+next) or ((due.before:tomorrow) and (priority:H))。

## 3. 谓词形态

| 写法 | 语义 |
|------|------|
| +tag | 含某标签 |
| -tag | 不含某标签 |
| /text/ | 标题子串匹配（非正则，大小写不敏感） |
| 裸字符串 | 标题子串匹配（同上） |
| field:value | 相等（对 title/description 实际是子串匹配） |
| field.before:value | 日期：严格早于 |
| field.after:value | 日期：严格晚于 |
| field.isnull | 字段为空（列表为空 / 指针为 nil） |
| field.notnull | 字段非空 |
| field: | 空值，等同 isnull |

**后缀只有 4 个**：before / after / isnull / notnull。没有 .eq / .neq / .contains / .gt / .lt 等。

## 4. 字段表

| 字段 | 值类型 | 说明 |
|------|--------|------|
| status | 枚举 | pending / completed / deleted / waiting |
| priority | 枚举 | H / M / L |
| project | 字符串 | 项目 slug（App 层解析为 project_id） |
| assignee | 列表 | 支持四种写法：用户 ID（UUID）/ 用户名 / email / provider:external_id（如 feishu:ou_xxx）；空值=未指派。解析按 ID → external_id → name → email 顺序短路匹配，找不到用户会报错（不是返回空结果），用户不在当前 workspace 也报错。无法解析 display_name 和不带 provider 的裸 external_id |
| due / start / wait / scheduled / until / end / entry / modified | 日期 | 支持 eq/before/after/isnull/notnull |
| recurrence_at | 日期 | 循环实例的发生时刻 |
| title / description / annotations | 子串 | eq 与 contains 同义，大小写不敏感；annotations 强制子串 |
| tags | 列表 | 只能通过 +tag / -tag |
| depends | 列表 | 依赖的任务引用 |
| parent | 字符串 | 手动父任务引用 |
| uuid | 字符串 | 任务 UUID，精确匹配 |
| task_type | 枚举 | normal / occurrence，仅支持相等 |
| series_id | 字符串 | 所属循环系列 ID |

注：project_id 不能手写（仅内部注入）；tag 没有冒号写法。

## 5. 值的语法

### 日期

| 写法 | 含义 |
|------|------|
| today / tomorrow | 今天 / 明天 00:00 |
| eod | 今天 23:59:59 |
| eow | 本周日 23:59:59 |
| eom | 本月最后一天 23:59:59 |
| now | 当前时间戳 |
| now+24h / now-2h | 相对偏移，**只认 Go duration 单位**（h/m/s/ms），不支持 now+1d |
| YYYY-MM-DD | 当日 00:00 |
| RFC3339 | 如 2026-05-28T10:00:00Z |
| 3days | 今天起第 3 天 00:00 |

日期边界：
- due:today 匹配今天任意时刻（整天区间）。
- due.before:today 严格在今天 00:00 之前（不含今天）；"今天及之前到期"要写 due.before:tomorrow。
- due.after:today 严格在今天 00:00 之后（不含今天）。

### 字符串引号

含空格或特殊字符的值用单引号或双引号包裹：project:'Home & Garden'。引号会被剥离，无转义机制，空引号非法。

## 6. UDA（用户自定义字段）

任何非内置字段名都会被当成 UDA：
- estimate:3 — 相等
- estimate.notnull — 非空
- estimate: — 为空（UDA 用空值判断 isnull，**没有 .isnull 后缀**）
- uda.reviewed:2026-05-28 — 显式 uda. 前缀

UDA 值类型自动推断：能解析为 RFC3339 日期则按日期比较；都能 ParseFloat 则按数字比较；否则按字符串子串匹配。

保留字段（不可当 UDA）：uuid / title / description / status / entry / modified / end / due / start / wait / scheduled / until / project / priority / depends / annotations / parent / assignee / tag / recur / mask / imask / series_id / recurrence_at / task_type。

## 7. 循环任务字段

| 字段 | 说明 |
|------|------|
| task_type | normal = 普通任务，occurrence = 循环实例；**仅支持相等**，其它算符报错 |
| series_id | 所属循环系列 ID |
| recurrence_at | 该次发生的时间点（日期型） |

## 8. 陷阱清单

1. **标签必须用 +tag / -tag**，写 tag:xxx 会报错。
2. **后缀只有 4 个**：before / after / isnull / notnull；没有 .eq / .neq / .contains。
3. **UDA 没有 .isnull 后缀**，用 field:（空值）判空。
4. **duration 单位**：filter 里 now+1d 非法，只认 h/m/s/ms。
5. **空值即 isnull**：field: 是判空，不是清除字段。
6. **裸词不是判空**：due（无冒号）是"标题包含 due"，不是判空；判空写 due.isnull 或 due:。
7. **逻辑关键字必须小写**：AND / Or 会被当成普通词。
8. **不等用 not**：not status:completed；没有 .neq 后缀。
9. **recur / mask / imask 已废弃**，写出来报错。
10. **assignee 找不到用户会报错**：不是返回空结果，而是整个查询失败（错误码 assignee_not_found / assignee_not_member）。display_name、不带 provider 的裸 external_id、含冒号的用户名/邮箱都无法解析。
11. **status / priority 无效值不会报错**：parser 不校验枚举，求值时静默不匹配。

## 9. 完整示例

逻辑组合：
  +work status:pending
  +next or due.before:tomorrow and priority:H
  (project:ops and +urgent) or priority:H
  not +work

日期：
  due.before:now+24h
  start.notnull
  due:today status:pending

负责人：
  assignee:550e8400-e29b-41d4-a716-446655440000
  assignee:alice
  assignee:feishu:ou_xxx
  assignee:alice@example.com
  (assignee:id-a or assignee:id-b)

UDA：
  estimate:3
  estimate.notnull
  uda.reviewed:2026-05-28

循环任务：
  task_type:occurrence
  series_id:s1

引号 + 子串：
  (project:'Home & Garden' and /spec/) or +next

## 10. 语法速查（EBNF）

表达式    := or_expr
or_expr   := xor_expr ("or" xor_expr)*
and_expr  := not_expr (("and")? not_expr)*    # 省略 and 即隐式 AND
not_expr  := "not" not_expr | primary
primary   := "(" or_expr ")" | predicate

predicate := "+" TAG | "-" TAG | "/" TEXT "/" | BARE
           | FIELD ":" VALUE
           | FIELD "." SUFFIX (":" VALUE)?

SUFFIX    := before | after | isnull | notnull
FIELD     := uuid | title | description | status | entry | modified | end
           | due | start | wait | scheduled | until | project | priority
           | depends | annotations | parent | assignee
           | series_id | recurrence_at | task_type
status    := pending | completed | deleted | waiting
priority  := H | M | L
task_type := normal | occurrence          # 仅相等
DATE      := now | now+DUR | today | tomorrow | eod | eow | eom
           | YYYY-MM-DD | RFC3339 | Ndays
DUR       := 仅 Go duration 单位（h, m, s, ms…），不支持 d / min
`
