package storage

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openSQLite(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(sqliteDSN(path)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, dialect: "sqlite"}
	if err := configureSQLite(store); err != nil {
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

func configureSQLite(s *Store) error {
	return s.db.Exec("PRAGMA foreign_keys = ON").Error
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "foreign_keys(1)")
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + values.Encode()
}
