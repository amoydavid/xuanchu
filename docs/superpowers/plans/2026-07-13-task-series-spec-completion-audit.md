# 循环任务主 Spec 完成审计与补全计划

**日期：** 2026-07-13

**状态：** 已完成；25 项验收与定向 Spec 10 项验收均有直接证据，最终门禁通过

**权威需求：** `docs/superpowers/specs/2026-07-11-task-series-calendar-recurrence-design.md` §24，以及 `2026-07-13-task-reference-and-recurring-form-alignment-design.md` §15.6。

## 审计原则

- 计划 checkbox、已有提交和全量测试通过都不能单独证明主 spec 完成；每个验收项必须有直接实现与行为测试。
- “代码看起来支持”只记为间接证据；跨协议一致性必须有集合/响应对照测试。
- Web 的路由状态、滚动、焦点和可访问性必须通过组件测试或浏览器 E2E 证明。
- 本表在所有 `待补` 和 `待强化` 项关闭前，不得把主 spec 标记为完成。

## §24 逐项证据矩阵

| # | 要求摘要 | 当前证据 | 结论 | 补全动作 |
|---|---|---|---|---|
| 1 | daily 前次未完成不阻塞新 occurrence | `TestDailySeriesKeepsYesterdayPendingWhenTodayOccurrenceIsGenerated` 显式断言昨日、今日为不同 UUID 且同时 pending | 直接证据 | 全量复跑 |
| 2 | 有界未来投影只读，不写 task/audit/hook | `TestQueryTaskViewsExpandMergesProjectedAndMaterializedWithoutWrites` 同时断言 task row、audit 与 Hook delivery 数量不变 | 直接证据 | 全量复跑 |
| 3 | 停机补偿期间投影完整，reconcile 幂等 | `TestFiveDayBacklogRemainsFullyVisibleAndReconcileCompletesIdempotently` 覆盖 limit=2 时仍显示五槽位、后续补齐与重复 reconcile | 直接证据 | 全量复跑 |
| 4 | 普通任务与 Series 不互转 | `TestGenericTaskWritesRejectRemovedRecurrenceFields`、`TestGenericTaskToolsRejectRetiredRecurrenceFields`、`TestCLIGenericTaskCommandsRejectRetiredRecurrenceFields` 分别覆盖 HTTP/MCP/CLI 的旧字段拒绝与无副作用；Series 仅有专用资源入口 | 直接证据 | 全量复跑 |
| 5 | Web Series CRUD 与 occurrence done/reopen/skip | task-series 组件测试覆盖创建/编辑/停止/实例分组；`task-detail-page.test.tsx` 与 `smoke:task-series` 覆盖完成、重新打开、跳过入口和 projected 首次完成 | 直接证据 | Web 全量与 smoke 复跑 |
| 6 | 项目任务与五个 My Tasks 视图融合 occurrence | `my-task-tabs.test.ts` 锁定五个 preset 与 pending/waiting；`my-tasks-api/table/page` 测试覆盖 project/task_type/waiting；`smoke:task-series` 验证项目融合列表和五个 Tabs 可见 | 直接证据 | Web 全量与 smoke 复跑 |
| 7 | 无独立 Tab；面板深链及状态恢复 | `project-layout.test.tsx`、`my-tasks-return-state.test.ts` 与 `smoke:task-series` 覆盖无 Series Tab、任务页深链、浏览器前进后退，以及查询/选择/滚动/焦点恢复 | 直接证据 | smoke 复跑 |
| 8 | 创建/编辑/跳过/停止弹窗符合原型 | 统一创建、Series editor/stop、详情跳过 Dialog 组件测试锁定 shadcn、字段、影响摘要和危险文案；`smoke:task-series` 覆盖桌面创建/编辑/停止确认和移动端 Sheet/详情 | 直接证据 | Web 全量与视觉 smoke 复跑 |
| 9 | HTTP/MCP query 集合一致；写前物化原子 | `TestE2ERecurringQueriesReportsAndSeriesListsAreProtocolEquivalent` 对同 fixture 比较 HTTP/MCP/Remote CLI page；projected 写前物化与回滚由 App/HTTP/MCP alias 测试覆盖 | 直接证据 | 集成全量复跑 |
| 10 | MCP 专用 Series CRUD，不把 Series 当 Task | `TestMCPTaskSeriesAddAndGet/List/Modify/Stop/OccurrenceSkip`、tool list 与 schema golden；generic task tools 无 Series 字段 | 直接证据 | golden 与 MCP 全量复跑 |
| 11 | due 不改 recurrence_at；override 不被覆盖 | `TestModifyOccurrenceAliasesRecordsOnlyChangedOverrides`、`TestReplaceEditableTaskRecordsMaterializedOccurrenceOverrides`、`TestModifyTaskSeriesSyncsSharedFieldsToOpenOccurrences` | 直接证据 | 全量复跑 |
| 12 | 项目关闭后停止投影/物化 | `TestProjectTransitionStopsActiveSeries` 覆盖领域行为；`task-detail-page.test.tsx` 覆盖关闭项目只读，HTTP project transition suite 覆盖状态映射 | 直接证据 | 全量复跑 |
| 13 | 普通进度排除 occurrence，独立循环指标 | `TestProjectTaskSummaryExcludesOccurrencesAndReportsSeriesMetrics` 与项目 HTTP/Web summary 测试 | 直接证据 | 全量复跑 |
| 14 | SQLite/PostgreSQL/零 CGO | SQLite 与零 CGO 两次全量通过；真实临时 PostgreSQL storage suite 和 PostgreSQL HTTP/MCP E2E 通过 | 直接证据 | 最终门禁通过 |
| 15 | README/ROADMAP/manual/OpenAPI/MCP/Web 文案同步 | manual/requirements/Skill、Huma schema、MCP golden、双 locale 已更新；旧语义扫描只剩明确拒绝测试和“不使用 include_completed”说明 | 直接证据 | 静态扫描与文档门禁通过 |
| 16 | CLI occurrence 历史、list/report/working set/helper/render | `TestCLIEditProjectedOccurrenceOnlyMaterializesAfterValidDiff` 同时覆盖 projected info、list/report、`_get/_ids/_uuids/_tags/_unique/_urgency`、子资源读、失败写与物化后 working set；CLI Series 全流程覆盖 occurrence 日期分页 | 直接证据 | integration 全量复跑 |
| 17 | DSL 删除旧属性并新增三属性，SQL/evaluator 一致 | parser/compiler 旧属性拒绝；`TestTaskViewEvaluatorMatchesSQLCompilerOnMaterializedFixture` 覆盖保留属性，`TestTaskViewEvaluatorMatchesSQLCompilerOnOccurrenceAttributes` 同 fixture 对照 `series_id/recurrence_at/task_type` | 直接证据 | App/query/storage 全量复跑 |
| 18 | projected 子资源读/no-op/失败写不物化 | `TestProjectedOccurrenceReadSubresourcesStayEmptyWithoutMaterializing`、`TestWithTaskForWriteRollsBackOnActionError`、CLI external edit 矩阵及 HTTP/MCP 失败写测试 | 直接证据 | 全量复跑 |
| 19 | Remote DTO 与 native bundle 无损 | Remote DTO strict decode/alias 测试、`TaskBundle` round-trip/rollback 与 CLI/MCP import-export 测试；旧 JSONTask response 被明确拒绝 | 直接证据 | 全量复跑与旧 wrapper 扫描 |
| 20 | HTTP reports、task report、CLI、MCP/Remote 结果一致 | `TestE2ERecurringQueriesReportsAndSeriesListsAreProtocolEquivalent` 同时比较 `/reports/{name}`、`/tasks?report=`、MCP `report_run` 和 Remote CLI 的 items/total/sort/pagination | 直接证据 | integration 全量复跑 |
| 21 | external edit 取消/失败/no-op 不物化；有效 diff 原子物化 | `TestCLIEditProjectedOccurrenceOnlyMaterializesAfterValidDiff` 覆盖 no-op、editor failure、非法日期与有效 diff；`TestReplaceEditableTaskProjectedRollsBackMaterializationWhenAuditFails` 覆盖回滚 | 直接证据 | integration/App 与零 CGO 全量复跑 |
| 22 | occurrence status 枚举跨协议一致 | HTTP/MCP/Remote/CLI schema 和 `ListTaskSeriesOccurrences` 测试固定 `pending/waiting/completed/deleted/all`；MCP golden 固定 enum | 直接证据 | golden/全量复跑 |
| 23 | Series list 过滤/排序/分页跨协议一致 | `TestE2ERecurringQueriesReportsAndSeriesListsAreProtocolEquivalent` 比较 HTTP/MCP/Remote CLI 的 items 与 filtered total；Web API/面板测试覆盖相同参数 | 直接证据 | integration/Web 全量复跑 |
| 24 | My Tasks open 语义及返回查询/选择/滚动/焦点 | tabs/API/table/page 测试锁定 open=pending OR waiting、行级/批量动作和返回状态；`smoke:task-series` 真实浏览器验证 Series 往返、搜索、选择、滚动与焦点恢复 | 直接证据 | Web 全量与 smoke 复跑 |
| 25 | 普通/occurrence 详情动作和短链接语义 | 定向 spec §9.2、`task-detail-page.test.tsx` 和 `smoke:task-series` 覆盖 OPS-7 短链、循环 Alert、本次动作、projected→OPS-8 replace、移动端与 My Tasks 来源返回 | 直接证据 | Web 全量与 smoke 复跑 |

