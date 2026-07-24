package repository

import (
	"context"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/promocode"
	"github.com/Wei-Shaw/sub2api/ent/promocodeusage"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	portpromo "github.com/Wei-Shaw/sub2api/internal/port/promo"

	entsql "entgo.io/ent/dialect/sql"
)

type promoCodeRepository struct {
	client *dbent.Client
}

func NewPromoCodeRepository(client *dbent.Client) portpromo.PromoCodeRepository {
	return &promoCodeRepository{client: client}
}

func (r *promoCodeRepository) Create(ctx context.Context, code *domain.PromoCode) error {
	client := clientFromContext(ctx, r.client)
	builder := client.PromoCode.Create().
		SetCode(code.Code).
		SetBonusAmount(code.BonusAmount).
		SetMaxUses(code.MaxUses).
		SetUsedCount(code.UsedCount).
		SetStatus(code.Status).
		SetNotes(code.Notes)

	if code.ExpiresAt != nil {
		builder.SetExpiresAt(*code.ExpiresAt)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	code.ID = created.ID
	code.CreatedAt = created.CreatedAt
	code.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *promoCodeRepository) GetByID(ctx context.Context, id int64) (*domain.PromoCode, error) {
	m, err := r.client.PromoCode.Query().
		Where(promocode.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, domain.ErrPromoCodeNotFound
		}
		return nil, err
	}
	return promoCodeEntityToDomain(m), nil
}

func (r *promoCodeRepository) GetByCode(ctx context.Context, code string) (*domain.PromoCode, error) {
	m, err := r.client.PromoCode.Query().
		Where(promocode.CodeEqualFold(code)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, domain.ErrPromoCodeNotFound
		}
		return nil, err
	}
	return promoCodeEntityToDomain(m), nil
}

func (r *promoCodeRepository) GetByCodeForUpdate(ctx context.Context, code string) (*domain.PromoCode, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.PromoCode.Query().
		Where(promocode.CodeEqualFold(code)).
		ForUpdate().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, domain.ErrPromoCodeNotFound
		}
		return nil, err
	}
	return promoCodeEntityToDomain(m), nil
}

func (r *promoCodeRepository) Update(ctx context.Context, code *domain.PromoCode) error {
	client := clientFromContext(ctx, r.client)
	builder := client.PromoCode.UpdateOneID(code.ID).
		SetCode(code.Code).
		SetBonusAmount(code.BonusAmount).
		SetMaxUses(code.MaxUses).
		SetUsedCount(code.UsedCount).
		SetStatus(code.Status).
		SetNotes(code.Notes)

	if code.ExpiresAt != nil {
		builder.SetExpiresAt(*code.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return domain.ErrPromoCodeNotFound
		}
		return err
	}

	code.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *promoCodeRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.PromoCode.Delete().Where(promocode.IDEQ(id)).Exec(ctx)
	return err
}

func (r *promoCodeRepository) List(ctx context.Context, params pagination.PaginationParams) ([]domain.PromoCode, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "")
}

func (r *promoCodeRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, search string) ([]domain.PromoCode, *pagination.PaginationResult, error) {
	q := r.client.PromoCode.Query()

	if status != "" {
		q = q.Where(promocode.StatusEQ(status))
	}
	if search != "" {
		q = q.Where(promocode.CodeContainsFold(search))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	codesQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range promoCodeListOrder(params) {
		codesQuery = codesQuery.Order(order)
	}

	codes, err := codesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outCodes := promoCodeEntitiesToDomain(codes)

	return outCodes, paginationResultFromTotal(int64(total), params), nil
}

func promoCodeListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	var field string
	switch sortBy {
	case "bonus_amount":
		field = promocode.FieldBonusAmount
	case "status":
		field = promocode.FieldStatus
	case "expires_at":
		field = promocode.FieldExpiresAt
	case "created_at":
		field = promocode.FieldCreatedAt
	case "code":
		field = promocode.FieldCode
	default:
		field = promocode.FieldID
	}

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(promocode.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(promocode.FieldID)}
}

