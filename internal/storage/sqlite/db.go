package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const localWorkspaceSlug = "local"
const localUserName = "local"

type Store struct {
	db *gorm.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.configure(); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.migrate(); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.ensureLocalIdentity(); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
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
	return s.db.Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
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

func (s *Store) configure() error {
	return s.db.Exec("PRAGMA foreign_keys = ON").Error
}

func (s *Store) migrate() error {
	if err := s.db.AutoMigrate(&Meta{}, &User{}, &Workspace{}, &Membership{}, &AuditLog{}, &Context{}, &UDADefinition{}, &Task{}, &TaskTag{}, &TaskAnnotation{}, &TaskDependency{}, &TaskUDAValue{}); err != nil {
		return err
	}
	return s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_task_parent_due_open ON tasks(parent, due) WHERE status IN ('pending', 'waiting') AND parent IS NOT NULL AND due IS NOT NULL").Error
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
