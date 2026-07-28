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

func NewOrganizationRepository(db *sql.DB) service.OrganizationRepository {
	return &organizationRepository{db: db}
}

func (r *organizationRepository) GetContextForUser(ctx context.Context, userID int64) (*service.OrganizationContext, error) {
	const query = `
		SELECT o.id, o.account_id, o.owner_user_id, o.name, o.status,
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
		&out.OrganizationID, &out.AccountID, &out.OwnerUserID, &out.CompanyName, &out.OrganizationStatus,
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
	err := row.Scan(&app.ID, &app.ApplicantUserID, &app.ApplicantEmail, &app.RequestedName, &app.RequestedEnglishName, &app.CompanySize, &app.Status,
		&fee, &app.FeeCurrency, &app.ReviewerUserID, &app.ReviewReason, &app.OrganizationID, &app.CreatedAt, &app.DecidedAt)
	if err != nil {
		return nil, err
	}
	app.FeeAmount = fee
	app.SimilarNames = []string{}
	return &app, nil
}

const applicationSelect = `
	SELECT a.id, a.applicant_user_id, COALESCE(u.email, ''), a.requested_name, COALESCE(a.requested_english_name, ''), COALESCE(a.company_size, ''), a.status,
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

func (r *organizationRepository) SubmitApplication(ctx context.Context, userID int64, name, normalizedName, englishName, normalizedEnglishName, companySize, idempotencyKey, fee, currency string) (_ *service.CompanyApplication, err error) {
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
		INSERT INTO company_upgrade_applications(applicant_user_id,requested_name,normalized_name,requested_english_name,normalized_english_name,company_size,fee_amount,fee_currency,idempotency_key)
		VALUES($1,$2,$3,$4,$5,$6,$7::numeric,$8,$9) RETURNING id`, userID, name, normalizedName, englishName, normalizedEnglishName, companySize, fee, currency, idempotencyKey).Scan(&applicationID); err != nil {
		if isConstraintNamed(err, "company_upgrade_one_pending_per_user") {
			return nil, service.ErrCompanyPending
		}
		if isConstraintNamed(err, "company_upgrade_pending_english_unique") {
			return nil, service.ErrCompanyEnglishNameTaken
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

const adminOrganizationSelect = `SELECT o.id,o.account_id,o.name,o.status,o.owner_user_id,COALESCE(u.email,''),
	(SELECT count(*) FROM organization_memberships m WHERE m.organization_id=o.id AND m.role='member' AND m.status<>'archived'),
	o.member_limit,o.effective_at,o.created_at FROM organizations o JOIN users u ON u.id=o.owner_user_id`

func requireActiveAdminDB(ctx context.Context, db *sql.DB, actorID int64) error {
	var allowed bool
	if err := db.QueryRowContext(ctx, `SELECT role='admin' AND status='active' FROM users WHERE id=$1 AND deleted_at IS NULL`, actorID).Scan(&allowed); err != nil || !allowed {
		return service.ErrInsufficientPerms
	}
	return nil
}

func scanAdminOrganization(scanner interface{ Scan(...any) error }) (*service.AdminOrganization, error) {
	var organization service.AdminOrganization
	if err := scanner.Scan(&organization.ID, &organization.AccountID, &organization.Name, &organization.Status,
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
	var status, fee, currency, requestedName, normalizedName, englishName, normalizedEnglishName, companySize, accountID, email string
	if err := tx.QueryRowContext(ctx, `
		SELECT a.applicant_user_id,a.status,a.fee_amount::text,a.fee_currency,a.requested_name,a.normalized_name,COALESCE(a.requested_english_name,''),COALESCE(a.normalized_english_name,''),COALESCE(a.company_size,''),u.account_id,COALESCE(u.email,'')
		FROM company_upgrade_applications a JOIN users u ON u.id=a.applicant_user_id
		WHERE a.id=$1 FOR UPDATE`, applicationID).Scan(&applicantID, &status, &fee, &currency, &requestedName, &normalizedName, &englishName, &normalizedEnglishName, &companySize, &accountID, &email); err != nil {
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
		if err := tx.QueryRowContext(ctx, `INSERT INTO organizations(account_id,owner_user_id,name,normalized_name,english_name,normalized_english_name,company_size,status,member_limit,effective_at) VALUES($1,$2,$3,$4,$5,$6,$7,'active',$8,$9) RETURNING id`, accountID, applicantID, requestedName, normalizedName, nullableString(englishName), nullableString(normalizedEnglishName), nullableString(companySize), memberLimit, now).Scan(&orgID); err != nil {
			if isConstraintNamed(err, "organizations_normalized_english_name_unique") {
				return nil, service.ErrCompanyEnglishNameTaken
			}
			return nil, err
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
	var orgID int64
	var accountID, orgStatus string
	if err := tx.QueryRowContext(ctx, `SELECT o.id,o.account_id,o.status FROM organizations o JOIN organization_memberships m ON m.organization_id=o.id WHERE m.user_id=$1 AND m.role='owner' AND m.status='active' FOR UPDATE OF o`, ownerID).Scan(&orgID, &accountID, &orgStatus); err != nil || orgStatus != "active" {
		return nil, service.ErrOrganizationPermission
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1 AND role='member' AND status<>'archived'`, orgID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= memberLimit {
		return nil, service.ErrIAMMemberLimit
	}
	var userID int64
	var externalID string
	for attempt := 0; attempt < 20; attempt++ {
		externalID, err = accountid.GenerateIAM()
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `SAVEPOINT iam_external_id_retry`); err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `
			INSERT INTO users(account_id,external_user_id,identity_type,login_name,password_hash,role,balance,frozen_balance,concurrency,status,signup_source,must_change_password,recovery_email,authz_generation,created_at,updated_at)
			VALUES($1,$2,'iam',$3,$4,'user',0,0,5,'active','email',$5,$6,1,NOW(),NOW()) RETURNING id`,
			accountID, externalID, user.LoginName, user.PasswordHash, user.MustChangePassword, nullableString(user.RecoveryEmail)).Scan(&userID)
		if err == nil {
			_, err = tx.ExecContext(ctx, `RELEASE SAVEPOINT iam_external_id_retry`)
			break
		}
		insertErr := err
		if _, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT iam_external_id_retry`); rollbackErr != nil {
			return nil, fmt.Errorf("rollback IAM public ID retry savepoint: %w", rollbackErr)
		}
		if _, releaseErr := tx.ExecContext(ctx, `RELEASE SAVEPOINT iam_external_id_retry`); releaseErr != nil {
			return nil, fmt.Errorf("release IAM public ID retry savepoint: %w", releaseErr)
		}
		if !isConstraintNamed(insertErr, "external_user_id") {
			if isConstraintNamed(insertErr, "users_iam_login_unique_active") {
				return nil, service.ErrIAMLoginName
			}
			return nil, insertErr
		}
		accountid.RecordCollisionRetry()
		err = insertErr
	}
	if err != nil {
		return nil, fmt.Errorf("IAM external ID collision retry limit exhausted: %w", err)
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
	return &service.IAMMember{UserID: userID, ExternalUserID: externalID, LoginName: user.LoginName, Principal: service.CanonicalIAMPrincipal(user.LoginName, accountID), Status: "active", Balance: "0", FrozenBalance: "0", RecoveryEmail: user.RecoveryEmail, MustChangePassword: user.MustChangePassword, PolicyNames: []string{}, CreatedAt: time.Now().UTC()}, nil
}

func (r *organizationRepository) ListIAMMembers(ctx context.Context, actorID int64) ([]service.IAMMember, int, error) {
	org, err := r.GetContextForUser(ctx, actorID)
	if err != nil || !org.Active() || !org.Owner() {
		return nil, 0, service.ErrOrganizationPermission
	}
	var memberLimit int
	if err := r.db.QueryRowContext(ctx, `SELECT member_limit FROM organizations WHERE id=$1`, org.OrganizationID).Scan(&memberLimit); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id,u.external_user_id,u.login_name,m.status,u.balance::text,u.frozen_balance::text,
		       COALESCE(u.recovery_email,''),u.recovery_email_verified_at,u.must_change_password,u.created_at,
		       COALESCE(array_agg(DISTINCT p.policy_key) FILTER(WHERE a.detached_at IS NULL AND p.id IS NOT NULL),'{}')
		FROM organization_memberships m JOIN users u ON u.id=m.user_id
		LEFT JOIN member_policy_attachments a ON a.membership_id=m.id AND a.detached_at IS NULL
		LEFT JOIN managed_policies p ON p.id=a.policy_id
		WHERE m.organization_id=$1 AND m.role='member'
		GROUP BY u.id,m.status ORDER BY u.id`, org.OrganizationID)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	members := make([]service.IAMMember, 0)
	for rows.Next() {
		var member service.IAMMember
		if err := rows.Scan(&member.UserID, &member.ExternalUserID, &member.LoginName, &member.Status, &member.Balance, &member.FrozenBalance, &member.RecoveryEmail, &member.RecoveryVerifiedAt, &member.MustChangePassword, &member.CreatedAt, pq.Array(&member.PolicyNames)); err != nil {
			return nil, 0, err
		}
		member.Principal = service.CanonicalIAMPrincipal(member.LoginName, org.AccountID)
		members = append(members, member)
	}
	return members, memberLimit, rows.Err()
}

