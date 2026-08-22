package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/accountid"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type organizationRepository struct{ db *sql.DB }

func organizationCorrelationID(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	if value, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(value) != "" {
		return nullableString(value)
	}
	value, _ := ctx.Value(ctxkey.RequestID).(string)
	return nullableString(value)
}

func organizationExplicitCorrelationID(ctx context.Context, value string) any {
	if strings.TrimSpace(value) != "" {
		return nullableString(value)
	}
	return organizationCorrelationID(ctx)
}

// organizationNotesContainLine reports whether the multi-line notes field
// already contains an exact line, used to make paid-order subscription
// fulfillment idempotent (mirrors the payment order note convention).
func organizationNotesContainLine(notes, line string) bool {
	target := strings.TrimSpace(line)
	if target == "" {
		return false
	}
	for _, l := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) == target {
			return true
		}
	}
	return false
}

func NewOrganizationRepository(db *sql.DB) service.OrganizationRepository {
	return &organizationRepository{db: db}
}

func (r *organizationRepository) GetContextForUser(ctx context.Context, userID int64) (*service.OrganizationContext, error) {
	const query = `
		SELECT o.id, o.account_id, COALESCE(o.company_id, ''), o.owner_user_id, o.name, o.status,
		       m.id, m.role, m.status, m.authz_generation, o.effective_at,
		       COALESCE(array_agg(DISTINCT p.policy_key) FILTER (WHERE a.detached_at IS NULL AND p.id IS NOT NULL), '{}'),
		       COALESCE(array_agg(DISTINCT pa.action) FILTER (WHERE a.detached_at IS NULL AND pa.id IS NOT NULL), '{}')
		FROM organization_memberships m
		JOIN organizations o ON o.id = m.organization_id
		LEFT JOIN member_policy_attachments a ON a.membership_id = m.id AND a.detached_at IS NULL
		LEFT JOIN managed_policies p ON p.id = a.policy_id AND p.version = a.policy_version
		LEFT JOIN managed_policy_actions pa ON pa.policy_id = p.id
		WHERE m.user_id = $1
		GROUP BY o.id, m.id`
	var out service.OrganizationContext
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&out.OrganizationID, &out.AccountID, &out.CompanyID, &out.OwnerUserID, &out.CompanyName, &out.OrganizationStatus,
		&out.MembershipID, &out.Role, &out.MembershipStatus, &out.AuthzGeneration, &out.EffectiveAt,
		pq.Array(&out.PolicyNames), pq.Array(&out.Actions),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrCompanyNotFound
	}
	return &out, err
}

func scanCompanyApplication(row interface{ Scan(...any) error }) (*service.CompanyApplication, error) {
	var app service.CompanyApplication
	var fee string
	err := row.Scan(&app.ID, &app.ApplicantUserID, &app.ApplicantEmail, &app.RequestedName, &app.CompanySize, &app.Status,
		&fee, &app.FeeCurrency, &app.ReviewerUserID, &app.ReviewReason, &app.OrganizationID, &app.CreatedAt, &app.DecidedAt)
	if err != nil {
		return nil, err
	}
	app.FeeAmount = fee
	app.SimilarNames = []string{}
	return &app, nil
}

const applicationSelect = `
	SELECT a.id, a.applicant_user_id, COALESCE(u.email, ''), a.requested_name, COALESCE(a.company_size, ''), a.status,
	       a.fee_amount::text, a.fee_currency, a.reviewer_user_id, COALESCE(a.review_reason, ''),
	       a.organization_id, a.created_at, a.decided_at
	FROM company_upgrade_applications a JOIN users u ON u.id = a.applicant_user_id`

func (r *organizationRepository) GetApplicationForUser(ctx context.Context, userID int64) (*service.CompanyApplication, error) {
	app, err := scanCompanyApplication(r.db.QueryRowContext(ctx, applicationSelect+` WHERE a.applicant_user_id=$1 ORDER BY a.id DESC LIMIT 1`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrApplicationNotFound
	}
	return app, err
}

func (r *organizationRepository) getSimilarNames(ctx context.Context, requestedName string, excludeApplicationID *int64) []string {
	rows, err := r.db.QueryContext(ctx, `
		SELECT candidate FROM (
			SELECT name AS candidate, normalized_name, NULL::bigint AS application_id FROM organizations
			UNION ALL
			SELECT requested_name, normalized_name, id FROM company_upgrade_applications WHERE status='pending'
		) names WHERE (normalized_name % lower($1) OR normalized_name=lower($1))
		  AND ($2::bigint IS NULL OR application_id IS DISTINCT FROM $2)
		ORDER BY similarity(normalized_name,lower($1)) DESC LIMIT 5`, requestedName, excludeApplicationID)
	if err != nil {
		return []string{}
	}
	defer func() { _ = rows.Close() }()
	out := make([]string, 0)
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			out = append(out, name)
		}
	}
	return out
}

func (r *organizationRepository) GetApplication(ctx context.Context, applicationID int64) (*service.CompanyApplicationDetail, error) {
	app, err := scanCompanyApplication(r.db.QueryRowContext(ctx, applicationSelect+` WHERE a.id=$1`, applicationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrApplicationNotFound
	}
	if err != nil {
		return nil, err
	}
	app.SimilarNames = r.getSimilarNames(ctx, app.RequestedName, &applicationID)
	rows, err := r.db.QueryContext(ctx, `SELECT id,actor_user_id,subject_user_id,action,result,COALESCE(correlation_id,''),metadata,created_at FROM organization_audit_events WHERE metadata->>'application_id'=$1 ORDER BY id`, strconv.FormatInt(applicationID, 10))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	audit := make([]service.OrganizationAuditEvent, 0)
	for rows.Next() {
		var event service.OrganizationAuditEvent
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.ActorUserID, &event.SubjectUserID, &event.Action, &event.Result, &event.CorrelationID, &metadata, &event.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
			return nil, err
		}
		audit = append(audit, event)
	}
	return &service.CompanyApplicationDetail{Application: *app, Audit: audit}, rows.Err()
}

func enqueueNotification(ctx context.Context, tx *sql.Tx, event, dedup, recipient string, variables map[string]string) error {
	if strings.TrimSpace(recipient) == "" {
		return nil
	}
	keys := make([]string, 0, len(variables))
	values := make([]string, 0, len(variables))
	for key, value := range variables {
		keys = append(keys, key)
		values = append(values, value)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO notification_outbox(dedup_key,event,recipient,variables)
		VALUES($1,$2,$3,jsonb_object($4::text[],$5::text[])) ON CONFLICT(dedup_key) DO NOTHING`,
		dedup, event, recipient, pq.Array(keys), pq.Array(values))
	return err
}

func enqueueAdminNotifications(ctx context.Context, tx *sql.Tx, event string, applicationID int64, variables map[string]string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, email FROM users WHERE role='admin' AND status='active' AND deleted_at IS NULL AND email IS NOT NULL ORDER BY id`)
	if err != nil {
		return err
	}
	type recipient struct {
		id    int64
		email string
	}
	recipients := make([]recipient, 0)
	for rows.Next() {
		var item recipient
		if err := rows.Scan(&item.id, &item.email); err != nil {
			_ = rows.Close()
			return err
		}
		recipients = append(recipients, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range recipients {
		if err := enqueueNotification(ctx, tx, event, fmt.Sprintf("%s:%d:%d", event, applicationID, item.id), item.email, variables); err != nil {
			return err
		}
	}
	return nil
}

func (r *organizationRepository) SubmitApplication(ctx context.Context, userID int64, name, normalizedName, companySize, idempotencyKey, fee, currency string) (_ *service.CompanyApplication, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if existing, replayErr := scanCompanyApplication(tx.QueryRowContext(ctx, applicationSelect+` WHERE a.applicant_user_id=$1 AND a.idempotency_key=$2`, userID, idempotencyKey)); replayErr == nil {
		return existing, nil
	} else if !errors.Is(replayErr, sql.ErrNoRows) {
		return nil, replayErr
	}
	var identity, status, accountID string
	var membershipCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT u.identity_type,u.status,COALESCE(u.account_id,''),
		       (SELECT count(*) FROM organization_memberships m WHERE m.user_id=u.id)
		FROM users u WHERE u.id=$1 AND u.deleted_at IS NULL FOR UPDATE`, userID).
		Scan(&identity, &status, &accountID, &membershipCount); err != nil {
		return nil, service.ErrCompanyNotEligible
	}
	if identity != service.IdentityTypeRoot || status != service.StatusActive || accountID == "" || membershipCount != 0 {
		return nil, service.ErrCompanyNotEligible
	}
	var applicationID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO company_upgrade_applications(applicant_user_id,requested_name,normalized_name,company_size,fee_amount,fee_currency,idempotency_key)
		VALUES($1,$2,$3,$4,$5::numeric,$6,$7) RETURNING id`, userID, name, normalizedName, companySize, fee, currency, idempotencyKey).Scan(&applicationID); err != nil {
		if isConstraintNamed(err, "company_upgrade_one_pending_per_user") {
			return nil, service.ErrCompanyPending
		}
		return nil, err
	}
	var availableAfter, frozenAfter string
	if err := tx.QueryRowContext(ctx, `
		UPDATE users SET balance=balance-$2::numeric,frozen_balance=frozen_balance+$2::numeric,updated_at=NOW()
		WHERE id=$1 AND balance >= $2::numeric RETURNING balance::text,frozen_balance::text`, userID, fee).
		Scan(&availableAfter, &frozenAfter); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrInsufficientBalance
	} else if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO organization_financial_ledger(idempotency_key,kind,application_id,actor_user_id,source_user_id,amount,currency,source_balance_after)
		VALUES($1,'upgrade_reserve',$2,$3,$3,$4::numeric,$5,$6::numeric)`, fmt.Sprintf("upgrade:reserve:%d:%s", userID, idempotencyKey), applicationID, userID, fee, currency, availableAfter); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$1,'company.application.submit','success',$2,jsonb_build_object('application_id',$3::bigint,'company_name',$4::text))`, userID, organizationCorrelationID(ctx), applicationID, name); err != nil {
		return nil, err
	}
	if err := enqueueAdminNotifications(ctx, tx, service.NotificationEmailEventCompanyUpgradeSubmitted, applicationID, map[string]string{"company_name": name, "fee": fee, "currency": currency}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetApplicationForUser(ctx, userID)
}

