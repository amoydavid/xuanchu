# Audit Action 清单

`audit_list` 返回的每条审计日志含 `action` 字段，标识操作类型。按资源分组：

## 任务（task.*）

- `task.add`、`task.done`、`task.delete`、`task.modify`、`task.annotate`、`task.denotate`、`task.import`

## 项目（project.*）

- `project.add`、`project.modify`、`project.archive`、`project.annotate`、`project.denotate`、`project.config.set`、`project.config.unset`

## Workspace（workspace.*）

- `workspace.add`、`workspace.modify`、`workspace.archive`、`workspace.use`

## 用户与成员（user.* / member.*）

- `user.add`、`user.use`、`user.bind_external_id`、`user.unbind_external_id`
- `member.add`、`member.role`

## Context / Config（context.* / config.*）

- `context.define`、`context.use`、`context.none`、`context.delete`
- `config.set`、`config.unset`、`config.schema.set`、`config.schema.delete`

## Hook（hook.*）

- `hook.create`、`hook.modify`、`hook.delete`、`hook.replay`

## 通知与提醒（notification.* / reminder.*）

- `notification.sink.create`、`notification.sink.modify`、`notification.sink.enable`、`notification.sink.disable`、`notification.sink.delete`、`notification.delivery.replay`
- `reminder.rule.create`、`reminder.rule.modify`、`reminder.rule.enable`、`reminder.rule.disable`、`reminder.rule.delete`

## Token（token.*）

- `token.create`、`token.modified`、`token.revoke`
