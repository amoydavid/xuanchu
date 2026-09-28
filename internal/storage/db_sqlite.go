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
	// busy_timeout 吸收跨连接的锁竞争（默认 0 = 冲突立即报错）。
	// 注意 SQLite 对 deferred 事务「先读后写」的锁升级冲突不调用 busy
	// handler、直接返回 SQLITE_BUSY（升级死锁保护），该路径由
	// Store.Transaction 的有界重试兜底，不要用 _txlock=immediate——那会
	// 让读路径事务也串行抢全局写锁。
	values.Add("_pragma", "busy_timeout(5000)")
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + values.Encode()
}
