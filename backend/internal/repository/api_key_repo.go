package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type apiKeyRepository struct {
	client *dbent.Client
REDACTED

func NewAPIKeyRepository(client *dbent.Client) service.APIKeyRepository {
	return &apiKeyRepository{client: clientREDACTED
REDACTED

func (r *apiKeyRepository) activeQuery() *dbent.APIKeyQuery {
	// 默认过滤已软删除记录，避免删除后仍被查询到。
	return r.client.APIKey.Query().Where(apikey.DeletedAtIsNil())
REDACTED

func (r *apiKeyRepository) Create(ctx context.Context, key *service.APIKey) error {
	created, err := r.client.APIKey.Create().
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
	return translatePersistenceError(err, nil, service.ErrAPIKeyExists)
REDACTED

func (r *apiKeyRepository) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	m, err := r.activeQuery().
		Where(apikey.IDEQ(id)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
	REDACTED
		return nil, err
REDACTED
	return apiKeyEntityToService(m), nil
REDACTED

// GetOwnerID 根据 API Key ID 获取其所有者（用户）的 ID。
// 相比 GetByID，此方法性能更优，因为：
//   - 使用 Select() 只查询 user_id 字段，减少数据传输量
//   - 不加载完整的 API Key 实体及其关联数据（User、Group 等）
//   - 适用于权限验证等只需用户 ID 的场景（如删除前的所有权检查）
func (r *apiKeyRepository) GetOwnerID(ctx context.Context, id int64) (int64, error) {
	m, err := r.activeQuery().
		Where(apikey.IDEQ(id)).
		Select(apikey.FieldUserID).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return 0, service.ErrAPIKeyNotFound
	REDACTED
		return 0, err
REDACTED
	return m.UserID, nil
REDACTED

func (r *apiKeyRepository) GetByKey(ctx context.Context, key string) (*service.APIKey, error) {
	m, err := r.activeQuery().
		Where(apikey.KeyEQ(key)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
	REDACTED
		return nil, err
REDACTED
	return apiKeyEntityToService(m), nil
REDACTED

func (r *apiKeyRepository) Update(ctx context.Context, key *service.APIKey) error {
	// 使用原子操作：将软删除检查与更新合并到同一语句，避免竞态条件。
	// 之前的实现先检查 Exist 再 UpdateOneID，若在两步之间发生软删除，
	// 则会更新已删除的记录。
	// 这里选择 Update().Where()，确保只有未软删除记录能被更新。
	// 同时显式设置 updated_at，避免二次查询带来的并发可见性问题。
	now := time.Now()
	builder := r.client.APIKey.Update().
		Where(apikey.IDEQ(key.ID), apikey.DeletedAtIsNil()).
		SetName(key.Name).
		SetStatus(key.Status).
		SetUpdatedAt(now)
	if key.GroupID != nil {
		builder.SetGroupID(*key.GroupID)
REDACTED else {
		builder.ClearGroupID()
REDACTED

	affected, err := builder.Save(ctx)
	if err != nil {
		return err
REDACTED
	if affected == 0 {
		// 更新影响行数为 0，说明记录不存在或已被软删除。
		return service.ErrAPIKeyNotFound
REDACTED

	// 使用同一时间戳回填，避免并发删除导致二次查询失败。
	key.UpdatedAt = now
	return nil
REDACTED

func (r *apiKeyRepository) Delete(ctx context.Context, id int64) error {
	// 显式软删除：避免依赖 Hook 行为，确保 deleted_at 一定被设置。
	affected, err := r.client.APIKey.Update().
		Where(apikey.IDEQ(id), apikey.DeletedAtIsNil()).
		SetDeletedAt(time.Now()).
		Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrAPIKeyNotFound
	REDACTED
		return err
REDACTED
	if affected == 0 {
		exists, err := r.client.APIKey.Query().
			Where(apikey.IDEQ(id)).
			Exist(mixins.SkipSoftDelete(ctx))
		if err != nil {
			return err
	REDACTED
		if exists {
			return nil
	REDACTED
		return service.ErrAPIKeyNotFound
REDACTED
	return nil
REDACTED

func (r *apiKeyRepository) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	q := r.activeQuery().Where(apikey.UserIDEQ(userID))

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

	outKeys := make([]service.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
REDACTED

	return outKeys, paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *apiKeyRepository) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	if len(apiKeyIDs) == 0 {
		return []int64{REDACTED, nil
REDACTED

	ids, err := r.client.APIKey.Query().
		Where(apikey.UserIDEQ(userID), apikey.IDIn(apiKeyIDs...), apikey.DeletedAtIsNil()).
		IDs(ctx)
	if err != nil {
		return nil, err
REDACTED
	return ids, nil
REDACTED

func (r *apiKeyRepository) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	count, err := r.activeQuery().Where(apikey.UserIDEQ(userID)).Count(ctx)
	return int64(count), err
REDACTED

func (r *apiKeyRepository) ExistsByKey(ctx context.Context, key string) (bool, error) {
	count, err := r.activeQuery().Where(apikey.KeyEQ(key)).Count(ctx)
	return count > 0, err
REDACTED

func (r *apiKeyRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	q := r.activeQuery().Where(apikey.GroupIDEQ(groupID))

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

	outKeys := make([]service.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
REDACTED

	return outKeys, paginationResultFromTotal(int64(total), params), nil
REDACTED

// SearchAPIKeys searches API keys by user ID and/or keyword (name)
func (r *apiKeyRepository) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	q := r.activeQuery()
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

	outKeys := make([]service.APIKey, 0, len(keys))
	for i := range keys {
		outKeys = append(outKeys, *apiKeyEntityToService(keys[i]))
REDACTED
	return outKeys, nil
REDACTED

// ClearGroupIDByGroupID 将指定分组的所有 API Key 的 group_id 设为 nil
func (r *apiKeyRepository) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	n, err := r.client.APIKey.Update().
		Where(apikey.GroupIDEQ(groupID), apikey.DeletedAtIsNil()).
		ClearGroupID().
		Save(ctx)
	return int64(n), err
REDACTED

// CountByGroupID 获取分组的 API Key 数量
func (r *apiKeyRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	count, err := r.activeQuery().Where(apikey.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
REDACTED

func apiKeyEntityToService(m *dbent.APIKey) *service.APIKey {
	if m == nil {
		return nil
REDACTED
	out := &service.APIKey{
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
		ID:                  g.ID,
		Name:                g.Name,
		Description:         derefString(g.Description),
		Platform:            g.Platform,
		RateMultiplier:      g.RateMultiplier,
		IsExclusive:         g.IsExclusive,
		Status:              g.Status,
		SubscriptionType:    g.SubscriptionType,
		DailyLimitUSD:       g.DailyLimitUsd,
		WeeklyLimitUSD:      g.WeeklyLimitUsd,
		MonthlyLimitUSD:     g.MonthlyLimitUsd,
		ImagePrice1K:        g.ImagePrice1k,
		ImagePrice2K:        g.ImagePrice2k,
		ImagePrice4K:        g.ImagePrice4k,
		DefaultValidityDays: g.DefaultValidityDays,
		ClaudeCodeOnly:      g.ClaudeCodeOnly,
		FallbackGroupID:     g.FallbackGroupID,
		CreatedAt:           g.CreatedAt,
		UpdatedAt:           g.UpdatedAt,
REDACTED
REDACTED

func derefString(s *string) string {
	if s == nil {
		return ""
REDACTED
	return *s
REDACTED
