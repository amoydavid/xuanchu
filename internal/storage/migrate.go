package storage

import "fmt"

func (s *Store) migrate() error {
	switch s.dialect {
	case "sqlite":
		return s.migrateSQLite()
	case "postgres":
		return s.migratePostgres()
	default:
		return fmt.Errorf("unsupported dialect: %s", s.dialect)
	}
}