func (r *organizationRepository) WithdrawApplication(ctx context.Context, userID, applicationID int64) (_ *service.CompanyApplication, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var status, fee, currency, email, requestedName string
	if err := tx.QueryRowContext(ctx, `
		SELECT a.status,a.fee_amount::text,a.fee_currency,COALESCE(u.email,''),a.requested_name
		FROM company_upgrade_applications a JOIN users u ON u.id=a.applicant_user_id
		WHERE a.id=$1 AND a.applicant_user_id=$2 FOR UPDATE`, applicationID, userID).Scan(&status, &fee, &currency, &email, &requestedName); err != nil {
		return nil, service.ErrApplicationNotFound
	}
	if status == "withdrawn" {
		_ = tx.Rollback()
		return r.GetApplicationForUser(ctx, userID)
	}
	if status != "pending" {
		return nil, service.ErrApplicationTerminal
	}
	res, err := tx.ExecContext(ctx, `UPDATE users SET frozen_balance=frozen_balance-$2::numeric,balance=balance+$2::numeric,updated_at=NOW() WHERE id=$1 AND frozen_balance >= $2::numeric`, userID, fee)
	if err != nil {
		return nil, err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return nil, service.ErrInsufficientBalance
	}
	if _, err := tx.ExecContext(ctx, `UPDATE company_upgrade_applications SET status='withdrawn',decided_at=NOW(),updated_at=NOW() WHERE id=$1`, applicationID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_financial_ledger(idempotency_key,kind,application_id,actor_user_id,destination_user_id,amount,currency) VALUES($1,'upgrade_release',$2,$3,$3,$4::numeric,$5) ON CONFLICT DO NOTHING`, fmt.Sprintf("upgrade:withdraw:%d", applicationID), applicationID, userID, fee, currency); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$1,'company.application.withdraw','success',$2,jsonb_build_object('application_id',$3::bigint))`, userID, organizationCorrelationID(ctx), applicationID); err != nil {
		return nil, err
	}
	if err := enqueueNotification(ctx, tx, service.NotificationEmailEventCompanyUpgradeWithdrawn, fmt.Sprintf("%s:%d:%d", service.NotificationEmailEventCompanyUpgradeWithdrawn, applicationID, userID), email, map[string]string{"company_name": requestedName, "fee": fee, "currency": currency}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetApplicationForUser(ctx, userID)
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func (r *organizationRepository) ListApplications(ctx context.Context, status string, page, pageSize int) ([]service.CompanyApplication, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	where := ""
	args := []any{}
	if strings.TrimSpace(status) != "" {
		where = " WHERE a.status=$1"
		args = append(args, strings.TrimSpace(status))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM company_upgrade_applications a`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	limitPos := len(args) - 1
	rows, err := r.db.QueryContext(ctx, applicationSelect+where+fmt.Sprintf(" ORDER BY a.id DESC LIMIT $%d OFFSET $%d", limitPos, limitPos+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.CompanyApplication, 0)
	for rows.Next() {
		app, err := scanCompanyApplication(rows)
		if err != nil {
			return nil, 0, err
		}
		app.SimilarNames = r.getSimilarNames(ctx, app.RequestedName, &app.ID)
		out = append(out, *app)
	}
	return out, total, rows.Err()
}

func scanNameChangeRequest(row interface{ Scan(...any) error }) (*service.OrganizationNameChangeRequest, error) {
	var request service.OrganizationNameChangeRequest
	err := row.Scan(&request.ID, &request.OrganizationID, &request.ApplicantUserID, &request.CompanyName,
		&request.OldName, &request.NewName, &request.Status, &request.ReviewerUserID,
		&request.ReviewReason, &request.CreatedAt, &request.DecidedAt)
	request.SimilarNames = []string{}
	return &request, err
}

const nameChangeSelect = `
	SELECT n.id,n.organization_id,n.applicant_user_id,o.name,n.old_name,n.new_name,n.status,
	       n.reviewer_user_id,COALESCE(n.review_reason,''),n.created_at,n.decided_at
	FROM organization_name_change_requests n JOIN organizations o ON o.id=n.organization_id`

func (r *organizationRepository) ListNameChangeRequests(ctx context.Context, status string, page, pageSize int) ([]service.OrganizationNameChangeRequest, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	where, args := "", []any{}
	if strings.TrimSpace(status) != "" {
		where = " WHERE n.status=$1"
		args = append(args, strings.TrimSpace(status))
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM organization_name_change_requests n`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, nameChangeSelect+where+fmt.Sprintf(" ORDER BY n.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.OrganizationNameChangeRequest, 0)
	for rows.Next() {
		request, err := scanNameChangeRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		request.SimilarNames = r.getSimilarNames(ctx, request.NewName, nil)
		out = append(out, *request)
	}
	return out, total, rows.Err()
}

func (r *organizationRepository) GetNameChangeRequest(ctx context.Context, requestID int64) (*service.OrganizationNameChangeRequest, error) {
	request, err := scanNameChangeRequest(r.db.QueryRowContext(ctx, nameChangeSelect+` WHERE n.id=$1`, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrApplicationNotFound
	}
	if err != nil {
		return nil, err
	}
	request.SimilarNames = r.getSimilarNames(ctx, request.NewName, nil)
	return request, nil
}

const adminOrganizationSelect = `SELECT o.id,o.account_id,COALESCE(o.company_id,''),o.name,o.status,o.owner_user_id,COALESCE(u.email,''),
	(SELECT count(*) FROM organization_memberships m WHERE m.organization_id=o.id AND m.role='member' AND m.status<>'archived'),
	o.member_limit,o.effective_at,o.created_at FROM organizations o JOIN users u ON u.id=o.owner_user_id`

type organizationQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// resolveOrganizationForActor 查找 actorID 所属的 active 组织；actor 可以是 owner，
// 或者是持有指定 action 的 active member。SQL 中 lockOwnerRow 为 true 时会对组织行
// 加 FOR UPDATE OF o 行锁（用于成员创建等敏感事务）。
func resolveOrganizationForActor(ctx context.Context, db organizationQueryRower, actorID int64, action string, lockOwnerRow bool) (int64, error) {
	// Owner 分支：任何 owner action 都允许。
	suffix := ""
	if lockOwnerRow {
		suffix = " FOR UPDATE OF o"
	}
	var orgID int64
	if err := db.QueryRowContext(ctx, `SELECT o.id FROM organizations o JOIN organization_memberships m ON m.organization_id=o.id WHERE m.user_id=$1 AND m.role='owner' AND m.status='active' AND o.status='active'`+suffix, actorID).Scan(&orgID); err == nil {
		return orgID, nil
	}
	// Member 分支：需要持有指定 action。
	if action == "" {
		return 0, service.ErrOrganizationPermission
	}
	err := db.QueryRowContext(ctx, `SELECT o.id
		FROM organizations o
		JOIN organization_memberships m ON m.organization_id=o.id
		JOIN member_policy_attachments a ON a.membership_id=m.id AND a.detached_at IS NULL
		JOIN managed_policies p ON p.id=a.policy_id AND p.version=a.policy_version
		JOIN managed_policy_actions pa ON pa.policy_id=p.id
		WHERE m.user_id=$1 AND m.role='member' AND m.status='active' AND o.status='active' AND pa.action=$2
		LIMIT 1`+suffix, actorID, action).Scan(&orgID)
	if err != nil {
		return 0, service.ErrOrganizationPermission
	}
	return orgID, nil
}

func requireActiveAdminDB(ctx context.Context, db organizationQueryRower, actorID int64) error {
	var allowed bool
	if err := db.QueryRowContext(ctx, `SELECT role='admin' AND status='active' FROM users WHERE id=$1 AND deleted_at IS NULL`, actorID).Scan(&allowed); err != nil || !allowed {
		return service.ErrInsufficientPerms
	}
	return nil
}

func scanAdminOrganization(scanner interface{ Scan(...any) error }) (*service.AdminOrganization, error) {
	var organization service.AdminOrganization
	if err := scanner.Scan(&organization.ID, &organization.AccountID, &organization.CompanyID, &organization.Name, &organization.Status,
		&organization.OwnerUserID, &organization.OwnerEmail, &organization.MemberCount, &organization.MemberLimit,
		&organization.EffectiveAt, &organization.CreatedAt); err != nil {
		return nil, err
	}
	return &organization, nil
}

func (r *organizationRepository) ListOrganizations(ctx context.Context, actorID int64, status string, page, pageSize int) ([]service.AdminOrganization, int64, error) {
	if err := requireActiveAdminDB(ctx, r.db, actorID); err != nil {
		return nil, 0, err
	}
	page, pageSize = normalizePage(page, pageSize)
	where := ""
	args := []any{}
	if status != "" {
		if status != service.OrganizationStatusActive && status != service.OrganizationStatusSuspended {
			return nil, 0, infraerrors.BadRequest("ORGANIZATION_STATUS_INVALID", "organization status is invalid")
		}
		where = " WHERE o.status=$1"
		args = append(args, status)
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM organizations o`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, adminOrganizationSelect+where+fmt.Sprintf(" ORDER BY o.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.AdminOrganization, 0)
	for rows.Next() {
		organization, err := scanAdminOrganization(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *organization)
	}
	return items, total, rows.Err()
}

func (r *organizationRepository) GetOrganization(ctx context.Context, actorID, organizationID int64) (*service.AdminOrganizationDetail, error) {
	if err := requireActiveAdminDB(ctx, r.db, actorID); err != nil {
		return nil, err
	}
	organization, err := scanAdminOrganization(r.db.QueryRowContext(ctx, adminOrganizationSelect+` WHERE o.id=$1`, organizationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrCompanyNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,actor_user_id,subject_user_id,action,result,COALESCE(correlation_id,''),metadata,created_at FROM organization_audit_events WHERE organization_id=$1 ORDER BY id DESC LIMIT 200`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	audit := make([]service.OrganizationAuditEvent, 0)
	for rows.Next() {
		var event service.OrganizationAuditEvent
		var actorID, subjectID sql.NullInt64
		var raw []byte
		if err := rows.Scan(&event.ID, &actorID, &subjectID, &event.Action, &event.Result, &event.CorrelationID, &raw, &event.CreatedAt); err != nil {
			return nil, err
		}
		if actorID.Valid {
			event.ActorUserID = &actorID.Int64
		}
		if subjectID.Valid {
			event.SubjectUserID = &subjectID.Int64
		}
		_ = json.Unmarshal(raw, &event.Metadata)
		audit = append(audit, event)
	}
	return &service.AdminOrganizationDetail{Organization: *organization, Audit: audit}, rows.Err()
}

func requireActiveAdmin(ctx context.Context, tx *sql.Tx, userID int64) error {
	var ok bool
	if err := tx.QueryRowContext(ctx, `SELECT role='admin' AND status='active' FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&ok); err != nil || !ok {
		return service.ErrInsufficientPerms
	}
	return nil
}

func (r *organizationRepository) DecideApplication(ctx context.Context, reviewerID, applicationID int64, approve bool, reason string, memberLimit int) (_ *service.CompanyApplication, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveAdmin(ctx, tx, reviewerID); err != nil {
		return nil, err
	}
	var applicantID int64
	var status, fee, currency, requestedName, normalizedName, companySize, accountID, email string
	if err := tx.QueryRowContext(ctx, `
		SELECT a.applicant_user_id,a.status,a.fee_amount::text,a.fee_currency,a.requested_name,a.normalized_name,COALESCE(a.company_size,''),u.account_id,COALESCE(u.email,'')
		FROM company_upgrade_applications a JOIN users u ON u.id=a.applicant_user_id
		WHERE a.id=$1 FOR UPDATE`, applicationID).Scan(&applicantID, &status, &fee, &currency, &requestedName, &normalizedName, &companySize, &accountID, &email); err != nil {
		return nil, service.ErrApplicationNotFound
	}
	if status != "pending" {
		return nil, service.ErrApplicationTerminal
	}
	now := time.Now().UTC()
	var organizationID *int64
	event := service.NotificationEmailEventCompanyUpgradeRejected
	if approve {
		var orgID int64
		var companyID string
		// Companies get their own public identifier (a 'c' prefix followed by 15
		// digits) generated independently from the numeric account_id, which the
		// organization still shares with its IAM members. Retry on the unlikely
		// collision against the unique index.
		for attempt := 0; attempt < 20; attempt++ {
			companyID, err = accountid.GenerateCompany()
			if err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `SAVEPOINT org_company_id_retry`); err != nil {
				return nil, err
			}
			err = tx.QueryRowContext(ctx, `INSERT INTO organizations(account_id,company_id,owner_user_id,name,normalized_name,company_size,status,member_limit,effective_at) VALUES($1,$2,$3,$4,$5,$6,'active',$7,$8) RETURNING id`, accountID, companyID, applicantID, requestedName, normalizedName, nullableString(companySize), memberLimit, now).Scan(&orgID)
			if err == nil {
				_, err = tx.ExecContext(ctx, `RELEASE SAVEPOINT org_company_id_retry`)
				break
			}
			insertErr := err
			if _, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT org_company_id_retry`); rollbackErr != nil {
				return nil, fmt.Errorf("rollback company ID retry savepoint: %w", rollbackErr)
			}
			if _, releaseErr := tx.ExecContext(ctx, `RELEASE SAVEPOINT org_company_id_retry`); releaseErr != nil {
				return nil, fmt.Errorf("release company ID retry savepoint: %w", releaseErr)
			}
			if !isConstraintNamed(insertErr, "organizations_company_id_unique") {
				return nil, insertErr
			}
			accountid.RecordCollisionRetry()
			err = insertErr
		}
		if err != nil {
			return nil, fmt.Errorf("company ID collision retry limit exhausted: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,status) VALUES($1,$2,'owner','active')`, orgID, applicantID); err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx, `UPDATE users SET frozen_balance=frozen_balance-$2::numeric,updated_at=NOW() WHERE id=$1 AND frozen_balance >= $2::numeric`, applicantID, fee)
		if err != nil {
			return nil, err
		}
		if affected, _ := res.RowsAffected(); affected != 1 {
			return nil, service.ErrInsufficientBalance
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO organization_financial_ledger(idempotency_key,kind,organization_id,application_id,actor_user_id,source_user_id,amount,currency) VALUES($1,'upgrade_capture',$2,$3,$4,$5,$6::numeric,$7)`, fmt.Sprintf("upgrade:approve:%d", applicationID), orgID, applicationID, reviewerID, applicantID, fee, currency); err != nil {
			return nil, err
		}
		organizationID = &orgID
		event = service.NotificationEmailEventCompanyUpgradeApproved
	} else {
		if strings.TrimSpace(reason) == "" {
			return nil, service.ErrReasonRequired
		}
		res, err := tx.ExecContext(ctx, `UPDATE users SET frozen_balance=frozen_balance-$2::numeric,balance=balance+$2::numeric,updated_at=NOW() WHERE id=$1 AND frozen_balance >= $2::numeric`, applicantID, fee)
		if err != nil {
			return nil, err
		}
		if affected, _ := res.RowsAffected(); affected != 1 {
			return nil, service.ErrInsufficientBalance
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO organization_financial_ledger(idempotency_key,kind,application_id,actor_user_id,destination_user_id,amount,currency) VALUES($1,'upgrade_release',$2,$3,$4,$5::numeric,$6)`, fmt.Sprintf("upgrade:reject:%d", applicationID), applicationID, reviewerID, applicantID, fee, currency); err != nil {
			return nil, err
		}
	}
	decision := "rejected"
	if approve {
		decision = "approved"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE company_upgrade_applications SET status=$2,reviewer_user_id=$3,review_reason=$4,organization_id=$5,decided_at=$6,updated_at=$6 WHERE id=$1`, applicationID, decision, reviewerID, nullableString(reason), organizationID, now); err != nil {
		return nil, err
	}
	if err := enqueueNotification(ctx, tx, event, fmt.Sprintf("%s:%d:%d", event, applicationID, applicantID), email, map[string]string{"company_name": requestedName, "reason": reason, "fee": fee, "currency": currency}); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,'company.application.review','success',$4,jsonb_build_object('application_id',$5::bigint,'decision',$6::text))`, organizationID, reviewerID, applicantID, organizationCorrelationID(ctx), applicationID, decision); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return scanCompanyApplication(r.db.QueryRowContext(ctx, applicationSelect+` WHERE a.id=$1`, applicationID))
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (r *organizationRepository) RequestNameChange(ctx context.Context, userID int64, name, normalizedName string) error {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() || !org.Owner() {
		return service.ErrOrganizationPermission
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var requestID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO organization_name_change_requests(organization_id,applicant_user_id,old_name,new_name,normalized_name) VALUES($1,$2,$3,$4,$5) RETURNING id`, org.OrganizationID, userID, org.CompanyName, name, normalizedName).Scan(&requestID); err != nil {
		return err
	}
	if err := enqueueAdminNotifications(ctx, tx, service.NotificationEmailEventCompanyNameSubmitted, requestID, map[string]string{"company_name": name}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'company.name.request','success',$3,jsonb_build_object('request_id',$4::bigint,'new_name',$5::text))`, org.OrganizationID, userID, organizationCorrelationID(ctx), requestID, name); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) DecideNameChange(ctx context.Context, reviewerID, requestID int64, approve bool, reason string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveAdmin(ctx, tx, reviewerID); err != nil {
		return err
	}
	var orgID, applicantID int64
	var status, newName, normalized, email string
	if err := tx.QueryRowContext(ctx, `SELECT n.organization_id,n.applicant_user_id,n.status,n.new_name,n.normalized_name,COALESCE(u.email,'') FROM organization_name_change_requests n JOIN users u ON u.id=n.applicant_user_id WHERE n.id=$1 FOR UPDATE`, requestID).Scan(&orgID, &applicantID, &status, &newName, &normalized, &email); err != nil {
		return service.ErrApplicationNotFound
	}
	if status != "pending" {
		return service.ErrApplicationTerminal
	}
	decision, event := "rejected", service.NotificationEmailEventCompanyNameRejected
	if approve {
		decision, event = "approved", service.NotificationEmailEventCompanyNameApproved
		if _, err := tx.ExecContext(ctx, `UPDATE organizations SET name=$2,normalized_name=$3,updated_at=NOW() WHERE id=$1`, orgID, newName, normalized); err != nil {
			return err
		}
	} else if strings.TrimSpace(reason) == "" {
		return service.ErrReasonRequired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organization_name_change_requests SET status=$2,reviewer_user_id=$3,review_reason=$4,decided_at=NOW(),updated_at=NOW() WHERE id=$1`, requestID, decision, reviewerID, nullableString(reason)); err != nil {
		return err
	}
	if err := enqueueNotification(ctx, tx, event, fmt.Sprintf("%s:%d:%d", event, requestID, applicantID), email, map[string]string{"company_name": newName, "reason": reason}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,'company.name.review','success',$4,jsonb_build_object('request_id',$5::bigint,'decision',$6::text))`, orgID, reviewerID, applicantID, organizationCorrelationID(ctx), requestID, decision); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) SetOrganizationStatus(ctx context.Context, actorID, organizationID int64, status string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveAdmin(ctx, tx, actorID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE organizations SET status=$2,updated_at=NOW() WHERE id=$1`, organizationID, status)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return service.ErrCompanyNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organization_memberships SET authz_generation=authz_generation+1,updated_at=NOW() WHERE organization_id=$1`, organizationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET authz_generation=authz_generation+1,updated_at=NOW() WHERE id IN (SELECT user_id FROM organization_memberships WHERE organization_id=$1)`, organizationID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'organization.status','success',$3,jsonb_build_object('status',$4::text))`, organizationID, actorID, organizationCorrelationID(ctx), status)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) CreateIAMMember(ctx context.Context, ownerID int64, user *service.User, memberLimit int) (_ *service.IAMMember, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	orgID, err := resolveOrganizationForActor(ctx, tx, ownerID, service.ActionIAMMemberManage, true)
	if err != nil {
		return nil, err
	}
	var companyID string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(o.company_id,'') FROM organizations o WHERE o.id=$1`, orgID).Scan(&companyID); err != nil {
		return nil, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1 AND role='member' AND status<>'archived'`, orgID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= memberLimit {
		return nil, service.ErrIAMMemberLimit
	}
	var loginNameTaken bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM organization_memberships m
			JOIN users existing ON existing.id=m.user_id
			WHERE m.organization_id=$1
			  AND m.role='member'
			  AND m.status<>'archived'
			  AND existing.identity_type='iam'
			  AND existing.deleted_at IS NULL
			  AND lower(existing.login_name)=lower($2)
		)`, orgID, user.LoginName).Scan(&loginNameTaken); err != nil {
		return nil, err
	}
	if loginNameTaken {
		return nil, service.ErrIAMLoginName
	}
	var userID int64
	var memberAccountID string
	for attempt := 0; attempt < 20; attempt++ {
		accountID, generateErr := accountid.GenerateRoot()
		if generateErr != nil {
			return nil, generateErr
		}
		if _, err = tx.ExecContext(ctx, `SAVEPOINT iam_account_id_retry`); err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `
			INSERT INTO users(account_id,identity_type,login_name,username,password_hash,role,balance,frozen_balance,concurrency,status,signup_source,must_change_password,recovery_email,authz_generation,created_at,updated_at)
			VALUES($1,'iam',$2,$3,$4,'user',0,0,5,'active','email',$5,$6,1,NOW(),NOW()) RETURNING id`,
			accountID, user.LoginName, user.Username, user.PasswordHash, user.MustChangePassword, nullableString(user.RecoveryEmail)).Scan(&userID)
		if err == nil {
			memberAccountID = accountID
			_, err = tx.ExecContext(ctx, `RELEASE SAVEPOINT iam_account_id_retry`)
			break
		}
		insertErr := err
		if _, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT iam_account_id_retry`); rollbackErr != nil {
			return nil, fmt.Errorf("rollback IAM account ID retry savepoint: %w", rollbackErr)
		}
		if _, releaseErr := tx.ExecContext(ctx, `RELEASE SAVEPOINT iam_account_id_retry`); releaseErr != nil {
			return nil, fmt.Errorf("release IAM account ID retry savepoint: %w", releaseErr)
		}
		if isConstraintNamed(insertErr, "users_account_id_unique") {
			accountid.RecordCollisionRetry()
			err = insertErr
			continue
		}
		if isConstraintNamed(insertErr, "users_iam_login_unique_active") {
			return nil, service.ErrIAMLoginName
		}
		return nil, insertErr
	}
	if err != nil {
		return nil, fmt.Errorf("IAM account ID collision retry limit exhausted: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,status) VALUES($1,$2,'member','active')`, orgID, userID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id) VALUES($1,$2,$3,'iam.member.create','success',$4)`, orgID, ownerID, userID, organizationCorrelationID(ctx)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.IAMMember{UserID: userID, AccountID: memberAccountID, Username: user.Username, LoginName: user.LoginName, Principal: service.CanonicalIAMPrincipal(user.LoginName, companyID), Status: "active", Balance: "0", FrozenBalance: "0", RecoveryEmail: user.RecoveryEmail, MustChangePassword: user.MustChangePassword, PolicyNames: []string{}, CreatedAt: time.Now().UTC()}, nil
}

func (r *organizationRepository) ListIAMMembers(ctx context.Context, actorID int64) ([]service.IAMMember, int, error) {
	org, err := r.GetContextForUser(ctx, actorID)
	if err != nil || !org.Active() {
		return nil, 0, service.ErrOrganizationPermission
	}
	// IAM 管理员或 owner 均可查看成员列表；普通成员只能看到自己。
	mayListAll := org.Owner() || org.HasAction(service.ActionIAMMemberManage)
	var memberLimit int
	if err := r.db.QueryRowContext(ctx, `SELECT member_limit FROM organizations WHERE id=$1`, org.OrganizationID).Scan(&memberLimit); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id,u.account_id,u.username,u.login_name,m.status,u.balance::text,u.frozen_balance::text,
		       COALESCE(u.recovery_email,''),u.recovery_email_verified_at,u.must_change_password,u.created_at,
		       COALESCE(array_agg(DISTINCT p.policy_key) FILTER(WHERE a.detached_at IS NULL AND p.id IS NOT NULL),'{}')
		FROM organization_memberships m JOIN users u ON u.id=m.user_id
		LEFT JOIN member_policy_attachments a ON a.membership_id=m.id AND a.detached_at IS NULL
		LEFT JOIN managed_policies p ON p.id=a.policy_id
		WHERE m.organization_id=$1 AND m.role='member' AND ($2 OR m.user_id=$3)
		GROUP BY u.id,m.status ORDER BY u.id`, org.OrganizationID, mayListAll, actorID)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	members := make([]service.IAMMember, 0)
	for rows.Next() {
		var member service.IAMMember
		if err := rows.Scan(&member.UserID, &member.AccountID, &member.Username, &member.LoginName, &member.Status, &member.Balance, &member.FrozenBalance, &member.RecoveryEmail, &member.RecoveryVerifiedAt, &member.MustChangePassword, &member.CreatedAt, pq.Array(&member.PolicyNames)); err != nil {
			return nil, 0, err
		}
		member.Principal = service.CanonicalIAMPrincipal(member.LoginName, org.CompanyID)
		members = append(members, member)
	}
	return members, memberLimit, rows.Err()
}

func (r *organizationRepository) GetIAMMember(ctx context.Context, actorID, memberUserID int64) (*service.IAMMember, error) {
	org, err := r.GetContextForUser(ctx, actorID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	if !org.Owner() && !org.HasAction(service.ActionIAMMemberManage) {
		return nil, service.ErrOrganizationPermission
	}
	var member service.IAMMember
	err = r.db.QueryRowContext(ctx, `
		SELECT u.id,u.account_id,u.username,u.login_name,m.status,u.balance::text,u.frozen_balance::text,
		       COALESCE(u.recovery_email,''),u.recovery_email_verified_at,u.must_change_password,u.created_at,
		       COALESCE(array_agg(DISTINCT p.policy_key) FILTER(WHERE a.detached_at IS NULL AND p.id IS NOT NULL),'{}')
		FROM organization_memberships m JOIN users u ON u.id=m.user_id
		LEFT JOIN member_policy_attachments a ON a.membership_id=m.id AND a.detached_at IS NULL
		LEFT JOIN managed_policies p ON p.id=a.policy_id
		WHERE m.organization_id=$1 AND m.user_id=$2 AND m.role='member'
		GROUP BY u.id,m.status`, org.OrganizationID, memberUserID).
		Scan(&member.UserID, &member.AccountID, &member.Username, &member.LoginName, &member.Status, &member.Balance,
			&member.FrozenBalance, &member.RecoveryEmail, &member.RecoveryVerifiedAt,
			&member.MustChangePassword, &member.CreatedAt, pq.Array(&member.PolicyNames))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrIAMMemberNotFound
	}
	if err != nil {
		return nil, err
	}
	member.Principal = service.CanonicalIAMPrincipal(member.LoginName, org.CompanyID)
	return &member, nil
}

func (r *organizationRepository) SetIAMMemberStatus(ctx context.Context, ownerID, memberUserID int64, status string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// 禁用 / 启用 / 归档 IAM 成员均属 owner 专属操作，IAM 管理员权限只覆盖创建与密码重置。
	var orgID int64
	if err := tx.QueryRowContext(ctx, `SELECT o.id FROM organizations o JOIN organization_memberships m ON m.organization_id=o.id WHERE m.user_id=$1 AND m.role='owner' AND m.status='active' AND o.status='active'`, ownerID).Scan(&orgID); err != nil {
		return service.ErrOrganizationPermission
	}
	res, err := tx.ExecContext(ctx, `UPDATE organization_memberships SET status=$3::text,archived_at=CASE WHEN $3::text='archived' THEN NOW() ELSE archived_at END,authz_generation=authz_generation+1,updated_at=NOW() WHERE organization_id=$1 AND user_id=$2 AND role='member' AND status<>'archived'`, orgID, memberUserID, status)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		if _, auditErr := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,'iam.member.status','denied',$4,jsonb_build_object('requested_status',$5::text))`, orgID, ownerID, memberUserID, organizationCorrelationID(ctx), status); auditErr != nil {
			return auditErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return commitErr
		}
		return service.ErrIAMMemberNotFound
	}
	userStatus := service.StatusActive
	if status != service.MembershipStatusActive {
		userStatus = "disabled"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET status=$2,deleted_at=CASE WHEN $3::text='archived' THEN NOW() ELSE deleted_at END,authz_generation=authz_generation+1,updated_at=NOW() WHERE id=$1`, memberUserID, userStatus, status); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,'iam.member.status','success',$4,jsonb_build_object('status',$5::text))`, orgID, ownerID, memberUserID, organizationCorrelationID(ctx), status); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) UpdateIAMPassword(ctx context.Context, actorID, memberUserID int64, passwordHash string, requireChange bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var organizationID int64
	if actorID != memberUserID {
		orgID, err := resolveOrganizationForActor(ctx, tx, actorID, service.ActionIAMMemberManage, false)
		if err != nil {
			return err
		}
		organizationID = orgID
		var belongs bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 AND role='member' AND status<>'archived')`, organizationID, memberUserID).Scan(&belongs); err != nil {
			return err
		} else if !belongs {
			if _, auditErr := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id) VALUES($1,$2,$3,'iam.member.password.reset','denied',$4)`, organizationID, actorID, memberUserID, organizationCorrelationID(ctx)); auditErr != nil {
				return auditErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return commitErr
			}
			return service.ErrIAMMemberNotFound
		}
	} else if err := tx.QueryRowContext(ctx, `SELECT organization_id FROM organization_memberships WHERE user_id=$1 AND role='member' AND status='active'`, memberUserID).Scan(&organizationID); err != nil {
		return service.ErrIAMMemberNotFound
	}
	res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=$2,must_change_password=$3,authz_generation=authz_generation+1,updated_at=NOW() WHERE id=$1 AND identity_type='iam' AND deleted_at IS NULL`, memberUserID, passwordHash, requireChange)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return service.ErrIAMMemberNotFound
	}
	action := "iam.member.password.change"
	if actorID != memberUserID {
		action = "iam.member.password.reset"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,$4,'success',$5,jsonb_build_object('requires_password_change',$6::boolean))`, organizationID, actorID, memberUserID, action, organizationCorrelationID(ctx), requireChange); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) FindIAMByPrincipal(ctx context.Context, loginName, companyID string) (*service.User, *service.OrganizationContext, error) {
	var user service.User
	err := r.db.QueryRowContext(ctx, `
		SELECT u.id,COALESCE(u.email,''),u.account_id,u.identity_type,u.login_name,u.password_hash,u.role,u.balance,u.frozen_balance,u.concurrency,u.status,u.must_change_password,COALESCE(u.recovery_email,''),u.recovery_email_verified_at,u.authz_generation,u.created_at,u.updated_at
		FROM users u
		JOIN organization_memberships m ON m.user_id=u.id AND m.role='member' AND m.status='active'
		JOIN organizations o ON o.id=m.organization_id
		WHERE o.company_id=$1 AND lower(u.login_name)=lower($2) AND u.identity_type='iam' AND u.deleted_at IS NULL`, companyID, loginName).
		Scan(&user.ID, &user.Email, &user.AccountID, &user.IdentityType, &user.LoginName, &user.PasswordHash, &user.Role, &user.Balance, &user.FrozenBalance, &user.Concurrency, &user.Status, &user.MustChangePassword, &user.RecoveryEmail, &user.RecoveryEmailVerifiedAt, &user.AuthzGeneration, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, service.ErrInvalidCredentials
	}
	if err != nil {
		return nil, nil, err
	}
	org, err := r.GetContextForUser(ctx, user.ID)
	if err == nil && org != nil {
		user.CompanyID = org.CompanyID
	}
	return &user, org, err
}

