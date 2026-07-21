package storage

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const candidateSortKeyBatchSize = 200

type candidateSortKey struct {
	CandidateID string   `gorm:"column:candidate_id;primaryKey"`
	SortKey     *float64 `gorm:"column:sort_key"`
}

// withCandidateSortKeyTable 在同一数据库事务和连接上创建、使用并删除临时排序键表。
// 动态领域值只写入临时表；最终 ORDER/LIMIT/OFFSET 仍由 SQL 完成。
func withCandidateSortKeyTable(db *gorm.DB, fn func(tx *gorm.DB, tableName string) error) error {
	tableName := "candidate_sort_keys_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedName := quoteCandidateSortKeyTable(tableName)
	return db.Transaction(func(tx *gorm.DB) (err error) {
		createSQL := fmt.Sprintf("CREATE TEMPORARY TABLE %s (candidate_id TEXT PRIMARY KEY, sort_key DOUBLE PRECISION)", quotedName)
		if tx.Dialector.Name() == "postgres" {
			createSQL += " ON COMMIT DROP"
		}
		if err := tx.Exec(createSQL).Error; err != nil {
			return err
		}
		defer func() {
			dropErr := tx.Exec("DROP TABLE IF EXISTS " + quotedName).Error
			if err == nil {
				err = dropErr
			}
		}()
		return fn(tx, tableName)
	})
}

func quoteCandidateSortKeyTable(tableName string) string {
	return `"` + strings.ReplaceAll(tableName, `"`, `""`) + `"`
}
