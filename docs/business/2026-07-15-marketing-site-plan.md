# 营销页规划:卖什么、卖给谁、怎么卖、卖多少

> 本文档是 Dajee(代码仓库名 xuanchu / 璇础)面向全球市场的营销页落地规划。它不是产品 spec,而是**商业执行方案**——把"定位 / 竞品 / 凭证机制 / 定价 / 页面结构"收口成可执行动作。
>
> 创建:2026-07-15。本文档随市场反馈迭代,不视为冻结契约。

---

## 0. 品牌与定位(全站口径锚点)

**全球品牌名:Dajee**(域名 `dajee.net`)。中文"达吉"。公司名与产品名合一(单产品公司架构)。

> **Dajee — The governed task runtime for AI agents.**
> (企业级、可治理的 AI Agent 任务运行时)

品牌使用硬规则:
- **全球站全用 Dajee,完全不出现 Xuanchu / 璇础。** 代码内二进制名 `xuanchu` 是内部技术资产,市场不可见,无需改名。
- 读音引导:/dɑːˈdʒiː/(da-jee)。Dajee 是无义名,品牌识别完全靠定位语,所以 hero 那句 tagline 不可改不可省。
- 对外**绝不使用**以下自我稀释的词:task manager / todo app / project management tool / openclaw companion。

内部认知(不对客户说):openclaw/hermes 等执行引擎在企业落地的治理伴侣。
对外认知:**任何执行引擎(openclaw / hermes / Claude / 自研 agent)进企业时,缺少的那层治理 + 持久化 + 审计层。**

> 待办:上线前做一次 Dajee 商标快速检索(美/欧),确认无冲突。

---

## 1. 卖什么(Wedge)

不卖"功能多",卖**一个被竞品集体忽略的缺口**:

> **Agent 执行引擎(openclaw/hermes/LLM)能"产生行动",但企业要让 Agent 合法上路,缺的不是行动力,是治理。**

三堵墙(执行引擎答不上、企业合规必问):

| 企业必问 | 执行引擎有没有 | Dajee 补 |
|---|---|---|
| 这个行动属于哪个任务/项目? | ❌ | 任务/项目上下文持久化 |
| Agent 替员工操作,谁负责? | ❌ | impersonation + 行为归属员工 |
| 这个行动记录在哪、能追溯吗? | ❌ | 全量审计 |
| 不同客户的数据隔离了吗? | ❌ | 多租户 / 行级隔离 |
| 跨会话,Agent 还记得该做什么吗? | ❌ | 任务持久化 |
| 数据能留在企业内网吗? | ❌ | 自建 / 单二进制 |

一句话钉死:**执行引擎是发动机,Dajee 是让它能合法开上企业那条路的牌照、仪表和登记系统。**

### 1.1 代码已证实的差异化(可对外claim的事实)

以下能力**已实现且有代码证据**,营销页只讲这些,不讲 roadmap:

- **MCP-native(95+ tool,stdio + HTTP)**:纯标准协议,任意 MCP 客户端可接(见 `internal/mcpserver/`,无任何引擎硬编码)。
- **Impersonation**:`X-Xuanchu-As` 让 Agent 代表员工操作,审计区分"谁发的 token"和"代表谁"(见 `internal/app/request_scope.go:84-224`)。
- **多租户 + 行级隔离 + RBAC + 审计**:企业治理底线(`internal/app/` 全量覆盖)。
- **纯 Go 单二进制 + SQLite/PostgreSQL**:零 CGO,企业内网一键部署,数据不出域。
- **BYOA 开箱即用**:员工自助创建 token + 一键导出 MCP 配置(`GET /api/v1/tokens/{ref}/mcp-config`),任意 agent 接入(见 `internal/app/token.go:555-707`)。

---

## 2. 卖给谁(目标客户)

### 2.1 主力客户(第一梯队,OEM 路线)

**自研企业 Agent 产品 / 助理的创业公司(种子-A 轮)**

- 为什么是他们:他们的产品缺一个"帮用户记事、跟进、派单、可治理"的后端,自建 3-6 人月,买现成更划算。
- 决策人:CTO / 创始人,跳过企业采购流程,决策快。
- 怎么够到:他们正在用的执行引擎社区(openclaw / hermes 用户群)是暖线索。