func (r *promoCodeRepository) CreateUsage(ctx context.Context, usage *domain.PromoCodeUsage) error {
	client := clientFromContext(ctx, r.client)
	created, err := client.PromoCodeUsage.Create().
		SetPromoCodeID(usage.PromoCodeID).
		SetUserID(usage.UserID).
		SetBonusAmount(usage.BonusAmount).
		SetUsedAt(usage.UsedAt).
		Save(ctx)
	if err != nil {
		return err
	}

	usage.ID = created.ID
	return nil
}

func (r *promoCodeRepository) GetUsageByPromoCodeAndUser(ctx context.Context, promoCodeID, userID int64) (*domain.PromoCodeUsage, error) {
	m, err := r.client.PromoCodeUsage.Query().
		Where(
			promocodeusage.PromoCodeIDEQ(promoCodeID),
			promocodeusage.UserIDEQ(userID),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return promoCodeUsageEntityToDomain(m), nil
}

func (r *promoCodeRepository) ListUsagesByPromoCode(ctx context.Context, promoCodeID int64, params pagination.PaginationParams) ([]domain.PromoCodeUsage, *pagination.PaginationResult, error) {
	q := r.client.PromoCodeUsage.Query().
		Where(promocodeusage.PromoCodeIDEQ(promoCodeID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	usages, err := q.
		WithUser().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(promocodeusage.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outUsages := promoCodeUsageEntitiesToDomain(usages)

	return outUsages, paginationResultFromTotal(int64(total), params), nil
}

func (r *promoCodeRepository) IncrementUsedCount(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.PromoCode.UpdateOneID(id).
		AddUsedCount(1).
		Save(ctx)
	return err
}

// Entity to domain conversions

func promoCodeEntityToDomain(m *dbent.PromoCode) *domain.PromoCode {
	if m == nil {
		return nil
	}
	return &domain.PromoCode{
		ID:          m.ID,
		Code:        m.Code,
		BonusAmount: m.BonusAmount,
		MaxUses:     m.MaxUses,
		UsedCount:   m.UsedCount,
		Status:      m.Status,
		ExpiresAt:   m.ExpiresAt,
		Notes:       derefString(m.Notes),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func promoCodeEntitiesToDomain(models []*dbent.PromoCode) []domain.PromoCode {
	out := make([]domain.PromoCode, 0, len(models))
	for i := range models {
		if s := promoCodeEntityToDomain(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func promoCodeUsageEntityToDomain(m *dbent.PromoCodeUsage) *domain.PromoCodeUsage {
	if m == nil {
		return nil
	}
	out := &domain.PromoCodeUsage{
		ID:          m.ID,
		PromoCodeID: m.PromoCodeID,
		UserID:      m.UserID,
		BonusAmount: m.BonusAmount,
		UsedAt:      m.UsedAt,
	}
	if m.Edges.User != nil {
		out.User = promoUsageUserFromEntity(m.Edges.User)
	}
	return out
}

func promoCodeUsageEntitiesToDomain(models []*dbent.PromoCodeUsage) []domain.PromoCodeUsage {
	out := make([]domain.PromoCodeUsage, 0, len(models))
	for i := range models {
		if s := promoCodeUsageEntityToDomain(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

// promoUsageUserFromEntity maps the embedded ent user edge into the promo BC's
// shallow user projection. Avoids pulling service.User into the repository layer.
func promoUsageUserFromEntity(u *dbent.User) *domain.PromoUsageUser {
	if u == nil {
		return nil
	}
	return &domain.PromoUsageUser{
		ID:       u.ID,
		Email:    u.Email,
		Username: u.Username,
		Role:     u.Role,
		Balance:  u.Balance,
		Status:   u.Status,
	}
}
