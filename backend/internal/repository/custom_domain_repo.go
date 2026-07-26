package repository

import (
	"context"
	"errors"
	"sort"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/customdomain"
	"github.com/Wei-Shaw/sub2api/ent/customdomainuser"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type customDomainRepository struct {
	client *dbent.Client
}

func NewCustomDomainRepository(client *dbent.Client) service.CustomDomainRepository {
	return &customDomainRepository{client: client}
}

func (r *customDomainRepository) Create(ctx context.Context, domain *service.CustomDomain) (*service.CustomDomain, error) {
	if domain == nil {
		return nil, nil
	}
	originalCtx := ctx
	tx := dbent.TxFromContext(ctx)
	var ownTx *dbent.Tx
	var txClient *dbent.Client
	if tx == nil {
		var err error
		ownTx, err = r.client.Tx(ctx)
		if err != nil {
			return nil, err
		}
		tx = ownTx
		defer func() { _ = tx.Rollback() }()
		ctx = dbent.NewTxContext(ctx, tx)
		txClient = tx.Client()
	} else {
		txClient = tx.Client()
	}

	row, err := txClient.CustomDomain.Create().
		SetUserID(domain.UserID).
		SetDomain(domain.Domain).
		SetStatus(domain.Status).
		SetAllUsers(domain.AllUsers).
		SetVerificationToken(domain.VerificationToken).
		SetVerificationTxtName(domain.VerificationTXTName).
		SetVerificationTxtValue(domain.VerificationTXTValue).
		SetNillableCnameTarget(domain.CNAMETarget).
		Save(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	if err := syncCustomDomainUsersWithClient(ctx, txClient, row.ID, domain.AuthorizedUserIDs); err != nil {
		return nil, err
	}
	if ownTx != nil {
		if err := ownTx.Commit(); err != nil {
			return nil, err
		}
		return r.GetByID(originalCtx, row.ID)
	}
	return r.GetByID(ctx, row.ID)
}

func (r *customDomainRepository) GetByID(ctx context.Context, id int64) (*service.CustomDomain, error) {
	row, err := clientFromContext(ctx, r.client).CustomDomain.Query().
		Where(customdomain.IDEQ(id)).
		WithUser().
		WithAuthorizedUsers().
		Only(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return customDomainFromEnt(row), nil
}

func (r *customDomainRepository) GetByDomain(ctx context.Context, domainName string) (*service.CustomDomain, error) {
	row, err := clientFromContext(ctx, r.client).CustomDomain.Query().
		Where(customdomain.DomainEqualFold(domainName)).
		WithUser().
		WithAuthorizedUsers().
		Only(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return customDomainFromEnt(row), nil
}

func (r *customDomainRepository) ListByUserID(ctx context.Context, userID int64) ([]service.CustomDomain, error) {
	rows, err := clientFromContext(ctx, r.client).CustomDomain.Query().
		Where(customDomainAccessibleToUser(userID)).
		WithUser().
		WithAuthorizedUsers().
		Order(dbent.Desc(customdomain.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return customDomainSliceFromEnt(rows), nil
}

func (r *customDomainRepository) ListAll(ctx context.Context, filters service.CustomDomainListFilters) ([]service.CustomDomain, error) {
	query := r.client.CustomDomain.Query().WithUser().WithAuthorizedUsers()
	if filters.Domain != "" {
		query = query.Where(customdomain.DomainContainsFold(filters.Domain))
	}
	if filters.Status != "" {
		query = query.Where(customdomain.StatusEQ(filters.Status))
	}
	if filters.UserID > 0 {
		query = query.Where(customDomainAccessibleToUser(filters.UserID))
	}
	if filters.AllUsers != nil {
		query = query.Where(customdomain.AllUsersEQ(*filters.AllUsers))
	}
	rows, err := query.Order(dbent.Desc(customdomain.FieldCreatedAt)).Limit(500).All(ctx)
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
		SetAllUsers(domain.AllUsers).
		SetStatus(domain.Status).
		SetVerificationToken(domain.VerificationToken).
		SetVerificationTxtName(domain.VerificationTXTName).
		SetVerificationTxtValue(domain.VerificationTXTValue).
		SetUpdatedAt(time.Now())
	if domain.CNAMETarget == nil {
		update.ClearCnameTarget()
	} else {
		update.SetCnameTarget(*domain.CNAMETarget)
	}
	if domain.VerifiedAt == nil {
		update.ClearVerifiedAt()
	} else {
		update.SetVerifiedAt(*domain.VerifiedAt)
	}
	if domain.LastCheckedAt == nil {
		update.ClearLastCheckedAt()
	} else {
		update.SetLastCheckedAt(*domain.LastCheckedAt)
	}
	if domain.LastError == nil {
		update.ClearLastError()
	} else {
		update.SetLastError(*domain.LastError)
	}
	if domain.DisabledAt == nil {
		update.ClearDisabledAt()
	} else {
		update.SetDisabledAt(*domain.DisabledAt)
	}
	if domain.DisabledReason == nil {
		update.ClearDisabledReason()
	} else {
		update.SetDisabledReason(*domain.DisabledReason)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return nil, translateCustomDomainError(err)
	}
	return r.GetByID(ctx, row.ID)
}

func (r *customDomainRepository) Delete(ctx context.Context, id int64) error {
	err := clientFromContext(ctx, r.client).CustomDomain.DeleteOneID(id).Exec(ctx)
	return translateCustomDomainError(err)
}

func (r *customDomainRepository) SetAccess(ctx context.Context, id int64, allUsers bool, userIDs []int64) (*service.CustomDomain, error) {
	if existing := dbent.TxFromContext(ctx); existing != nil {
		txClient := existing.Client()
		if _, err := txClient.CustomDomain.UpdateOneID(id).SetAllUsers(allUsers).Save(ctx); err != nil {
			return nil, translateCustomDomainError(err)
		}
		if allUsers {
			userIDs = nil
		}
		if err := syncCustomDomainUsersWithClient(ctx, txClient, id, userIDs); err != nil {
			return nil, err
		}
		return r.GetByID(ctx, id)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	txClient := tx.Client()

	if _, err := txClient.CustomDomain.UpdateOneID(id).SetAllUsers(allUsers).Save(txCtx); err != nil {
		return nil, translateCustomDomainError(err)
	}
	if allUsers {
		userIDs = nil
	}
	if err := syncCustomDomainUsersWithClient(txCtx, txClient, id, userIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func customDomainAccessibleToUser(userID int64) predicate.CustomDomain {
	return customdomain.Or(
		customdomain.UserIDEQ(userID),
		customdomain.AllUsers(true),
		customdomain.HasAuthorizedUsersWith(user.IDEQ(userID)),
	)
}

func syncCustomDomainUsersWithClient(ctx context.Context, client *dbent.Client, domainID int64, userIDs []int64) error {
	if _, err := client.CustomDomainUser.Delete().Where(customdomainuser.CustomDomainIDEQ(domainID)).Exec(ctx); err != nil {
		return err
	}
	seen := make(map[int64]struct{}, len(userIDs))
	builders := make([]*dbent.CustomDomainUserCreate, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		builders = append(builders, client.CustomDomainUser.Create().SetCustomDomainID(domainID).SetUserID(userID))
	}
	if len(builders) == 0 {
		return nil
	}
	return client.CustomDomainUser.CreateBulk(builders...).
		OnConflictColumns(customdomainuser.FieldUserID, customdomainuser.FieldCustomDomainID).
		DoNothing().
		Exec(ctx)
}

func customDomainFromEnt(row *dbent.CustomDomain) *service.CustomDomain {
	if row == nil {
		return nil
	}
	users := make([]service.User, 0, len(row.Edges.AuthorizedUsers))
	for _, allowed := range row.Edges.AuthorizedUsers {
		if mapped := userEntityToService(allowed); mapped != nil {
			users = append(users, *mapped)
		}
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	userIDs := make([]int64, len(users))
	for i := range users {
		userIDs[i] = users[i].ID
	}
	return &service.CustomDomain{
		ID:                   row.ID,
		UserID:               row.UserID,
		Domain:               row.Domain,
		Status:               row.Status,
		AllUsers:             row.AllUsers,
		VerificationToken:    row.VerificationToken,
		VerificationTXTName:  row.VerificationTxtName,
		VerificationTXTValue: row.VerificationTxtValue,
		CNAMETarget:          row.CnameTarget,
		VerifiedAt:           row.VerifiedAt,
		LastCheckedAt:        row.LastCheckedAt,
		LastError:            row.LastError,
		DisabledAt:           row.DisabledAt,
		DisabledReason:       row.DisabledReason,
		DeletedAt:            row.DeletedAt,
		User:                 userEntityToService(row.Edges.User),
		AuthorizedUserIDs:    userIDs,
		AuthorizedUsers:      users,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

func customDomainSliceFromEnt(rows []*dbent.CustomDomain) []service.CustomDomain {
	out := make([]service.CustomDomain, 0, len(rows))
	for _, row := range rows {
		out = append(out, *customDomainFromEnt(row))
	}
	return out
}

func translateCustomDomainError(err error) error {
	if err == nil {
		return nil
	}
	var notFound *dbent.NotFoundError
	if errors.As(err, &notFound) {
		return service.ErrCustomDomainNotFound
	}
	var constraint *dbent.ConstraintError
	if errors.As(err, &constraint) {
		return service.ErrCustomDomainConflict
	}
	return err
}