## 定向 Spec §15.6 证据矩阵

| # | 验收要求 | 直接证据 |
|---|---|---|
| 1 | materialized occurrence 显示并链接 `ops-N` | App/HTTP/MCP alias tests、task reference Web tests、`smoke:task-series` 点击 OPS-7 |
| 2 | 详情首屏明确本次日期、Series 和本次动作 | `task-detail-page.test.tsx`、定向 spec §9.2 ASCII、桌面/移动 smoke 截图 |
| 3 | UUID/task_slug/occurrence_ref 等价 | `TestResolveTaskReferenceForReadTreatsOccurrenceAliasesAsOneResource`、HTTP/MCP alias shape tests |
| 4 | projected 不预留 slug，首次写后 replace | projected identity tests、Web mutation/router tests、`smoke:task-series` OPS-8 replace |
| 5 | encoded occurrence_ref 可读，非法编码拒绝 | `TestTaskHTTPGetsProjectedOccurrenceByEncodedReferenceWithoutMaterializing`、`TestTaskHTTPRejectsUnsafeOrDoubleEncodedReferences` |
| 6 | occurrence 完整绑定并稳定生成 slug | migration/runtime materialization tests、project invariant tests |
| 7 | 循环创建/编辑与普通任务共享字段 | unified create、Series editor tests：description/assignees/priority/tags/UDAs/clear |
| 8 | Series 共享字段同步与本次 override | shared-field sync、alias override、external edit tests |
| 9 | HTTP/MCP/Remote/CLI/Web 引用语义一致 | 跨协议 E2E、CLI projected 矩阵、Web smoke |
| 10 | spec/OpenAPI/MCP/manual/Skill 不残留旧 URL 语义 | 文档扫描、OpenAPI test、MCP golden、manual 与 Skill diff |

