package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const (
	affiliateCodeLength      = 12
	affiliateCodeMaxAttempts = 12
)

var affiliateCodeCharset = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

type affiliateQueryExecer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
REDACTED

type affiliateRepository struct {
	client *dbent.Client
REDACTED

func NewAffiliateRepository(client *dbent.Client, _ *sql.DB) service.AffiliateRepository {
	return &affiliateRepository{client: clientREDACTED
REDACTED

func (r *affiliateRepository) EnsureUserAffiliate(ctx context.Context, userID int64) (*service.AffiliateSummary, error) {
	if userID <= 0 {
		return nil, service.ErrUserNotFound
REDACTED
	client := clientFromContext(ctx, r.client)
	return ensureUserAffiliateWithClient(ctx, client, userID)
REDACTED

func (r *affiliateRepository) GetAffiliateByCode(ctx context.Context, code string) (*service.AffiliateSummary, error) {
	client := clientFromContext(ctx, r.client)
	return queryAffiliateByCode(ctx, client, code)
REDACTED

func (r *affiliateRepository) BindInviter(ctx context.Context, userID, inviterID int64) (bool, error) {
	var bound bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
	REDACTED
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, inviterID); err != nil {
			return err
	REDACTED

		res, err := txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET inviter_id = $1, updated_at = NOW() WHERE user_id = $2 AND inviter_id IS NULL",
			inviterID, userID,
		)
		if err != nil {
			return fmt.Errorf("bind inviter: %w", err)
	REDACTED
		affected, _ := res.RowsAffected()
		if affected == 0 {
			bound = false
			return nil
	REDACTED

		if _, err = txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET aff_count = aff_count + 1, updated_at = NOW() WHERE user_id = $1",
			inviterID,
		); err != nil {
			return fmt.Errorf("increment inviter aff_count: %w", err)
	REDACTED
		bound = true
		return nil
REDACTED)
	if err != nil {
		return false, err
REDACTED
	return bound, nil
REDACTED

func (r *affiliateRepository) AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64) (bool, error) {
	if amount <= 0 {
		return false, nil
REDACTED

	var applied bool
	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		res, err := txClient.ExecContext(txCtx,
			"UPDATE user_affiliates SET aff_quota = aff_quota + $1, aff_history_quota = aff_history_quota + $1, updated_at = NOW() WHERE user_id = $2",
			amount, inviterID,
		)
		if err != nil {
			return err
	REDACTED
		affected, _ := res.RowsAffected()
		if affected == 0 {
			applied = false
			return nil
	REDACTED

		if _, err = txClient.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, created_at, updated_at)
VALUES ($1, 'accrue', $2, $3, NOW(), NOW())`, inviterID, amount, inviteeUserID); err != nil {
			return fmt.Errorf("insert affiliate accrue ledger: %w", err)
	REDACTED

		applied = true
		return nil
REDACTED)
	if err != nil {
		return false, err
REDACTED
	return applied, nil
REDACTED

func (r *affiliateRepository) TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error) {
	var transferred float64
	var newBalance float64

	err := r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if _, err := ensureUserAffiliateWithClient(txCtx, txClient, userID); err != nil {
			return err
	REDACTED

		rows, err := txClient.QueryContext(txCtx, `
WITH claimed AS (
	SELECT aff_quota::double precision AS amount
	FROM user_affiliates
	WHERE user_id = $1
	  AND aff_quota > 0
	FOR UPDATE
),
cleared AS (
	UPDATE user_affiliates ua
	SET aff_quota = 0,
	    updated_at = NOW()
	FROM claimed c
	WHERE ua.user_id = $1
	RETURNING c.amount
)
SELECT amount
FROM cleared`, userID)
		if err != nil {
			return fmt.Errorf("claim affiliate quota: %w", err)
	REDACTED

		if !rows.Next() {
			_ = rows.Close()
			if err := rows.Err(); err != nil {
				return err
		REDACTED
			return service.ErrAffiliateQuotaEmpty
	REDACTED
		if err := rows.Scan(&transferred); err != nil {
			_ = rows.Close()
			return err
	REDACTED
		if err := rows.Close(); err != nil {
			return err
	REDACTED
		if transferred <= 0 {
			return service.ErrAffiliateQuotaEmpty
	REDACTED

		affected, err := txClient.User.Update().
			Where(user.IDEQ(userID)).
			AddBalance(transferred).
			AddTotalRecharged(transferred).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("credit user balance by affiliate quota: %w", err)
	REDACTED
		if affected == 0 {
			return service.ErrUserNotFound
	REDACTED

		newBalance, err = queryUserBalance(txCtx, txClient, userID)
		if err != nil {
			return err
	REDACTED

		if _, err = txClient.ExecContext(txCtx, `
INSERT INTO user_affiliate_ledger (user_id, action, amount, source_user_id, created_at, updated_at)
VALUES ($1, 'transfer', $2, NULL, NOW(), NOW())`, userID, transferred); err != nil {
			return fmt.Errorf("insert affiliate transfer ledger: %w", err)
	REDACTED

		return nil
REDACTED)
	if err != nil {
		return 0, 0, err
REDACTED

	return transferred, newBalance, nil
REDACTED

func (r *affiliateRepository) ListInvitees(ctx context.Context, inviterID int64, limit int) ([]service.AffiliateInvitee, error) {
	if limit <= 0 {
		limit = 100
REDACTED
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT ua.user_id,
       COALESCE(u.email, ''),
       COALESCE(u.username, ''),
       ua.created_at
FROM user_affiliates ua
LEFT JOIN users u ON u.id = ua.user_id
WHERE ua.inviter_id = $1
ORDER BY ua.created_at DESC
LIMIT $2`, inviterID, limit)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	invitees := make([]service.AffiliateInvitee, 0)
	for rows.Next() {
		var item service.AffiliateInvitee
		var createdAt time.Time
		if err := rows.Scan(&item.UserID, &item.Email, &item.Username, &createdAt); err != nil {
			return nil, err
	REDACTED
		item.CreatedAt = &createdAt
		invitees = append(invitees, item)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return invitees, nil
REDACTED

func (r *affiliateRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
REDACTED

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin affiliate transaction: %w", err)
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
REDACTED

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit affiliate transaction: %w", err)
REDACTED
	return nil
REDACTED

func ensureUserAffiliateWithClient(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	summary, err := queryAffiliateByUserID(ctx, client, userID)
	if err == nil {
		return summary, nil
REDACTED
	if !errors.Is(err, service.ErrAffiliateProfileNotFound) {
		return nil, err
REDACTED

	for i := 0; i < affiliateCodeMaxAttempts; i++ {
		code, codeErr := generateAffiliateCode()
		if codeErr != nil {
			return nil, codeErr
	REDACTED
		_, insertErr := client.ExecContext(ctx, `
INSERT INTO user_affiliates (user_id, aff_code, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
ON CONFLICT (user_id) DO NOTHING`, userID, code)
		if insertErr == nil {
			break
	REDACTED
		if isAffiliateUniqueViolation(insertErr) {
			continue
	REDACTED
		return nil, insertErr
REDACTED

	return queryAffiliateByUserID(ctx, client, userID)
REDACTED

func queryAffiliateByUserID(ctx context.Context, client affiliateQueryExecer, userID int64) (*service.AffiliateSummary, error) {
	rows, err := client.QueryContext(ctx, `
SELECT user_id,
       aff_code,
       inviter_id,
       aff_count,
       aff_quota::double precision,
       aff_history_quota::double precision,
       created_at,
       updated_at
FROM user_affiliates
WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
	REDACTED
		return nil, service.ErrAffiliateProfileNotFound
REDACTED

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffHistoryQuota,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
REDACTED
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
REDACTED
	return &out, nil
REDACTED

func queryAffiliateByCode(ctx context.Context, client affiliateQueryExecer, code string) (*service.AffiliateSummary, error) {
	rows, err := client.QueryContext(ctx, `
SELECT user_id,
       aff_code,
       inviter_id,
       aff_count,
       aff_quota::double precision,
       aff_history_quota::double precision,
       created_at,
       updated_at
FROM user_affiliates
WHERE aff_code = $1
LIMIT 1`, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
	REDACTED
		return nil, service.ErrAffiliateProfileNotFound
REDACTED

	var out service.AffiliateSummary
	var inviterID sql.NullInt64
	if err := rows.Scan(
		&out.UserID,
		&out.AffCode,
		&inviterID,
		&out.AffCount,
		&out.AffQuota,
		&out.AffHistoryQuota,
		&out.CreatedAt,
		&out.UpdatedAt,
	); err != nil {
		return nil, err
REDACTED
	if inviterID.Valid {
		out.InviterID = &inviterID.Int64
REDACTED
	return &out, nil
REDACTED

func queryUserBalance(ctx context.Context, client affiliateQueryExecer, userID int64) (float64, error) {
	rows, err := client.QueryContext(ctx,
		"SELECT balance::double precision FROM users WHERE id = $1 LIMIT 1",
		userID,
	)
	if err != nil {
		return 0, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
	REDACTED
		return 0, service.ErrUserNotFound
REDACTED
	var balance float64
	if err := rows.Scan(&balance); err != nil {
		return 0, err
REDACTED
	return balance, nil
REDACTED

func generateAffiliateCode() (string, error) {
	buf := make([]byte, affiliateCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate affiliate code: %w", err)
REDACTED
	for i := range buf {
		buf[i] = affiliateCodeCharset[int(buf[i])%len(affiliateCodeCharset)]
REDACTED
	return string(buf), nil
REDACTED

func isAffiliateUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "23505"
REDACTED
	return false
REDACTED
