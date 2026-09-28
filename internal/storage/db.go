package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const localWorkspaceSlug = "local"
const localUserName = "local"
const m5ProjectsSkippedMetaKey = "migration.m5.projects.skipped"

type Store struct {
	db      *gorm.DB
	dialect string
}

func (s *Store) Dialect() string { return s.dialect }

type MigrationWarning struct {
	WorkspaceID string `json:"workspace_id"`
	TaskUUID    string `json:"task_uuid"`
	RawProject  string `json:"raw_project"`
	Reason      string `json:"reason"`
}

type m5SkippedProject struct {
	WorkspaceID string `json:"workspace_id"`
	TaskUUID    string `json:"task_uuid"`
	RawProject  string `json:"raw_project"`
	Reason      string `json:"reason"`
}

func Open(dbURL string) (*Store, error) {
	if isPostgresURL(dbURL) {
		return openPostgres(dbURL)
	}
	if strings.Contains(dbURL, "://") {
		return nil, fmt.Errorf("unsupported database scheme: %s", dbURL)
	}
	return openSQLite(dbURL)
}

func isPostgresURL(s string) bool {
	return strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://")
}

func (s *Store) Close() error {
	sqlDB, err := s.sqlDB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) DB() *gorm.DB {
	return s.db
}

func (s *Store) Transaction(fn func(*Store) error) error {
	// SQLite deferred 事务「先读后写」在升级 RESERVED 锁时，若另一连接正
	// 持有写锁，busy handler 不会被调用、立即返回 SQLITE_BUSY（升级死锁
	// 保护）。服务端写事务（withAuditAndEvents 等）与后台 dispatcher 的
	// 写事务撞上该窗口时会以 "database is locked" 直接外溢为 500。
	// 锁持有者是活跃写事务、很快提交，这里对整个事务做有界退避重试，
	// 把瞬时锁冲突转为排队效果。fn 必须只含 DB 操作（外部副作用一律放
	// 事务提交之后，与 withAuditEntriesAndEvents 的现有约定一致）。
	const maxAttempts = 5
	backoffs := []time.Duration{20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond, 160 * time.Millisecond}
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			sleep := backoffs[len(backoffs)-1]
			if attempt-1 < len(backoffs) {
				sleep = backoffs[attempt-1]
			}
			time.Sleep(sleep)
		}
		err = s.db.Transaction(func(tx *gorm.DB) error {
			return fn(&Store{db: tx, dialect: s.dialect})
		})
		if !isSQLiteBusy(err) {
			return err
		}
	}
	return err
}

// isSQLiteBusy 识别 SQLite 立即返回的锁冲突错误（SQLITE_BUSY，错误码 5）。
// glebarez/modernc 侧错误为 *sqlite.Error，经 GORM 包装后按消息匹配，
// 与 project_template 相关重试逻辑的判定方式保持一致；PostgreSQL 的锁
// 冲突消息不同，不会命中。
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "sqlite_locked")
}

func (s *Store) LocalWorkspace() (Workspace, error) {
	var ws Workspace
	err := s.db.Where("slug = ?", localWorkspaceSlug).First(&ws).Error
	return ws, err
}

func (s *Store) GetMeta(key string) (string, bool, error) {
	var meta Meta
	err := s.db.Where("key = ?", key).First(&meta).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return meta.Value, true, nil
}

func (s *Store) SetMeta(key, value string) error {
	return s.db.Save(&Meta{Key: key, Value: value}).Error
}

func (s *Store) DeleteMeta(key string) error {
	return s.db.Delete(&Meta{Key: key}).Error
}

func (s *Store) ListMeta() (map[string]string, error) {
	var rows []Meta
	if err := s.db.Order("key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return values, nil
}

func (s *Store) M5ProjectMigrationReport() ([]MigrationWarning, error) {
	raw, ok, err := s.GetMeta(m5ProjectsSkippedMetaKey)
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var skipped []m5SkippedProject
	if err := json.Unmarshal([]byte(raw), &skipped); err != nil {
		return nil, err
	}
	out := make([]MigrationWarning, 0, len(skipped))
	for _, row := range skipped {
		out = append(out, MigrationWarning{
			WorkspaceID: row.WorkspaceID,
			TaskUUID:    row.TaskUUID,
			RawProject:  row.RawProject,
			Reason:      row.Reason,
		})
	}
	return out, nil
}

func (s *Store) ensureLocalIdentity() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()

		var user User
		err := tx.Where("name = ?", localUserName).First(&user).Error
		switch {
		case err == nil:
		case errors.Is(err, gorm.ErrRecordNotFound):
			user = User{
				ID:         uuid.NewString(),
				Name:       localUserName,
				CreatedAt:  now,
				ModifiedAt: now,
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		default:
			return err
		}

		var ws Workspace
		err = tx.Where("slug = ?", localWorkspaceSlug).First(&ws).Error
		switch {
		case err == nil:
			updates := map[string]any{}
			if ws.Visibility == "" {
				updates["visibility"] = "private"
			}
			if ws.SettingsJSON == "" {
				updates["settings_json"] = "{}"
			}
			if ws.Name == "" {
				updates["name"] = "Local"
			}
			if ws.CreatedByUserID == nil {
				updates["created_by_user_id"] = user.ID
			}
			if ws.ModifiedAt == 0 {
				updates["modified_at"] = ws.CreatedAt
			}
			if len(updates) > 0 {
				if err := tx.Model(&Workspace{}).Where("id = ?", ws.ID).Updates(updates).Error; err != nil {
					return err
				}
				if err := tx.Where("id = ?", ws.ID).First(&ws).Error; err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			ws = Workspace{
				ID:              uuid.NewString(),
				Slug:            localWorkspaceSlug,
				Name:            "Local",
				CreatedByUserID: &user.ID,
				Visibility:      "private",
				SettingsJSON:    "{}",
				CreatedAt:       now,
				ModifiedAt:      now,
			}
			if err := tx.Create(&ws).Error; err != nil {
				return err
			}
		default:
			return err
		}

		if user.DefaultWorkspaceID == nil || *user.DefaultWorkspaceID != ws.ID {
			if err := tx.Model(&User{}).Where("id = ?", user.ID).Updates(map[string]any{
				"default_workspace_id": ws.ID,
				"modified_at":          now,
			}).Error; err != nil {
				return err
			}
		}

		member := Membership{
			UserID:      user.ID,
			WorkspaceID: ws.ID,
			Role:        "owner",
			JoinedAt:    now,
			ModifiedAt:  now,
		}
		if err := tx.Where("user_id = ? AND workspace_id = ?", user.ID, ws.ID).First(&Membership{}).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		var oldMeta Meta
		err = tx.Where("key = ?", "context.active").First(&oldMeta).Error
		switch {
		case err == nil:
			if err := tx.Save(&Meta{Key: "active_context." + user.ID + "." + ws.ID, Value: oldMeta.Value}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&Meta{Key: "context.active"}).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
		default:
			return err
		}
		return nil
	})
}

func (s *Store) sqlDB() (*sql.DB, error) {
	return s.db.DB()
}
