package storage

func (s *Store) migratePostgres() error {
	if err := s.db.AutoMigrate(
		&Meta{}, &User{}, &Workspace{}, &Membership{},
		&AuditLog{}, &Project{}, &ProjectAnnotation{}, &Config{}, &ConfigDefinition{}, &ApiToken{},
		&Context{}, &UDADefinition{}, &HookDefinition{}, &HookDelivery{},
		&UserExternalID{}, &Task{},
	); err != nil {
		return err
	}
	return s.db.AutoMigrate(
		&TaskTag{}, &TaskAnnotation{}, &TaskDependency{},
		&TaskAssignee{}, &TaskUDAValue{}, &TaskLink{},
	)
}
