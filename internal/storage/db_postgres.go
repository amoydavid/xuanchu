package storage

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openPostgres(dbURL string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, dialect: "postgres"}
	if err := configurePostgres(store); err != nil {
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

func configurePostgres(s *Store) error {
	return nil
}
