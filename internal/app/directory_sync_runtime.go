package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	xuanchuOIDC "git.dajee.net/dajee/xuanchu/internal/auth/oidc"
	"git.dajee.net/dajee/xuanchu/internal/storage"
)

// DirectorySyncRuntime 是通讯录同步的后台 dispatcher + scheduler，
// 复用现有 hook/notification dispatcher 的 claim/lease 持久化范式。
type DirectorySyncRuntime struct {
	store        *storage.Store
	cfg          *OIDCConfigService
	sync         *DirectorySyncService
	jobRepo      *storage.DirectorySyncJobRepository
	wsRepo       *storage.WorkspaceRepository
	pollInterval time.Duration
}

func NewDirectorySyncRuntime(store *storage.Store, cfg *OIDCConfigService, sync *DirectorySyncService) *DirectorySyncRuntime {
	return &DirectorySyncRuntime{
		store:        store,
		cfg:          cfg,
		sync:         sync,
		jobRepo:      storage.NewDirectorySyncJobRepository(store.DB()),
		wsRepo:       storage.NewWorkspaceRepository(store.DB()),
		pollInterval: 30 * time.Second,
	}
}

// Run 启动 dispatcher + scheduler 循环，随 ctx 取消退出。
func (r *DirectorySyncRuntime) Run(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	// 启动即跑一次
	r.tick(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx, time.Now())
		}
	}
}

func (r *DirectorySyncRuntime) tick(ctx context.Context, now time.Time) {
	// 1. 调度：为到期 workspace 创建 job
	r.scheduleWorkspaces(now.Unix())
	// 2. 执行：认领并执行一个 pending job
	r.runOneJob(ctx, now.Unix())
}

// scheduleWorkspaces 遍历所有 workspace，对启用了 OIDC 且到期的 workspace 插入 pending job。
func (r *DirectorySyncRuntime) scheduleWorkspaces(now int64) {
	workspaces, err := r.wsRepo.ListAll(false)
	if err != nil {
		return
	}
	for _, ws := range workspaces {
		cfg, enabled := r.cfg.Get(ws.ID)
		if !enabled {
			continue
		}
		intervalStr := cfg.SyncInterval
		if intervalStr == "" {
			intervalStr = "1h"
		}
		// 0 表示禁用定时同步
		if intervalStr == "0" {
			continue
		}
		interval, err := time.ParseDuration(intervalStr)
		if err != nil || interval <= 0 {
			continue
		}
		// 比较最近一次 job 的 created_at，是否已过 interval
		latest, err := r.jobRepo.LatestForWorkspace(ws.ID)
		if errors.Is(err, storage.ErrNotFound) {
			// 从未同步 → 立即插入
			if _, err := r.jobRepo.Create(ws.ID, now); err != nil {
				continue
			}
			continue
		}
		if err != nil {
			continue
		}
		if latest.CreatedAt+int64(interval.Seconds()) <= now {
			_, _ = r.jobRepo.Create(ws.ID, now)
		}
	}
}

func (r *DirectorySyncRuntime) runOneJob(ctx context.Context, now int64) {
	claimTTL := int64(r.pollInterval.Seconds()) * 3
	job, err := r.jobRepo.ClaimNextPending(now, claimTTL)
	if err != nil {
		return // 无 pending 或被抢
	}
	cfg, enabled := r.cfg.Get(job.WorkspaceID)
	if !enabled {
		_ = r.jobRepo.MarkFailed(job.ID, now, "sso_not_enabled")
		return
	}
	secrets, err := r.cfg.ResolveSecrets(job.WorkspaceID)
	if err != nil {
		// 详细错误打日志，持久化固定码（防 secret/解密细节泄漏进 ErrorMessage）
		log.Printf("directory sync: resolve secrets failed for workspace %s: %v", job.WorkspaceID, err)
		_ = r.jobRepo.MarkFailed(job.ID, now, "resolve_secrets_failed")
		return
	}
	// 用 client_credentials grant 向 IdP 换取 directory 访问 token
	// （复用 OIDC client_id + client_secret，无需单独配置 directory_access_token）
	directoryToken, err := r.fetchDirectoryToken(ctx, cfg, secrets.ClientSecret)
	if err != nil {
		log.Printf("directory token fetch failed for workspace %s: %v", job.WorkspaceID, err)
		_ = r.jobRepo.MarkFailed(job.ID, now, "directory_token_fetch_failed")
		return
	}
	stats, err := r.sync.SyncOnce(ctx, job.WorkspaceID, cfg.IssuerBaseURL, cfg.OrgID, directoryToken)
	if err != nil {
		log.Printf("directory sync failed for workspace %s: %v", job.WorkspaceID, err)
		_ = r.jobRepo.MarkFailed(job.ID, time.Now().Unix(), "directory_sync_failed")
		return
	}
	statsJSON, _ := json.Marshal(map[string]int{"added": stats.Added, "removed": stats.Removed, "updated": stats.Updated})
	_ = r.jobRepo.MarkSucceeded(job.ID, time.Now().Unix(), string(statsJSON))
}

// fetchDirectoryToken 用 OIDC client_id + client_secret 走 client_credentials grant 换 access token。
// 每次同步都重新换取，确保 token 不会过期。
func (r *DirectorySyncRuntime) fetchDirectoryToken(ctx context.Context, cfg OIDCConfig, clientSecret string) (string, error) {
	p, err := xuanchuOIDC.NewProviderSafe(ctx, cfg.IssuerBaseURL, cfg.ClientID, clientSecret)
	if err != nil {
		return "", err
	}
	return p.ClientCredentialsToken(ctx, "org.members.read")
}
