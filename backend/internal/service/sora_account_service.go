package service

import "context"

// SoraAccountRepository Sora 账号扩展表仓储接口
// 用于管理 sora_accounts 表，与 accounts 主表形成双表结构。
//
// 设计说明：
//   - sora_accounts 表存储 Sora 账号的 OAuth 凭证副本
//   - Sora gateway 优先读取此表的字段以获得更好的查询性能
//   - 主表 accounts 通过 credentials JSON 字段也存储相同信息
//   - Token 刷新时需要同时更新两个表以保持数据一致性
type SoraAccountRepository interface {
	// Upsert 创建或更新 Sora 账号扩展信息
	// accountID: 关联的 accounts.id
	// updates: 要更新的字段，支持 access_token、refresh_token、session_token
	//
	// 如果记录不存在则创建，存在则更新。
	// 用于：
	//   1. 创建 Sora 账号时初始化扩展表
	//   2. Token 刷新时同步更新扩展表
	Upsert(ctx context.Context, accountID int64, updates map[string]any) error

	// GetByAccountID 根据账号 ID 获取 Sora 扩展信息
	// 返回 nil, nil 表示记录不存在（非错误）
	GetByAccountID(ctx context.Context, accountID int64) (*SoraAccount, error)

	// Delete 删除 Sora 账号扩展信息
	// 通常由外键 ON DELETE CASCADE 自动处理，此方法用于手动清理
	Delete(ctx context.Context, accountID int64) error
REDACTED

// SoraAccount Sora 账号扩展信息
// 对应 sora_accounts 表，存储 Sora 账号的 OAuth 凭证副本
type SoraAccount struct {
	AccountID    int64  // 关联的 accounts.id
	AccessToken  string // OAuth access_token
	RefreshToken string // OAuth refresh_token
	SessionToken string // Session token（可选，用于 ST→AT 兜底）
REDACTED