## 横切质量证据

- Series list 不允许按行查询派生计数或用户：`TestListTaskSeriesLoadsDerivedDataWithoutNPlusOne` 对 5 条 Series 断言查询数不超过 15；`SummarizeSeriesOccurrences` 用一次条件聚合读取所有计数和最大槽位，`resolveUserInfos` 使用 `ListByIDs`。
- `TestSummarizeSeriesOccurrencesReturnsCountsAndMaxForEveryRequestedSeries` 覆盖 SQLite 的 pending/waiting/completed/deleted/overdue/max/空 Series；`TestPostgres_SummarizeSeriesOccurrences` 在真实 PostgreSQL 临时库执行同一 SQL。
- MCP 输入在服务端按 closed schema 严格解码，不依赖 client 自觉校验；未知字段失败，旧循环字段返回 `task_series_endpoint_required`。
- Series occurrence 范围四端统一为 `YYYY-MM-DD`，App 使用左闭右开 Unix 区间；无范围历史不再隐式截断为未来十年。
- Series/occurrence 分页四端统一为默认 200、最大 1000、非负 offset；App 公共边界兜底校验，OpenAPI 与 MCP schema 同步声明默认值和范围。
- Series get 按主 spec 返回最多 200 条 due 升序未完成实例，以及各 10 条 due 降序的最近完成/跳过实例。
- occurrence history 与 Series get 在 Storage 层先 count、再按 due 稳定排序并执行 limit/offset；不会为了返回一页而加载完整历史及其负责人/链接关联。
- OpenAPI 的 TaskSeries 响应补齐派生计数、下一槽位、建议生效时间、创建者和实例分组；TaskOccurrence 补齐关系引用与完整 UserInfo schema。

## 当前新鲜验证记录

- `go test ./... -count=1 -timeout 30m`：通过。
- `CGO_ENABLED=0 go test ./... -count=1 -timeout 30m`：通过。
- `CGO_ENABLED=0 go build ./cmd/xuanchu`：通过。
- `go vet ./...`：通过。
- 真实临时 PostgreSQL：`go test ./internal/storage -run Postgres -count=1` 与 `go test ./tests/integration -run TestPostgresE2E -count=1` 均通过。
- `pnpm --dir web typecheck`、`lint`、`build`：通过。
- `pnpm --dir web test`：117 个文件、606 项测试通过。
- `pnpm --dir web run smoke:editing` 与 `smoke:task-series`：通过；后者覆盖桌面、My Tasks 返回、短链接、projected 首次物化与移动端。
- `git diff --check` 与旧接口/旧语义静态扫描：通过；无 dist、二进制、数据库或截图进入提交范围。

## 实施批次

1. My Tasks 完整执行面：路由筛选、项目与任务类型、行级动作、Series 入口、返回状态。
2. External edit occurrence：只读预览、diff 判断、写前物化、override、失败回滚。
3. 日历/只读投影直接证据：验收 1–3、17、18。
4. 跨协议等价 E2E：验收 9、20、23，并收口 CLI/working set。
5. ASCII UI、文档和 schema 终审；双数据库、零 CGO、Web、浏览器 E2E 全量门。
