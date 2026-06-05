package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	errExpandAccountNotFound    = infraerrors.NotFound("EXPAND_ACCOUNT_NOT_FOUND", "expand account not found")
	errExpandAccountExists      = infraerrors.Conflict("EXPAND_ACCOUNT_EXISTS", "expand account already exists")
	errExpandAccountUsed        = infraerrors.Conflict("EXPAND_ACCOUNT_ALREADY_USED", "expand account already marked as used")
	errExpandInvalidProxy       = infraerrors.BadRequest("INVALID_PROXY_INFO", "invalid proxy_info")
	errExpandAccountUnavailable = infraerrors.NotFound("EXPAND_ACCOUNT_UNAVAILABLE", "no unused expand account available")
	errExpandAccountEmailMismatch = infraerrors.BadRequest("EXPAND_ACCOUNT_EMAIL_MISMATCH", "expand account email mismatch")
)

type expandAccountRepository struct {
	db *sql.DB
}

type expandAccountScanner interface {
	Scan(dest ...any) error
}

func NewExpandAccountService(db *sql.DB) service.ExpandAccountService {
	return &expandAccountRepository{db: db}
}

func (r *expandAccountRepository) ListExpandAccounts(ctx context.Context, page, pageSize int, filters service.ExpandAccountListFilters) ([]service.ExpandAccount, int64, error) {
	whereClause, args := buildExpandAccountListWhere(filters)

	var total int64
	countQuery := "SELECT COUNT(*) FROM expand_accounts" + whereClause
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	listArgs := append(append([]any{}, args...), pageSize, offset)
	query := `
SELECT id, email, platform, subscription_type, country, session_key, proxy_id, proxy_info, used, account_id, login_status, device_id, api_key, email_pwd, help_email, help_email_url, channel, created_at, updated_at
FROM expand_accounts` + whereClause + `
ORDER BY created_at DESC, id DESC
LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)

	rows, err := r.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]service.ExpandAccount, 0, pageSize)
	for rows.Next() {
		item, err := scanExpandAccount(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *expandAccountRepository) GetExpandAccount(ctx context.Context, id int64) (*service.ExpandAccount, error) {
	const query = `
SELECT id, email, platform, subscription_type, country, session_key, proxy_id, proxy_info, used, account_id, login_status, device_id, api_key, email_pwd, help_email, help_email_url, channel, created_at, updated_at
FROM expand_accounts
WHERE id = $1`

	item, err := scanExpandAccount(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errExpandAccountNotFound.WithCause(err)
		}
		return nil, err
	}
	return item, nil
}

func (r *expandAccountRepository) CreateExpandAccount(ctx context.Context, input *service.ExpandAccountCreateInput) (*service.ExpandAccount, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	proxyID, proxyInfoJSON, err := resolveExpandAccountProxyTx(ctx, tx, input.ProxyInfo)
	if err != nil {
		return nil, err
	}

	const query = `
INSERT INTO expand_accounts (email, platform, subscription_type, country, session_key, proxy_id, proxy_info, used, email_pwd, help_email, help_email_url, channel)
VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, false), $9, $10, $11, $12)
RETURNING id, email, platform, subscription_type, country, session_key, proxy_id, proxy_info, used, account_id, login_status, device_id, api_key, email_pwd, help_email, help_email_url, channel, created_at, updated_at`

	item, err := scanExpandAccount(tx.QueryRowContext(
		ctx,
		query,
		input.Email,
		input.Platform,
		input.SubscriptionType,
		input.Country,
		input.SessionKey,
		proxyID,
		proxyInfoJSON,
		input.Used,
		nullableExpandString(input.EmailPwd),
		nullableExpandString(input.HelpEmail),
		nullableExpandString(input.HelpEmailURL),
		nullableExpandString(input.Channel),
	))
	if err != nil {
		return nil, translatePersistenceError(err, nil, errExpandAccountExists)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *expandAccountRepository) UpdateExpandAccount(ctx context.Context, id int64, input *service.ExpandAccountUpdateInput) (*service.ExpandAccount, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	proxyID, proxyInfoJSON, err := resolveExpandAccountProxyTx(ctx, tx, input.ProxyInfo)
	if err != nil {
		return nil, err
	}

	const query = `
UPDATE expand_accounts
SET
	email = $2,
	platform = $3,
	subscription_type = $4,
	country = $5,
	session_key = $6,
	proxy_id = COALESCE($7, proxy_id),
	proxy_info = COALESCE($8::jsonb, proxy_info),
	used = COALESCE($9, used),
	email_pwd = $10,
	help_email = $11,
	help_email_url = $12,
	channel = $13,
	updated_at = NOW()
