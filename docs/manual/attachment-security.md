# 附件安全说明

任务附件支持任意二进制资源（图片、PDF、Office、压缩包），但所有写入与读取都经过
workspace 行级鉴权，不存在匿名访问。本页汇总运维侧需要关注的安全边界。

## 存储后端

### filesystem（默认）

二进制写入 `<data-dir>/attachments`，路径分片为
`<root>/<workspace-id>/<attachment-id[0:2]>/<attachment-id[2:4]>/<attachment-id>`：

- 根目录与 workspace 子目录使用 `0700`，文件使用 `0600`。
- 上传先写同文件系统 `.tmp/<random>`，校验 SHA-256 + fsync 后原子 rename。
- 服务端生成 storage key，不拼接用户文件名，不接受调用方传入。
- 备份与恢复：迁移服务器时必须同时迁移本目录与数据库，不能只迁数据库。

### S3 / MinIO（可选）

- bucket 必须是 private；璇础不自动创建 bucket、不修改 bucket policy。
- 所有上传/下载经服务端代理，不返回预签名 URL，不让浏览器直接持有 S3 凭证。
- AWS 凭证只走 SDK 默认链路（环境变量、shared credentials、IAM role、Web Identity），
  TOML 中不保存 access key/secret。
- 自定义 endpoint 默认要求 HTTPS；仅 `allow_insecure_endpoint=true` 才允许 HTTP（本机 MinIO）。
- 支持 `AES256` 和 `aws:kms` server-side encryption；KMS 必须配置 key ID。
- 切换 backend（filesystem → S3）不自动迁移历史对象；只要旧目录仍挂载，历史可继续读取。

## 远程图片转存（egress）

粘贴带公网 `<img src="https://...">` 的富文本时，由服务端 `internal/safefetch` 抓取并
转存为附件，浏览器不直接加载第三方图片：

- **网段禁止**：loopback、RFC1918、link-local、multicast、unspecified、运营商级 NAT
  (RFC 6598)、IPv6 ULA 全部拒绝；DNS 解析得到任一私网地址即整次失败。
- **redirect**：每跳重新校验 scheme/host/DNS/IP；HTTPS→HTTP 降级直接拒绝。
- **请求**：固定无身份信息的 User-Agent；不携带 Cookie、Authorization、Referer；
  HTTP client 禁用环境代理。
- **响应**：`Content-Length` 超过单文件上限立即中止；未知长度用 `io.LimitedReader(max+1)`。
  只接受最终 magic 校验为 PNG/JPEG/GIF/WebP 的内容。
- **失败降级**：私网/认证/超限图片不会被服务端抓取；前端显示失败占位，用户必须显式
  选择「重试 / 移除 / 保留为普通 HTTPS 外链」三者之一，未解决前禁止保存。

可通过 `[attachments].remote_fetch_enabled = false` 关闭整个远程转存；关闭后
import-url 返回稳定错误 `attachment_remote_fetch_disabled`，不退化为浏览器直连。

## 文件类型白名单

| 类别 | 扩展名 |
|---|---|
| 内联图片 | `.png .jpg/.jpeg .gif .webp`（宽高 ≤ 20000，总像素 ≤ 4000 万） |
| 仅下载 | `.pdf .txt .md .csv .json .docx .xlsx .pptx .zip .7z .tar .gz .tgz` |
| 固定拒绝 | `.html .htm .xhtml .svg .xml .js .mjs .cjs .wasm .sh .bash .zsh .fish .bat .cmd .ps1 .vbs .exe .dll .msi .com .scr .apk .app .dmg .pkg .jar .docm .xlsm .pptm` |

OOXML 文件必须是有效 ZIP，包含 `[Content_Types].xml` 与对应 `word/`、`xl/`、`ppt/` 目录，
且不能包含 `vbaProject.bin`（带宏 Office 文件一律拒绝）。压缩包不打开内容检查，仅以
`Content-Disposition: attachment` 下载，界面明确显示「未经过内容扫描」。

## 配额

默认上限（可在 `[attachments]` 调整）：

- 单文件 25 MiB
- 单 attached resource（task）尚未 purge 的附件总量 200 MiB
- 单 workspace 尚未 purge 的附件总量 10 GiB
- 单 task 的 `uploading + draft + active` 最多 100 个

配额检查在开始上传前做一次声明值检查，在流式接收结束后按实际大小再次检查；最终提交
在事务内重新统计，避免并发越过限制。`deleted` 状态在 retention 内仍计入配额。

## 生命周期与清理

```
uploading -> draft -> active -> deleted -> purged
          \-> active
```

- `uploading` 超过 1 小时或 `draft` 超过 `draft_ttl`（默认 24h）后由 janitor 清理。
- `active` 删除前检查当前 description 是否仍引用；被引用返回 `attachment_in_use`。
- `deleted` 在 `deleted_retention`（默认 720h）内仍可鉴权读取，用于 description audit；
  到期 janitor 删除 blob 后再删 metadata row（blob 删除失败保留 row 重试，避免孤儿）。

`xuanchu server` 启动 `AttachmentJanitor`，默认每小时运行一次小批量清理，复用 shutdown
coordinator 支持优雅停止。本地 CLI 模式不启动常驻循环；执行附件写命令后最多 opportunistic
清理一批过期记录。

## 审计

附件写操作产生 `attachment.add` / `attachment.rename` / `attachment.remove` 审计：

- target 是 attachment；payload 含 attachment ID、`attached_to.type/id`、display name、
  media type、size、SHA-256、source type、source host。
- 不记录完整 source URL、storage key、bucket、endpoint 或文件内容。
