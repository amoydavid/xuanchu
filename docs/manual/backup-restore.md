---
title: "备份与恢复"
weight: 110
---

# 备份与恢复

Xuanchu 支持 SQLite（默认）和 PostgreSQL。备份方式取决于数据库类型。

## 在线备份

推荐使用 SQLite `VACUUM INTO`：

```bash
sqlite3 ~/.local/share/xuanchu/xuanchu.db "VACUUM INTO '/path/to/backup.db'"
```

`VACUUM INTO` 会生成一致快照，服务运行期间也可以执行。

## 停机复制

如果可以停止服务，也可以直接复制数据库：

```bash
cp ~/.local/share/xuanchu/xuanchu.db /path/to/backup.db
```

## 备份内容

完整数据库备份包含：

- task、project、workspace、user 数据
- membership 和 audit log
- config、context、UDA schema
- token metadata
- Hook definition，包括 secret
- Hook delivery record，包括 payload

备份文件包含密钥材料，必须保护：

```bash
chmod 600 /path/to/backup.db
```

不要把备份放在公开可访问路径或未加密共享存储中。

## 恢复演练

1. 停止 xuanchu server。
2. 替换数据库文件。

```bash
cp backup.db ~/.local/share/xuanchu/xuanchu.db
```

3. 启动 xuanchu server。
4. 验证关键数据。

```bash
xuanchu hook list
xuanchu list
xuanchu audit list --limit 5
```

## JSON Export/Import 的边界

`xuanchu export --json` 只导出 task 数据，不包含 Hook、token、audit、workspace、membership 等数据。

```bash
xuanchu export --json > tasks-backup.json
xuanchu import tasks-backup.json
```

如果你要完整恢复服务端，请使用数据库备份（SQLite 文件或 `pg_dump`）。

## 自动化备份示例

```bash
0 2 * * * sqlite3 ~/.local/share/xuanchu/xuanchu.db "VACUUM INTO '/backup/xuanchu-$(date +\%Y\%m\%d).db'"
```

建议定期清理旧备份，并定期做恢复演练。

## PostgreSQL 备份

如果使用 PostgreSQL，请使用 `pg_dump`：

```bash
pg_dump -h localhost -U user xuanchu > /backup/xuanchu-$(date +%Y%m%d).sql
```

恢复：

```bash
psql -h localhost -U user xuanchu < /backup/xuanchu-20260605.sql
```

PostgreSQL 不支持 SQLite 的 `VACUUM INTO`，请使用 `pg_dump` 或 PostgreSQL 自身的 PITR / streaming replication。