func (r *organizationRepository) ListPolicies(ctx context.Context, actorID int64) ([]service.ManagedPolicyView, error) {
	org, err := r.GetContextForUser(ctx, actorID)
	if err != nil || !org.Active() || !org.Owner() {
		return nil, service.ErrOrganizationPermission
	}
	rows, err := r.db.QueryContext(ctx, `SELECT p.id,p.policy_key,p.display_name,p.policy_type,p.description,p.version,COALESCE(array_agg(pa.action ORDER BY pa.action),'{}') FROM managed_policies p LEFT JOIN managed_policy_actions pa ON pa.policy_id=p.id GROUP BY p.id ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.ManagedPolicyView, 0)
	for rows.Next() {
		var policy service.ManagedPolicyView
		if err := rows.Scan(&policy.ID, &policy.Key, &policy.DisplayName, &policy.Type, &policy.Description, &policy.Version, pq.Array(&policy.Actions)); err != nil {
			return nil, err
		}
		out = append(out, policy)
	}
	return out, rows.Err()
}

func (r *organizationRepository) ListMemberPolicyAttachments(ctx context.Context, ownerID, memberUserID int64) ([]service.ManagedPolicyView, error) {
	org, err := r.GetContextForUser(ctx, ownerID)
	if err != nil || !org.Active() || !org.Owner() {
		return nil, service.ErrOrganizationPermission
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id,p.policy_key,p.display_name,p.policy_type,p.description,a.policy_version,
		       COALESCE(array_agg(pa.action ORDER BY pa.action),'{}')
		FROM organization_memberships m
		JOIN member_policy_attachments a ON a.membership_id=m.id AND a.detached_at IS NULL
		JOIN managed_policies p ON p.id=a.policy_id AND p.version=a.policy_version
		LEFT JOIN managed_policy_actions pa ON pa.policy_id=p.id
		WHERE m.organization_id=$1 AND m.user_id=$2 AND m.role='member'
		GROUP BY p.id,a.policy_version ORDER BY p.id`, org.OrganizationID, memberUserID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.ManagedPolicyView, 0)
	for rows.Next() {
		var policy service.ManagedPolicyView
		if err := rows.Scan(&policy.ID, &policy.Key, &policy.DisplayName, &policy.Type, &policy.Description, &policy.Version, pq.Array(&policy.Actions)); err != nil {
			return nil, err
		}
		out = append(out, policy)
	}
	if len(out) == 0 {
		var belongs bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 AND role='member')`, org.OrganizationID, memberUserID).Scan(&belongs); err != nil {
			return nil, err
		}
		if !belongs {
			return nil, service.ErrIAMMemberNotFound
		}
	}
	return out, rows.Err()
}

func (r *organizationRepository) SetPolicyAttachment(ctx context.Context, ownerID, memberUserID int64, policyKey string, attach bool, correlationID string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var orgID, membershipID, policyID int64
	var policyVersion int
	if err := tx.QueryRowContext(ctx, `SELECT o.id FROM organizations o JOIN organization_memberships m ON m.organization_id=o.id WHERE m.user_id=$1 AND m.role='owner' AND m.status='active' AND o.status='active'`, ownerID).Scan(&orgID); err != nil {
		return service.ErrOrganizationPermission
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 AND role='member' AND status<>'archived' FOR UPDATE`, orgID, memberUserID).Scan(&membershipID); err != nil {
		if _, auditErr := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id) VALUES($1,$2,$3,'iam.policy.change','denied',$4)`, orgID, ownerID, memberUserID, organizationExplicitCorrelationID(ctx, correlationID)); auditErr != nil {
			return auditErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return commitErr
		}
		return service.ErrIAMMemberNotFound
	}
	if err := tx.QueryRowContext(ctx, `SELECT id,version FROM managed_policies WHERE policy_key=$1 AND policy_type='system'`, policyKey).Scan(&policyID, &policyVersion); err != nil {
		return service.ErrOrganizationPermission
	}
	action := "detach"
	if attach {
		action = "attach"
		_, err = tx.ExecContext(ctx, `INSERT INTO member_policy_attachments(organization_id,membership_id,policy_id,policy_version,attached_by_user_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, orgID, membershipID, policyID, policyVersion, ownerID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE member_policy_attachments SET detached_at=NOW(),detached_by_user_id=$4,updated_at=NOW() WHERE organization_id=$1 AND membership_id=$2 AND policy_id=$3 AND detached_at IS NULL`, orgID, membershipID, policyID, ownerID)
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organization_memberships SET authz_generation=authz_generation+1,updated_at=NOW() WHERE id=$1`, membershipID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET authz_generation=authz_generation+1,updated_at=NOW() WHERE id=$1`, memberUserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,'iam.policy.change','success',$4,jsonb_build_object('operation',$5::text,'policy',$6::text))`, orgID, ownerID, memberUserID, organizationExplicitCorrelationID(ctx, correlationID), action, policyKey); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) TransferBalance(ctx context.Context, ownerID, memberUserID int64, amount, idempotencyKey string, reclaim bool) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	orgID, err := resolveOrganizationForActor(ctx, tx, ownerID, service.ActionBalanceAllocate, false)
	if err != nil {
		return err
	}
	var memberExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 AND role='member' AND status='active')`, orgID, memberUserID).Scan(&memberExists); err != nil || !memberExists {
		if err == nil {
			if _, auditErr := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,'organization.balance.transfer','denied',$4,jsonb_build_object('operation',$5::text))`, orgID, ownerID, memberUserID, organizationCorrelationID(ctx), map[bool]string{true: "reclaim", false: "allocate"}[reclaim]); auditErr != nil {
				return auditErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return commitErr
			}
		}
		return service.ErrIAMMemberNotFound
	}
	// 划拨/收回资金在「公司余额」(organizations.balance) 与成员个人余额之间流动：
	// allocate 从公司余额转入成员，reclaim 从成员转回公司余额。
	// 锁定顺序：先锁成员 users 行，再锁 organizations 行，保持与其他操作一致。
	if _, err := tx.ExecContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, memberUserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, orgID); err != nil {
		return err
	}
	kind := "allocate"
	if reclaim {
		kind = "reclaim"
	}
	ledgerKey := fmt.Sprintf("organization:transfer:%d:%s", orgID, idempotencyKey)
	var existingKind, existingAmount string
	var existingActor, existingSource, existingDestination int64
	replayErr := tx.QueryRowContext(ctx, `
		SELECT kind,actor_user_id,COALESCE(source_user_id,0),COALESCE(destination_user_id,0),amount::text
		FROM organization_financial_ledger WHERE idempotency_key=$1`, ledgerKey).
		Scan(&existingKind, &existingActor, &existingSource, &existingDestination, &existingAmount)
	if replayErr == nil {
		requestedAmount, parseErr := decimal.NewFromString(amount)
		persistedAmount, persistedParseErr := decimal.NewFromString(existingAmount)
		// 公司侧记为 NULL(0)。allocate 时成员为目的方，reclaim 时成员为来源方。
		expectedSource, expectedDestination := int64(0), memberUserID
		if reclaim {
			expectedSource, expectedDestination = memberUserID, 0
		}
		if parseErr != nil || persistedParseErr != nil || existingKind != kind || existingActor != ownerID ||
			existingSource != expectedSource || existingDestination != expectedDestination || !requestedAmount.Equal(persistedAmount) {
			return infraerrors.Conflict("IDEMPOTENCY_KEY_CONFLICT", "idempotency key was already used for another balance transfer")
		}
		return nil
	}
	if !errors.Is(replayErr, sql.ErrNoRows) {
		return replayErr
	}
	var memberAfter, companyAfter string
	var sourceUser, destinationUser sql.NullInt64
	if !reclaim {
		// allocate: 公司余额 -> 成员
		if err := tx.QueryRowContext(ctx, `UPDATE organizations SET balance=balance-$2::numeric,updated_at=NOW() WHERE id=$1 AND balance >= $2::numeric RETURNING balance::text`, orgID, amount).Scan(&companyAfter); errors.Is(err, sql.ErrNoRows) {
			return service.ErrInsufficientBalance
		} else if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance+$2::numeric,updated_at=NOW() WHERE id=$1 RETURNING balance::text`, memberUserID, amount).Scan(&memberAfter); err != nil {
			return err
		}
		destinationUser = sql.NullInt64{Int64: memberUserID, Valid: true}
	} else {
		// reclaim: 成员 -> 公司余额
		if err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance-$2::numeric,updated_at=NOW() WHERE id=$1 AND balance >= $2::numeric RETURNING balance::text`, memberUserID, amount).Scan(&memberAfter); errors.Is(err, sql.ErrNoRows) {
			return service.ErrInsufficientBalance
		} else if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `UPDATE organizations SET balance=balance+$2::numeric,updated_at=NOW() WHERE id=$1 RETURNING balance::text`, orgID, amount).Scan(&companyAfter); err != nil {
			return err
		}
		sourceUser = sql.NullInt64{Int64: memberUserID, Valid: true}
	}
	// 被扣减一侧记为 source 快照：allocate 来源是公司，reclaim 来源是成员。
	sourceAfter, destinationAfter := companyAfter, memberAfter
	if reclaim {
		sourceAfter, destinationAfter = memberAfter, companyAfter
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_financial_ledger(idempotency_key,kind,organization_id,actor_user_id,source_user_id,destination_user_id,amount,currency,source_balance_after,destination_balance_after) VALUES($1,$2,$3,$4,$5,$6,$7::numeric,'USD',$8::numeric,$9::numeric)`, ledgerKey, kind, orgID, ownerID, sourceUser, destinationUser, amount, sourceAfter, destinationAfter); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,$4,'success',$5,jsonb_build_object('amount',$6::numeric))`, orgID, ownerID, memberUserID, "organization.balance."+kind, organizationCorrelationID(ctx), amount); err != nil {
		return err
	}
	return tx.Commit()
}

