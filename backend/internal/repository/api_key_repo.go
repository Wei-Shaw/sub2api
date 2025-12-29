package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type apiKeyRepository struct {
	client *dbent.Client
REDACTED

func NewApiKeyRepository(client *dbent.Client) service.ApiKeyRepository {
	return &apiKeyRepository{client: clientREDACTED
REDACTED

func (r *apiKeyRepository) Create(ctx context.Context, key *service.ApiKey) error {
	created, err := r.client.ApiKey.Create().
		SetUserID(key.UserID).
		SetKey(key.Key).
		SetName(key.Name).
		SetStatus(key.Status).
		SetNillableGroupID(key.GroupID).
		Save(ctx)
	if err == nil {
		key.ID = created.ID
		key.CreatedAt = created.CreatedAt
		key.UpdatedAt = created.UpdatedAt
REDACTED
	return translatePersistenceError(err, nil, service.ErrApiKeyExists)
REDACTED

func (r *apiKeyRepository) GetByID(ctx context.Context, id int64) (*service.ApiKey, error) {
	m, err := r.client.ApiKey.Query().
		Where(apikey.IDEQ(id)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrApiKeyNotFound
	REDACTED
		return nil, err
REDACTED
	return apiKeyEntityToService(m), nil
REDACTED

func (r *apiKeyRepository) GetByKey(ctx context.Context, key string) (*service.ApiKey, error) {
	m, err := r.client.ApiKey.Query().
		Where(apikey.KeyEQ(key)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrApiKeyNotFound
	REDACTED
		return nil, err
REDACTED
	return apiKeyEntityToService(m), nil
REDACTED

func (r *apiKeyRepository) Update(ctx context.Context, key *service.ApiKey) error {
	builder := r.client.ApiKey.UpdateOneID(key.ID).
		SetName(key.Name).
		SetStatus(key.Status)
	if key.GroupID != nil {
		builder.SetGroupID(*key.GroupID)
REDACTED else {
		builder.ClearGroupID()
REDACTED

	updated, err := builder.Save(ctx)
	if err == nil {
		key.UpdatedAt = updated.UpdatedAt
		return nil
REDACTED
	if dbent.IsNotFound(err) {
		return service.ErrApiKeyNotFound
REDACTED
	return err
REDACTED

func (r *apiKeyRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.client.ApiKey.Delete().Where(apikey.IDEQ(id)).Exec(ctx)
	return err
REDACTED

func (r *apiKeyRepository) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.ApiKey, *pagination.PaginationResult, error) {
	q := r.client.ApiKey.Query().Where(apikey.UserIDEQ(userID))

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	keys, err := q.
		WithGroup().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(apikey.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	outKeys := make([]service.ApiKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
REDACTED

	return outKeys, paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *apiKeyRepository) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{REDACTED, nil
REDACTED

	ids, err := r.client.ApiKey.Query().
		Where(apikey.UserIDEQ(userID), apikey.IDIn(apiKeyIDs...)).
		IDs(ctx)
	if err != nil {
		return nil, err
REDACTED
	return ids, nil
REDACTED

func (r *apiKeyRepository) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	count, err := r.client.ApiKey.Query().Where(apikey.UserIDEQ(userID)).Count(ctx)
	return int64(count), err
REDACTED

func (r *apiKeyRepository) ExistsByKey(ctx context.Context, key string) (bool, error) {
	count, err := r.client.ApiKey.Query().Where(apikey.KeyEQ(key)).Count(ctx)
	return count > 0, err
REDACTED

func (r *apiKeyRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.ApiKey, *pagination.PaginationResult, error) {
	q := r.client.ApiKey.Query().Where(apikey.GroupIDEQ(groupID))

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	keys, err := q.
		WithUser().
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(apikey.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	outKeys := make([]service.ApiKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
REDACTED

	return outKeys, paginationResultFromTotal(int64(total), params), nil
REDACTED

// SearchApiKeys searches API keys by user ID and/or keyword (name)
func (r *apiKeyRepository) SearchApiKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.ApiKey, error) {
	q := r.client.ApiKey.Query()
	if userID > 0 {
		q = q.Where(apikey.UserIDEQ(userID))
REDACTED

	if keyword != "" {
		q = q.Where(apikey.NameContainsFold(keyword))
REDACTED

	keys, err := q.Limit(limit).Order(dbent.Desc(apikey.FieldID)).All(ctx)
	if err != nil {
		return nil, err
REDACTED

	outKeys := make([]service.ApiKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
REDACTED
	return outKeys, nil
REDACTED

// ClearGroupIDByGroupID 将指定分组的所有 API Key 的 group_id 设为 nil
func (r *apiKeyRepository) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	n, err := r.client.ApiKey.Update().
		Where(apikey.GroupIDEQ(groupID)).
		ClearGroupID().
		Save(ctx)
	return int64(n), err
REDACTED

// CountByGroupID 获取分组的 API Key 数量
func (r *apiKeyRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	count, err := r.client.ApiKey.Query().Where(apikey.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
REDACTED

func apiKeyEntityToService(m *dbent.ApiKey) *service.ApiKey {
	if m == nil {
		return nil
REDACTED
	out := &service.ApiKey{
		ID:        m.ID,
		UserID:    m.UserID,
		Key:       m.Key,
		Name:      m.Name,
		Status:    m.Status,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		GroupID:   m.GroupID,
REDACTED
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
REDACTED
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
REDACTED
	return out
REDACTED

func userEntityToService(u *dbent.User) *service.User {
	if u == nil {
		return nil
REDACTED
	return &service.User{
		ID:           u.ID,
		Email:        u.Email,
		Username:     u.Username,
		Wechat:       u.Wechat,
		Notes:        u.Notes,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		Balance:      u.Balance,
		Concurrency:  u.Concurrency,
		Status:       u.Status,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
REDACTED
REDACTED

func groupEntityToService(g *dbent.Group) *service.Group {
	if g == nil {
		return nil
REDACTED
	return &service.Group{
		ID:               g.ID,
		Name:             g.Name,
		Description:      derefString(g.Description),
		Platform:         g.Platform,
		RateMultiplier:   g.RateMultiplier,
		IsExclusive:      g.IsExclusive,
		Status:           g.Status,
		SubscriptionType: g.SubscriptionType,
		DailyLimitUSD:    g.DailyLimitUsd,
		WeeklyLimitUSD:   g.WeeklyLimitUsd,
		MonthlyLimitUSD:  g.MonthlyLimitUsd,
		CreatedAt:        g.CreatedAt,
		UpdatedAt:        g.UpdatedAt,
REDACTED
REDACTED

func derefString(s *string) string {
	if s == nil {
		return ""
REDACTED
	return *s
REDACTED
