# Urgency 因素

`urgency_explain` 返回 `data.factors`，每个因素 `{name, value}` 表示该因素对 urgency 总分的贡献。`data.urgency` 是各因素之和。

## 常见因素

| 因素 | 含义 |
|---|---|
| `priority` | 任务优先级（H/M/L）的贡献 |
| `due` | 临近 due 的贡献（越近越高，逾期更高） |

系统还可能根据配置包含其他因素（如 active、age、dependency 等）。以 `urgency_explain` 实际返回为准。

## 调整系数

各因素权重可在 workspace 级 config 用 `urgency.*` 键调整（如 `urgency.priority.coeff`）。配置操作详见 xuanchu-manage-access-and-config skill。

> urgency 系数是 workspace 级业务配置；`agent.*` 配置不在此列。本 skill 只读，改配置请用 xuanchu-manage-access-and-config。