WHERE id = $1
RETURNING id, email, platform, subscription_type, country, session_key, proxy_id, proxy_info, used, account_id, login_status, device_id, api_key, email_pwd, help_email, help_email_url, channel, created_at, updated_at`

	item, err := scanExpandAccount(tx.QueryRowContext(
		ctx,
		query,
		id,
		input.Email,
		input.Platform,
		input.SubscriptionType,
		input.Country,
		input.SessionKey,
		proxyID,
		proxyInfoJSON,
		input.Used,
		nullableExpandString(input.EmailPwd),
		nullableExpandString(input.HelpEmail),
		nullableExpandString(input.HelpEmailURL),
		nullableExpandString(input.Channel),
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errExpandAccountNotFound.WithCause(err)
		}
		return nil, translatePersistenceError(err, nil, errExpandAccountExists)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *expandAccountRepository) DeleteExpandAccount(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM expand_accounts WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errExpandAccountNotFound
	}
	return nil
}

func (r *expandAccountRepository) MarkExpandAccountUsed(ctx context.Context, id int64) (*service.ExpandAccount, error) {
	const query = `
UPDATE expand_accounts
SET used = true, updated_at = NOW()
WHERE id = $1 AND used = false
RETURNING id, email, platform, subscription_type, country, session_key, proxy_id, proxy_info, used, account_id, login_status, device_id, api_key, email_pwd, help_email, help_email_url, channel, created_at, updated_at`

	item, err := scanExpandAccount(r.db.QueryRowContext(ctx, query, id))
	if err == nil {
		return item, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	existing, getErr := r.GetExpandAccount(ctx, id)
	if getErr != nil {
		return nil, getErr
	}
	if existing.Used {
		return nil, errExpandAccountUsed
	}
	return nil, errExpandAccountNotFound
}

func (r *expandAccountRepository) GetAndMarkExpandAccountByPlatform(ctx context.Context, platform string) (*service.ExpandAccount, error) {
	const query = `
WITH picked AS (
	SELECT id
	FROM expand_accounts
	WHERE used = false
	  AND LOWER(platform) = LOWER($1)
	ORDER BY id ASC
	FOR UPDATE SKIP LOCKED
	LIMIT 1
)
UPDATE expand_accounts ea
SET used = true, updated_at = NOW()
FROM picked
WHERE ea.id = picked.id
RETURNING ea.id, ea.email, ea.platform, ea.subscription_type, ea.country, ea.session_key, ea.proxy_id, ea.proxy_info, ea.used, ea.account_id, ea.login_status, ea.device_id, ea.api_key, ea.email_pwd, ea.help_email, ea.help_email_url, ea.channel, ea.created_at, ea.updated_at`

	item, err := scanExpandAccount(r.db.QueryRowContext(ctx, query, strings.TrimSpace(platform)))
	if err == nil {
		return item, nil
	}
	if err == sql.ErrNoRows {
		return nil, errExpandAccountUnavailable.WithCause(err)
	}
	return nil, err
}

func (r *expandAccountRepository) ReportExpandAccountLogin(ctx context.Context, input *service.ExpandAccountReportInput) (*service.ExpandAccount, error) {
	existing, err := r.GetExpandAccount(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(existing.Email), strings.TrimSpace(input.Email)) {
		return nil, errExpandAccountEmailMismatch
	}

	const query = `
UPDATE expand_accounts
SET login_status = $2,
    device_id = $3,
    api_key = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING id, email, platform, subscription_type, country, session_key, proxy_id, proxy_info, used, account_id, login_status, device_id, api_key, email_pwd, help_email, help_email_url, channel, created_at, updated_at`

	var (
		deviceID any
		apiKey   any
	)
	if input.DeviceID != "" {
		deviceID = input.DeviceID
	}
	if input.APIKey != "" {
		apiKey = input.APIKey
	}

	item, scanErr := scanExpandAccount(r.db.QueryRowContext(ctx, query, input.ID, input.LoginStatus, deviceID, apiKey))
	if scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return nil, errExpandAccountNotFound.WithCause(scanErr)
		}
		return nil, scanErr
	}
	return item, nil
}

func scanExpandAccount(scanner expandAccountScanner) (*service.ExpandAccount, error) {
	var (
		item          service.ExpandAccount
		proxyID       sql.NullInt64
		proxyInfoJSON []byte
		accountID     sql.NullInt64
		deviceID      sql.NullString
		apiKey        sql.NullString
		emailPwd      sql.NullString
		helpEmail     sql.NullString
		helpEmailURL  sql.NullString
		channel       sql.NullString
	)
	err := scanner.Scan(
		&item.ID,
		&item.Email,
		&item.Platform,
		&item.SubscriptionType,
		&item.Country,
		&item.SessionKey,
		&proxyID,
		&proxyInfoJSON,
		&item.Used,
		&accountID,
		&item.LoginStatus,
		&deviceID,
		&apiKey,
		&emailPwd,
		&helpEmail,
		&helpEmailURL,
		&channel,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if proxyID.Valid {
		item.ProxyID = &proxyID.Int64
	}
	if len(proxyInfoJSON) > 0 {
		var proxyInfo service.ProxyInfo
		if err := json.Unmarshal(proxyInfoJSON, &proxyInfo); err != nil {
			return nil, fmt.Errorf("unmarshal expand account proxy_info: %w", err)
		}
		item.ProxyInfo = &proxyInfo
	}
	if accountID.Valid {
		item.AccountID = &accountID.Int64
	}
	if deviceID.Valid {
		item.DeviceID = deviceID.String
	}
	if apiKey.Valid {
		item.APIKey = apiKey.String
	}
	if emailPwd.Valid {
		item.EmailPwd = emailPwd.String
	}
	if helpEmail.Valid {
		item.HelpEmail = helpEmail.String
	}
	if helpEmailURL.Valid {
		item.HelpEmailURL = helpEmailURL.String
	}
	if channel.Valid {
		item.Channel = channel.String
	}
	return &item, nil
}

func resolveExpandAccountProxyTx(ctx context.Context, tx *sql.Tx, input *service.ProxyInfo) (any, any, error) {
	if input == nil {
		return nil, nil, nil
	}

	normalized, err := normalizeExpandProxyInfo(input)
	if err != nil {
		return nil, nil, err
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal proxy_info: %w", err)
	}

	rows, err := tx.QueryContext(ctx, "SELECT pg_advisory_xact_lock($1)", expandAccountProxyLockHash(expandProxyLockKey(normalized)))
	if err != nil {
		return nil, nil, fmt.Errorf("lock proxy_info: %w", err)
	}
	_ = rows.Close()

	var proxyID int64
	err = tx.QueryRowContext(ctx, `
