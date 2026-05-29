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
	if err := store.ensureLocalWorkspace(); err != nil {
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
	if err := s.db.AutoMigrate(&Meta{}, &Workspace{}, &Context{}, &UDADefinition{}, &Task{}, &TaskTag{}, &TaskAnnotation{}, &TaskDependency{}, &TaskUDAValue{}); err != nil {
		return err
	}
	return s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_task_parent_due_open ON tasks(parent, due) WHERE status IN ('pending', 'waiting') AND parent IS NOT NULL AND due IS NOT NULL").Error
}

func (s *Store) ensureLocalWorkspace() error {
	var count int64
	if err := s.db.Model(&Workspace{}).Where("slug = ?", localWorkspaceSlug).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.db.Create(&Workspace{
		ID:        uuid.NewString(),
		Slug:      localWorkspaceSlug,
		Name:      "Local",
		CreatedAt: time.Now().Unix(),
	}).Error
}

func (s *Store) sqlDB() (*sql.DB, error) {
	return s.db.DB()
}
