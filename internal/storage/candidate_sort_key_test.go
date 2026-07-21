package storage

import (
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestCandidateSortKeyTableUsesTransactionConnectionAndCleansUp(t *testing.T) {
	store, _, _ := newTestRepo(t)
	sqlDB, err := store.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	var tableName string
	err = withCandidateSortKeyTable(store.DB(), func(tx *gorm.DB, name string) error {
		tableName = name
		if err := tx.Table(name).Create(map[string]any{"candidate_id": "same", "sort_key": 11}).Error; err != nil {
			return err
		}
		var score float64
		if err := tx.Table(name).Select("sort_key").Where("candidate_id = ?", "same").Scan(&score).Error; err != nil {
			return err
		}
		if score != 11 {
			t.Fatalf("sort key = %v, want 11", score)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := store.DB().Raw("SELECT COUNT(*) FROM sqlite_temp_master WHERE type = 'table' AND name = ?", tableName).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("temporary table %q remains after transaction", tableName)
	}

	wantErr := errors.New("stop candidate sort")
	err = withCandidateSortKeyTable(store.DB(), func(tx *gorm.DB, name string) error {
		tableName = name
		if err := tx.Table(name).Create(map[string]any{"candidate_id": "rollback", "sort_key": 12}).Error; err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error path = %v, want %v", err, wantErr)
	}
	if err := store.DB().Raw("SELECT COUNT(*) FROM sqlite_temp_master WHERE type = 'table' AND name = ?", tableName).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("temporary table %q remains after rollback", tableName)
	}
}

func TestCandidateSortKeyTablesAreConcurrentIsolated(t *testing.T) {
	store, _, _ := newTestRepo(t)
	sqlDB, err := store.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(2)

	release := make(chan struct{})
	results := make(chan float64, 2)
	tableNames := make(chan string, 2)
	started := make(chan error, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, want := range []float64{11, 22} {
		want := want
		wg.Add(1)
		go func() {
			defer wg.Done()
			startedSent := false
			err := withCandidateSortKeyTable(store.DB(), func(tx *gorm.DB, name string) error {
				if err := tx.Table(name).Create(map[string]any{"candidate_id": "same", "sort_key": want}).Error; err != nil {
					return err
				}
				tableNames <- name
				started <- nil
				startedSent = true
				<-release
				var got float64
				if err := tx.Table(name).Select("sort_key").Where("candidate_id = ?", "same").Scan(&got).Error; err != nil {
					return err
				}
				results <- got
				return nil
			})
			if !startedSent {
				started <- err
			}
			errs <- err
		}()
	}
	for range 2 {
		if err := <-started; err != nil {
			close(release)
			wg.Wait()
			t.Fatal(err)
		}
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	close(results)
	seen := map[float64]bool{}
	for got := range results {
		seen[got] = true
	}
	if !seen[11] || !seen[22] || len(seen) != 2 {
		t.Fatalf("isolated sort keys = %#v", seen)
	}
	close(tableNames)
	names := map[string]bool{}
	for name := range tableNames {
		names[name] = true
	}
	if len(names) != 2 {
		t.Fatalf("concurrent temporary table names = %#v", names)
	}
}
