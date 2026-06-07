# Xuanchu 备份与恢复

## SQLite 备份

```bash
# 在线备份（推荐，服务运行期间可执行）
sqlite3 ~/.local/share/xuanchu/xuanchu.db "VACUUM INTO '/path/to/backup.db'"

# 或直接复制（需停止服务）
cp ~/.local/share/xuanchu/xuanchu.db /path/to/backup.db
```

`VACUUM INTO` 会在 SQLite 层面生成一致快照，不需要停止服务。

## 备份内容

完整 SQLite 备份包含所有 xuanchu 数据：

- Hook definition（包括 secret）
- Hook delivery record（包括 payload）
- 所有 task、project、workspace、user 数据
- Membership 和 audit log
- Config、context、UDA schema

## 重要提醒

备份文件包含 Hook secret，必须按生产密钥材料保护：

```bash
chmod 600 /path/to/backup.db
```

不要将备份文件存放在公开可访问的路径或未加密的共享存储中。

## 恢复演练

1. 停止 xuanchu server
2. 替换数据库文件：

```bash
cp backup.db ~/.local/share/xuanchu/xuanchu.db
```

3. 启动 xuanchu server
4. 验证关键数据：

```bash
./xuanchu hook list
./xuanchu hook deliveries <hook-id>
./xuanchu list
./xuanchu audit list --limit 5
```

## JSON Export/Import

`xuanchu export --json` 仅导出 task 数据，不包含 Hook definition 和 delivery record。完整恢复请使用上述 SQLite 备份方案。

```bash
# 仅导出 task（不包含 hook）
./xuanchu export --json > tasks-backup.json

# 导入 task
./xuanchu import tasks-backup.json
```

## 自动化备份建议

```bash
# crontab 每日备份
0 2 * * * sqlite3 ~/.local/share/xuanchu/xuanchu.db "VACUUM INTO '/backup/xuanchu-$(date +\%Y\%m\%d).db'"
```

建议定期清理旧备份并验证恢复流程可用。
