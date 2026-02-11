package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type redeemCodeRepository struct {
	client *dbent.Client
REDACTED

func NewRedeemCodeRepository(client *dbent.Client) service.RedeemCodeRepository {
	return &redeemCodeRepository{client: clientREDACTED
REDACTED

func (r *redeemCodeRepository) Create(ctx context.Context, code *service.RedeemCode) error {
	created, err := r.client.RedeemCode.Create().
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays).
		SetNillableUsedBy(code.UsedBy).
		SetNillableUsedAt(code.UsedAt).
		SetNillableGroupID(code.GroupID).
		Save(ctx)
	if err == nil {
		code.ID = created.ID
		code.CreatedAt = created.CreatedAt
REDACTED
	return err
REDACTED

func (r *redeemCodeRepository) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	if len(codes) == 0 {
		return nil
REDACTED

	builders := make([]*dbent.RedeemCodeCreate, 0, len(codes))
	for i := range codes {
		c := &codes[i]
		b := r.client.RedeemCode.Create().
			SetCode(c.Code).
			SetType(c.Type).
			SetValue(c.Value).
			SetStatus(c.Status).
			SetNotes(c.Notes).
			SetValidityDays(c.ValidityDays).
			SetNillableUsedBy(c.UsedBy).
			SetNillableUsedAt(c.UsedAt).
			SetNillableGroupID(c.GroupID)
		builders = append(builders, b)
REDACTED

	return r.client.RedeemCode.CreateBulk(builders...).Exec(ctx)
REDACTED

func (r *redeemCodeRepository) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	m, err := r.client.RedeemCode.Query().
		Where(redeemcode.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
	REDACTED
		return nil, err
REDACTED
	return redeemCodeEntityToService(m), nil
REDACTED

func (r *redeemCodeRepository) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	m, err := r.client.RedeemCode.Query().
		Where(redeemcode.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
	REDACTED
		return nil, err
REDACTED
	return redeemCodeEntityToService(m), nil
REDACTED

func (r *redeemCodeRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.RedeemCode.Delete().Where(redeemcode.IDEQ(id)).Exec(ctx)
	return err
REDACTED

func (r *redeemCodeRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

func (r *redeemCodeRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	q := r.client.RedeemCode.Query()

	if codeType != "" {
		q = q.Where(redeemcode.TypeEQ(codeType))
REDACTED
	if status != "" {
		q = q.Where(redeemcode.StatusEQ(status))
REDACTED
	if search != "" {
		q = q.Where(
			redeemcode.Or(
				redeemcode.CodeContainsFold(search),
				redeemcode.HasUserWith(user.EmailContainsFold(search)),
			),
		)
REDACTED

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	codes, err := q.
		WithUser().
		WithGroup().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(redeemcode.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	outCodes := redeemCodeEntitiesToService(codes)

	return outCodes, paginationResultFromTotal(int64(total), params), nil
REDACTED

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
REDACTED else {
		up.ClearUsedBy()
REDACTED
	if code.UsedAt != nil {
		up.SetUsedAt(*code.UsedAt)
REDACTED else {
		up.ClearUsedAt()
REDACTED
	if code.GroupID != nil {
		up.SetGroupID(*code.GroupID)
REDACTED else {
		up.ClearGroupID()
REDACTED

	updated, err := up.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrRedeemCodeNotFound
	REDACTED
		return err
REDACTED
	code.CreatedAt = updated.CreatedAt
	return nil
REDACTED

func (r *redeemCodeRepository) Use(ctx context.Context, id, userID int64) error {
	now := time.Now()
	client := clientFromContext(ctx, r.client)
	affected, err := client.RedeemCode.Update().
		Where(redeemcode.IDEQ(id), redeemcode.StatusEQ(service.StatusUnused)).
		SetStatus(service.StatusUsed).
		SetUsedBy(userID).
		SetUsedAt(now).
		Save(ctx)
	if err != nil {
		return err
REDACTED
	if affected == 0 {
		return service.ErrRedeemCodeUsed
REDACTED
	return nil
REDACTED

func (r *redeemCodeRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if limit <= 0 {
		limit = 10
REDACTED

	codes, err := r.client.RedeemCode.Query().
		Where(redeemcode.UsedByEQ(userID)).
		WithGroup().
		Order(dbent.Desc(redeemcode.FieldUsedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	return redeemCodeEntitiesToService(codes), nil
REDACTED

// ListByUserPaginated returns paginated balance/concurrency history for a user.
// Supports optional type filter (e.g. "balance", "admin_balance", "concurrency", "admin_concurrency", "subscription").
func (r *redeemCodeRepository) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	q := r.client.RedeemCode.Query().
		Where(redeemcode.UsedByEQ(userID))

	// Optional type filter
	if codeType != "" {
		q = q.Where(redeemcode.TypeEQ(codeType))
REDACTED

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	codes, err := q.
		WithGroup().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(redeemcode.FieldUsedAt)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	return redeemCodeEntitiesToService(codes), paginationResultFromTotal(int64(total), params), nil
REDACTED

// SumPositiveBalanceByUser returns total recharged amount (sum of value > 0 where type is balance/admin_balance).
func (r *redeemCodeRepository) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	var result []struct {
		Sum float64 `json:"sum"`
REDACTED
	err := r.client.RedeemCode.Query().
		Where(
			redeemcode.UsedByEQ(userID),
			redeemcode.ValueGT(0),
			redeemcode.TypeIn("balance", "admin_balance"),
		).
		Aggregate(dbent.As(dbent.Sum(redeemcode.FieldValue), "sum")).
		Scan(ctx, &result)
	if err != nil {
		return 0, err
REDACTED
	if len(result) == 0 {
		return 0, nil
REDACTED
	return result[0].Sum, nil
REDACTED

func redeemCodeEntityToService(m *dbent.RedeemCode) *service.RedeemCode {
	if m == nil {
		return nil
REDACTED
	out := &service.RedeemCode{
		ID:           m.ID,
		Code:         m.Code,
		Type:         m.Type,
		Value:        m.Value,
		Status:       m.Status,
		UsedBy:       m.UsedBy,
		UsedAt:       m.UsedAt,
		Notes:        derefString(m.Notes),
		CreatedAt:    m.CreatedAt,
		GroupID:      m.GroupID,
		ValidityDays: m.ValidityDays,
REDACTED
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
REDACTED
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
REDACTED
	return out
REDACTED

func redeemCodeEntitiesToService(models []*dbent.RedeemCode) []service.RedeemCode {
	out := make([]service.RedeemCode, 0, len(models))
	for i := range models {
		if s := redeemCodeEntityToService(models[i]); s != nil {
			out = append(out, *s)
	REDACTED
REDACTED
	return out
REDACTED