### 2.2 第二梯队(中长期)

- **自研企业 AI 助手的甲方公司**:CTO / AI 平台负责人,数据不出域的强需求。
- **平台工程 / SRE 团队**:需要 API-first、脚本化任务运行簿,不需要 Agent 也会买。

### 2.3 不碰

- 通用个人用户 / 小团队 todo(产品形态和分发都不匹配)。
- 用封闭全栈 Agent 平台(Sierra / Decagon / Agentforce)的企业——他们不会单独买一层运行时。

---

## 3. 怎么卖(商业模式 + GTM)

### 3.1 商业模式:OEM + 托管双轨(用户已确认)

**闭源,不走开源。** 第一桶金靠**付费定制集成**,OEM 授权是后续收割。

| 形态 | 客户 | 接触成本 | 定位 |
|---|---|---|---|
| **付费 pilot / 定制嵌入** | Agent 创业公司 | 中(2-4 周交付) | **第一笔现金来源** |
| **托管 SaaS** | 开发者自助 | 低 | 漏斗 + 长期现金流 + OEM 试用入口 |
| **OEM / 私有授权** | pilot 跑顺的客户 | 高 | 高客单收割 |

### 3.2 销售路径(闭环)

```
冷 BD(带 live demo)
   → 托管版上跑通(低摩擦试用)
   → 付费 pilot $3k-8k / 2周(第一笔现金)
   → pilot 沉淀为 case
   → 转 OEM 授权 $500-2k/月 或 一次性 $10k+
```

**关键:第一桶金来自"卖人 + 产品"(定制集成),不是"卖纯 license"。** 纯 license 从冷到签约不可能 3 个月内完成。

### 3.3 BYOA 安全指引(销售物料,优先产出)

代码已证实"员工用自己的 agent 代自己操作"开箱即用。写一份**面向企业 IT 的 BYOA 安全指引**,把已有能力包装成可销售方案:
- 员工怎么自助创建 token 并配进自己的 agent。
- token scope 怎么收窄(`scope list`)。
- 离职怎么吊销(`token revoke`)。
- 形态 B(共享 impersonation)为什么需要 admin,留给企业版。

这是**文档活,不是代码活**,ROI 最高。

---

## 4. 卖多少(全球定价,美元)

### 4.1 价格表(起步锚点,按市场反馈调整)

| 档位 | 价格 | 包含 | 目标 |
|---|---|---|---|
| **Free** | $0 | 1 workspace,限 MCP 调用数/任务数,社区支持 | 跑通 demo |
| **Pro(托管)** | $29/月/席位起 | 多 workspace、更高调用上限、邮件支持 | 创业公司、小团队 |
| **Enterprise(自建授权)** | 联系销售 | 私有部署、SSO、全量审计、SLA、官方支持 | 大甲方 |
| **OEM / 嵌入授权** | $500-2k/月 或 一次性 $10k+ | 嵌入到客户产品、按部署/调用量 | Agent 创业公司 |
| **付费 pilot / 定制** | $3k-8k / 一次性(2 周交付) | 定制嵌入 + 交付支持 | **第一桶金** |

### 4.2 定价原则

- **按席位 / 实例 / 调用量定价,不按"任务数"收费**——用量是护城河,不是成本中心。鼓励多用。
- Agent 调用越多越值钱,放开调用次数限制。
- 企业版 / OEM 的治理能力(RBAC、impersonation、审计、tenant token、SSO)是天然的付费墙——社区版不送。

---

## 5. 营销页结构(英文落地页)

面向全球,全英文,品牌名 Dajee。页面按**漏斗**组织:先钉定位 → 建可信 → 证差异 → 给试用 → 收口行动。

### Section 1:Hero(3 秒钉定位)

```
Dajee

Headline:
  The governed task runtime for AI agents.

Subhead:
  Give your agents persistent, auditable, multi-tenant task management.
  Built for enterprise. Works with any MCP client.

CTA primary:   Start free
CTA secondary: Book a demo
```

要点:不提"todo / task manager"。只用 "governed task runtime"。

