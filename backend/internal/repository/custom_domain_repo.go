package repository

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	dbcustomdomain "github.com/Wei-Shaw/sub2api/ent/customdomain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type customDomainRepository struct {
	client *ent.Client
}

func NewCustomDomainRepository(client *ent.Client) service.CustomDomainRepository {
	return &customDomainRepository{client: client}
}

func (r *customDomainRepository) Create(ctx context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	if domain == nil {
		return nil, nil
	}
	builder := r.client.CustomDomain.Create().
		SetUserID(domain.UserID).
		SetDomain(domain.Domain).
		SetStatus(domain.Status).
		SetVerificationToken(domain.VerificationToken).
		SetVerificationTxtName(domain.VerificationTXTName).
		SetVerificationTxtValue(domain.VerificationTXTValue)
	if domain.CNAMETarget != nil {
		builder.SetCnameTarget(*domain.CNAMETarget)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return customDomainFromEnt(created), nil
}

func (r *customDomainRepository) GetByID(ctx context.Context, id int64) (*service.CustomDomain, error) {
	row, err := r.client.CustomDomain.Query().
		Where(dbcustomdomain.IDEQ(id)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return customDomainFromEnt(row), nil
}

func (r *customDomainRepository) GetByDomain(ctx context.Context, domain string) (*service.CustomDomain, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	row, err := r.client.CustomDomain.Query().
		Where(dbcustomdomain.DomainEqualFold(domain)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return customDomainFromEnt(row), nil
}

func (r *customDomainRepository) ListByUserID(ctx context.Context, userID int64) ([]service.CustomDomain, error) {
	rows, err := r.client.CustomDomain.Query().
		Where(dbcustomdomain.UserIDEQ(userID)).
		Order(ent.Desc(dbcustomdomain.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return customDomainSliceFromEnt(rows), nil
}

func (r *customDomainRepository) ListAll(ctx context.Context, filters service.CustomDomainListFilters) ([]service.CustomDomain, error) {
	query := r.client.CustomDomain.Query().WithUser()
	if filters.Domain != "" {
		query = query.Where(dbcustomdomain.DomainContainsFold(filters.Domain))
	}
	if filters.Status != "" {
		query = query.Where(dbcustomdomain.StatusEQ(filters.Status))
	}
	if filters.UserID != nil && *filters.UserID > 0 {
		query = query.Where(dbcustomdomain.UserIDEQ(*filters.UserID))
	}
	rows, err := query.
		Order(ent.Desc(dbcustomdomain.FieldCreatedAt)).
		Limit(500).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return customDomainSliceFromEnt(rows), nil
}

func (r *customDomainRepository) Update(ctx context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	if domain == nil {
		return nil, nil
	}
	update := r.client.CustomDomain.UpdateOneID(domain.ID).
		SetStatus(domain.Status).
		SetVerificationToken(domain.VerificationToken).
		SetVerificationTxtName(domain.VerificationTXTName).
		SetVerificationTxtValue(domain.VerificationTXTValue).
		SetUpdatedAt(time.Now())

	if domain.CNAMETarget != nil {
		update.SetCnameTarget(*domain.CNAMETarget)
	} else {
		update.ClearCnameTarget()
	}
	if domain.LastError != nil {
		update.SetLastError(*domain.LastError)
	} else {
		update.ClearLastError()
	}
	if domain.VerifiedAt != nil {
		update.SetVerifiedAt(*domain.VerifiedAt)
	} else {
		update.ClearVerifiedAt()
	}
	if domain.LastCheckedAt != nil {
		update.SetLastCheckedAt(*domain.LastCheckedAt)
	} else {
		update.ClearLastCheckedAt()
	}
	if domain.DisabledAt != nil {
		update.SetDisabledAt(*domain.DisabledAt)
	} else {
		update.ClearDisabledAt()
	}
	if domain.DisabledReason != nil {
		update.SetDisabledReason(*domain.DisabledReason)
	} else {
		update.ClearDisabledReason()
	}

	updated, err := update.Save(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return r.GetByID(ctx, updated.ID)
}

func (r *customDomainRepository) Delete(ctx context.Context, id int64) error {
	if err := r.client.CustomDomain.DeleteOneID(id).Exec(ctx); err != nil {
		return translateCustomDomainError(err)
	}
	return nil
}

func translateCustomDomainError(err error) error {
	if err == nil {
		return nil
	}
	if ent.IsNotFound(err) {
		return service.ErrCustomDomainNotFound
	}
	if ent.IsConstraintError(err) {
		return service.ErrCustomDomainConflict
	}
	return err
}

func customDomainSliceFromEnt(rows []*ent.CustomDomain) []service.CustomDomain {
	out := make([]service.CustomDomain, 0, len(rows))
	for _, row := range rows {
		if converted := customDomainFromEnt(row); converted != nil {
			out = append(out, *converted)
		}
	}
	return out
}

func customDomainFromEnt(row *ent.CustomDomain) *service.CustomDomain {
	if row == nil {
		return nil
	}
	out := &service.CustomDomain{
		ID:                   row.ID,
		UserID:               row.UserID,
		Domain:               row.Domain,
		Status:               row.Status,
		VerificationToken:    row.VerificationToken,
		VerificationTXTName:  row.VerificationTxtName,
		VerificationTXTValue: row.VerificationTxtValue,
		CNAMETarget:          row.CnameTarget,
		LastError:            row.LastError,
		VerifiedAt:           row.VerifiedAt,
		LastCheckedAt:        row.LastCheckedAt,
		DisabledAt:           row.DisabledAt,
		DisabledReason:       row.DisabledReason,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		DeletedAt:            row.DeletedAt,
	}
	if row.Edges.User != nil {
		user := &service.User{}
		applyUserEntityToService(user, row.Edges.User)
		out.User = user
	}
	return out
}
