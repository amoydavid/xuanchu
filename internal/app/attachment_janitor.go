package app

import (
	"context"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/logging"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// AttachmentJanitor 周期性清理过期 uploading/draft 和到期 deleted。
//
// 删除顺序：先删除 blob，再删除 metadata row；blob 删除失败保留 row 等待重试，
// 不能先删 metadata 制造不可追踪孤儿（spec §11.3）。
type AttachmentJanitor struct {
	store    *storage.Store
	runtime  *AttachmentRuntime
	clock    Clock
	batchSize int
	interval time.Duration
	logger   *logging.Logger
}

// AttachmentJanitorOptions 描述 janitor 行为。
type AttachmentJanitorOptions struct {
	Store     *storage.Store
	Runtime   *AttachmentRuntime
	Clock     Clock
	BatchSize int
	Interval  time.Duration
	Logger    *logging.Logger
}

// NewAttachmentJanitor 构造附件清理器。默认批次 100、间隔 1 小时。
func NewAttachmentJanitor(opts AttachmentJanitorOptions) *AttachmentJanitor {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	if opts.Interval <= 0 {
		opts.Interval = time.Hour
	}
	if opts.Clock == nil {
		opts.Clock = RealClock{}
	}
	return &AttachmentJanitor{
		store:     opts.Store,
		runtime:   opts.Runtime,
		clock:     opts.Clock,
		batchSize: opts.BatchSize,
		interval:  opts.Interval,
		logger:    opts.Logger,
	}
}

// RunOnce 执行一轮清理。
func (j *AttachmentJanitor) RunOnce(ctx context.Context) (AttachmentCleanupResult, error) {
	svc, err := NewService(ServiceOptions{
		Store:       j.store,
		Clock:       j.clock,
		Attachments: j.runtime,
	})
	if err != nil {
		return AttachmentCleanupResult{}, err
	}
	return svc.CleanupAttachments(ctx, j.batchSize)
}

// Run 启动周期清理循环，直到 ctx 取消。
func (j *AttachmentJanitor) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	// 启动时立刻跑一次，清理进程崩溃留下的 stale uploading。
	j.runRound(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.runRound(ctx)
		}
	}
}

func (j *AttachmentJanitor) runRound(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	result, err := j.RunOnce(ctx)
	if j.logger != nil {
		j.logger.Info("attachment janitor round",
			"component", "attachment_janitor",
			"operation", "janitor_round",
			"scanned", result.Scanned,
			"removed", result.Removed,
			"retried", result.Retried,
			"err", err,
		)
	}
}