// DepositToCompany moves funds between the owner's personal users.balance and
// the organization's own balance. When withdraw is false funds flow from the
// owner into the company balance (a top-up); when true they flow back. Only the
// active owner of an active organization may perform this. The movement is
// idempotent per (organization, idempotency key) and is recorded in
// organization_financial_ledger with the user side referenced and the company
// side left NULL.
func (r *organizationRepository) DepositToCompany(ctx context.Context, ownerID int64, amount, idempotencyKey string, withdraw bool) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var orgID, rootID int64
	if err := tx.QueryRowContext(ctx, `SELECT o.id,o.owner_user_id FROM organizations o JOIN organization_memberships m ON m.organization_id=o.id WHERE m.user_id=$1 AND m.role='owner' AND m.status='active' AND o.status='active'`, ownerID).Scan(&orgID, &rootID); err != nil {
		return service.ErrOrganizationPermission
	}
	// Lock the owner user row and the organization row (users first, then
	// organizations) to keep a stable ordering with concurrent operations.
	if _, err := tx.ExecContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, rootID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, orgID); err != nil {
		return err
	}
	kind := "company_deposit"
	if withdraw {
		kind = "company_withdraw"
	}
	ledgerKey := fmt.Sprintf("organization:company_balance:%d:%s", orgID, idempotencyKey)
	var existingKind, existingAmount string
	var existingActor int64
	replayErr := tx.QueryRowContext(ctx, `SELECT kind,actor_user_id,amount::text FROM organization_financial_ledger WHERE idempotency_key=$1`, ledgerKey).
		Scan(&existingKind, &existingActor, &existingAmount)
	if replayErr == nil {
		requestedAmount, parseErr := decimal.NewFromString(amount)
		persistedAmount, persistedParseErr := decimal.NewFromString(existingAmount)
		if parseErr != nil || persistedParseErr != nil || existingKind != kind || existingActor != ownerID || !requestedAmount.Equal(persistedAmount) {
			return infraerrors.Conflict("IDEMPOTENCY_KEY_CONFLICT", "idempotency key was already used for another company balance operation")
		}
		return nil
	}
	if !errors.Is(replayErr, sql.ErrNoRows) {
		return replayErr
	}
	var userAfter, companyAfter string
	var sourceUser, destinationUser sql.NullInt64
	if !withdraw {
		if err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance-$2::numeric,updated_at=NOW() WHERE id=$1 AND balance >= $2::numeric RETURNING balance::text`, rootID, amount).Scan(&userAfter); errors.Is(err, sql.ErrNoRows) {
			return service.ErrInsufficientBalance
		} else if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `UPDATE organizations SET balance=balance+$2::numeric,updated_at=NOW() WHERE id=$1 RETURNING balance::text`, orgID, amount).Scan(&companyAfter); err != nil {
			return err
		}
		sourceUser = sql.NullInt64{Int64: rootID, Valid: true}
	} else {
		if err := tx.QueryRowContext(ctx, `UPDATE organizations SET balance=balance-$2::numeric,updated_at=NOW() WHERE id=$1 AND balance >= $2::numeric RETURNING balance::text`, orgID, amount).Scan(&companyAfter); errors.Is(err, sql.ErrNoRows) {
			return service.ErrInsufficientBalance
		} else if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance+$2::numeric,updated_at=NOW() WHERE id=$1 RETURNING balance::text`, rootID, amount).Scan(&userAfter); err != nil {
			return err
		}
		destinationUser = sql.NullInt64{Int64: rootID, Valid: true}
	}
	// The debited side is recorded as the source balance snapshot.
	sourceAfter, destinationAfter := userAfter, companyAfter
	if withdraw {
		sourceAfter, destinationAfter = companyAfter, userAfter
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_financial_ledger(idempotency_key,kind,organization_id,actor_user_id,source_user_id,destination_user_id,amount,currency,source_balance_after,destination_balance_after) VALUES($1,$2,$3,$4,$5,$6,$7::numeric,'USD',$8::numeric,$9::numeric)`, ledgerKey, kind, orgID, ownerID, sourceUser, destinationUser, amount, sourceAfter, destinationAfter); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,$4,'success',$5,jsonb_build_object('amount',$6::numeric))`, orgID, ownerID, rootID, "organization.balance."+kind, organizationCorrelationID(ctx), amount); err != nil {
		return err
	}
	return tx.Commit()
}

func organizationNullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

// CreateOrganizationSubscription provisions a subscription plan (group) for the
// caller's company. Only the active owner of an active organization may do
// this. When validityDays is 0 the group's default validity is used.
func (r *organizationRepository) CreateOrganizationSubscription(ctx context.Context, userID, groupID int64, validityDays int, notes string) (_ *service.OrganizationSubscription, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	orgID, err := resolveOrganizationForActor(ctx, tx, userID, service.ActionSubscriptionManage, false)
	if err != nil {
		return nil, err
	}
	var (
		groupStatus, groupName, platform, subscriptionType string
		groupDefaultValidity                               int
		rateMultiplier                                     float64
		dailyLimit, weeklyLimit, monthlyLimit              sql.NullString
	)
	if err := tx.QueryRowContext(ctx, `SELECT status,name,platform,subscription_type,default_validity_days,rate_multiplier,daily_limit_usd::text,weekly_limit_usd::text,monthly_limit_usd::text FROM groups WHERE id=$1 AND deleted_at IS NULL`, groupID).
		Scan(&groupStatus, &groupName, &platform, &subscriptionType, &groupDefaultValidity, &rateMultiplier, &dailyLimit, &weeklyLimit, &monthlyLimit); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionGroupInvalid
	} else if err != nil {
		return nil, err
	}
	if groupStatus != "active" {
		return nil, service.ErrSubscriptionGroupInvalid
	}
	if validityDays <= 0 {
		validityDays = groupDefaultValidity
	}
	if validityDays <= 0 {
		validityDays = 30
	}
	var (
		id                                         int64
		startsAt, expiresAt, assignedAt, createdAt time.Time
		status                                     string
	)
	insertErr := tx.QueryRowContext(ctx, `INSERT INTO organization_subscriptions(organization_id,group_id,starts_at,expires_at,status,assigned_by,assigned_at,notes) VALUES($1,$2,NOW(),NOW()+($3::int * INTERVAL '1 day'),'active',$4,NOW(),NULLIF($5,'')) RETURNING id,starts_at,expires_at,status,assigned_at,created_at`, orgID, groupID, validityDays, userID, notes).
		Scan(&id, &startsAt, &expiresAt, &status, &assignedAt, &createdAt)
	if isUniqueViolation(insertErr) {
		return nil, service.ErrOrgSubscriptionExists
	} else if insertErr != nil {
		return nil, insertErr
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'organization.subscription.create','success',$3,jsonb_build_object('group_id',$4::bigint,'subscription_id',$5::bigint))`, orgID, userID, organizationCorrelationID(ctx), groupID, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	assignedBy := userID
	return &service.OrganizationSubscription{
		ID:               id,
		OrganizationID:   orgID,
		GroupID:          groupID,
		GroupName:        groupName,
		Platform:         platform,
		SubscriptionType: subscriptionType,
		RateMultiplier:   rateMultiplier,
		StartsAt:         startsAt,
		ExpiresAt:        expiresAt,
		Status:           status,
		DailyLimitUSD:    organizationNullStringPtr(dailyLimit),
		WeeklyLimitUSD:   organizationNullStringPtr(weeklyLimit),
		MonthlyLimitUSD:  organizationNullStringPtr(monthlyLimit),
		DailyUsageUSD:    "0",
		WeeklyUsageUSD:   "0",
		MonthlyUsageUSD:  "0",
		Notes:            notes,
		AssignedBy:       &assignedBy,
		AssignedAt:       assignedAt,
		CreatedAt:        createdAt,
	}, nil
}

func (r *organizationRepository) AdminCreateOrganizationSubscription(ctx context.Context, actorID, organizationID, groupID int64, validityDays int, notes string) (_ *service.OrganizationSubscription, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveAdminDB(ctx, tx, actorID); err != nil {
		return nil, err
	}
	var organizationStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM organizations WHERE id=$1 FOR UPDATE`, organizationID).Scan(&organizationStatus); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrCompanyNotFound
	} else if err != nil {
		return nil, err
	}
	if organizationStatus != service.OrganizationStatusActive {
		return nil, service.ErrOrganizationSuspended
	}
	var (
		groupStatus, groupName, platform, subscriptionType string
		rateMultiplier                                     float64
		dailyLimit, weeklyLimit, monthlyLimit              sql.NullString
	)
	if err := tx.QueryRowContext(ctx, `SELECT status,name,platform,subscription_type,rate_multiplier,daily_limit_usd::text,weekly_limit_usd::text,monthly_limit_usd::text FROM groups WHERE id=$1 AND deleted_at IS NULL`, groupID).
		Scan(&groupStatus, &groupName, &platform, &subscriptionType, &rateMultiplier, &dailyLimit, &weeklyLimit, &monthlyLimit); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionGroupInvalid
	} else if err != nil {
		return nil, err
	}
	if groupStatus != "active" || subscriptionType != service.SubscriptionTypeSubscription {
		return nil, service.ErrSubscriptionGroupInvalid
	}
	var (
		id                                         int64
		startsAt, expiresAt, assignedAt, createdAt time.Time
		status                                     string
	)
	insertErr := tx.QueryRowContext(ctx, `INSERT INTO organization_subscriptions(organization_id,group_id,starts_at,expires_at,status,assigned_by,assigned_at,notes) VALUES($1,$2,NOW(),NOW()+($3::int * INTERVAL '1 day'),'active',$4,NOW(),NULLIF($5,'')) RETURNING id,starts_at,expires_at,status,assigned_at,created_at`, organizationID, groupID, validityDays, actorID, notes).
		Scan(&id, &startsAt, &expiresAt, &status, &assignedAt, &createdAt)
	if isUniqueViolation(insertErr) {
		return nil, service.ErrOrgSubscriptionExists
	} else if insertErr != nil {
		return nil, insertErr
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'organization.subscription.admin_assign','success',$3,jsonb_build_object('group_id',$4::bigint,'subscription_id',$5::bigint,'validity_days',$6::int))`, organizationID, actorID, organizationCorrelationID(ctx), groupID, id, validityDays); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	assignedBy := actorID
	return &service.OrganizationSubscription{
		ID: id, OrganizationID: organizationID, GroupID: groupID, GroupName: groupName, Platform: platform,
		SubscriptionType: subscriptionType, RateMultiplier: rateMultiplier, StartsAt: startsAt, ExpiresAt: expiresAt, Status: status,
		DailyLimitUSD: organizationNullStringPtr(dailyLimit), WeeklyLimitUSD: organizationNullStringPtr(weeklyLimit), MonthlyLimitUSD: organizationNullStringPtr(monthlyLimit),
		DailyUsageUSD: "0", WeeklyUsageUSD: "0", MonthlyUsageUSD: "0", Notes: notes,
		AssignedBy: &assignedBy, AssignedAt: assignedAt, CreatedAt: createdAt,
	}, nil
}

func (r *organizationRepository) AdminListOrganizationSubscriptions(ctx context.Context, actorID int64, page, pageSize int, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.OrganizationSubscription, int64, error) {
	if err := requireActiveAdminDB(ctx, r.db, actorID); err != nil {
		return nil, 0, err
	}
	page, pageSize = normalizePage(page, pageSize)
	conditions := []string{"s.deleted_at IS NULL"}
	args := make([]any, 0, 4)
	add := func(format string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(format, len(args)))
	}
	if groupID != nil {
		add("s.group_id=$%d", *groupID)
	}
	if platform != "" {
		add("g.platform=$%d", platform)
	}
	switch status {
	case service.SubscriptionStatusActive:
		conditions = append(conditions, "s.status='active' AND s.expires_at>NOW()")
	case service.SubscriptionStatusExpired:
		conditions = append(conditions, "(s.status='expired' OR (s.status='active' AND s.expires_at<=NOW()))")
	case service.SubscriptionStatusSuspended:
		conditions = append(conditions, "s.status='suspended'")
	case "":
	default:
		return nil, 0, infraerrors.BadRequest("SUBSCRIPTION_STATUS_INVALID", "subscription status is invalid")
	}
	where := " WHERE " + strings.Join(conditions, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM organization_subscriptions s JOIN groups g ON g.id=s.group_id`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	sortField := "s.created_at"
	switch sortBy {
	case "expires_at":
		sortField = "s.expires_at"
	case "status":
		sortField = "s.status"
	}
	direction := "DESC"
	if sortOrder == "asc" {
		direction = "ASC"
	}
	args = append(args, pageSize, (page-1)*pageSize)
	query := `SELECT s.id,s.organization_id,o.name,COALESCE(o.company_id,''),s.group_id,g.name,g.platform,g.subscription_type,g.rate_multiplier,
		s.starts_at,s.expires_at,s.status,g.daily_limit_usd::text,g.weekly_limit_usd::text,g.monthly_limit_usd::text,
		s.daily_usage_usd::text,s.weekly_usage_usd::text,s.monthly_usage_usd::text,COALESCE(s.notes,''),s.assigned_by,s.assigned_at,s.created_at
		FROM organization_subscriptions s JOIN organizations o ON o.id=s.organization_id JOIN groups g ON g.id=s.group_id` + where +
		fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", sortField, direction, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.OrganizationSubscription, 0, pageSize)
	for rows.Next() {
		var item service.OrganizationSubscription
		var dailyLimit, weeklyLimit, monthlyLimit sql.NullString
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.OrganizationName, &item.CompanyID, &item.GroupID, &item.GroupName, &item.Platform, &item.SubscriptionType,
			&item.RateMultiplier, &item.StartsAt, &item.ExpiresAt, &item.Status, &dailyLimit, &weeklyLimit, &monthlyLimit, &item.DailyUsageUSD, &item.WeeklyUsageUSD, &item.MonthlyUsageUSD,
			&item.Notes, &item.AssignedBy, &item.AssignedAt, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.DailyLimitUSD = organizationNullStringPtr(dailyLimit)
		item.WeeklyLimitUSD = organizationNullStringPtr(weeklyLimit)
		item.MonthlyLimitUSD = organizationNullStringPtr(monthlyLimit)
		if item.Status == service.SubscriptionStatusActive && !item.ExpiresAt.After(time.Now()) {
			item.Status = service.SubscriptionStatusExpired
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *organizationRepository) AdminExtendOrganizationSubscription(ctx context.Context, actorID, subscriptionID int64, days int) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveAdminDB(ctx, tx, actorID); err != nil {
		return err
	}
	var organizationID int64
	var expiresAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT organization_id,expires_at FROM organization_subscriptions WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, subscriptionID).Scan(&organizationID, &expiresAt); errors.Is(err, sql.ErrNoRows) {
		return service.ErrOrgSubscriptionNotFound
	} else if err != nil {
		return err
	}
	now := time.Now()
	base := expiresAt
	if !base.After(now) {
		if days < 0 {
			return infraerrors.BadRequest("CANNOT_SHORTEN_EXPIRED", "cannot shorten an expired subscription")
		}
		base = now
	}
	newExpiresAt := base.AddDate(0, 0, days)
	if !newExpiresAt.After(now) {
		return service.ErrAdjustWouldExpire
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organization_subscriptions SET expires_at=$2,status='active',updated_at=NOW() WHERE id=$1`, subscriptionID, newExpiresAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'organization.subscription.admin_extend','success',$3,jsonb_build_object('subscription_id',$4::bigint,'days',$5::int))`, organizationID, actorID, organizationCorrelationID(ctx), subscriptionID, days); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) AdminResetOrganizationSubscriptionQuota(ctx context.Context, actorID, subscriptionID int64) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveAdminDB(ctx, tx, actorID); err != nil {
		return err
	}
	var organizationID int64
	if err := tx.QueryRowContext(ctx, `UPDATE organization_subscriptions SET daily_usage_usd=0,weekly_usage_usd=0,monthly_usage_usd=0,daily_window_start=NOW(),weekly_window_start=NOW(),monthly_window_start=NOW(),updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL RETURNING organization_id`, subscriptionID).Scan(&organizationID); errors.Is(err, sql.ErrNoRows) {
		return service.ErrOrgSubscriptionNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'organization.subscription.admin_reset_quota','success',$3,jsonb_build_object('subscription_id',$4::bigint))`, organizationID, actorID, organizationCorrelationID(ctx), subscriptionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) AdminRevokeOrganizationSubscription(ctx context.Context, actorID, subscriptionID int64) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActiveAdminDB(ctx, tx, actorID); err != nil {
		return err
	}
	var organizationID int64
	if err := tx.QueryRowContext(ctx, `UPDATE organization_subscriptions SET status='cancelled',deleted_at=NOW(),updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL RETURNING organization_id`, subscriptionID).Scan(&organizationID); errors.Is(err, sql.ErrNoRows) {
		return service.ErrOrgSubscriptionNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'organization.subscription.admin_revoke','success',$3,jsonb_build_object('subscription_id',$4::bigint))`, organizationID, actorID, organizationCorrelationID(ctx), subscriptionID); err != nil {
		return err
	}
	return tx.Commit()
}