SELECT id
FROM proxies
WHERE deleted_at IS NULL
  AND protocol = $1
  AND host = $2
  AND port = $3
  AND (($4 = '' AND (username IS NULL OR username = '')) OR username = $4)
  AND (($5 = '' AND (password IS NULL OR password = '')) OR password = $5)
ORDER BY id ASC
LIMIT 1`,
		normalized.Protocol,
		normalized.Host,
		normalized.Port,
		normalized.Username,
		normalized.Password,
	).Scan(&proxyID)
	switch {
	case err == nil:
		return proxyID, payload, nil
	case err != sql.ErrNoRows:
		return nil, nil, err
	}

	var username any
	if normalized.Username != "" {
		username = normalized.Username
	}
	var password any
	if normalized.Password != "" {
		password = normalized.Password
	}

	name := buildExpandAccountProxyName(normalized)
	if err := tx.QueryRowContext(ctx, `
INSERT INTO proxies (name, protocol, host, port, username, password, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`,
		name,
		normalized.Protocol,
		normalized.Host,
		normalized.Port,
		username,
		password,
		service.StatusActive,
	).Scan(&proxyID); err != nil {
		return nil, nil, err
	}

	return proxyID, payload, nil
}

func normalizeExpandProxyInfo(input *service.ProxyInfo) (*service.ProxyInfo, error) {
	if input == nil {
		return nil, nil
	}

	normalized := &service.ProxyInfo{
		Protocol: strings.ToLower(strings.TrimSpace(input.Protocol)),
		Host:     strings.ToLower(strings.TrimSpace(input.Host)),
		Port:     input.Port,
		Username: strings.TrimSpace(input.Username),
		Password: strings.TrimSpace(input.Password),
	}
	if normalized.Protocol == "" || normalized.Host == "" || normalized.Port <= 0 {
		return nil, errExpandInvalidProxy
	}
	return normalized, nil
}

func buildExpandAccountProxyName(proxyInfo *service.ProxyInfo) string {
	base := fmt.Sprintf("%s://%s:%d", proxyInfo.Protocol, proxyInfo.Host, proxyInfo.Port)
	if proxyInfo.Username == "" {
		return base
	}
	return base + "#" + proxyInfo.Username
}

func expandProxyLockKey(proxyInfo *service.ProxyInfo) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", proxyInfo.Protocol, proxyInfo.Host, proxyInfo.Port, proxyInfo.Username, proxyInfo.Password)
}

func expandAccountProxyLockHash(key string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))
	return int64(hasher.Sum64())
}

func nullableExpandString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func buildExpandAccountListWhere(filters service.ExpandAccountListFilters) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 6)

	search := strings.TrimSpace(filters.Search)
	if search != "" {
		search = "%" + search + "%"
		args = append(args, search, search, search)
		clauses = append(clauses,
			"(email ILIKE $"+itoa(len(args)-2)+" OR platform ILIKE $"+itoa(len(args)-1)+" OR country ILIKE $"+itoa(len(args))+")",
		)
	}

	switch strings.TrimSpace(filters.Used) {
	case "unused":
		args = append(args, false)
		clauses = append(clauses, "used = $"+itoa(len(args)))
	case "used":
		args = append(args, true)
		clauses = append(clauses, "used = $"+itoa(len(args)))
	}

	if filters.LoginStatus != nil {
		args = append(args, *filters.LoginStatus)
		clauses = append(clauses, "login_status = $"+itoa(len(args)))
	}

	switch strings.TrimSpace(filters.AccountType) {
	case "old":
		clauses = append(clauses, "account_id IS NOT NULL")
	case "new":
		clauses = append(clauses, "account_id IS NULL")
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
