package repository

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	dbcustomdomain "github.com/Wei-Shaw/sub2api/ent/customdomain"
	dbcustomdomainuser "github.com/Wei-Shaw/sub2api/ent/customdomainuser"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
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
	client := clientFromContext(ctx, r.client)
	var tx *ent.Tx
	var txClient *ent.Client
	if existingTx := ent.TxFromContext(ctx); existingTx != nil {
		txClient = existingTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback() }()
		ctx = ent.NewTxContext(ctx, tx)
		txClient = tx.Client()
		client = txClient
	}

	builder := client.CustomDomain.Create().
		SetUserID(domain.UserID).
		SetAllUsers(domain.AllUsers).
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
	if err := syncCustomDomainUsersWithClient(ctx, txClient, created.ID, domain.UserIDs); err != nil {
		return nil, translateCustomDomainError(err)
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return r.GetByID(ctx, created.ID)
}

func (r *customDomainRepository) GetByID(ctx context.Context, id int64) (*service.CustomDomain, error) {
	row, err := r.client.CustomDomain.Query().
		Where(dbcustomdomain.IDEQ(id)).
		WithUser().
		WithAuthorizedUsers().
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
		WithAuthorizedUsers().
		Only(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return customDomainFromEnt(row), nil
}

func (r *customDomainRepository) ListByUserID(ctx context.Context, userID int64) ([]service.CustomDomain, error) {
	rows, err := r.client.CustomDomain.Query().
		Where(customDomainAccessibleToUser(userID)).
		WithUser().
		WithAuthorizedUsers().
		Order(ent.Desc(dbcustomdomain.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return customDomainSliceFromEnt(rows), nil
}

func (r *customDomainRepository) ListAll(ctx context.Context, filters service.CustomDomainListFilters) ([]service.CustomDomain, error) {
	query := r.client.CustomDomain.Query().
		WithUser().
		WithAuthorizedUsers()
	if filters.Domain != "" {
		query = query.Where(dbcustomdomain.DomainContainsFold(filters.Domain))
	}
	if filters.Status != "" {
		query = query.Where(dbcustomdomain.StatusEQ(filters.Status))
	}
	if filters.UserID != nil && *filters.UserID > 0 {
		query = query.Where(customDomainAccessibleToUser(*filters.UserID))
	}
	if filters.AllUsers != nil {
		query = query.Where(dbcustomdomain.AllUsersEQ(*filters.AllUsers))
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

func (r *customDomainRepository) SetAccess(ctx context.Context, id int64, allUsers bool, userIDs []int64) (*service.CustomDomain, error) {
	client := clientFromContext(ctx, r.client)
	if _, err := client.CustomDomain.UpdateOneID(id).
		SetAllUsers(allUsers).
		SetUpdatedAt(time.Now()).
		Save(ctx); err != nil {
		return nil, translateCustomDomainError(err)
	}
	if allUsers {
		userIDs = nil
	}
	if err := syncCustomDomainUsersWithClient(ctx, client, id, userIDs); err != nil {
		return nil, translateCustomDomainError(err)
	}
	return r.GetByID(ctx, id)
}

func (r *customDomainRepository) Update(ctx context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	if domain == nil {
		return nil, nil
	}
	update := r.client.CustomDomain.UpdateOneID(domain.ID).
		SetAllUsers(domain.AllUsers).
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
		AllUsers:             row.AllUsers,
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
		out.User = userEntityToService(row.Edges.User)
	}
	if len(row.Edges.AuthorizedUsers) > 0 {
		out.UserIDs = make([]int64, 0, len(row.Edges.AuthorizedUsers))
		out.Users = make([]service.User, 0, len(row.Edges.AuthorizedUsers))
		for _, userEnt := range row.Edges.AuthorizedUsers {
			if userEnt == nil {
				continue
			}
			if user := userEntityToService(userEnt); user != nil {
				out.UserIDs = append(out.UserIDs, user.ID)
				out.Users = append(out.Users, *user)
			}
		}
	}
	return out
}

func customDomainAccessibleToUser(userID int64) predicate.CustomDomain {
	return dbcustomdomain.Or(
		dbcustomdomain.UserIDEQ(userID),
		dbcustomdomain.AllUsers(true),
		dbcustomdomain.HasAuthorizedUsersWith(dbuser.IDEQ(userID)),
	)
}

func syncCustomDomainUsersWithClient(ctx context.Context, client *ent.Client, domainID int64, userIDs []int64) error {
	client = clientFromContext(ctx, client)
	if client == nil {
		return nil
	}
	if _, err := client.CustomDomainUser.Delete().
		Where(dbcustomdomainuser.CustomDomainIDEQ(domainID)).
		Exec(ctx); err != nil {
		return err
	}

	unique := map[int64]struct{}{}
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		unique[userID] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}

	creates := make([]*ent.CustomDomainUserCreate, 0, len(unique))
	for userID := range unique {
		creates = append(creates, client.CustomDomainUser.Create().
			SetCustomDomainID(domainID).
			SetUserID(userID))
	}
	if err := client.CustomDomainUser.CreateBulk(creates...).
		OnConflictColumns(dbcustomdomainuser.FieldCustomDomainID, dbcustomdomainuser.FieldUserID).
		DoNothing().
		Exec(ctx); err != nil {
		if isSQLNoRowsError(err) {
			return nil
		}
		return err
	}
	return nil
}