// AssignOrExtendOrganizationSubscription provisions or extends a company
// subscription as part of paid-order fulfillment. Unlike
// CreateOrganizationSubscription it operates directly on orgID (fulfillment is
// system-triggered, not owner-triggered) and is idempotent on orderID via the
// order note stored on the subscription row:
//   - If an active, non-deleted subscription for (orgID, groupID) already
//     carries this order's note, it is a retry and we no-op.
//   - Else if an active, non-deleted subscription exists, we extend expires_at
//     (from the later of now / current expiry) and append the order note.
//   - Otherwise we insert a fresh subscription.
func (r *organizationRepository) AssignOrExtendOrganizationSubscription(ctx context.Context, orgID, groupID int64, validityDays int, orderID int64) (err error) {
	if validityDays <= 0 {
		validityDays = 30
	}
	orderNote := fmt.Sprintf("payment order %d", orderID)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var groupStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM groups WHERE id=$1 AND deleted_at IS NULL`, groupID).Scan(&groupStatus); errors.Is(err, sql.ErrNoRows) {
		return service.ErrSubscriptionGroupInvalid
	} else if err != nil {
		return err
	}
	if groupStatus != "active" {
		return service.ErrSubscriptionGroupInvalid
	}

	var (
		existingID    int64
		existingNotes sql.NullString
	)
	lookupErr := tx.QueryRowContext(ctx, `SELECT id,notes FROM organization_subscriptions WHERE organization_id=$1 AND group_id=$2 AND deleted_at IS NULL FOR UPDATE`, orgID, groupID).
		Scan(&existingID, &existingNotes)
	switch {
	case lookupErr == nil:
		// Idempotency: this order was already applied to the subscription.
		if existingNotes.Valid && organizationNotesContainLine(existingNotes.String, orderNote) {
			return tx.Commit()
		}
		if _, err := tx.ExecContext(ctx, `UPDATE organization_subscriptions SET expires_at=GREATEST(expires_at,NOW())+($1::int * INTERVAL '1 day'),status='active',notes=CASE WHEN COALESCE(notes,'')='' THEN $2 ELSE notes||E'\n'||$2 END,updated_at=NOW() WHERE id=$3`, validityDays, orderNote, existingID); err != nil {
			return err
		}
	case errors.Is(lookupErr, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO organization_subscriptions(organization_id,group_id,starts_at,expires_at,status,assigned_at,notes) VALUES($1,$2,NOW(),NOW()+($3::int * INTERVAL '1 day'),'active',NOW(),$4)`, orgID, groupID, validityDays, orderNote); err != nil {
			if isUniqueViolation(err) {
				// A concurrent fulfillment inserted first; treat as success.
				return tx.Commit()
			}
			return err
		}
	default:
		return lookupErr
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,NULL,'organization.subscription.create','success',$2,jsonb_build_object('group_id',$3::bigint,'order_id',$4::bigint,'validity_days',$5::int))`, orgID, organizationCorrelationID(ctx), groupID, orderID, validityDays); err != nil {
		return err
	}
	return tx.Commit()
}

// ListOrganizationSubscriptions returns the company's non-deleted subscriptions
// joined with their group. Visible to the owner and to accounts holding
// organization.finance.balance.read.
func (r *organizationRepository) ListOrganizationSubscriptions(ctx context.Context, userID int64) ([]service.OrganizationSubscription, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	if !org.Owner() && !org.HasAction(service.ActionFinanceBalanceRead) && !org.HasAction(service.ActionSubscriptionManage) {
		return nil, service.ErrOrganizationPermission
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+organizationSubscriptionSelectColumns+` FROM organization_subscriptions s JOIN groups g ON g.id=s.group_id WHERE s.organization_id=$1 AND s.deleted_at IS NULL ORDER BY s.created_at DESC`, org.OrganizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	subscriptions := make([]service.OrganizationSubscription, 0)
	for rows.Next() {
		s, err := scanOrganizationSubscription(rows.Scan)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, s)
	}
	return subscriptions, rows.Err()
}

// CancelOrganizationSubscription soft-deletes a company subscription. Only the
// active owner of an active organization may do this.
func (r *organizationRepository) CancelOrganizationSubscription(ctx context.Context, userID, subscriptionID int64) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	orgID, err := resolveOrganizationForActor(ctx, tx, userID, service.ActionSubscriptionManage, false)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE organization_subscriptions SET status='cancelled',deleted_at=NOW(),updated_at=NOW() WHERE id=$1 AND organization_id=$2 AND deleted_at IS NULL`, subscriptionID, orgID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return service.ErrOrgSubscriptionNotFound
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,action,result,correlation_id,metadata) VALUES($1,$2,'organization.subscription.cancel','success',$3,jsonb_build_object('subscription_id',$4::bigint))`, orgID, userID, organizationCorrelationID(ctx), subscriptionID); err != nil {
		return err
	}
	return tx.Commit()
}

const organizationSubscriptionSelectColumns = `s.id,s.organization_id,s.group_id,g.name,g.platform,g.subscription_type,g.rate_multiplier,s.starts_at,s.expires_at,s.status,g.daily_limit_usd::text,g.weekly_limit_usd::text,g.monthly_limit_usd::text,s.daily_usage_usd::text,s.weekly_usage_usd::text,s.monthly_usage_usd::text,COALESCE(s.notes,''),s.assigned_by,s.assigned_at,s.created_at`

func scanOrganizationSubscription(scan func(dest ...any) error) (service.OrganizationSubscription, error) {
	var (
		s                                     service.OrganizationSubscription
		dailyLimit, weeklyLimit, monthlyLimit sql.NullString
		assignedBy                            sql.NullInt64
	)
	if err := scan(&s.ID, &s.OrganizationID, &s.GroupID, &s.GroupName, &s.Platform, &s.SubscriptionType, &s.RateMultiplier, &s.StartsAt, &s.ExpiresAt, &s.Status, &dailyLimit, &weeklyLimit, &monthlyLimit, &s.DailyUsageUSD, &s.WeeklyUsageUSD, &s.MonthlyUsageUSD, &s.Notes, &assignedBy, &s.AssignedAt, &s.CreatedAt); err != nil {
		return service.OrganizationSubscription{}, err
	}
	s.DailyLimitUSD = organizationNullStringPtr(dailyLimit)
	s.WeeklyLimitUSD = organizationNullStringPtr(weeklyLimit)
	s.MonthlyLimitUSD = organizationNullStringPtr(monthlyLimit)
	if assignedBy.Valid {
		by := assignedBy.Int64
		s.AssignedBy = &by
	}
	return s, nil
}

// ListActiveOrganizationSubscriptionsForMember returns the active, non-expired
// company subscriptions that the given user (as an active member of an active
// organization) may bind an enterprise API key to. Non-members receive an empty
// list rather than an error so the create dialog can degrade gracefully.
func (r *organizationRepository) ListActiveOrganizationSubscriptionsForMember(ctx context.Context, userID int64) ([]service.OrganizationSubscription, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() {
		return []service.OrganizationSubscription{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+organizationSubscriptionSelectColumns+` FROM organization_subscriptions s JOIN groups g ON g.id=s.group_id WHERE s.organization_id=$1 AND s.deleted_at IS NULL AND s.status='active' AND s.expires_at > NOW() AND g.status='active' AND g.deleted_at IS NULL ORDER BY s.created_at DESC`, org.OrganizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	subscriptions := make([]service.OrganizationSubscription, 0)
	for rows.Next() {
		s, err := scanOrganizationSubscription(rows.Scan)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, s)
	}
	return subscriptions, rows.Err()
}

// GetActiveOrganizationSubscriptionForMember validates that the user is an active
// member of the organization owning the subscription and that the subscription
// is active and not expired, then returns it.
func (r *organizationRepository) GetActiveOrganizationSubscriptionForMember(ctx context.Context, userID, subscriptionID int64) (*service.OrganizationSubscription, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	s, err := scanOrganizationSubscription(func(dest ...any) error {
		return r.db.QueryRowContext(ctx, `SELECT `+organizationSubscriptionSelectColumns+` FROM organization_subscriptions s JOIN groups g ON g.id=s.group_id WHERE s.id=$1 AND s.organization_id=$2 AND s.deleted_at IS NULL AND s.status='active' AND s.expires_at > NOW() AND g.status='active' AND g.deleted_at IS NULL`, subscriptionID, org.OrganizationID).Scan(dest...)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrOrgSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetOrganizationSubscriptionOrganizationID returns the organization_id of the
// given subscription regardless of its lifecycle state (allows soft-deleted,
// expired, or cancelled rows, and does not require the referenced group to
// still exist). Used purely as an ACL helper for views that must remain
// visible after a subscription is revoked or cleaned (e.g. the API Key list
// wants to show the fallback chain even if the current subscription has been
// removed). Returns ErrOrgSubscriptionNotFound only when the row is entirely
// missing.
func (r *organizationRepository) GetOrganizationSubscriptionOrganizationID(ctx context.Context, subscriptionID int64) (int64, error) {
	var orgID int64
	err := r.db.QueryRowContext(ctx, `SELECT organization_id FROM organization_subscriptions WHERE id=$1`, subscriptionID).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrOrgSubscriptionNotFound
	}
	if err != nil {
		return 0, err
	}
	return orgID, nil
}

// GetOrganizationSubscriptionForBilling loads a company subscription's usage
// windows, counters and group limits for request-time validation and billing.
func (r *organizationRepository) GetOrganizationSubscriptionForBilling(ctx context.Context, subscriptionID int64) (*service.OrgSubscriptionRuntime, error) {
	var (
		rt                     service.OrgSubscriptionRuntime
		dWin, wWin, mWin       sql.NullTime
		dLimit, wLimit, mLimit sql.NullFloat64
	)
	err := r.db.QueryRowContext(ctx, `SELECT s.id,s.organization_id,s.group_id,s.status,s.starts_at,s.expires_at,s.daily_window_start,s.weekly_window_start,s.monthly_window_start,s.daily_usage_usd,s.weekly_usage_usd,s.monthly_usage_usd,g.daily_limit_usd,g.weekly_limit_usd,g.monthly_limit_usd FROM organization_subscriptions s JOIN groups g ON g.id=s.group_id WHERE s.id=$1 AND s.deleted_at IS NULL`, subscriptionID).
		Scan(&rt.ID, &rt.OrganizationID, &rt.GroupID, &rt.Status, &rt.StartsAt, &rt.ExpiresAt, &dWin, &wWin, &mWin, &rt.DailyUsageUSD, &rt.WeeklyUsageUSD, &rt.MonthlyUsageUSD, &dLimit, &wLimit, &mLimit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrOrgSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}
	if dWin.Valid {
		rt.DailyWindowStart = &dWin.Time
	}
	if wWin.Valid {
		rt.WeeklyWindowStart = &wWin.Time
	}
	if mWin.Valid {
		rt.MonthlyWindowStart = &mWin.Time
	}
	if dLimit.Valid {
		v := dLimit.Float64
		rt.DailyLimitUSD = &v
	}
	if wLimit.Valid {
		v := wLimit.Float64
		rt.WeeklyLimitUSD = &v
	}
	if mLimit.Valid {
		v := mLimit.Float64
		rt.MonthlyLimitUSD = &v
	}
	return &rt, nil
}

// IncrementOrganizationSubscriptionUsage atomically adds costUSD to the
// subscription's daily/weekly/monthly usage counters. It is window-aware:
// a NULL or expired window is (re)started at NOW() with usage reset to costUSD,
// mirroring the rolling-window semantics used for personal subscriptions.
func (r *organizationRepository) IncrementOrganizationSubscriptionUsage(ctx context.Context, subscriptionID int64, costUSD float64) error {
	if costUSD == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `UPDATE organization_subscriptions SET
        daily_usage_usd = CASE WHEN daily_window_start IS NULL OR NOW() - daily_window_start >= INTERVAL '24 hours' THEN $2 ELSE daily_usage_usd + $2 END,
        daily_window_start = CASE WHEN daily_window_start IS NULL OR NOW() - daily_window_start >= INTERVAL '24 hours' THEN NOW() ELSE daily_window_start END,
        weekly_usage_usd = CASE WHEN weekly_window_start IS NULL OR NOW() - weekly_window_start >= INTERVAL '7 days' THEN $2 ELSE weekly_usage_usd + $2 END,
        weekly_window_start = CASE WHEN weekly_window_start IS NULL OR NOW() - weekly_window_start >= INTERVAL '7 days' THEN NOW() ELSE weekly_window_start END,
        monthly_usage_usd = CASE WHEN monthly_window_start IS NULL OR NOW() - monthly_window_start >= INTERVAL '30 days' THEN $2 ELSE monthly_usage_usd + $2 END,
        monthly_window_start = CASE WHEN monthly_window_start IS NULL OR NOW() - monthly_window_start >= INTERVAL '30 days' THEN NOW() ELSE monthly_window_start END,
        updated_at = NOW()
        WHERE id = $1 AND deleted_at IS NULL`, subscriptionID, costUSD)
	return err
}

// ListAuditEvents returns paginated audit records for the organization used by
// the "Audit log" page. Access control is enforced upstream in the service
// layer (owner-only). Actor / subject login names are LEFT JOINed from users so
// the UI can show human-readable rows without an extra roundtrip.
func (r *organizationRepository) ListAuditEvents(ctx context.Context, organizationID int64, filter service.OrganizationAuditFilter) ([]service.OrganizationAuditLogEntry, int64, error) {
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	where := []string{"e.organization_id = $1"}
	args := []any{organizationID}
	if actions := service.AuditActionsForCategory(filter.Category); len(actions) > 0 {
		where = append(where, fmt.Sprintf("e.action = ANY($%d)", len(args)+1))
		args = append(args, pq.Array(actions))
	}
	if !filter.Start.IsZero() {
		where = append(where, fmt.Sprintf("e.created_at >= $%d", len(args)+1))
		args = append(args, filter.Start)
	}
	if !filter.End.IsZero() {
		where = append(where, fmt.Sprintf("e.created_at < $%d", len(args)+1))
		args = append(args, filter.End)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM organization_audit_events e WHERE %s`, whereSQL)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []service.OrganizationAuditLogEntry{}, 0, nil
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	listQuery := fmt.Sprintf(`
		SELECT e.id, e.organization_id, e.actor_user_id, e.subject_user_id, e.action, e.result,
		       COALESCE(e.metadata::text, '{}'), e.created_at,
		       COALESCE(actor.login_name, actor.username, '')       AS actor_login_name,
		       COALESCE(actor.username, '')                          AS actor_username,
		       COALESCE(actor.email, '')                             AS actor_email,
		       COALESCE(subject.login_name, subject.username, '')   AS subject_login_name,
		       COALESCE(subject.username, '')                        AS subject_username,
		       COALESCE(subject.email, '')                           AS subject_email
		FROM organization_audit_events e
		LEFT JOIN users actor   ON actor.id   = e.actor_user_id
		LEFT JOIN users subject ON subject.id = e.subject_user_id
		WHERE %s
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $%d OFFSET $%d`, whereSQL, len(listArgs)-1, len(listArgs))

	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]service.OrganizationAuditLogEntry, 0, pageSize)
	for rows.Next() {
		var (
			ev            service.OrganizationAuditLogEntry
			orgID         sql.NullInt64
			actorID       sql.NullInt64
			subjectID     sql.NullInt64
			metadataRaw   string
			actorLogin    string
			actorUsername string
			actorEmail    string
			subjectLogin  string
			subjectName   string
			subjectEmail  string
		)
		if err := rows.Scan(&ev.ID, &orgID, &actorID, &subjectID, &ev.Action, &ev.Result, &metadataRaw, &ev.CreatedAt, &actorLogin, &actorUsername, &actorEmail, &subjectLogin, &subjectName, &subjectEmail); err != nil {
			return nil, 0, err
		}
		if orgID.Valid {
			v := orgID.Int64
			ev.OrganizationID = &v
		}
		if actorID.Valid {
			v := actorID.Int64
			ev.ActorUserID = &v
		}
		if subjectID.Valid {
			v := subjectID.Int64
			ev.SubjectUserID = &v
		}
		ev.ActorLoginName = actorLogin
		ev.ActorUsername = actorUsername
		ev.ActorEmail = actorEmail
		ev.SubjectLoginName = subjectLogin
		ev.SubjectUsername = subjectName
		ev.SubjectEmail = subjectEmail
		if metadataRaw != "" && metadataRaw != "{}" {
			var meta map[string]any
			if err := json.Unmarshal([]byte(metadataRaw), &meta); err == nil {
				ev.Metadata = meta
			}
		}
		ev.Category = service.AuditCategoryForAction(ev.Action)
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func (r *organizationRepository) FinanceSummary(ctx context.Context, userID int64) (*service.FinanceSummary, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	viewRoot := org.Owner() || org.HasAction(service.ActionFinanceBalanceRead)
	shared := org.HasAction(service.ActionSharedBalanceUse) && !org.Owner()
	targetID := userID
	source := "allocated"
	if org.Owner() {
		source = "self"
	}
	if shared {
		source = "shared"
	}
	if viewRoot {
		targetID = org.OwnerUserID
	}
	var available, frozen string
	if err := r.db.QueryRowContext(ctx, `SELECT balance::text,frozen_balance::text FROM users WHERE id=$1`, targetID).Scan(&available, &frozen); err != nil {
		return nil, err
	}
	if shared && !viewRoot {
		return &service.FinanceSummary{BalanceSource: source}, nil
	}
	total, err := decimal.NewFromString(available)
	if err != nil {
		return nil, err
	}
	frozenValue, err := decimal.NewFromString(frozen)
	if err != nil {
		return nil, err
	}
	total = total.Add(frozenValue)
	summary := &service.FinanceSummary{BalanceSource: source, Available: &available, Frozen: &frozen, Total: organizationStringPtr(total.String())}
	// Privileged viewers additionally see the company's own balance, which is
	// independent from the personal balance reported above.
	if viewRoot {
		var companyAvailable, companyFrozen string
		if err := r.db.QueryRowContext(ctx, `SELECT balance::text,frozen_balance::text FROM organizations WHERE id=$1`, org.OrganizationID).Scan(&companyAvailable, &companyFrozen); err != nil {
			return nil, err
		}
		companyTotal, err := decimal.NewFromString(companyAvailable)
		if err != nil {
			return nil, err
		}
		companyFrozenValue, err := decimal.NewFromString(companyFrozen)
		if err != nil {
			return nil, err
		}
		companyTotal = companyTotal.Add(companyFrozenValue)
		summary.CompanyAvailable = &companyAvailable
		summary.CompanyFrozen = &companyFrozen
		summary.CompanyTotal = organizationStringPtr(companyTotal.String())
	}
	return summary, nil
}

func organizationStringPtr(value string) *string { return &value }

func (r *organizationRepository) ResolveBillingContext(ctx context.Context, consumerUserID int64, requiredAmount float64) (*service.BillingContext, error) {
	var identity string
	if err := r.db.QueryRowContext(ctx, `SELECT identity_type FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL`, consumerUserID).Scan(&identity); err != nil {
		return nil, service.ErrUserNotFound
	}
	if identity != service.IdentityTypeIAM {
		org, err := r.GetContextForUser(ctx, consumerUserID)
		if err == nil && !org.Active() {
			return nil, service.ErrOrganizationPermission
		}
		var orgID *int64
		var generation int64 = 1
		if err == nil {
			orgID, generation = &org.OrganizationID, org.AuthzGeneration
		}
		// DIAG_BILLING_BYPASS: 非 IAM 身份走 self 分支
		var orgIDLog int64
		if orgID != nil {
			orgIDLog = *orgID
		}
		logger.LegacyPrintf("repository.organization",
			"DIAG_BILLING_BYPASS resolve_non_iam consumer_user_id=%d identity=%s organization_id=%d required_amount=%f branch=self",
			consumerUserID, identity, orgIDLog, requiredAmount)
		return &service.BillingContext{ConsumerUserID: consumerUserID, OrganizationID: orgID, PayerUserID: consumerUserID, BalanceSource: service.BalanceSourceSelf, AuthzGeneration: generation}, nil
	}
	org, err := r.GetContextForUser(ctx, consumerUserID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	payer, source := consumerUserID, service.BalanceSourceAllocated
	hasSharedBalanceUse := org.HasAction(service.ActionSharedBalanceUse)
	// 只要 IAM 用户仍有正的划拨余额，就先选择 allocated；账本结算时
	// 如果本次金额超过该余额，再在同一事务内把扣款切换到企业钱包。
	allocationHasPositiveBalance := false
	if hasSharedBalanceUse {
		if err := r.db.QueryRowContext(ctx, `
			SELECT balance > 0
			FROM users
			WHERE id=$1 AND status='active' AND deleted_at IS NULL
		`, consumerUserID).Scan(&allocationHasPositiveBalance); err != nil {
			return nil, err
		}
		if !allocationHasPositiveBalance {
			// PayerUserID remains owner attribution for compatibility, while the
			// company source routes all money movement to organizations.balance.
			payer, source = org.OwnerUserID, service.BalanceSourceCompany
		}
	}
	// DIAG_BILLING_BYPASS: IAM 分支决策关键字段
	logger.LegacyPrintf("repository.organization",
		"DIAG_BILLING_BYPASS resolve_iam consumer_user_id=%d organization_id=%d owner_user_id=%d has_shared_balance_use=%v allocation_has_positive_balance=%v required_amount=%f -> payer_user_id=%d balance_source=%s",
		consumerUserID, org.OrganizationID, org.OwnerUserID, hasSharedBalanceUse, allocationHasPositiveBalance, requiredAmount, payer, source)
	return &service.BillingContext{ConsumerUserID: consumerUserID, OrganizationID: &org.OrganizationID, PayerUserID: payer, BalanceSource: source, AuthzGeneration: org.AuthzGeneration}, nil
}

func (r *organizationRepository) GetOrganizationBalance(ctx context.Context, organizationID int64) (float64, error) {
	return r.organizationBalanceValue(ctx, `SELECT balance FROM organizations WHERE id=$1`, organizationID)
}

func (r *organizationRepository) DeductOrganizationBalance(ctx context.Context, organizationID int64, amount float64) (float64, error) {
	// 语义变更（透支策略）：允许企业钱包一次性透支到负数。
	// 理由：预检读到微量正余额而放行、结算实际金额远大于余额时，若坚持 balance >= amount
	// 会让请求已经返回却扣不到钱；改为强扣到负后，下一次预检 balance > 0 不成立即被拒绝，
	// 不会造成重复透支。
	balance, err := r.organizationBalanceValue(ctx, `
		UPDATE organizations SET balance=balance-$2,updated_at=NOW()
		WHERE id=$1
		RETURNING balance`, organizationID, amount)
	if !errors.Is(err, sql.ErrNoRows) {
		return balance, err
	}
	exists, existsErr := r.organizationExists(ctx, organizationID)
	if existsErr != nil {
		return 0, existsErr
	}
	if !exists {
		return 0, service.ErrCompanyNotFound
	}
	// 理论上不会走到（UPDATE 只有在 id 不存在时才 0 行），保留兜底。
	return 0, service.ErrBalanceInsufficient
}

func (r *organizationRepository) CreditOrganizationBalance(ctx context.Context, organizationID int64, amount float64) (float64, error) {
	balance, err := r.organizationBalanceValue(ctx, `
		UPDATE organizations SET balance=balance+$2,updated_at=NOW()
		WHERE id=$1
		RETURNING balance`, organizationID, amount)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrCompanyNotFound
	}
	return balance, err
}

func (r *organizationRepository) organizationBalanceValue(ctx context.Context, query string, args ...any) (float64, error) {
	exec := txAwareSQLExecutor(ctx, r.db, nil)
	if exec == nil {
		return 0, errors.New("organization balance SQL executor is unavailable")
	}
	rows, err := exec.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, sql.ErrNoRows
	}
	var balance float64
	if err := rows.Scan(&balance); err != nil {
		return 0, err
	}
	return balance, rows.Err()
}

