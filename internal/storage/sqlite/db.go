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
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
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

func (s *Store) configure() error {
	return s.db.Exec("PRAGMA foreign_keys = ON").Error
}

func (s *Store) migrate() error {
	return s.db.AutoMigrate(&Meta{}, &Workspace{}, &Task{}, &TaskTag{})
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