func (r *organizationRepository) GetIAMMember(ctx context.Context, actorID, memberUserID int64) (*service.IAMMember, error) {
	org, err := r.GetContextForUser(ctx, actorID)
	if err != nil || !org.Active() || !org.Owner() {
		return nil, service.ErrOrganizationPermission
	}
	var member service.IAMMember
	err = r.db.QueryRowContext(ctx, `
		SELECT u.id,u.external_user_id,u.login_name,m.status,u.balance::text,u.frozen_balance::text,
		       COALESCE(u.recovery_email,''),u.recovery_email_verified_at,u.must_change_password,u.created_at,
		       COALESCE(array_agg(DISTINCT p.policy_key) FILTER(WHERE a.detached_at IS NULL AND p.id IS NOT NULL),'{}')
		FROM organization_memberships m JOIN users u ON u.id=m.user_id
		LEFT JOIN member_policy_attachments a ON a.membership_id=m.id AND a.detached_at IS NULL
		LEFT JOIN managed_policies p ON p.id=a.policy_id
		WHERE m.organization_id=$1 AND m.user_id=$2 AND m.role='member'
		GROUP BY u.id,m.status`, org.OrganizationID, memberUserID).
		Scan(&member.UserID, &member.ExternalUserID, &member.LoginName, &member.Status, &member.Balance,
			&member.FrozenBalance, &member.RecoveryEmail, &member.RecoveryVerifiedAt,
			&member.MustChangePassword, &member.CreatedAt, pq.Array(&member.PolicyNames))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrIAMMemberNotFound
	}
	if err != nil {
		return nil, err
	}
	member.Principal = service.CanonicalIAMPrincipal(member.LoginName, org.AccountID)
	return &member, nil
}