func (r *organizationRepository) organizationExists(ctx context.Context, organizationID int64) (bool, error) {
	exec := txAwareSQLExecutor(ctx, r.db, nil)
	if exec == nil {
		return false, errors.New("organization balance SQL executor is unavailable")
	}
	rows, err := exec.QueryContext(ctx, `SELECT 1 FROM organizations WHERE id=$1`, organizationID)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, rows.Err()
}

func (r *organizationRepository) organizationUsageScope(ctx context.Context, userID int64, filter service.OrganizationUsageFilter) (string, []any, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() || (!org.Owner() && !org.HasAction(service.ActionFinanceBalanceRead)) {
		return "", nil, service.ErrOrganizationPermission
	}
	conditions := []string{"l.organization_id=$1", "l.created_at >= $2"}
	args := []any{org.OrganizationID, org.EffectiveAt}
	add := func(sqlCondition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(sqlCondition, len(args)))
	}
	// 排除主账号(owner)使用个人余额或个人套餐（即 balance_source='self'/NULL 且
	// api_key 未绑定企业订阅）的消费记录，避免个人消费混入企业记录。
	args = append(args, org.OwnerUserID)
	conditions = append(conditions, fmt.Sprintf("(l.user_id <> $%d OR (l.balance_source IS NOT NULL AND l.balance_source <> 'self') OR EXISTS(SELECT 1 FROM api_keys ak WHERE ak.id=l.api_key_id AND ak.organization_subscription_id IS NOT NULL))", len(args)))
	if !filter.Start.IsZero() && filter.Start.After(org.EffectiveAt) {
		args[1] = filter.Start
	}
	if !filter.End.IsZero() {
		add("l.created_at < $%d", filter.End)
	}
	if filter.MemberID != nil {
		add("l.user_id = $%d AND EXISTS(SELECT 1 FROM organization_memberships mx WHERE mx.organization_id=l.organization_id AND mx.user_id=l.user_id)", *filter.MemberID)
	}
	if filter.APIKeyID != nil {
		add("l.api_key_id = $%d", *filter.APIKeyID)
	}
	if filter.GroupID != nil {
		add("l.group_id = $%d", *filter.GroupID)
	}
	if filter.BillingType != nil {
		add("l.billing_type = $%d", *filter.BillingType)
	}
	if filter.BillingMode != "" {
		add("COALESCE(l.billing_mode,CASE WHEN l.image_count>0 THEN 'image' ELSE 'token' END) = $%d", filter.BillingMode)
	}
	if filter.Model != "" {
		add("COALESCE(l.requested_model,l.model) = $%d", filter.Model)
	}
	if filter.Endpoint != "" {
		add("l.inbound_endpoint = $%d", filter.Endpoint)
	}
	if filter.Status != "" {
		add("COALESCE(l.billing_status,'charged') = $%d", filter.Status)
	}
	return strings.Join(conditions, " AND "), args, nil
}

