package storage

import "gorm.io/gorm"

// 本文件提供 Task 1 期间的 deprecated 兼容 alias，让 App 包在 Task 2 完成
// scope 化迁移之前继续编译。Task 2 会删除本文件。

// ProjectAutomationRuleRepository 兼容旧调用方；内部直接转发到通用仓储。
type ProjectAutomationRuleRepository = AutomationRuleRepository

// ProjectAutomationDeliveryRepository 兼容旧调用方。
type ProjectAutomationDeliveryRepository = AutomationDeliveryRepository

// ProjectAutomationCandidateListOptions 兼容旧调用方。
type ProjectAutomationCandidateListOptions = AutomationCandidateListOptions

// ProjectAutomationCandidatePage 兼容旧调用方。
type ProjectAutomationCandidatePage = AutomationCandidatePage

// ProjectAutomationDeliveryListOptions 兼容旧调用方。
type ProjectAutomationDeliveryListOptions = AutomationDeliveryListOptions

// NewProjectAutomationRuleRepository 兼容旧调用方。
func NewProjectAutomationRuleRepository(db *gorm.DB) *AutomationRuleRepository {
	return NewAutomationRuleRepository(db)
}

// NewProjectAutomationDeliveryRepository 兼容旧调用方。
func NewProjectAutomationDeliveryRepository(db *gorm.DB) *AutomationDeliveryRepository {
	return NewAutomationDeliveryRepository(db)
}
