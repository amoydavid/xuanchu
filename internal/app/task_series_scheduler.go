package app

import (
	"context"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// TaskSeriesSchedulerOptions 是 scheduler 的构造参数（spec §9.1）。
type TaskSeriesSchedulerOptions struct {
	Store          *storage.Store
	Clock          Clock
	ServiceFactory func(workspaceID string) *Service
	PerSeriesLimit int
	GlobalLimit    int
}

// TaskSeriesScheduler 按日历补齐所有 workspace 的 active series（spec §9）。
//
// 复用 ProjectAutomationScheduler 的 wiring 模式：
// 默认 ServiceFactory、workspace-aware services、RunOnce、context cancellation、60 秒 Run。
type TaskSeriesScheduler struct {
	opts TaskSeriesSchedulerOptions
}

// NewTaskSeriesScheduler 构造 scheduler。
func NewTaskSeriesScheduler(opts TaskSeriesSchedulerOptions) *TaskSeriesScheduler {
	if opts.PerSeriesLimit <= 0 {
		opts.PerSeriesLimit = 100
	}
	if opts.GlobalLimit <= 0 {
		opts.GlobalLimit = 1000
	}
	return &TaskSeriesScheduler{opts: opts}
}

// RunOnce 扫描所有 active series 并补齐已进入执行期的槽位（spec §9.2、§8.3）。
//
// 全局最多创建 GlobalLimit 条；超出返回 backlog_remaining。
// 重复/并发调用幂等（唯一索引保护）。
func (s *TaskSeriesScheduler) RunOnce(ctx context.Context) (TaskSeriesReconcileResult, error) {
	result := TaskSeriesReconcileResult{}
	// 列出所有 workspace。
	workspaces, err := storage.NewWorkspaceRepository(s.opts.Store.DB()).ListAll(false)
	if err != nil {
		return result, err
	}
	globalCreated := 0
	for _, ws := range workspaces {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		svc := s.opts.ServiceFactory(ws.ID)
		if svc == nil {
			continue
		}
		// 分页扫描 active series。
		offset := 0
		for {
			seriesList, err := svc.taskSeriesRepo.ListActive(ws.ID, 100, offset)
			if err != nil {
				return result, err
			}
			if len(seriesList) == 0 {
				break
			}
			for _, series := range seriesList {
				if globalCreated >= s.opts.GlobalLimit {
					result.BacklogRemaining++
					continue
				}
				remaining := s.opts.GlobalLimit - globalCreated
				if remaining > s.opts.PerSeriesLimit {
					remaining = s.opts.PerSeriesLimit
				}
				res, err := svc.ReconcileTaskSeries(series.ID, s.opts.Clock.Unix(), remaining)
				if err != nil {
					return result, err
				}
				result.Created += res.Created
				result.BacklogRemaining += res.BacklogRemaining
				if res.Ended {
					result.Ended = true
				}
				globalCreated += res.Created
			}
			offset += len(seriesList)
			if len(seriesList) < 100 {
				break
			}
		}
	}
	return result, nil
}

// Run 每 interval 执行一次 RunOnce，直到 ctx 取消（spec §9.1）。
func (s *TaskSeriesScheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}
