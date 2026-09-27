package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/studentverification"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type studentVerificationRepository struct {
	client *dbent.Client
}

func NewStudentVerificationRepository(client *dbent.Client) service.StudentVerificationRepository {
	return &studentVerificationRepository{client: client}
}

func studentVerificationEntityToService(m *dbent.StudentVerification) *service.StudentVerification {
	if m == nil {
		return nil
	}
	var userID int64
	if m.UserID != nil {
		userID = *m.UserID
	}
	return &service.StudentVerification{
		ID:                 m.ID,
		UserID:             userID,
		Email:              m.Email,
		EmailRaw:           m.EmailRaw,
		Status:             m.Status,
		VerifiedAt:         m.VerifiedAt,
		ExpiresAt:          m.ExpiresAt,
		RevokedAt:          m.RevokedAt,
		RevokedBy:          m.RevokedBy,
		RevokeReason:       m.RevokeReason,
		RebateRateApplied:  m.RebateRateApplied,
		PreviousRebateRate: m.PreviousRebateRate,
		GrantedGroupIDs:    append([]int64(nil), m.GrantedGroupIds...),
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

func (r *studentVerificationRepository) GetByID(ctx context.Context, id int64) (*service.StudentVerification, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.StudentVerification.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrStudentVerificationNotFound, nil)
	}
	return studentVerificationEntityToService(m), nil
}

func (r *studentVerificationRepository) GetByUserID(ctx context.Context, userID int64) (*service.StudentVerification, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.StudentVerification.Query().
		Where(studentverification.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrStudentVerificationNotFound, nil)
	}
	return studentVerificationEntityToService(m), nil
}

func (r *studentVerificationRepository) GetByEmail(ctx context.Context, normalizedEmail string) (*service.StudentVerification, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.StudentVerification.Query().
		Where(studentverification.EmailEQ(normalizedEmail)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrStudentVerificationNotFound, nil)
	}
	return studentVerificationEntityToService(m), nil
}

// translateStudentVerificationConflict maps a unique-violation to the email or
// user claim error depending on which index was hit.
func translateStudentVerificationConflict(err error) error {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && strings.Contains(pgErr.Constraint, "email") {
		return service.ErrStudentEmailClaimed.WithCause(err)
	}
	if msg := strings.ToLower(err.Error()); strings.Contains(msg, "student_verifications_email") {
		return service.ErrStudentEmailClaimed.WithCause(err)
	}
	// user_id unique conflict means a concurrent verify for the same user; the
	// retry-safe answer is the generic claimed error too, since the request can
	// be repeated by reading the fresh record.
	return service.ErrStudentEmailClaimed.WithCause(err)
}

func (r *studentVerificationRepository) UpsertActive(ctx context.Context, v *service.StudentVerification) error {
	client := clientFromContext(ctx, r.client)

	existing, err := client.StudentVerification.Query().
		Where(studentverification.UserIDEQ(v.UserID)).
		Only(ctx)
	switch {
	case err == nil:
		_, err = client.StudentVerification.UpdateOneID(existing.ID).
			SetEmail(v.Email).
			SetEmailRaw(v.EmailRaw).
			SetStatus(service.StudentVerificationStatusActive).
			SetVerifiedAt(v.VerifiedAt).
			SetExpiresAt(v.ExpiresAt).
			ClearRevokedAt().
			ClearRevokedBy().
			ClearRevokeReason().
			SetNillableRebateRateApplied(v.RebateRateApplied).
			SetNillablePreviousRebateRate(v.PreviousRebateRate).
			SetGrantedGroupIds(v.GrantedGroupIDs).
			Save(ctx)
		if err != nil {
			if isUniqueConstraintViolation(err) {
				return translateStudentVerificationConflict(err)
			}
			return err
		}
		v.ID = existing.ID
		return nil
	case dbent.IsNotFound(err):
		created, err := client.StudentVerification.Create().
			SetUserID(v.UserID).
			SetEmail(v.Email).
			SetEmailRaw(v.EmailRaw).
			SetStatus(service.StudentVerificationStatusActive).
			SetVerifiedAt(v.VerifiedAt).
			SetExpiresAt(v.ExpiresAt).
			SetNillableRebateRateApplied(v.RebateRateApplied).
			SetNillablePreviousRebateRate(v.PreviousRebateRate).
			SetGrantedGroupIds(v.GrantedGroupIDs).
			Save(ctx)
		if err != nil {
			if isUniqueConstraintViolation(err) {
				return translateStudentVerificationConflict(err)
			}
			return err
		}
		v.ID = created.ID
		return nil
	default:
		return err
	}
}

func (r *studentVerificationRepository) UpdateStatus(ctx context.Context, id int64, status string, revokedBy *int64, reason *string) error {
	client := clientFromContext(ctx, r.client)
	update := client.StudentVerification.UpdateOneID(id).
		SetStatus(status)
	if status == service.StudentVerificationStatusRevoked {
		now := time.Now().UTC()
		update = update.SetRevokedAt(now)
		if revokedBy != nil {
			update = update.SetRevokedBy(*revokedBy)
		}
		if reason != nil {
			update = update.SetRevokeReason(*reason)
		}
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrStudentVerificationNotFound, nil)
}

func (r *studentVerificationRepository) ListActiveExpired(ctx context.Context, now time.Time, limit int) ([]service.StudentVerification, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.client.StudentVerification.Query().
		Where(
			studentverification.StatusEQ(service.StudentVerificationStatusActive),
			studentverification.ExpiresAtLTE(now),
		).
		Order(dbent.Asc(studentverification.FieldExpiresAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.StudentVerification, 0, len(rows))
	for _, m := range rows {
		out = append(out, *studentVerificationEntityToService(m))
	}
	return out, nil
}

func (r *studentVerificationRepository) List(ctx context.Context, params pagination.PaginationParams, status string, keyword string) ([]service.StudentVerification, *pagination.PaginationResult, error) {
	query := r.client.StudentVerification.Query()
	countQuery := r.client.StudentVerification.Query()

	if status != "" {
		query = query.Where(studentverification.StatusEQ(status))
		countQuery = countQuery.Where(studentverification.StatusEQ(status))
	}
	if kw := strings.ToLower(strings.TrimSpace(keyword)); kw != "" {
		pred := studentverification.Or(
			studentverification.EmailContainsFold(kw),
			studentverification.EmailRawContainsFold(kw),
		)
		query = query.Where(pred)
		countQuery = countQuery.Where(pred)
	}

	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	order := params.NormalizedSortOrder(pagination.SortOrderDesc)
	ordered := query.Order(dbent.Desc(studentverification.FieldCreatedAt))
	if order == pagination.SortOrderAsc {
		ordered = query.Order(dbent.Asc(studentverification.FieldCreatedAt))
	}
	rows, err := ordered.
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]service.StudentVerification, 0, len(rows))
	for _, m := range rows {
		out = append(out, *studentVerificationEntityToService(m))
	}
	pages := int64(0)
	limit := int64(params.Limit())
	if total > 0 {
		pages = (int64(total) + limit - 1) / limit
	}
	return out, &pagination.PaginationResult{
		Total:    int64(total),
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    int(pages),
	}, nil
}

func (r *studentVerificationRepository) Delete(ctx context.Context, id int64) error {
	err := r.client.StudentVerification.DeleteOneID(id).Exec(ctx)
	return translatePersistenceError(err, service.ErrStudentVerificationNotFound, nil)
}
