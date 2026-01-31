package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// soraAccountRepository 实现 service.SoraAccountRepository 接口。
// 使用原生 SQL 操作 sora_accounts 表，因为该表不在 Ent ORM 管理范围内。
//
// 设计说明：
//   - sora_accounts 表是独立迁移创建的，不通过 Ent Schema 管理
//   - 使用 ON CONFLICT (account_id) DO UPDATE 实现 Upsert 语义
//   - 与 accounts 主表通过外键关联，ON DELETE CASCADE 确保级联删除
type soraAccountRepository struct {
	sql *sql.DB
REDACTED

// NewSoraAccountRepository 创建 Sora 账号扩展表仓储实例
func NewSoraAccountRepository(sqlDB *sql.DB) service.SoraAccountRepository {
	return &soraAccountRepository{sql: sqlDBREDACTED
REDACTED

// Upsert 创建或更新 Sora 账号扩展信息
// 使用 PostgreSQL ON CONFLICT ... DO UPDATE 实现原子性 upsert
func (r *soraAccountRepository) Upsert(ctx context.Context, accountID int64, updates map[string]any) error {
	accessToken, accessOK := updates["access_token"].(string)
	refreshToken, refreshOK := updates["refresh_token"].(string)
	sessionToken, sessionOK := updates["session_token"].(string)

	if !accessOK || accessToken == "" || !refreshOK || refreshToken == "" {
		if !sessionOK {
			return errors.New("缺少 access_token/refresh_token，且未提供可更新字段")
	REDACTED
		result, err := r.sql.ExecContext(ctx, `
			UPDATE sora_accounts
			SET session_token = CASE WHEN $2 = '' THEN session_token ELSE $2 END,
				updated_at = NOW()
			WHERE account_id = $1
		`, accountID, sessionToken)
		if err != nil {
			return err
	REDACTED
		rows, err := result.RowsAffected()
		if err != nil {
			return err
	REDACTED
		if rows == 0 {
			return errors.New("sora_accounts 记录不存在，无法仅更新 session_token")
	REDACTED
		return nil
REDACTED

	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO sora_accounts (account_id, access_token, refresh_token, session_token, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (account_id) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			session_token = CASE WHEN EXCLUDED.session_token = '' THEN sora_accounts.session_token ELSE EXCLUDED.session_token END,
			updated_at = NOW()
	`, accountID, accessToken, refreshToken, sessionToken)
	return err
REDACTED

// GetByAccountID 根据账号 ID 获取 Sora 扩展信息
func (r *soraAccountRepository) GetByAccountID(ctx context.Context, accountID int64) (*service.SoraAccount, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT account_id, access_token, refresh_token, COALESCE(session_token, '')
		FROM sora_accounts
		WHERE account_id = $1
	`, accountID)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	if !rows.Next() {
		return nil, nil // 记录不存在
REDACTED

	var sa service.SoraAccount
	if err := rows.Scan(&sa.AccountID, &sa.AccessToken, &sa.RefreshToken, &sa.SessionToken); err != nil {
		return nil, err
REDACTED
	return &sa, nil
REDACTED

// Delete 删除 Sora 账号扩展信息
func (r *soraAccountRepository) Delete(ctx context.Context, accountID int64) error {
	_, err := r.sql.ExecContext(ctx, `
		DELETE FROM sora_accounts WHERE account_id = $1
	`, accountID)
	return err
REDACTED