### Section 2:Problem(为什么需要这一层)

```
Your agent can take actions. But can it operate in the enterprise?

The gap between "agent works in a demo" and "agent is allowed in production":
  - Whose task is this?           → context & ownership
  - Who is responsible?           → impersonation & audit
  - Is the data isolated?         → multi-tenancy
  - Can it survive a session?     → persistence
  - Can it stay on-prem?          → self-hosted, single binary
```

### Section 3:Solution(差异化,只讲已实现)

四张卡片:
- **MCP-native**: 95+ tools, works with any MCP client (openclaw, hermes, Claude, custom).
- **Governance**: RBAC, row-level isolation, full audit, impersonation.
- **Self-hosted**: single Go binary, SQLite/PostgreSQL, zero CGO, data never leaves your network.
- **BYOA-ready**: employees connect their own agents with self-issued scoped tokens.

### Section 4:How it works(架构图 + BYOA 流)

```
Employee's agent (any MCP client)
   │  ① self-issue a scoped token
   │  ② export MCP config (one click)
   │  ③ configure the agent
   ▼
Dajee Server (enterprise-deployed)
   - audited to the employee
   - workspace isolation / RBAC
   - data stays on-prem
```

### Section 5:vs Alternatives(battlecard 上墙,轻量版)

| | Generic SaaS + Zapier | Dajee |
|---|---|---|
| Data residency | Vendor's public cloud | Your network |
| Action attribution | "It was changed" | "Agent X did it as Alice" |
| Multi-tenancy | None | Row-level isolation |
| Approach | Rent someone else's task system | Own a governed task runtime |

### Section 6:Pricing(第 4 节价格表)

### Section 7:CTA 收口

```
Start free — no credit card.
Or talk to us about embedding Dajee in your agent product.
```

---

## 6. 命名与品牌(已锁定)

- **全球品牌名:Dajee**,域名 `dajee.net`。中文"达吉"。
- 公司名与产品名合一(单产品公司,摩擦最低)。若未来有多产品计划再拆。
- 全球站全英文,完全不出现 Xuanchu / 璇础。二进制名 `xuanchu` 是内部技术资产,不改。
- 待办:上线前做 Dajee 商标快速检索(美/欧),确认无冲突。

---

## 7. 90 天行动表(按"快回款"优先级)

| 周次 | 动作 | 产出 |
|---|---|---|
| 1-2 | 海外云起托管实例 + 英文 landing + 英文名 + live demo + 录屏 | 能 demo 的门面 |
| 3-6 | 冷 BD 20 家种子/A 轮 Agent 创业公司,带 demo 链接 | 5-8 个 demo 通话,谈出 1-2 个 pilot |
| 7-12 | 交付 pilot,沉淀 case + 写 1 篇硬核技术文发 HN/Reddit | 第一笔现金 + 第一个真实 case |
| 并行(每周1天) | 写 BYOA 安全指引 + 2-3 篇技术内容 + 提交 MCP 目录 | 长期 inbound 杠杆 |

---

## 8. 附:关键决策记录(本次对话得出)

- ❌ 不走开源(无法盈利)。
- ✅ OEM / 嵌入授权优先,但第一桶金靠付费定制集成。
- ✅ 独立 / 小团队,扛不住重销售接触。
- ✅ 愿意做托管 SaaS(低摩擦漏斗 + OEM 试用入口)。
- ✅ 全球市场(海外云),全英文,美元定价。
- ✅ 需要快回款(1-3 个月)。
- ✅ openclaw/hermes 与 Dajee 独立中立,Dajee 兼容所有 MCP 客户端。
- ✅ BYOA 形态 A(每员工自助 token)开箱即用;形态 B(共享 impersonation)留企业版。

---

## 9. 不做清单(防止定位漂移)

- 不蹭"任务管理"赛道,不和 Linear/Jira/Todoist 比功能。
- 不把记忆框架(mem0/Zep)当竞品,它们是集成伙伴(语义记忆 vs 结构化任务,不同层)。
- 不在产品里硬绑任何执行引擎,保持 MCP 中立。
- 不按"任务数"定价。
- 不把"伴侣 / companion"放到对外文案(会降级成附属品)。
