package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/redeemcodeusage"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type redeemCodeRepository struct {
	client *dbent.Client
}

func NewRedeemCodeRepository(client *dbent.Client) service.RedeemCodeRepository {
	return &redeemCodeRepository{client: client}
}

func (r *redeemCodeRepository) Create(ctx context.Context, code *service.RedeemCode) error {
	return r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		prepareRedeemCodeForCreate(code)

		created, err := redeemCodeCreateBuilder(client, code).Save(txCtx)
		if err != nil {
			return err
		}
		code.ID = created.ID
		code.CreatedAt = created.CreatedAt

		if shouldWriteRedeemUsageLedger(code) {
			if err := createRedeemUsageLedger(txCtx, client, code); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *redeemCodeRepository) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	if len(codes) == 0 {
		return nil
	}

	requiresLedger := false
	for i := range codes {
		prepareRedeemCodeForCreate(&codes[i])
		if shouldWriteRedeemUsageLedger(&codes[i]) {
			requiresLedger = true
		}
	}

	return r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		builders := make([]*dbent.RedeemCodeCreate, 0, len(codes))
		for i := range codes {
			builders = append(builders, redeemCodeCreateBuilder(client, &codes[i]))
		}

		created, err := client.RedeemCode.CreateBulk(builders...).Save(txCtx)
		if err != nil {
			return err
		}
		if !requiresLedger {
			return nil
		}
		for i := range created {
			codes[i].ID = created[i].ID
			codes[i].CreatedAt = created[i].CreatedAt
			if shouldWriteRedeemUsageLedger(&codes[i]) {
				if err := createRedeemUsageLedger(txCtx, client, &codes[i]); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *redeemCodeRepository) withTx(ctx context.Context, fn func(context.Context, *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin redeem code transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit redeem code transaction: %w", err)
	}
	return nil
}

func redeemCodeCreateBuilder(client *dbent.Client, code *service.RedeemCode) *dbent.RedeemCodeCreate {
	builder := client.RedeemCode.Create().
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays).
		SetUsedCount(code.UsedCount).
		SetNillableExpiresAt(code.ExpiresAt).
		SetNillableUsedBy(code.UsedBy).
		SetNillableUsedAt(code.UsedAt).
		SetNillableGroupID(code.GroupID).
		SetNillableCreatedBy(code.CreatedBy)
	if code.UsagePolicy != "" {
		builder.SetUsagePolicy(code.UsagePolicy)
	}
	if code.UsageScope != "" {
		builder.SetUsageScope(code.UsageScope)
	}
	if code.MaxTotalUses != nil {
		builder.SetMaxTotalUses(*code.MaxTotalUses)
	}
	if code.MaxUsesPerUser != nil {
		builder.SetMaxUsesPerUser(*code.MaxUsesPerUser)
	}
	return builder
}

func prepareRedeemCodeForCreate(code *service.RedeemCode) {
	if !shouldWriteRedeemUsageLedger(code) {
		return
	}
	if code.UsedAt == nil {
		now := time.Now().UTC()
		code.UsedAt = &now
	}
	if code.UsedCount < 1 {
		code.UsedCount = 1
	}
}

func shouldWriteRedeemUsageLedger(code *service.RedeemCode) bool {
	return code != nil && code.Status == service.StatusUsed && code.UsedBy != nil
}

func createRedeemUsageLedger(ctx context.Context, client *dbent.Client, code *service.RedeemCode) error {
	if !shouldWriteRedeemUsageLedger(code) || code.ID == 0 {
		return nil
	}
	usedAt := time.Now().UTC()
	if code.UsedAt != nil {
		usedAt = code.UsedAt.UTC()
	}
	scope := code.EffectiveUsageScope()
	if scope == "" {
		return service.ErrRedeemCodeUsed
	}

	_, err := client.RedeemCodeUsage.Create().
		SetRedeemCodeID(code.ID).
		SetUsageScope(scope).
		SetUserID(*code.UsedBy).
		SetCodeSnapshot(code.Code).
		SetTypeSnapshot(code.Type).
		SetValueSnapshot(code.Value).
		SetNillableGroupIDSnapshot(code.GroupID).
		SetValidityDaysSnapshot(code.ValidityDays).
		SetUsedAt(usedAt).
		SetMetadata(redeemUsageMetadata(code)).
		Save(ctx)
	if err != nil {
		if isUniqueConstraintViolation(err) {
			return service.ErrRedeemCodeUsed
		}
		return err
	}
	return nil
}

func redeemUsageMetadata(code *service.RedeemCode) map[string]any {
	metadata := map[string]any{}
	if code == nil {
		return metadata
	}
	if notes := strings.TrimSpace(code.Notes); notes != "" {
		metadata["notes"] = notes
	}
	return metadata
}

func (r *redeemCodeRepository) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	m, err := r.client.RedeemCode.Query().
		Where(redeemcode.IDEQ(id)).
		WithCreator().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
		}
		return nil, err
	}
	return redeemCodeEntityToService(m), nil
}

