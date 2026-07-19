package cli

import (
	"git.dajee.net/dajee/xuanchu/internal/app"
	"git.dajee.net/dajee/xuanchu/internal/attachments"
	"git.dajee.net/dajee/xuanchu/internal/safefetch"
)

// buildAttachmentRemoteFetcher 根据附件配置构造远程图片抓取器。
//
// 返回值类型是 *safefetch.Fetcher，但 app.AttachmentRuntime.Fetcher 字段是 any，
// 避免在 app 包引入 safefetch 依赖。当配置关闭 remote fetch 时，调用方不应调用本函数。
func buildAttachmentRemoteFetcher(cfg attachments.Config) *safefetch.Fetcher {
	fetcher, err := safefetch.New(safefetch.Config{
		Timeout:      cfg.RemoteFetchTimeout,
		MaxRedirects: cfg.RemoteFetchMaxRedirects,
		MaxBytes:     cfg.MaxFileSizeBytes,
	})
	if err != nil {
		// 配置已经在 attachments.Config.Validate 校验过，这里再失败只能说明逻辑错误。
		// 返回 nil 让 app 层把 remote_fetch 视为不可用，而不是让 server 启动失败。
		return nil
	}
	return fetcher
}

// attachmentRuntimeForCLI 是一个占位说明，提醒 buildAttachmentRuntime 由 server.go 直接调用 app.NewAttachmentRuntime。
var _ = app.AttachmentRuntime{}
