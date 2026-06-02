---
title: "备份与恢复"
weight: 110
---

# 备份与恢复

taskg 使用 SQLite。完整备份应备份 SQLite 数据库文件。

## 在线备份

推荐使用 SQLite `VACUUM INTO`：

```bash
sqlite3 ~/.local/share/taskg/taskg.db "VACUUM INTO '/path/to/backup.db'"
```

`VACUUM INTO` 会生成一致快照，服务运行期间也可以执行。

## 停机复制

如果可以停止服务，也可以直接复制数据库：

```bash
cp ~/.local/share/taskg/taskg.db /path/to/backup.db
```

## 备份内容

完整 SQLite 备份包含：

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

1. 停止 taskg server。
2. 替换数据库文件。

```bash
cp backup.db ~/.local/share/taskg/taskg.db
```

3. 启动 taskg server。
4. 验证关键数据。

```bash
taskg hook list
taskg list
taskg audit list --limit 5
```

## JSON Export/Import 的边界

`taskg export --json` 只导出 task 数据，不包含 Hook、token、audit、workspace、membership 等数据。

```bash
taskg export --json > tasks-backup.json
taskg import tasks-backup.json
```

如果你要完整恢复服务端，请使用 SQLite 备份。

## 自动化备份示例

```bash
0 2 * * * sqlite3 ~/.local/share/taskg/taskg.db "VACUUM INTO '/backup/taskg-$(date +\%Y\%m\%d).db'"
```

建议定期清理旧备份，并定期做恢复演练。