func (r *redeemCodeRepository) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	m, err := r.client.RedeemCode.Query().
		Where(redeemcode.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
		}
		return nil, err
	}
	return redeemCodeEntityToService(m), nil
}

func (r *redeemCodeRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.RedeemCode.Delete().Where(redeemcode.IDEQ(id)).Exec(ctx)
	return err
}

func (r *redeemCodeRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", nil)
}

func (r *redeemCodeRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string, createdBy *int64) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	q := r.client.RedeemCode.Query()

	if codeType != "" {
		q = q.Where(redeemcode.TypeEQ(codeType))
	}
	if status != "" {
		now := time.Now()
		switch status {
		case service.StatusExpired:
			q = q.Where(redeemcode.Or(
				redeemcode.StatusEQ(service.StatusExpired),
				redeemcode.And(
					redeemcode.StatusEQ(service.StatusUnused),
					redeemcode.ExpiresAtNotNil(),
					redeemcode.ExpiresAtLTE(now),
				),
			))
		case service.StatusUnused:
			q = q.Where(
				redeemcode.StatusEQ(service.StatusUnused),
				redeemcode.Or(
					redeemcode.ExpiresAtIsNil(),
					redeemcode.ExpiresAtGT(now),
				),
			)
		default:
			q = q.Where(redeemcode.StatusEQ(status))
		}
	}
	if search != "" {
		q = q.Where(
			redeemcode.Or(
				redeemcode.CodeContainsFold(search),
				redeemcode.NotesContainsFold(search),
				redeemcode.HasUserWith(user.EmailContainsFold(search)),
				redeemcode.HasCreatorWith(user.EmailContainsFold(search)),
			),
		)
	}
	if createdBy != nil {
		q = q.Where(redeemcode.CreatedByEQ(*createdBy))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	codesQuery := q.
		WithUser().
		WithGroup().
		WithCreator().
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range redeemCodeListOrder(params) {
		codesQuery = codesQuery.Order(order)
	}

	codes, err := codesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outCodes := redeemCodeEntitiesToService(codes)

	return outCodes, paginationResultFromTotal(int64(total), params), nil
}

func redeemCodeListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	var field string
	switch sortBy {
	case "type":
		field = redeemcode.FieldType
	case "value":
		field = redeemcode.FieldValue
	case "status":
		field = redeemcode.FieldStatus
	case "used_at":
		field = redeemcode.FieldUsedAt
	case "used_count":
		field = redeemcode.FieldUsedCount
	case "created_at":
		field = redeemcode.FieldCreatedAt
	case "expires_at":
		field = redeemcode.FieldExpiresAt
	case "code":
		field = redeemcode.FieldCode
	default:
		field = redeemcode.FieldID
	}

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(redeemcode.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(redeemcode.FieldID)}
}

func (r *redeemCodeRepository) Update(ctx context.Context, code *service.RedeemCode) error {
	up := r.client.RedeemCode.UpdateOneID(code.ID).
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays)

	if code.UsedBy != nil {
		up.SetUsedBy(*code.UsedBy)
	} else {
		up.ClearUsedBy()
	}
	if code.UsedAt != nil {
		up.SetUsedAt(*code.UsedAt)
	} else {
		up.ClearUsedAt()
	}
	if code.GroupID != nil {
		up.SetGroupID(*code.GroupID)
	} else {
		up.ClearGroupID()
	}
	if code.ExpiresAt != nil {
		up.SetExpiresAt(*code.ExpiresAt)
	} else {
		up.ClearExpiresAt()
	}

	updated, err := up.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrRedeemCodeNotFound
		}
		return err
	}
	code.CreatedAt = updated.CreatedAt
	return nil
}

