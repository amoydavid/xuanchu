package storage

import (
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

func newProjectTemplateRepoTest(t *testing.T) (*Store, *ProjectTemplateRepository, Workspace) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "xuanchu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ws, err := store.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	return store, NewProjectTemplateRepository(store.DB()), ws
}

func templateRow(id, workspaceID, key string) ProjectTemplate {
	return ProjectTemplate{
		ID:                 id,
		WorkspaceID:        workspaceID,
		Key:                key,
		Name:               "项目模板 " + key,
		Description:        "固定测试模板",
		Status:             "active",
		CreatedByActorType: "user",
		CreatedAt:          100,
		ModifiedAt:         100,
	}
}

func snapshotRow(id, hash string) ProjectTemplateSnapshot {
	return ProjectTemplateSnapshot{
		ID:                 id,
		SourceProjectID:    "project-source",
		SnapshotJSON:       `{"schema":"fixture/v1","payload":"storage keeps this opaque"}`,
		SnapshotHash:       hash,
		CreatedByActorType: "user",
		CreatedAt:          100,
	}
}

func TestProjectTemplateRepositoryAppendIsImmutableAndScoped(t *testing.T) {
	store, repo, ws := newProjectTemplateRepoTest(t)
	tpl := templateRow("tpl-1", ws.ID, "launch")
	if err := repo.Create(tpl); err != nil {
		t.Fatal(err)
	}
	var first, second ProjectTemplateSnapshot
	err := store.Transaction(func(tx *Store) error {
		var appendErr error
		first, appendErr = NewProjectTemplateRepository(tx.DB()).AppendSnapshotLocked(ws.ID, tpl.ID, snapshotRow("snap-1", "hash-1"))
		return appendErr
	})
	if err != nil || first.Version != 1 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	err = store.Transaction(func(tx *Store) error {
		var appendErr error
		second, appendErr = NewProjectTemplateRepository(tx.DB()).AppendSnapshotLocked(ws.ID, tpl.ID, snapshotRow("snap-2", "hash-2"))
		return appendErr
	})
	if err != nil || second.Version != 2 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	got, err := repo.GetSnapshot(ws.ID, tpl.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SnapshotJSON != first.SnapshotJSON || got.SnapshotHash != first.SnapshotHash || got.Version != 1 {
		t.Fatalf("old snapshot mutated: %#v", got)
	}
	if _, err := repo.GetByRef("another-workspace", tpl.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByRef(cross workspace) err=%v, want ErrNotFound", err)
	}
	if _, err := repo.GetSnapshot("another-workspace", tpl.ID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSnapshot(cross workspace) err=%v, want ErrNotFound", err)
	}
	current, err := repo.GetByRef(ws.ID, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentSnapshotID == nil || *current.CurrentSnapshotID != second.ID {
		t.Fatalf("CurrentSnapshotID = %#v, want %q", current.CurrentSnapshotID, second.ID)
	}
}

func TestProjectTemplateRepositoryAppendRequiresTransaction(t *testing.T) {
	_, repo, ws := newProjectTemplateRepoTest(t)
	tpl := templateRow("tpl-1", ws.ID, "launch")
	if err := repo.Create(tpl); err != nil {
		t.Fatal(err)
	}
	_, err := repo.AppendSnapshotLocked(ws.ID, tpl.ID, snapshotRow("snap-1", "hash-1"))
	if !errors.Is(err, ErrProjectTemplateTransactionRequired) {
		t.Fatalf("AppendSnapshotLocked outside transaction err=%v, want ErrProjectTemplateTransactionRequired", err)
	}
}

func TestProjectTemplateRepositoryMapsUniqueConflicts(t *testing.T) {
	store, repo, ws := newProjectTemplateRepoTest(t)
	tpl := templateRow("tpl-1", ws.ID, "launch")
	if err := repo.Create(tpl); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(templateRow("tpl-2", ws.ID, "launch")); !errors.Is(err, ErrProjectTemplateKeyConflict) {
		t.Fatalf("Create(duplicate key) err=%v, want ErrProjectTemplateKeyConflict", err)
	}
	if err := store.Transaction(func(tx *Store) error {
		_, err := NewProjectTemplateRepository(tx.DB()).AppendSnapshotLocked(ws.ID, tpl.ID, snapshotRow("snap-1", "same-hash"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Transaction(func(tx *Store) error {
		_, err := NewProjectTemplateRepository(tx.DB()).AppendSnapshotLocked(ws.ID, tpl.ID, snapshotRow("snap-2", "same-hash"))
		return err
	}); !errors.Is(err, ErrProjectTemplateHashConflict) {
		t.Fatalf("AppendSnapshotLocked(duplicate hash) err=%v, want ErrProjectTemplateHashConflict", err)
	}
}

func TestProjectTemplateRepositoryConcurrentSQLiteAppendAllocatesDistinctVersions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "xuanchu.db")
	seed, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = seed.Close() })
	ws, err := seed.LocalWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if err := NewProjectTemplateRepository(seed.DB()).Create(templateRow("tpl-1", ws.ID, "launch")); err != nil {
		t.Fatal(err)
	}

	stores := make([]*Store, 2)
	for i := range stores {
		stores[i], err = Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = stores[i].Close() })
	}

	start := make(chan struct{})
	versions := make(chan int64, len(stores))
	errs := make(chan error, len(stores))
	var group sync.WaitGroup
	for i, store := range stores {
		group.Add(1)
		go func(i int, store *Store) {
			defer group.Done()
			<-start
			err := store.Transaction(func(tx *Store) error {
				row, appendErr := NewProjectTemplateRepository(tx.DB()).AppendSnapshotLocked(ws.ID, "tpl-1", snapshotRow("snap-concurrent-"+string(rune('a'+i)), "hash-concurrent-"+string(rune('a'+i))))
				if appendErr == nil {
					versions <- row.Version
				}
				return appendErr
			})
			errs <- err
		}(i, store)
	}
	close(start)
	group.Wait()
	close(errs)
	close(versions)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent append error = %v", err)
		}
	}
	var gotVersions []int64
	for version := range versions {
		gotVersions = append(gotVersions, version)
	}
	sort.Slice(gotVersions, func(i, j int) bool { return gotVersions[i] < gotVersions[j] })
	if len(gotVersions) != 2 || gotVersions[0] != 1 || gotVersions[1] != 2 {
		t.Fatalf("versions = %#v, want [1 2]", gotVersions)
	}
	current, err := NewProjectTemplateRepository(seed.DB()).GetByRef(ws.ID, "tpl-1")
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentSnapshotID == nil {
		t.Fatal("CurrentSnapshotID is nil")
	}
	currentSnapshot, err := NewProjectTemplateRepository(seed.DB()).GetSnapshot(ws.ID, "tpl-1", *current.CurrentSnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if currentSnapshot.Version != 2 {
		t.Fatalf("current snapshot version = %d, want 2", currentSnapshot.Version)
	}
}