func (r *organizationRepository) SetIAMMemberStatus(ctx context.Context, ownerID, memberUserID int64, status string) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
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
		if err := tx.QueryRowContext(ctx, `
			SELECT o.id
			FROM organizations o
			JOIN organization_memberships m ON m.organization_id=o.id
			WHERE m.user_id=$1 AND m.role='owner' AND m.status='active' AND o.status='active'`, actorID).
			Scan(&organizationID); err != nil {
			return service.ErrOrganizationPermission
		}
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

func (r *organizationRepository) FindIAMByPrincipal(ctx context.Context, loginName, accountID string) (*service.User, *service.OrganizationContext, error) {
	var user service.User
	err := r.db.QueryRowContext(ctx, `
		SELECT id,COALESCE(email,''),account_id,external_user_id,identity_type,login_name,password_hash,role,balance,frozen_balance,concurrency,status,must_change_password,COALESCE(recovery_email,''),recovery_email_verified_at,authz_generation,created_at,updated_at
		FROM users WHERE account_id=$1 AND lower(login_name)=lower($2) AND identity_type='iam' AND deleted_at IS NULL`, accountID, loginName).
		Scan(&user.ID, &user.Email, &user.AccountID, &user.ExternalUserID, &user.IdentityType, &user.LoginName, &user.PasswordHash, &user.Role, &user.Balance, &user.FrozenBalance, &user.Concurrency, &user.Status, &user.MustChangePassword, &user.RecoveryEmail, &user.RecoveryEmailVerifiedAt, &user.AuthzGeneration, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, service.ErrInvalidCredentials
	}
	if err != nil {
		return nil, nil, err
	}
	org, err := r.GetContextForUser(ctx, user.ID)
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
	var orgID, rootID int64
	if err := tx.QueryRowContext(ctx, `SELECT o.id,o.owner_user_id FROM organizations o JOIN organization_memberships m ON m.organization_id=o.id WHERE m.user_id=$1 AND m.role='owner' AND m.status='active' AND o.status='active'`, ownerID).Scan(&orgID, &rootID); err != nil {
		return service.ErrOrganizationPermission
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
	first, second := rootID, memberUserID
	if first > second {
		first, second = second, first
	}
	if _, err := tx.ExecContext(ctx, `SELECT id FROM users WHERE id IN ($1,$2) ORDER BY id FOR UPDATE`, first, second); err != nil {
		return err
	}
	source, destination, kind := rootID, memberUserID, "allocate"
	if reclaim {
		source, destination, kind = memberUserID, rootID, "reclaim"
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
		if parseErr != nil || persistedParseErr != nil || existingKind != kind || existingActor != ownerID ||
			existingSource != source || existingDestination != destination || !requestedAmount.Equal(persistedAmount) {
			return infraerrors.Conflict("IDEMPOTENCY_KEY_CONFLICT", "idempotency key was already used for another balance transfer")
		}
		return nil
	}
	if !errors.Is(replayErr, sql.ErrNoRows) {
		return replayErr
	}
	var sourceAfter string
	if err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance-$2::numeric,updated_at=NOW() WHERE id=$1 AND balance >= $2::numeric RETURNING balance::text`, source, amount).Scan(&sourceAfter); errors.Is(err, sql.ErrNoRows) {
		return service.ErrInsufficientBalance
	} else if err != nil {
		return err
	}
	var destinationAfter string
	if err := tx.QueryRowContext(ctx, `UPDATE users SET balance=balance+$2::numeric,updated_at=NOW() WHERE id=$1 RETURNING balance::text`, destination, amount).Scan(&destinationAfter); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_financial_ledger(idempotency_key,kind,organization_id,actor_user_id,source_user_id,destination_user_id,amount,currency,source_balance_after,destination_balance_after) VALUES($1,$2,$3,$4,$5,$6,$7::numeric,'USD',$8::numeric,$9::numeric)`, ledgerKey, kind, orgID, ownerID, source, destination, amount, sourceAfter, destinationAfter); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_audit_events(organization_id,actor_user_id,subject_user_id,action,result,correlation_id,metadata) VALUES($1,$2,$3,$4,'success',$5,jsonb_build_object('amount',$6::numeric))`, orgID, ownerID, memberUserID, "organization.balance."+kind, organizationCorrelationID(ctx), amount); err != nil {
		return err
	}
	return tx.Commit()
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
	return &service.FinanceSummary{BalanceSource: source, Available: &available, Frozen: &frozen, Total: organizationStringPtr(total.String())}, nil
}

func organizationStringPtr(value string) *string { return &value }

func (r *organizationRepository) ResolveBillingContext(ctx context.Context, consumerUserID int64) (*service.BillingContext, error) {
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
		return &service.BillingContext{ConsumerUserID: consumerUserID, OrganizationID: orgID, PayerUserID: consumerUserID, BalanceSource: "self", AuthzGeneration: generation}, nil
	}
	org, err := r.GetContextForUser(ctx, consumerUserID)
	if err != nil || !org.Active() {
		return nil, service.ErrOrganizationPermission
	}
	payer, source := consumerUserID, "allocated"
	if org.HasAction(service.ActionSharedBalanceUse) {
		payer, source = org.OwnerUserID, "shared"
	}
	return &service.BillingContext{ConsumerUserID: consumerUserID, OrganizationID: &org.OrganizationID, PayerUserID: payer, BalanceSource: source, AuthzGeneration: org.AuthzGeneration}, nil
}

func (r *organizationRepository) organizationUsageScope(ctx context.Context, userID int64, filter service.OrganizationUsageFilter) (string, []any, error) {
	org, err := r.GetContextForUser(ctx, userID)
	if err != nil || !org.Active() || !org.Owner() {
		return "", nil, service.ErrOrganizationPermission
	}
	conditions := []string{"l.organization_id=$1", "l.created_at >= $2"}
	args := []any{org.OrganizationID, org.EffectiveAt}
	add := func(sqlCondition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(sqlCondition, len(args)))
	}
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
		SELECT l.id,l.user_id,COALESCE(u.login_name,u.username,u.email,''),COALESCE(k.name,''),
		       COALESCE(l.requested_model,l.model),l.input_tokens,l.output_tokens,l.actual_cost::text,
		       COALESCE(l.inbound_endpoint,''),COALESCE(l.billing_status,'charged'),l.duration_ms,l.created_at,
		       COALESCE(l.balance_source,'self')
		FROM usage_logs l JOIN users u ON u.id=l.user_id LEFT JOIN api_keys k ON k.id=l.api_key_id
		WHERE %s ORDER BY l.created_at DESC,l.id DESC LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.OrganizationUsageRow, 0)
	for rows.Next() {
		var item service.OrganizationUsageRow
		if err := rows.Scan(&item.ID, &item.MemberUserID, &item.MemberLogin, &item.APIKeyName, &item.Model, &item.InputTokens, &item.OutputTokens, &item.ActualCost, &item.Endpoint, &item.Status, &item.DurationMS, &item.CreatedAt, &item.BalanceSource); err != nil {
			return nil, 0, err
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
	err = r.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(l.input_tokens),0),COALESCE(sum(l.output_tokens),0),COALESCE(sum(l.actual_cost),0)::text FROM usage_logs l WHERE `+where, args...).Scan(&out.Requests, &out.InputTokens, &out.OutputTokens, &out.ActualCost)
	return &out, err
}

func (r *organizationRepository) UsageTrend(ctx context.Context, userID int64, filter service.OrganizationUsageFilter) ([]service.OrganizationUsageTrendPoint, error) {
	where, args, err := r.organizationUsageScope(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT date_trunc('day',l.created_at) AS bucket,count(*),COALESCE(sum(l.input_tokens+l.output_tokens),0),COALESCE(sum(l.actual_cost),0)::text FROM usage_logs l WHERE `+where+` GROUP BY bucket ORDER BY bucket`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.OrganizationUsageTrendPoint, 0)
	for rows.Next() {
		var point service.OrganizationUsageTrendPoint
		if err := rows.Scan(&point.Bucket, &point.Requests, &point.Tokens, &point.ActualCost); err != nil {
			return nil, err
		}
		out = append(out, point)
	}
	return out, rows.Err()
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

func isConstraintNamed(err error, name string) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return strings.Contains(strings.ToLower(pqErr.Constraint), strings.ToLower(name))
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), strings.ToLower(name))
}