func (r *redeemCodeRepository) BatchUpdate(ctx context.Context, ids []int64, fields service.RedeemCodeBatchUpdateFields) (int64, error) {
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return 0, nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.batchUpdate(ctx, tx.Client(), uniqueIDs, fields)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	defer func() { _ = tx.Rollback() }()

	updated, err := r.batchUpdate(txCtx, tx.Client(), uniqueIDs, fields)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

func (r *redeemCodeRepository) batchUpdate(ctx context.Context, client *dbent.Client, ids []int64, fields service.RedeemCodeBatchUpdateFields) (int64, error) {
	existing, err := client.RedeemCode.Query().
		Where(redeemcode.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return 0, err
	}
	if len(existing) != len(ids) {
		return 0, service.ErrRedeemCodeNotFound
	}
	if fields.TouchesUsedSensitiveFields() {
		for _, code := range existing {
			if code.Status == service.StatusUsed {
				return 0, service.ErrRedeemCodeUsed
			}
		}
	}

	up := client.RedeemCode.Update().Where(redeemcode.IDIn(ids...))
	if fields.Status != nil {
		up.SetStatus(*fields.Status)
	}
	if fields.Notes != nil {
		up.SetNotes(*fields.Notes)
	}
	if fields.ExpiresAt.Set {
		if fields.ExpiresAt.Value != nil {
			up.SetExpiresAt(*fields.ExpiresAt.Value)
		} else {
			up.ClearExpiresAt()
		}
	}
	if fields.GroupID.Set {
		if fields.GroupID.Value != nil {
			up.SetGroupID(*fields.GroupID.Value)
		} else {
			up.ClearGroupID()
		}
	}

	affected, err := up.Save(ctx)
	if err != nil {
		return 0, err
	}
	if affected != len(ids) {
		return 0, service.ErrRedeemCodeNotFound
	}
	return int64(affected), nil
}

func (r *redeemCodeRepository) Use(ctx context.Context, id, userID int64) error {
	return r.withTx(ctx, func(txCtx context.Context, client *dbent.Client) error {
		now := time.Now().UTC()

		m, err := client.RedeemCode.Query().
			Where(redeemcode.IDEQ(id)).
			ForUpdate().
			Only(txCtx)
		if err != nil && strings.Contains(err.Error(), "FOR UPDATE/SHARE not supported in SQLite") {
			m, err = client.RedeemCode.Query().
				Where(redeemcode.IDEQ(id)).
				Only(txCtx)
		}
		if err != nil {
			if dbent.IsNotFound(err) {
				return service.ErrRedeemCodeNotFound
			}
			return err
		}
		redeemCode := redeemCodeEntityToService(m)
		if redeemCode == nil {
			return service.ErrRedeemCodeNotFound
		}
		if redeemCode.IsExpired() {
			return service.ErrRedeemCodeExpired
		}
		if redeemCode.Status != service.StatusUnused {
			return service.ErrRedeemCodeUsed
		}

		policy := redeemCode.EffectiveUsagePolicy()
		scope := redeemCode.EffectiveUsageScope()
		if scope == "" {
			return service.ErrRedeemCodeUsed
		}

		if policy == service.RedeemUsagePolicyOncePerUser {
			perUserCap := 1
			if redeemCode.MaxUsesPerUser != nil {
				perUserCap = *redeemCode.MaxUsesPerUser
			}
			if perUserCap != 1 {
				return service.ErrRedeemCodeUsed
			}
			exists, err := client.RedeemCodeUsage.Query().
				Where(
					redeemcodeusage.UsageScopeEQ(scope),
					redeemcodeusage.UserIDEQ(userID),
				).
				Exist(txCtx)
			if err != nil {
				return err
			}
			if exists {
				return service.ErrRedeemCodeUsed
			}
		} else if policy != service.RedeemUsagePolicySingleUse {
			return service.ErrRedeemCodeUsed
		}

		maxTotalUses := 1
		if policy == service.RedeemUsagePolicyOncePerUser {
			maxTotalUses = 0
		}
		if redeemCode.MaxTotalUses != nil {
			maxTotalUses = *redeemCode.MaxTotalUses
		}
		if maxTotalUses > 0 && redeemCode.UsedCount >= maxTotalUses {
			return service.ErrRedeemCodeUsed
		}

		usageBuilder := client.RedeemCodeUsage.Create().
			SetRedeemCodeID(redeemCode.ID).
			SetUsageScope(scope).
			SetUserID(userID).
			SetCodeSnapshot(redeemCode.Code).
			SetTypeSnapshot(redeemCode.Type).
			SetValueSnapshot(redeemCode.Value).
			SetNillableGroupIDSnapshot(redeemCode.GroupID).
			SetValidityDaysSnapshot(redeemCode.ValidityDays).
			SetUsedAt(now).
			SetMetadata(redeemUsageMetadata(redeemCode))
		if _, err := usageBuilder.Save(txCtx); err != nil {
			if isUniqueConstraintViolation(err) {
				return service.ErrRedeemCodeUsed
			}
			return err
		}

		newUsedCount := redeemCode.UsedCount + 1
		update := client.RedeemCode.UpdateOneID(redeemCode.ID).
			SetUsedCount(newUsedCount)
		if redeemCode.UsageScope == "" {
			update.SetUsageScope(scope)
		}
		if maxTotalUses > 0 && newUsedCount >= maxTotalUses {
			update.SetStatus(service.StatusUsed).
				SetUsedBy(userID).
				SetUsedAt(now)
		}
		if _, err := update.Save(txCtx); err != nil {
			if dbent.IsNotFound(err) {
				return service.ErrRedeemCodeNotFound
			}
			return err
		}
		return nil
	})
}

func (r *redeemCodeRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if limit <= 0 {
		limit = 10
	}

	usages, err := clientFromContext(ctx, r.client).RedeemCodeUsage.Query().
		Where(redeemcodeusage.UserIDEQ(userID)).
		WithRedeemCode(func(q *dbent.RedeemCodeQuery) {
			q.WithGroup()
		}).
		Order(dbent.Desc(redeemcodeusage.FieldUsedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	return redeemCodeUsagesToService(usages), nil
}

// ListByUserPaginated returns paginated balance/concurrency history for a user.
// Supports optional type filter (e.g. "balance", "admin_balance", "concurrency", "admin_concurrency", "subscription").
func (r *redeemCodeRepository) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	q := clientFromContext(ctx, r.client).RedeemCodeUsage.Query().
		Where(redeemcodeusage.UserIDEQ(userID))

	if codeType != "" {
		q = q.Where(redeemcodeusage.TypeSnapshotEQ(codeType))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	usages, err := q.
		WithRedeemCode(func(q *dbent.RedeemCodeQuery) {
			q.WithGroup()
		}).
		Order(dbent.Desc(redeemcodeusage.FieldUsedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return redeemCodeUsagesToService(usages), paginationResultFromTotal(int64(total), params), nil
}

// SumPositiveBalanceByUser returns total recharged amount (sum of value > 0 where type is balance/admin_balance).
func (r *redeemCodeRepository) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	var result []struct {
		Sum float64 `json:"sum"`
	}
	err := clientFromContext(ctx, r.client).RedeemCodeUsage.Query().
		Where(
			redeemcodeusage.UserIDEQ(userID),
			redeemcodeusage.ValueSnapshotGT(0),
			redeemcodeusage.TypeSnapshotIn(service.RedeemTypeBalance, service.AdjustmentTypeAdminBalance),
		).
		Aggregate(dbent.As(dbent.Sum(redeemcodeusage.FieldValueSnapshot), "sum")).
		Scan(ctx, &result)
	if err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}
	return result[0].Sum, nil
}

func redeemCodeEntityToService(m *dbent.RedeemCode) *service.RedeemCode {
	if m == nil {
		return nil
	}
	out := &service.RedeemCode{
		ID:           m.ID,
		Code:         m.Code,
		Type:         m.Type,
		Value:        m.Value,
		Status:       m.Status,
		UsedBy:       m.UsedBy,
		UsedAt:       m.UsedAt,
		Notes:        derefString(m.Notes),
		CreatedBy:    m.CreatedBy,
		CreatedAt:    m.CreatedAt,
		ExpiresAt:    m.ExpiresAt,
		GroupID:      m.GroupID,
		ValidityDays: m.ValidityDays,
		UsagePolicy:  m.UsagePolicy,
		UsedCount:    m.UsedCount,
	}
	if m.UsageScope != nil {
		out.UsageScope = *m.UsageScope
	}
	if m.MaxTotalUses != nil {
		out.MaxTotalUses = new(int)
		*out.MaxTotalUses = *m.MaxTotalUses
	}
	if m.MaxUsesPerUser != nil {
		out.MaxUsesPerUser = new(int)
		*out.MaxUsesPerUser = *m.MaxUsesPerUser
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	if m.Edges.Creator != nil {
		out.CreatedByUser = userEntityToService(m.Edges.Creator)
	}
	return out
}

func redeemCodeEntitiesToService(models []*dbent.RedeemCode) []service.RedeemCode {
	out := make([]service.RedeemCode, 0, len(models))
	for i := range models {
		if s := redeemCodeEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func redeemCodeUsageEntityToService(m *dbent.RedeemCodeUsage) *service.RedeemCode {
	if m == nil {
		return nil
	}
	out := &service.RedeemCode{
		ID:           m.RedeemCodeID,
		Code:         m.CodeSnapshot,
		Type:         m.TypeSnapshot,
		Value:        m.ValueSnapshot,
		Status:       service.StatusUsed,
		UsedAt:       &m.UsedAt,
		GroupID:      m.GroupIDSnapshot,
		ValidityDays: m.ValidityDaysSnapshot,
		UsageScope:   m.UsageScope,
		UsedCount:    1,
		Notes:        redeemUsageMetadataString(m.Metadata, "notes"),
	}
	userID := m.UserID
	out.UsedBy = &userID
	if m.Edges.RedeemCode != nil && m.Edges.RedeemCode.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.RedeemCode.Edges.Group)
	} else if m.GroupIDSnapshot != nil {
		groupID := *m.GroupIDSnapshot
		out.Group = &service.Group{ID: groupID, Hydrated: true}
	}
	return out
}

func redeemUsageMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return s
}

func redeemCodeUsagesToService(models []*dbent.RedeemCodeUsage) []service.RedeemCode {
	out := make([]service.RedeemCode, 0, len(models))
	for i := range models {
		if s := redeemCodeUsageEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}