func (r *organizationRepository) ListUsage(ctx context.Context, userID int64, filter service.OrganizationUsageFilter) ([]service.OrganizationUsageRow, int64, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM usage_logs l WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT l.id,l.user_id,COALESCE(u.login_name,u.email,''),COALESCE(u.username,''),COALESCE(k.name,''),
		       COALESCE(l.requested_model,l.model),l.input_tokens,l.output_tokens,l.cache_creation_tokens,l.cache_read_tokens,l.cache_creation_5m_tokens,l.cache_creation_1h_tokens,l.actual_cost::text,
		       l.total_cost::text,COALESCE(l.rate_multiplier,1)::float8,
		       COALESCE(l.inbound_endpoint,''),l.group_id,COALESCE(g.name,''),
		       CASE COALESCE(l.request_type,CASE WHEN l.stream THEN 2 ELSE 1 END) WHEN 1 THEN 'sync' WHEN 2 THEN 'stream' WHEN 3 THEN 'ws_v2' WHEN 4 THEN 'cyber' ELSE 'unknown' END,
		       l.billing_type,COALESCE(l.billing_mode,CASE WHEN l.image_count>0 THEN 'image' ELSE 'token' END),
		       l.image_count,COALESCE(l.image_urls,'[]'::jsonb),COALESCE(l.cos_url,'[]'::jsonb),COALESCE(l.ip_address,''),COALESCE(l.user_agent,''),
		       COALESCE(l.billing_status,'charged'),l.first_token_ms,l.duration_ms,l.created_at,
		       CASE WHEN l.balance_source='subscription' OR (l.billing_type=1 AND k.organization_subscription_id IS NOT NULL)
		            THEN 'subscription' ELSE COALESCE(l.balance_source,'self') END,
		       l.task_id
		FROM usage_logs l JOIN users u ON u.id=l.user_id LEFT JOIN api_keys k ON k.id=l.api_key_id LEFT JOIN groups g ON g.id=l.group_id
		WHERE %s ORDER BY l.created_at DESC,l.id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.OrganizationUsageRow, 0)
	for rows.Next() {
		var item service.OrganizationUsageRow
		var imageURLs, cosURLs []byte
		var taskID sql.NullInt64
		if err := rows.Scan(&item.ID, &item.MemberUserID, &item.MemberLogin, &item.MemberUsername, &item.APIKeyName, &item.Model, &item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheReadTokens, &item.CacheCreation5mTokens, &item.CacheCreation1hTokens, &item.ActualCost, &item.TotalCost, &item.RateMultiplier, &item.Endpoint, &item.GroupID, &item.GroupName, &item.RequestType, &item.BillingType, &item.BillingMode, &item.ImageCount, &imageURLs, &cosURLs, &item.IPAddress, &item.UserAgent, &item.Status, &item.FirstTokenMS, &item.DurationMS, &item.CreatedAt, &item.BalanceSource, &taskID); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(imageURLs, &item.ImageURLs)
		_ = json.Unmarshal(cosURLs, &item.CosURLs)
		if taskID.Valid {
			v := taskID.Int64
			item.TaskID = &v
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (r *organizationRepository) UsageStats(ctx context.Context, userID int64, filter service.OrganizationUsageFilter) (*service.OrganizationUsageStats, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	var out service.OrganizationUsageStats
	err = r.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(l.input_tokens),0),COALESCE(sum(l.output_tokens),0),
		COALESCE(sum(l.cache_creation_tokens),0),COALESCE(sum(l.cache_read_tokens),0),
		COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0),
		COALESCE(sum(l.actual_cost),0)::text FROM usage_logs l WHERE `+where, args...).Scan(
		&out.Requests, &out.InputTokens, &out.OutputTokens, &out.CacheCreationTokens, &out.CacheReadTokens, &out.TotalTokens, &out.ActualCost)
	return &out, err
}

func (r *organizationRepository) UsageTrend(ctx context.Context, userID int64, filter service.OrganizationUsageFilter) ([]service.OrganizationUsageTrendPoint, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	granularity := "day"
	if filter.Granularity == "hour" {
		granularity = "hour"
	}
	dateFormat := safeDateFormat(granularity)
	rows, err := r.db.QueryContext(ctx, `SELECT TO_CHAR(l.created_at,'`+dateFormat+`') AS bucket,count(*),
		COALESCE(sum(l.input_tokens),0),COALESCE(sum(l.output_tokens),0),COALESCE(sum(l.cache_creation_tokens),0),COALESCE(sum(l.cache_read_tokens),0),
		COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0),
		COALESCE(sum(l.total_cost),0)::float8,COALESCE(sum(l.actual_cost),0)::float8
		FROM usage_logs l WHERE `+where+` GROUP BY bucket ORDER BY bucket`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.OrganizationUsageTrendPoint, 0)
	for rows.Next() {
		var point service.OrganizationUsageTrendPoint
		if err := rows.Scan(&point.Date, &point.Requests, &point.InputTokens, &point.OutputTokens, &point.CacheCreationTokens, &point.CacheReadTokens, &point.TotalTokens, &point.Cost, &point.ActualCost); err != nil {
			return nil, err
		}
		out = append(out, point)
	}
	return out, rows.Err()
}

func (r *organizationRepository) UsageCharts(ctx context.Context, userID int64, filter service.OrganizationUsageFilter) (*service.OrganizationUsageCharts, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	trend, err := r.UsageTrend(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	out := &service.OrganizationUsageCharts{Trend: trend, Models: []service.OrganizationModelStat{}, Groups: []service.OrganizationGroupStat{}, Endpoints: []service.OrganizationEndpointStat{}}
	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(l.requested_model,l.model,''),count(*),COALESCE(sum(l.input_tokens),0),COALESCE(sum(l.output_tokens),0),COALESCE(sum(l.cache_creation_tokens),0),COALESCE(sum(l.cache_read_tokens),0),COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0),COALESCE(sum(l.total_cost),0)::float8,COALESCE(sum(l.actual_cost),0)::float8 FROM usage_logs l WHERE `+where+` GROUP BY 1 ORDER BY 7 DESC`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v service.OrganizationModelStat
		if err := rows.Scan(&v.Model, &v.Requests, &v.InputTokens, &v.OutputTokens, &v.CacheCreationTokens, &v.CacheReadTokens, &v.TotalTokens, &v.Cost, &v.ActualCost); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out.Models = append(out.Models, v)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = r.db.QueryContext(ctx, `SELECT COALESCE(l.group_id,0),COALESCE(g.name,''),count(*),COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0),COALESCE(sum(l.total_cost),0)::float8,COALESCE(sum(l.actual_cost),0)::float8 FROM usage_logs l LEFT JOIN groups g ON g.id=l.group_id WHERE `+where+` GROUP BY 1,2 ORDER BY 4 DESC`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v service.OrganizationGroupStat
		if err := rows.Scan(&v.GroupID, &v.GroupName, &v.Requests, &v.TotalTokens, &v.Cost, &v.ActualCost); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out.Groups = append(out.Groups, v)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = r.db.QueryContext(ctx, `SELECT COALESCE(NULLIF(l.inbound_endpoint,''),'unknown'),count(*),COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0),COALESCE(sum(l.total_cost),0)::float8,COALESCE(sum(l.actual_cost),0)::float8 FROM usage_logs l WHERE `+where+` GROUP BY 1 ORDER BY 3 DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v service.OrganizationEndpointStat
		if err := rows.Scan(&v.Endpoint, &v.Requests, &v.TotalTokens, &v.Cost, &v.ActualCost); err != nil {
			return nil, err
		}
		out.Endpoints = append(out.Endpoints, v)
	}
	return out, rows.Err()
}

func (r *organizationRepository) OrganizationDashboard(ctx context.Context, userID int64) (*usagestats.DashboardStats, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() || (!org.Owner() && !org.HasAction(service.ActionFinanceBalanceRead)) {
		return nil, service.ErrOrganizationPermission
	}

	stats := &usagestats.DashboardStats{}
	now := time.Now().UTC()
	todayStart := now.Truncate(24 * time.Hour)
	if org.EffectiveAt.After(todayStart) {
		todayStart = org.EffectiveAt
	}
	// 主账号(owner) 使用个人余额/个人套餐的消费不应计入企业统计。
	// 使用 usage_logs 别名 l 时统一附加此过滤条件。
	excludeOwnerSelfSpend := "(l.user_id <> $3 OR (l.balance_source IS NOT NULL AND l.balance_source <> 'self') OR EXISTS(SELECT 1 FROM api_keys ak WHERE ak.id=l.api_key_id AND ak.organization_subscription_id IS NOT NULL))"
	var totalIAMUsers, activeIAMUsers int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE created_at >= $2),
			count(*) FILTER (WHERE role='member'),
			count(*) FILTER (WHERE role='member' AND status='active')
		FROM organization_memberships
		WHERE organization_id=$1 AND status<>'archived'`, org.OrganizationID, todayStart).
		Scan(&stats.TotalUsers, &stats.TodayNewUsers, &totalIAMUsers, &activeIAMUsers); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE k.status='active')
		FROM api_keys k JOIN organization_memberships m ON m.user_id=k.user_id
		WHERE m.organization_id=$1 AND m.status<>'archived' AND k.deleted_at IS NULL`, org.OrganizationID).
		Scan(&stats.TotalAPIKeys, &stats.ActiveAPIKeys); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `
		WITH used_accounts AS (
			SELECT DISTINCT l.account_id FROM usage_logs l
			WHERE l.organization_id=$1 AND l.created_at >= $2 AND l.account_id IS NOT NULL
			  AND `+excludeOwnerSelfSpend+`
		)
		SELECT count(*),
			count(*) FILTER (WHERE a.status='active' AND a.schedulable=true),
			count(*) FILTER (WHERE a.status='error'),
			count(*) FILTER (WHERE a.rate_limited_at IS NOT NULL AND a.rate_limit_reset_at > $4),
			count(*) FILTER (WHERE a.overload_until IS NOT NULL AND a.overload_until > $4)
		FROM accounts a JOIN used_accounts ua ON ua.account_id=a.id
		WHERE a.deleted_at IS NULL`, org.OrganizationID, org.EffectiveAt, org.OwnerUserID, now).
		Scan(&stats.TotalAccounts, &stats.NormalAccounts, &stats.ErrorAccounts, &stats.RateLimitAccounts, &stats.OverloadAccounts); err != nil {
		return nil, err
	}
	stats.TotalAccounts += totalIAMUsers
	stats.NormalAccounts += activeIAMUsers
	if err := r.db.QueryRowContext(ctx, `
		SELECT count(*), COALESCE(sum(l.input_tokens),0), COALESCE(sum(l.output_tokens),0),
			COALESCE(sum(l.cache_creation_tokens),0), COALESCE(sum(l.cache_read_tokens),0),
			COALESCE(sum(l.total_cost),0)::float8, COALESCE(sum(l.actual_cost),0)::float8,
			COALESCE(sum(COALESCE(l.account_stats_cost,l.total_cost)*COALESCE(l.account_rate_multiplier,1)),0)::float8,
			COALESCE(avg(l.duration_ms),0)::float8
		FROM usage_logs l WHERE l.organization_id=$1 AND l.created_at >= $2 AND `+excludeOwnerSelfSpend, org.OrganizationID, org.EffectiveAt, org.OwnerUserID).
		Scan(&stats.TotalRequests, &stats.TotalInputTokens, &stats.TotalOutputTokens,
			&stats.TotalCacheCreationTokens, &stats.TotalCacheReadTokens, &stats.TotalCost,
			&stats.TotalActualCost, &stats.TotalAccountCost, &stats.AverageDurationMs); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT count(*), count(DISTINCT l.user_id), COALESCE(sum(l.input_tokens),0), COALESCE(sum(l.output_tokens),0),
			COALESCE(sum(l.cache_creation_tokens),0), COALESCE(sum(l.cache_read_tokens),0),
			COALESCE(sum(l.total_cost),0)::float8, COALESCE(sum(l.actual_cost),0)::float8,
			COALESCE(sum(COALESCE(l.account_stats_cost,l.total_cost)*COALESCE(l.account_rate_multiplier,1)),0)::float8
		FROM usage_logs l WHERE l.organization_id=$1 AND l.created_at >= $2 AND `+excludeOwnerSelfSpend, org.OrganizationID, todayStart, org.OwnerUserID).
		Scan(&stats.TodayRequests, &stats.ActiveUsers, &stats.TodayInputTokens, &stats.TodayOutputTokens,
			&stats.TodayCacheCreationTokens, &stats.TodayCacheReadTokens, &stats.TodayCost,
			&stats.TodayActualCost, &stats.TodayAccountCost); err != nil {
		return nil, err
	}
	stats.TotalTokens = stats.TotalInputTokens + stats.TotalOutputTokens + stats.TotalCacheCreationTokens + stats.TotalCacheReadTokens
	stats.TodayTokens = stats.TodayInputTokens + stats.TodayOutputTokens + stats.TodayCacheCreationTokens + stats.TodayCacheReadTokens

	var recentRequests, recentTokens int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT count(*), COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0)
		FROM usage_logs l WHERE l.organization_id=$1 AND l.created_at >= $2 AND `+excludeOwnerSelfSpend, org.OrganizationID, now.Add(-5*time.Minute), org.OwnerUserID).
		Scan(&recentRequests, &recentTokens); err != nil {
		return nil, err
	}
	stats.Rpm = recentRequests / 5
	stats.Tpm = recentTokens / 5
	return stats, nil
}

func (r *organizationRepository) SearchOrganizationAPIKeys(ctx context.Context, userID int64, memberID *int64, query string, limit int) ([]service.OrganizationAPIKeyOption, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() || (!org.Owner() && !org.HasAction(service.ActionFinanceBalanceRead)) {
		return nil, service.ErrOrganizationPermission
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	conditions := []string{"m.organization_id=$1", "m.status<>'archived'", "k.deleted_at IS NULL"}
	args := []any{org.OrganizationID}
	if memberID != nil {
		args = append(args, *memberID)
		conditions = append(conditions, fmt.Sprintf("k.user_id=$%d", len(args)))
	}
	if value := strings.TrimSpace(query); value != "" {
		args = append(args, "%"+value+"%")
		conditions = append(conditions, fmt.Sprintf("k.name ILIKE $%d", len(args)))
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT k.id,k.name FROM api_keys k
		JOIN organization_memberships m ON m.user_id=k.user_id
		WHERE %s ORDER BY k.name,k.id LIMIT $%d`, strings.Join(conditions, " AND "), len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.OrganizationAPIKeyOption, 0)
	for rows.Next() {
		var item service.OrganizationAPIKeyOption
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *organizationRepository) OrganizationSpendingRanking(ctx context.Context, userID int64, filter service.OrganizationUsageFilter, limit int) (*usagestats.UserSpendingRankingResponse, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 12
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		WITH user_spend AS (
			SELECT l.user_id, COALESCE(u.username,'') AS username, COALESCE(u.email,'') AS email,
				COALESCE(u.login_name,'') AS login_name, COALESCE(u.identity_type,'') AS identity_type,
				COALESCE(o.company_id,'') AS company_id,
				COALESCE(sum(l.actual_cost),0)::float8 AS actual_cost, count(*) AS requests,
				COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0) AS tokens
			FROM usage_logs l
			JOIN users u ON u.id=l.user_id
			LEFT JOIN organizations o ON o.id=l.organization_id
			WHERE %s
			GROUP BY l.user_id,u.username,u.email,u.login_name,u.identity_type,o.company_id
		), ranked AS (
			SELECT *, sum(actual_cost) OVER () AS total_actual_cost, sum(requests) OVER () AS total_requests,
				sum(tokens) OVER () AS total_tokens FROM user_spend
			ORDER BY actual_cost DESC,tokens DESC,user_id LIMIT $%d
		)
		SELECT user_id,username,email,login_name,identity_type,company_id,
			actual_cost,requests,tokens,total_actual_cost,total_requests,total_tokens
		FROM ranked ORDER BY actual_cost DESC,tokens DESC,user_id`, where, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := &usagestats.UserSpendingRankingResponse{Ranking: []usagestats.UserSpendingRankingItem{}}
	for rows.Next() {
		var item usagestats.UserSpendingRankingItem
		var username, email, loginName, identityType, companyID string
		if err := rows.Scan(&item.UserID, &username, &email, &loginName, &identityType, &companyID,
			&item.ActualCost, &item.Requests, &item.Tokens,
			&result.TotalActualCost, &result.TotalRequests, &result.TotalTokens); err != nil {
			return nil, err
		}
		item.Email = organizationRankingUserLabel(username, email, loginName, identityType, companyID)
		item.Username = username
		item.LoginName = loginName
		result.Ranking = append(result.Ranking, item)
	}
	return result, rows.Err()
}

func organizationRankingUserLabel(username, email, loginName, identityType, companyID string) string {
	if value := strings.TrimSpace(username); value != "" {
		return value
	}
	if identityType == service.IdentityTypeIAM && strings.TrimSpace(loginName) != "" && strings.TrimSpace(companyID) != "" {
		return service.CanonicalIAMPrincipal(loginName, companyID)
	}
	if value := strings.TrimSpace(email); value != "" {
		return value
	}
	return strings.TrimSpace(loginName)
}

func (r *organizationRepository) OrganizationUsersTrend(ctx context.Context, userID int64, filter service.OrganizationUsageFilter, limit int) ([]usagestats.UserUsageTrendPoint, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 12
	}
	args = append(args, limit)
	dateFormat := safeDateFormat(filter.Granularity)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		WITH top_users AS (
			SELECT l.user_id FROM usage_logs l WHERE %s
			GROUP BY l.user_id ORDER BY sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens) DESC
			LIMIT $%d
		)
		SELECT TO_CHAR(l.created_at,'%s'),l.user_id,COALESCE(u.email,''),COALESCE(u.username,''),count(*),
			COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0),
			COALESCE(sum(l.total_cost),0)::float8,COALESCE(sum(l.actual_cost),0)::float8
		FROM usage_logs l JOIN users u ON u.id=l.user_id
		WHERE %s AND l.user_id IN (SELECT user_id FROM top_users)
		GROUP BY 1,l.user_id,u.email,u.username ORDER BY 1,6 DESC`, where, len(args), dateFormat, where), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]usagestats.UserUsageTrendPoint, 0)
	for rows.Next() {
		var item usagestats.UserUsageTrendPoint
		if err := rows.Scan(&item.Date, &item.UserID, &item.Email, &item.Username, &item.Requests, &item.Tokens, &item.Cost, &item.ActualCost); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *organizationRepository) OrganizationUserBreakdown(ctx context.Context, userID int64, filter service.OrganizationUsageFilter, limit int) ([]usagestats.UserBreakdownItem, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT l.user_id,COALESCE(NULLIF(u.username,''),NULLIF(u.email,''),NULLIF(u.login_name,''),''),count(*),
			COALESCE(sum(l.input_tokens),0),COALESCE(sum(l.output_tokens),0),
			COALESCE(sum(l.cache_creation_tokens+l.cache_read_tokens),0),
			COALESCE(sum(l.input_tokens+l.output_tokens+l.cache_creation_tokens+l.cache_read_tokens),0),
			COALESCE(sum(l.total_cost),0)::float8,COALESCE(sum(l.actual_cost),0)::float8,
			COALESCE(sum(COALESCE(l.account_stats_cost,l.total_cost)*COALESCE(l.account_rate_multiplier,1)),0)::float8
		FROM usage_logs l JOIN users u ON u.id=l.user_id WHERE %s
		GROUP BY l.user_id,u.username,u.email,u.login_name ORDER BY 9 DESC,7 DESC LIMIT $%d`, where, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]usagestats.UserBreakdownItem, 0)
	for rows.Next() {
		var item usagestats.UserBreakdownItem
		if err := rows.Scan(&item.UserID, &item.Email, &item.Requests, &item.InputTokens, &item.OutputTokens,
			&item.CacheTokens, &item.TotalTokens, &item.Cost, &item.ActualCost, &item.AccountCost); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanSpendLimitRule(row interface{ Scan(...any) error }) (*service.OrganizationSpendLimitRule, error) {
	var rule service.OrganizationSpendLimitRule
	var daily, monthly sql.NullString
	if err := row.Scan(&rule.ID, &rule.OrganizationID, &rule.MemberUserID, &rule.MemberLogin, &rule.MemberUsername, &daily, &monthly,
		&rule.AlertEnabled, &rule.AlertThresholdPct, pq.Array(&rule.AdditionalRecipients), &rule.Revision,
		&rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return nil, err
	}
	if daily.Valid {
		rule.DailyLimitUSD = &daily.String
	}
	if monthly.Valid {
		rule.MonthlyLimitUSD = &monthly.String
	}
	if rule.AdditionalRecipients == nil {
		rule.AdditionalRecipients = []string{}
	}
	return &rule, nil
}

const spendLimitRuleSelect = `
	SELECT l.id,l.organization_id,l.member_user_id,COALESCE(u.login_name,''),COALESCE(u.username,''),
	       l.daily_limit_usd::text,l.monthly_limit_usd::text,l.alert_enabled,
	       l.alert_threshold_pct::float8,l.additional_recipients,l.revision,l.created_at,l.updated_at
	FROM organization_member_spend_limits l
	LEFT JOIN users u ON u.id=l.member_user_id`

func (r *organizationRepository) ListSpendLimitRules(ctx context.Context, actorID int64) ([]service.OrganizationSpendLimitRule, error) {
	org, err := r.GetContextForUser(ctx, actorID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	if !org.Owner() && !org.HasAction(service.ActionSpendLimitManage) {
		return nil, service.ErrOrganizationPermission
	}
	rows, err := r.db.QueryContext(ctx, spendLimitRuleSelect+` WHERE l.organization_id=$1 ORDER BY l.member_user_id NULLS FIRST,l.id`, org.OrganizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	rules := make([]service.OrganizationSpendLimitRule, 0)
	for rows.Next() {
		rule, scanErr := scanSpendLimitRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		rules = append(rules, *rule)
	}
	return rules, rows.Err()
}

func (r *organizationRepository) UpsertSpendLimitRules(ctx context.Context, ownerID int64, memberIDs []int64, daily, monthly *string, alertEnabled bool, threshold float64, recipients []string) (_ []service.OrganizationSpendLimitRule, err error) {
	if recipients == nil {
		recipients = []string{}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	organizationID, err := resolveOrganizationForActor(ctx, tx, ownerID, service.ActionSpendLimitManage, true)
	if err != nil {
		return nil, err
	}
	if len(memberIDs) > 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1 AND user_id=ANY($2) AND role='member' AND status<>'archived'`, organizationID, pq.Array(memberIDs)).Scan(&count); err != nil {
			return nil, err
		}
		if count != len(memberIDs) {
			return nil, service.ErrIAMMemberNotFound
		}
	}
	targets := make([]*int64, 0, len(memberIDs)+1)
	if len(memberIDs) == 0 {
		targets = append(targets, nil)
	} else {
		for i := range memberIDs {
			targets = append(targets, &memberIDs[i])
		}
	}
	for _, memberID := range targets {
		var query string
		if memberID == nil {
			query = `INSERT INTO organization_member_spend_limits(organization_id,member_user_id,daily_limit_usd,monthly_limit_usd,alert_enabled,alert_threshold_pct,additional_recipients)
				VALUES($1,NULL,$2::numeric,$3::numeric,$4,$5,$6)
				ON CONFLICT (organization_id) WHERE member_user_id IS NULL DO UPDATE SET
				daily_limit_usd=EXCLUDED.daily_limit_usd,monthly_limit_usd=EXCLUDED.monthly_limit_usd,
				alert_enabled=EXCLUDED.alert_enabled,alert_threshold_pct=EXCLUDED.alert_threshold_pct,
				additional_recipients=EXCLUDED.additional_recipients,revision=organization_member_spend_limits.revision+1,updated_at=NOW()`
		} else {
			query = `INSERT INTO organization_member_spend_limits(organization_id,member_user_id,daily_limit_usd,monthly_limit_usd,alert_enabled,alert_threshold_pct,additional_recipients)
				VALUES($1,$7,$2::numeric,$3::numeric,$4,$5,$6)
				ON CONFLICT (organization_id,member_user_id) WHERE member_user_id IS NOT NULL DO UPDATE SET
				daily_limit_usd=EXCLUDED.daily_limit_usd,monthly_limit_usd=EXCLUDED.monthly_limit_usd,
				alert_enabled=EXCLUDED.alert_enabled,alert_threshold_pct=EXCLUDED.alert_threshold_pct,
				additional_recipients=EXCLUDED.additional_recipients,revision=organization_member_spend_limits.revision+1,updated_at=NOW()`
		}
		args := []any{organizationID, daily, monthly, alertEnabled, threshold, pq.Array(recipients)}
		if memberID != nil {
			args = append(args, *memberID)
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,'spend_limit.upsert','success',$4,jsonb_build_object('scope',CASE WHEN $3::bigint IS NULL THEN 'all' ELSE 'member' END))`, organizationID, ownerID, memberID, organizationCorrelationID(ctx)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.ListSpendLimitRules(ctx, ownerID)
}

func (r *organizationRepository) DeleteSpendLimitRule(ctx context.Context, ownerID int64, memberID *int64) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	organizationID, err := resolveOrganizationForActor(ctx, tx, ownerID, service.ActionSpendLimitManage, false)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM organization_member_spend_limits WHERE organization_id=$1 AND member_user_id IS NOT DISTINCT FROM $2`, organizationID, memberID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return service.ErrIAMMemberNotFound
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id) VALUES($1,$2,$3,'spend_limit.delete','success',$4)`, organizationID, ownerID, memberID, organizationCorrelationID(ctx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *organizationRepository) ListSpendLimitUsage(ctx context.Context, actorID int64) ([]service.OrganizationSpendUsage, error) {
	org, err := r.GetContextForUser(ctx, actorID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.user_id,COALESCE(NULLIF(u.login_name,''),NULLIF(u.username,''),COALESCE(u.email,'')),
		       COALESCE(u.username,''),
		       COALESCE((SELECT GREATEST(sum(CASE WHEN l.billing_status='refunded' THEN -abs(l.actual_cost) ELSE l.actual_cost END),0)::text
		                 FROM usage_logs l WHERE l.organization_id=m.organization_id AND l.user_id=m.user_id
		                   AND l.balance_source IN ('company','shared','subscription')
		                   AND l.created_at >= date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),'0'),
		       COALESCE((SELECT GREATEST(sum(CASE WHEN l.billing_status='refunded' THEN -abs(l.actual_cost) ELSE l.actual_cost END),0)::text
		                 FROM usage_logs l WHERE l.organization_id=m.organization_id AND l.user_id=m.user_id
		                   AND l.balance_source IN ('company','shared','subscription')
		                   AND l.created_at >= date_trunc('month',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),'0'),
		       rule.daily_limit_usd::text,rule.monthly_limit_usd::text
		FROM organization_memberships m JOIN users u ON u.id=m.user_id
		LEFT JOIN LATERAL (
			SELECT l.daily_limit_usd,l.monthly_limit_usd
			FROM organization_member_spend_limits l
			WHERE l.organization_id=m.organization_id AND (l.member_user_id=m.user_id OR l.member_user_id IS NULL)
			ORDER BY (l.member_user_id IS NOT NULL) DESC LIMIT 1
		) rule ON TRUE
			WHERE m.organization_id=$1 AND m.role='member' AND m.status<>'archived' AND ($2 OR m.user_id=$3)
			ORDER BY m.user_id`, org.OrganizationID, org.Owner() || org.HasAction(service.ActionSpendLimitManage), actorID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.OrganizationSpendUsage, 0)
	for rows.Next() {
		var item service.OrganizationSpendUsage
		var daily, monthly sql.NullString
		if err := rows.Scan(&item.MemberUserID, &item.MemberLogin, &item.MemberUsername, &item.DailyUsedUSD, &item.MonthlyUsedUSD, &daily, &monthly); err != nil {
			return nil, err
		}
		if daily.Valid {
			item.DailyLimitUSD = &daily.String
		}
		if monthly.Valid {
			item.MonthlyLimitUSD = &monthly.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *organizationRepository) CheckOrganizationSpendLimit(ctx context.Context, consumerUserID int64, balanceSource string, amount float64) error {
	if amount < 0 || (balanceSource != service.BalanceSourceCompany && balanceSource != service.BalanceSourceLegacyShared && balanceSource != service.BalanceSourceSubscription) {
		return nil
	}
	var dailyLimit, monthlyLimit, dailyUsed, monthlyUsed sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT rule.daily_limit_usd::text,rule.monthly_limit_usd::text,
		       COALESCE((SELECT GREATEST(sum(CASE WHEN l.billing_status='refunded' THEN -abs(l.actual_cost) ELSE l.actual_cost END),0)::text
		                 FROM usage_logs l WHERE l.organization_id=m.organization_id AND l.user_id=m.user_id
		                   AND l.balance_source IN ('company','shared','subscription')
		                   AND l.created_at >= date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),'0'),
		       COALESCE((SELECT GREATEST(sum(CASE WHEN l.billing_status='refunded' THEN -abs(l.actual_cost) ELSE l.actual_cost END),0)::text
		                 FROM usage_logs l WHERE l.organization_id=m.organization_id AND l.user_id=m.user_id
		                   AND l.balance_source IN ('company','shared','subscription')
		                   AND l.created_at >= date_trunc('month',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),'0')
		FROM organization_memberships m JOIN organizations o ON o.id=m.organization_id
		JOIN LATERAL (
			SELECT l.daily_limit_usd,l.monthly_limit_usd FROM organization_member_spend_limits l
			WHERE l.organization_id=m.organization_id AND (l.member_user_id=m.user_id OR l.member_user_id IS NULL)
			ORDER BY (l.member_user_id IS NOT NULL) DESC LIMIT 1
		) rule ON TRUE
		WHERE m.user_id=$1 AND m.role='member' AND m.status='active' AND o.status='active'`, consumerUserID).
		Scan(&dailyLimit, &monthlyLimit, &dailyUsed, &monthlyUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	charge := decimal.NewFromFloat(amount)
	exceeds := func(limit, used sql.NullString) (bool, error) {
		if !limit.Valid {
			return false, nil
		}
		limitAmount, parseErr := decimal.NewFromString(limit.String)
		if parseErr != nil {
			return false, parseErr
		}
		usedAmount := decimal.Zero
		if used.Valid {
			usedAmount, parseErr = decimal.NewFromString(used.String)
			if parseErr != nil {
				return false, parseErr
			}
		}
		if charge.IsZero() {
			return usedAmount.GreaterThanOrEqual(limitAmount), nil
		}
		return usedAmount.Add(charge).GreaterThan(limitAmount), nil
	}
	dailyExceeded, err := exceeds(dailyLimit, dailyUsed)
	if err != nil {
		return err
	}
	monthlyExceeded, err := exceeds(monthlyLimit, monthlyUsed)
	if err != nil {
		return err
	}
	switch {
	case dailyExceeded && monthlyExceeded:
		return service.ErrDailyAndMonthlySpendLimitExceeded
	case dailyExceeded:
		return service.ErrDailySpendLimitExceeded
	case monthlyExceeded:
		return service.ErrMonthlySpendLimitExceeded
	}
	return nil
}

func (r *organizationRepository) RecordSpendLimitAlert(ctx context.Context, consumerUserID int64, balanceSource string) (err error) {
	if balanceSource != service.BalanceSourceCompany && balanceSource != service.BalanceSourceLegacyShared && balanceSource != service.BalanceSourceSubscription {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var organizationID, revision int64
	var companyName, memberLogin, memberEmail string
	var alertEnabled bool
	var threshold float64
	var recipients []string
	var dailyLimit, monthlyLimit sql.NullString
	var dailyUsed, monthlyUsed string
	var dailyStart, monthlyStart time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT o.id,o.name,COALESCE(NULLIF(u.login_name,''),NULLIF(u.username,''),COALESCE(u.email,'')),
		       COALESCE(CASE WHEN u.recovery_email_verified_at IS NOT NULL THEN NULLIF(u.recovery_email,'') END,NULLIF(u.email,''),''),
		       rule.alert_enabled,rule.alert_threshold_pct::float8,rule.additional_recipients,rule.revision,
		       rule.daily_limit_usd::text,rule.monthly_limit_usd::text,
		       COALESCE((SELECT GREATEST(sum(CASE WHEN l.billing_status='refunded' THEN -abs(l.actual_cost) ELSE l.actual_cost END),0)::text
		                 FROM usage_logs l WHERE l.organization_id=m.organization_id AND l.user_id=m.user_id
		                   AND l.balance_source IN ('company','shared','subscription')
		                   AND l.created_at >= date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),'0'),
		       COALESCE((SELECT GREATEST(sum(CASE WHEN l.billing_status='refunded' THEN -abs(l.actual_cost) ELSE l.actual_cost END),0)::text
		                 FROM usage_logs l WHERE l.organization_id=m.organization_id AND l.user_id=m.user_id
		                   AND l.balance_source IN ('company','shared','subscription')
		                   AND l.created_at >= date_trunc('month',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'),'0'),
		       date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
		       date_trunc('month',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		FROM organization_memberships m JOIN organizations o ON o.id=m.organization_id JOIN users u ON u.id=m.user_id
		JOIN LATERAL (
			SELECT l.* FROM organization_member_spend_limits l
			WHERE l.organization_id=m.organization_id AND (l.member_user_id=m.user_id OR l.member_user_id IS NULL)
			ORDER BY (l.member_user_id IS NOT NULL) DESC LIMIT 1
		) rule ON TRUE
		WHERE m.user_id=$1 AND m.role='member'`, consumerUserID).
		Scan(&organizationID, &companyName, &memberLogin, &memberEmail, &alertEnabled, &threshold, pq.Array(&recipients), &revision,
			&dailyLimit, &monthlyLimit, &dailyUsed, &monthlyUsed, &dailyStart, &monthlyStart)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !alertEnabled {
		return nil
	}
	recipientSet := make(map[string]struct{}, len(recipients)+1)
	if memberEmail = strings.ToLower(strings.TrimSpace(memberEmail)); memberEmail != "" {
		recipientSet[memberEmail] = struct{}{}
	}
	for _, recipient := range recipients {
		if recipient = strings.ToLower(strings.TrimSpace(recipient)); recipient != "" {
			recipientSet[recipient] = struct{}{}
		}
	}
	type periodUsage struct {
		name   string
		used   string
		limit  sql.NullString
		window time.Time
	}
	periods := []periodUsage{{name: "daily", used: dailyUsed, limit: dailyLimit, window: dailyStart}, {name: "monthly", used: monthlyUsed, limit: monthlyLimit, window: monthlyStart}}
	for _, period := range periods {
		if !period.limit.Valid {
			continue
		}
		used, parseErr := decimal.NewFromString(period.used)
		if parseErr != nil {
			return parseErr
		}
		limit, parseErr := decimal.NewFromString(period.limit.String)
		if parseErr != nil {
			return parseErr
		}
		trigger := limit.Mul(decimal.NewFromFloat(threshold)).Div(decimal.NewFromInt(100))
		if used.LessThan(trigger) {
			continue
		}
		variables := map[string]string{
			"company_name": companyName, "member_name": memberLogin, "period": period.name,
			"used": used.StringFixed(10), "limit": limit.StringFixed(10), "threshold": decimal.NewFromFloat(threshold).String(),
		}
		for recipient := range recipientSet {
			dedup := fmt.Sprintf("company.spend_limit.alert:%d:%d:%d:%s:%s:%s", organizationID, consumerUserID, revision, period.name, period.window.UTC().Format(time.RFC3339), recipient)
			if err := enqueueNotification(ctx, tx, service.NotificationEmailEventCompanySpendLimitAlert, dedup, recipient, variables); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (r *organizationRepository) Reconcile(ctx context.Context) (map[string]int64, error) {
	checks := map[string]string{
		"pending_reservation_mismatch": `SELECT count(*) FROM company_upgrade_applications a WHERE a.status='pending' AND (SELECT count(*) FROM organization_financial_ledger l WHERE l.application_id=a.id AND l.kind='upgrade_reserve' AND l.amount=a.fee_amount AND l.currency=a.fee_currency) <> 1`,
		"pending_frozen_shortfall":     `SELECT count(*) FROM (SELECT a.applicant_user_id,sum(a.fee_amount) AS reserved FROM company_upgrade_applications a WHERE a.status='pending' GROUP BY a.applicant_user_id) p JOIN users u ON u.id=p.applicant_user_id WHERE u.frozen_balance < p.reserved`,
		"upgrade_settlement_mismatch": `SELECT count(*) FROM company_upgrade_applications a WHERE
			(a.status='approved' AND (SELECT count(*) FROM organization_financial_ledger l WHERE l.application_id=a.id AND l.kind='upgrade_capture' AND l.amount=a.fee_amount AND l.currency=a.fee_currency) <> 1) OR
			(a.status IN ('rejected','withdrawn') AND (SELECT count(*) FROM organization_financial_ledger l WHERE l.application_id=a.id AND l.kind='upgrade_release' AND l.amount=a.fee_amount AND l.currency=a.fee_currency) <> 1) OR
			(a.status='pending' AND EXISTS(SELECT 1 FROM organization_financial_ledger l WHERE l.application_id=a.id AND l.kind IN ('upgrade_capture','upgrade_release')))`,
		"owner_cardinality_violation":     `SELECT count(*) FROM organizations o WHERE (SELECT count(*) FROM organization_memberships m WHERE m.organization_id=o.id AND m.role='owner') <> 1`,
		"member_limit_violation":          `SELECT count(*) FROM organizations o WHERE (SELECT count(*) FROM organization_memberships m WHERE m.organization_id=o.id AND m.role='member' AND m.status<>'archived') > o.member_limit`,
		"transfer_conservation_violation": `SELECT count(*) FROM organization_financial_ledger WHERE kind IN ('allocate','reclaim') AND (source_user_id IS NULL OR destination_user_id IS NULL OR source_user_id=destination_user_id OR source_balance_after IS NULL OR destination_balance_after IS NULL OR amount <= 0)`,
		"missing_usage_payer_snapshots":   `SELECT count(*) FROM usage_logs WHERE organization_id IS NOT NULL AND (payer_user_id IS NULL OR balance_source IS NULL)`,
		"missing_async_payer_snapshots":   `SELECT count(*) FROM async_media_tasks WHERE organization_id IS NOT NULL AND (payer_user_id IS NULL OR balance_source IS NULL)`,
		"missing_batch_payer_snapshots":   `SELECT count(*) FROM batch_image_jobs WHERE organization_id IS NOT NULL AND (payer_user_id IS NULL OR balance_source IS NULL)`,
		"oldest_review_queue_age_seconds": `SELECT COALESCE(EXTRACT(EPOCH FROM (NOW()-min(created_at)))::bigint,0) FROM company_upgrade_applications WHERE status='pending'`,
	}
	out := make(map[string]int64, len(checks))
	for name, query := range checks {
		var count int64
		if err := r.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		out[name] = count
	}
	return out, nil
}

func (r *organizationRepository) ListOrganizationUserIDs(ctx context.Context, organizationID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id FROM organization_memberships WHERE organization_id=$1 ORDER BY user_id`, organizationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		out = append(out, userID)
	}
	return out, rows.Err()
}

// GetOrganizationSettings returns the current settings row for the caller's
// organization. Access allowed to owner or holder of CompanyFinanceManage.
// When no row exists yet, defaults (AutoSwitchSubscription=true) are returned.
func (r *organizationRepository) GetOrganizationSettings(ctx context.Context, actorID int64) (*service.OrganizationSettings, error) {
	orgID, err := resolveOrganizationForActor(ctx, r.db, actorID, service.ActionSubscriptionManage, false)
	if err != nil {
		// Fall back to CompanyFinanceManage — either policy grants visibility.
		orgID, err = resolveOrganizationForActor(ctx, r.db, actorID, service.ActionFinanceBalanceRead, false)
		if err != nil {
			return nil, err
		}
	}
	return r.loadOrganizationSettings(ctx, orgID)
}

// UpsertOrganizationSettings persists the settings for the caller's org.
// Requires owner or CompanyFinanceManage (which bundles subscription.manage).
func (r *organizationRepository) UpsertOrganizationSettings(ctx context.Context, actorID int64, settings service.OrganizationSettings) (*service.OrganizationSettings, error) {
	orgID, err := resolveOrganizationForActor(ctx, r.db, actorID, service.ActionSubscriptionManage, false)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO organization_settings (organization_id, auto_switch_subscription)
		VALUES ($1, $2)
		ON CONFLICT (organization_id) DO UPDATE SET auto_switch_subscription = EXCLUDED.auto_switch_subscription, updated_at = NOW()`,
		orgID, settings.AutoSwitchSubscription); err != nil {
		return nil, err
	}
	return r.loadOrganizationSettings(ctx, orgID)
}

// GetOrganizationSettingsByID is a hot-path variant used from the auth
// middleware; it skips user ACL checks because the caller was authenticated by
// its API key and only needs to know whether auto-fallback is enabled.
func (r *organizationRepository) GetOrganizationSettingsByID(ctx context.Context, organizationID int64) (*service.OrganizationSettings, error) {
	return r.loadOrganizationSettings(ctx, organizationID)
}

func (r *organizationRepository) loadOrganizationSettings(ctx context.Context, organizationID int64) (*service.OrganizationSettings, error) {
	out := &service.OrganizationSettings{OrganizationID: organizationID, AutoSwitchSubscription: true}
	err := r.db.QueryRowContext(ctx, `SELECT organization_id, auto_switch_subscription, created_at, updated_at
		FROM organization_settings WHERE organization_id=$1`, organizationID).
		Scan(&out.OrganizationID, &out.AutoSwitchSubscription, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// No row yet → return defaults so the UI can render the switch.
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListFallbackCandidateSubscriptions returns other active org subscriptions on
// the same platform as `currentSubscriptionID`, sorted by starts_at ASC (D1:
// earliest first — consume older plans before newer ones). The current
// subscription itself is excluded.
func (r *organizationRepository) ListFallbackCandidateSubscriptions(ctx context.Context, organizationID, currentSubscriptionID int64) ([]service.OrganizationSubscription, error) {
	// First resolve the current subscription's platform. If the current
	// subscription is missing / soft-deleted we still allow enumerating any
	// active org subscription; the middleware will pick the first usable one.
	var platform string
	if err := r.db.QueryRowContext(ctx, `SELECT g.platform FROM organization_subscriptions s JOIN groups g ON g.id=s.group_id WHERE s.id=$1 AND s.organization_id=$2`, currentSubscriptionID, organizationID).Scan(&platform); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	query := `SELECT ` + organizationSubscriptionSelectColumns + `
		FROM organization_subscriptions s JOIN groups g ON g.id=s.group_id
		WHERE s.organization_id=$1 AND s.id <> $2 AND s.deleted_at IS NULL
		  AND s.status='active' AND s.expires_at > NOW()
		  AND g.status='active' AND g.deleted_at IS NULL`
	args := []any{organizationID, currentSubscriptionID}
	if platform != "" {
		query += ` AND g.platform=$3`
		args = append(args, platform)
	}
	query += ` ORDER BY s.starts_at ASC, s.id ASC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.OrganizationSubscription, 0)
	for rows.Next() {
		s, err := scanOrganizationSubscription(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ResolveNextOrganizationSubscription walks the candidate chain in order and
// returns the first plan that is still under its daily / weekly / monthly
// limits. Returns ErrOrgSubscriptionNotFound when nothing qualifies. The
// returned runtime carries the target subscription's group id and limits so
// the caller can rebind the request in-memory (C1 semantics: no DB write).
func (r *organizationRepository) ResolveNextOrganizationSubscription(ctx context.Context, organizationID, currentSubscriptionID int64) (*service.OrgSubscriptionRuntime, error) {
	candidates, err := r.ListFallbackCandidateSubscriptions(ctx, organizationID, currentSubscriptionID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		runtime, err := r.GetOrganizationSubscriptionForBilling(ctx, candidate.ID)
		if err != nil {
			continue
		}
		if !runtime.IsActive() {
			continue
		}
		daily, weekly, monthly := runtime.CheckAllLimits(0)
		if !daily || !weekly || !monthly {
			continue
		}
		return runtime, nil
	}
	return nil, service.ErrOrgSubscriptionNotFound
}

func isConstraintNamed(err error, name string) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return strings.Contains(strings.ToLower(pqErr.Constraint), strings.ToLower(name))
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), strings.ToLower(name))
}
