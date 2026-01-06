// Package repository 实现数据访问层（Repository Pattern）。
//
// 该包提供了与数据库交互的所有操作，包括 CRUD、复杂查询和批量操作。
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离，便于测试和维护。
//
// 主要特性：
//   - 使用 Ent ORM 进行类型安全的数据库操作
//   - 对于复杂查询（如批量更新、聚合统计）使用原生 SQL
//   - 提供统一的错误翻译机制，将数据库错误转换为业务错误
//   - 支持软删除，所有查询自动过滤已删除记录
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	dbproxy "github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

// accountRepository 实现 service.AccountRepository 接口。
// 提供 AI API 账户的完整数据访问功能。
//
// 设计说明：
//   - client: Ent 客户端，用于类型安全的 ORM 操作
//   - sql: 原生 SQL 执行器，用于复杂查询和批量操作
type accountRepository struct {
	client *dbent.Client // Ent ORM 客户端
	sql    sqlExecutor   // 原生 SQL 执行接口
REDACTED

type tempUnschedSnapshot struct {
	until  *time.Time
	reason string
REDACTED

// NewAccountRepository 创建账户仓储实例。
// 这是对外暴露的构造函数，返回接口类型以便于依赖注入。
func NewAccountRepository(client *dbent.Client, sqlDB *sql.DB) service.AccountRepository {
	return newAccountRepositoryWithSQL(client, sqlDB)
REDACTED

// newAccountRepositoryWithSQL 是内部构造函数，支持依赖注入 SQL 执行器。
// 这种设计便于单元测试时注入 mock 对象。
func newAccountRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *accountRepository {
	return &accountRepository{client: client, sql: sqlqREDACTED
REDACTED

func (r *accountRepository) Create(ctx context.Context, account *service.Account) error {
	if account == nil {
		return service.ErrAccountNilInput
REDACTED

	builder := r.client.Account.Create().
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(normalizeJSONMap(account.Extra)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(account.Schedulable)

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
REDACTED
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
REDACTED
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
REDACTED
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
REDACTED
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
REDACTED
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
REDACTED
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
REDACTED
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
REDACTED

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
REDACTED

	account.ID = created.ID
	account.CreatedAt = created.CreatedAt
	account.UpdatedAt = created.UpdatedAt
	return nil
REDACTED

func (r *accountRepository) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	m, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
REDACTED

	accounts, err := r.accountsToService(ctx, []*dbent.Account{mREDACTED)
	if err != nil {
		return nil, err
REDACTED
	if len(accounts) == 0 {
		return nil, service.ErrAccountNotFound
REDACTED
	return &accounts[0], nil
REDACTED

func (r *accountRepository) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	if len(ids) == 0 {
		return []*service.Account{REDACTED, nil
REDACTED

	// De-duplicate while preserving order of first occurrence.
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{REDACTED, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
	REDACTED
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		uniqueIDs = append(uniqueIDs, id)
REDACTED
	if len(uniqueIDs) == 0 {
		return []*service.Account{REDACTED, nil
REDACTED

	entAccounts, err := r.client.Account.
		Query().
		Where(dbaccount.IDIn(uniqueIDs...)).
		WithProxy().
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	if len(entAccounts) == 0 {
		return []*service.Account{REDACTED, nil
REDACTED

	accountIDs := make([]int64, 0, len(entAccounts))
	entByID := make(map[int64]*dbent.Account, len(entAccounts))
	for _, acc := range entAccounts {
		entByID[acc.ID] = acc
		accountIDs = append(accountIDs, acc.ID)
REDACTED

	tempUnschedMap, err := r.loadTempUnschedStates(ctx, accountIDs)
	if err != nil {
		return nil, err
REDACTED

	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
REDACTED

	outByID := make(map[int64]*service.Account, len(entAccounts))
	for _, entAcc := range entAccounts {
		out := accountEntityToService(entAcc)
		if out == nil {
			continue
	REDACTED

		// Prefer the preloaded proxy edge when available.
		if entAcc.Edges.Proxy != nil {
			out.Proxy = proxyEntityToService(entAcc.Edges.Proxy)
	REDACTED

		if groups, ok := groupsByAccount[entAcc.ID]; ok {
			out.Groups = groups
	REDACTED
		if groupIDs, ok := groupIDsByAccount[entAcc.ID]; ok {
			out.GroupIDs = groupIDs
	REDACTED
		if ags, ok := accountGroupsByAccount[entAcc.ID]; ok {
			out.AccountGroups = ags
	REDACTED
		if snap, ok := tempUnschedMap[entAcc.ID]; ok {
			out.TempUnschedulableUntil = snap.until
			out.TempUnschedulableReason = snap.reason
	REDACTED
		outByID[entAcc.ID] = out
REDACTED

	// Preserve input order (first occurrence), and ignore missing IDs.
	out := make([]*service.Account, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if _, ok := entByID[id]; !ok {
			continue
	REDACTED
		if acc, ok := outByID[id]; ok && acc != nil {
			out = append(out, acc)
	REDACTED
REDACTED

	return out, nil
REDACTED

// ExistsByID 检查指定 ID 的账号是否存在。
// 相比 GetByID，此方法性能更优，因为：
//   - 使用 Exist() 方法生成 SELECT EXISTS 查询，只返回布尔值
//   - 不加载完整的账号实体及其关联数据（Groups、Proxy 等）
//   - 适用于删除前的存在性检查等只需判断有无的场景
func (r *accountRepository) ExistsByID(ctx context.Context, id int64) (bool, error) {
	exists, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Exist(ctx)
	if err != nil {
		return false, err
REDACTED
	return exists, nil
REDACTED

func (r *accountRepository) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	if crsAccountID == "" {
		return nil, nil
REDACTED

	// 使用 sqljson.ValueEQ 生成 JSON 路径过滤，避免手写 SQL 片段导致语法兼容问题。
	m, err := r.client.Account.Query().
		Where(func(s *entsql.Selector) {
			s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, crsAccountID, sqljson.Path("crs_account_id")))
	REDACTED).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED

	accounts, err := r.accountsToService(ctx, []*dbent.Account{mREDACTED)
	if err != nil {
		return nil, err
REDACTED
	if len(accounts) == 0 {
		return nil, nil
REDACTED
	return &accounts[0], nil
REDACTED

func (r *accountRepository) Update(ctx context.Context, account *service.Account) error {
	if account == nil {
		return nil
REDACTED

	builder := r.client.Account.UpdateOneID(account.ID).
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(normalizeJSONMap(account.Extra)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(account.Schedulable)

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
REDACTED else {
		builder.ClearProxyID()
REDACTED
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
REDACTED else {
		builder.ClearLastUsedAt()
REDACTED
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
REDACTED else {
		builder.ClearRateLimitedAt()
REDACTED
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
REDACTED else {
		builder.ClearRateLimitResetAt()
REDACTED
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
REDACTED else {
		builder.ClearOverloadUntil()
REDACTED
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
REDACTED else {
		builder.ClearSessionWindowStart()
REDACTED
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
REDACTED else {
		builder.ClearSessionWindowEnd()
REDACTED
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
REDACTED else {
		builder.ClearSessionWindowStatus()
REDACTED
	if account.Notes == nil {
		builder.ClearNotes()
REDACTED

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
REDACTED
	account.UpdatedAt = updated.UpdatedAt
	return nil
REDACTED

func (r *accountRepository) Delete(ctx context.Context, id int64) error {
	// 使用事务保证账号与关联分组的删除原子性
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
REDACTED

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() REDACTED()
		txClient = tx.Client()
REDACTED else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client
		txClient = r.client
REDACTED

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(id)).Exec(ctx); err != nil {
		return err
REDACTED
	if _, err := txClient.Account.Delete().Where(dbaccount.IDEQ(id)).Exec(ctx); err != nil {
		return err
REDACTED

	if tx != nil {
		return tx.Commit()
REDACTED
	return nil
REDACTED

func (r *accountRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", "")
REDACTED

func (r *accountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string) ([]service.Account, *pagination.PaginationResult, error) {
	q := r.client.Account.Query()

	if platform != "" {
		q = q.Where(dbaccount.PlatformEQ(platform))
REDACTED
	if accountType != "" {
		q = q.Where(dbaccount.TypeEQ(accountType))
REDACTED
	if status != "" {
		q = q.Where(dbaccount.StatusEQ(status))
REDACTED
	if search != "" {
		q = q.Where(dbaccount.NameContainsFold(search))
REDACTED

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	accounts, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(dbaccount.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	outAccounts, err := r.accountsToService(ctx, accounts)
	if err != nil {
		return nil, nil, err
REDACTED
	return outAccounts, paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *accountRepository) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	accounts, err := r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status: service.StatusActive,
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	return accounts, nil
REDACTED

func (r *accountRepository) ListActive(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(dbaccount.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.accountsToService(ctx, accounts)
REDACTED

func (r *accountRepository) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.accountsToService(ctx, accounts)
REDACTED

func (r *accountRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetLastUsedAt(now).
		Save(ctx)
	return err
REDACTED

func (r *accountRepository) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
REDACTED

	ids := make([]int64, 0, len(updates))
	args := make([]any, 0, len(updates)*2+1)
	caseSQL := "UPDATE accounts SET last_used_at = CASE id"

	idx := 1
	for id, ts := range updates {
		caseSQL += " WHEN $" + itoa(idx) + " THEN $" + itoa(idx+1) + "::timestamptz"
		args = append(args, id, ts)
		ids = append(ids, id)
		idx += 2
REDACTED

	caseSQL += " END, updated_at = NOW() WHERE id = ANY($" + itoa(idx) + ") AND deleted_at IS NULL"
	args = append(args, pq.Array(ids))

	_, err := r.sql.ExecContext(ctx, caseSQL, args...)
	return err
REDACTED

func (r *accountRepository) SetError(ctx context.Context, id int64, errorMsg string) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetStatus(service.StatusError).
		SetErrorMessage(errorMsg).
		Save(ctx)
	return err
REDACTED

func (r *accountRepository) AddToGroup(ctx context.Context, accountID, groupID int64, priority int) error {
	_, err := r.client.AccountGroup.Create().
		SetAccountID(accountID).
		SetGroupID(groupID).
		SetPriority(priority).
		Save(ctx)
	return err
REDACTED

func (r *accountRepository) RemoveFromGroup(ctx context.Context, accountID, groupID int64) error {
	_, err := r.client.AccountGroup.Delete().
		Where(
			dbaccountgroup.AccountIDEQ(accountID),
			dbaccountgroup.GroupIDEQ(groupID),
		).
		Exec(ctx)
	return err
REDACTED

func (r *accountRepository) GetGroups(ctx context.Context, accountID int64) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(
			dbgroup.HasAccountsWith(dbaccount.IDEQ(accountID)),
		).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *groupEntityToService(groups[i]))
REDACTED
	return outGroups, nil
REDACTED

func (r *accountRepository) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	// 使用事务保证删除旧绑定与创建新绑定的原子性
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
REDACTED

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() REDACTED()
		txClient = tx.Client()
REDACTED else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client
		txClient = r.client
REDACTED

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(accountID)).Exec(ctx); err != nil {
		return err
REDACTED

	if len(groupIDs) == 0 {
		if tx != nil {
			return tx.Commit()
	REDACTED
		return nil
REDACTED

	builders := make([]*dbent.AccountGroupCreate, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		builders = append(builders, txClient.AccountGroup.Create().
			SetAccountID(accountID).
			SetGroupID(groupID).
			SetPriority(i+1),
		)
REDACTED

	if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
		return err
REDACTED

	if tx != nil {
		return tx.Commit()
REDACTED
	return nil
REDACTED

func (r *accountRepository) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.accountsToService(ctx, accounts)
REDACTED

func (r *accountRepository) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
REDACTED)
REDACTED

func (r *accountRepository) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.accountsToService(ctx, accounts)
REDACTED

func (r *accountRepository) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	// 单平台查询复用多平台逻辑，保持过滤条件与排序策略一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   []string{platformREDACTED,
REDACTED)
REDACTED

func (r *accountRepository) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
REDACTED
	// 仅返回可调度的活跃账号，并过滤处于过载/限流窗口的账号。
	// 代理与分组信息统一在 accountsToService 中批量加载，避免 N+1 查询。
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformIn(platforms...),
			dbaccount.StatusEQ(service.StatusActive),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED
	return r.accountsToService(ctx, accounts)
REDACTED

func (r *accountRepository) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
REDACTED
	// 复用按分组查询逻辑，保证分组优先级 + 账号优先级的排序与筛选一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   platforms,
REDACTED)
REDACTED

func (r *accountRepository) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetRateLimitedAt(now).
		SetRateLimitResetAt(resetAt).
		Save(ctx)
	return err
REDACTED

func (r *accountRepository) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetOverloadUntil(until).
		Save(ctx)
	return err
REDACTED

func (r *accountRepository) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = $1,
			temp_unschedulable_reason = $2,
			updated_at = NOW()
		WHERE id = $3
			AND deleted_at IS NULL
			AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until < $1)
	`, until, reason, id)
	return err
REDACTED

func (r *accountRepository) ClearTempUnschedulable(ctx context.Context, id int64) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET temp_unschedulable_until = NULL,
			temp_unschedulable_reason = NULL,
			updated_at = NOW()
		WHERE id = $1
			AND deleted_at IS NULL
	`, id)
	return err
REDACTED

func (r *accountRepository) ClearRateLimit(ctx context.Context, id int64) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		ClearRateLimitedAt().
		ClearRateLimitResetAt().
		ClearOverloadUntil().
		Save(ctx)
	return err
REDACTED

func (r *accountRepository) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	builder := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSessionWindowStatus(status)
	if start != nil {
		builder.SetSessionWindowStart(*start)
REDACTED
	if end != nil {
		builder.SetSessionWindowEnd(*end)
REDACTED
	_, err := builder.Save(ctx)
	return err
REDACTED

func (r *accountRepository) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSchedulable(schedulable).
		Save(ctx)
	return err
REDACTED

func (r *accountRepository) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
REDACTED

	// 使用 JSONB 合并操作实现原子更新，避免读-改-写的并发丢失更新问题
	payload, err := json.Marshal(updates)
	if err != nil {
		return err
REDACTED

	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(
		ctx,
		"UPDATE accounts SET extra = COALESCE(extra, '{REDACTED'::jsonb) || $1::jsonb, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL",
		payload, id,
	)
	if err != nil {
		return err
REDACTED

	affected, err := result.RowsAffected()
	if err != nil {
		return err
REDACTED
	if affected == 0 {
		return service.ErrAccountNotFound
REDACTED
	return nil
REDACTED

func (r *accountRepository) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
REDACTED

	setClauses := make([]string, 0, 8)
	args := make([]any, 0, 8)

	idx := 1
	if updates.Name != nil {
		setClauses = append(setClauses, "name = $"+itoa(idx))
		args = append(args, *updates.Name)
		idx++
REDACTED
	if updates.ProxyID != nil {
		// 0 表示清除代理（前端发送 0 而不是 null 来表达清除意图）
		if *updates.ProxyID == 0 {
			setClauses = append(setClauses, "proxy_id = NULL")
	REDACTED else {
			setClauses = append(setClauses, "proxy_id = $"+itoa(idx))
			args = append(args, *updates.ProxyID)
			idx++
	REDACTED
REDACTED
	if updates.Concurrency != nil {
		setClauses = append(setClauses, "concurrency = $"+itoa(idx))
		args = append(args, *updates.Concurrency)
		idx++
REDACTED
	if updates.Priority != nil {
		setClauses = append(setClauses, "priority = $"+itoa(idx))
		args = append(args, *updates.Priority)
		idx++
REDACTED
	if updates.Status != nil {
		setClauses = append(setClauses, "status = $"+itoa(idx))
		args = append(args, *updates.Status)
		idx++
REDACTED
	// JSONB 需要合并而非覆盖，使用 raw SQL 保持旧行为。
	if len(updates.Credentials) > 0 {
		payload, err := json.Marshal(updates.Credentials)
		if err != nil {
			return 0, err
	REDACTED
		setClauses = append(setClauses, "credentials = COALESCE(credentials, '{REDACTED'::jsonb) || $"+itoa(idx)+"::jsonb")
		args = append(args, payload)
		idx++
REDACTED
	if len(updates.Extra) > 0 {
		payload, err := json.Marshal(updates.Extra)
		if err != nil {
			return 0, err
	REDACTED
		setClauses = append(setClauses, "extra = COALESCE(extra, '{REDACTED'::jsonb) || $"+itoa(idx)+"::jsonb")
		args = append(args, payload)
		idx++
REDACTED

	if len(setClauses) == 0 {
		return 0, nil
REDACTED

	setClauses = append(setClauses, "updated_at = NOW()")

	query := "UPDATE accounts SET " + joinClauses(setClauses, ", ") + " WHERE id = ANY($" + itoa(idx) + ") AND deleted_at IS NULL"
	args = append(args, pq.Array(ids))

	result, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
REDACTED
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
REDACTED
	return rows, nil
REDACTED

type accountGroupQueryOptions struct {
	status      string
	schedulable bool
	platforms   []string // 允许的多个平台，空切片表示不进行平台过滤
REDACTED

func (r *accountRepository) queryAccountsByGroup(ctx context.Context, groupID int64, opts accountGroupQueryOptions) ([]service.Account, error) {
	q := r.client.AccountGroup.Query().
		Where(dbaccountgroup.GroupIDEQ(groupID))

	// 通过 account_groups 中间表查询账号，并按需叠加状态/平台/调度能力过滤。
	preds := make([]dbpredicate.Account, 0, 6)
	preds = append(preds, dbaccount.DeletedAtIsNil())
	if opts.status != "" {
		preds = append(preds, dbaccount.StatusEQ(opts.status))
REDACTED
	if len(opts.platforms) > 0 {
		preds = append(preds, dbaccount.PlatformIn(opts.platforms...))
REDACTED
	if opts.schedulable {
		now := time.Now()
		preds = append(preds,
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
			dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
		)
REDACTED

	if len(preds) > 0 {
		q = q.Where(dbaccountgroup.HasAccountWith(preds...))
REDACTED

	groups, err := q.
		Order(
			dbaccountgroup.ByPriority(),
			dbaccountgroup.ByAccountField(dbaccount.FieldPriority),
		).
		WithAccount().
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	orderedIDs := make([]int64, 0, len(groups))
	accountMap := make(map[int64]*dbent.Account, len(groups))
	for _, ag := range groups {
		if ag.Edges.Account == nil {
			continue
	REDACTED
		if _, exists := accountMap[ag.AccountID]; exists {
			continue
	REDACTED
		accountMap[ag.AccountID] = ag.Edges.Account
		orderedIDs = append(orderedIDs, ag.AccountID)
REDACTED

	accounts := make([]*dbent.Account, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if acc, ok := accountMap[id]; ok {
			accounts = append(accounts, acc)
	REDACTED
REDACTED

	return r.accountsToService(ctx, accounts)
REDACTED

func (r *accountRepository) accountsToService(ctx context.Context, accounts []*dbent.Account) ([]service.Account, error) {
	if len(accounts) == 0 {
		return []service.Account{REDACTED, nil
REDACTED

	accountIDs := make([]int64, 0, len(accounts))
	proxyIDs := make([]int64, 0, len(accounts))
	for _, acc := range accounts {
		accountIDs = append(accountIDs, acc.ID)
		if acc.ProxyID != nil {
			proxyIDs = append(proxyIDs, *acc.ProxyID)
	REDACTED
REDACTED

	proxyMap, err := r.loadProxies(ctx, proxyIDs)
	if err != nil {
		return nil, err
REDACTED
	tempUnschedMap, err := r.loadTempUnschedStates(ctx, accountIDs)
	if err != nil {
		return nil, err
REDACTED
	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
REDACTED

	outAccounts := make([]service.Account, 0, len(accounts))
	for _, acc := range accounts {
		out := accountEntityToService(acc)
		if out == nil {
			continue
	REDACTED
		if acc.ProxyID != nil {
			if proxy, ok := proxyMap[*acc.ProxyID]; ok {
				out.Proxy = proxy
		REDACTED
	REDACTED
		if groups, ok := groupsByAccount[acc.ID]; ok {
			out.Groups = groups
	REDACTED
		if groupIDs, ok := groupIDsByAccount[acc.ID]; ok {
			out.GroupIDs = groupIDs
	REDACTED
		if ags, ok := accountGroupsByAccount[acc.ID]; ok {
			out.AccountGroups = ags
	REDACTED
		if snap, ok := tempUnschedMap[acc.ID]; ok {
			out.TempUnschedulableUntil = snap.until
			out.TempUnschedulableReason = snap.reason
	REDACTED
		outAccounts = append(outAccounts, *out)
REDACTED

	return outAccounts, nil
REDACTED

func tempUnschedulablePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("temp_unschedulable_until")
		s.Where(entsql.Or(
			entsql.IsNull(col),
			entsql.LTE(col, entsql.Expr("NOW()")),
		))
REDACTED)
REDACTED

func (r *accountRepository) loadTempUnschedStates(ctx context.Context, accountIDs []int64) (map[int64]tempUnschedSnapshot, error) {
	out := make(map[int64]tempUnschedSnapshot)
	if len(accountIDs) == 0 {
		return out, nil
REDACTED

	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, temp_unschedulable_until, temp_unschedulable_reason
		FROM accounts
		WHERE id = ANY($1)
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	for rows.Next() {
		var id int64
		var until sql.NullTime
		var reason sql.NullString
		if err := rows.Scan(&id, &until, &reason); err != nil {
			return nil, err
	REDACTED
		var untilPtr *time.Time
		if until.Valid {
			tmp := until.Time
			untilPtr = &tmp
	REDACTED
		if reason.Valid {
			out[id] = tempUnschedSnapshot{until: untilPtr, reason: reason.StringREDACTED
	REDACTED else {
			out[id] = tempUnschedSnapshot{until: untilPtr, reason: ""REDACTED
	REDACTED
REDACTED

	if err := rows.Err(); err != nil {
		return nil, err
REDACTED

	return out, nil
REDACTED

func (r *accountRepository) loadProxies(ctx context.Context, proxyIDs []int64) (map[int64]*service.Proxy, error) {
	proxyMap := make(map[int64]*service.Proxy)
	if len(proxyIDs) == 0 {
		return proxyMap, nil
REDACTED

	proxies, err := r.client.Proxy.Query().Where(dbproxy.IDIn(proxyIDs...)).All(ctx)
	if err != nil {
		return nil, err
REDACTED

	for _, p := range proxies {
		proxyMap[p.ID] = proxyEntityToService(p)
REDACTED
	return proxyMap, nil
REDACTED

func (r *accountRepository) loadAccountGroups(ctx context.Context, accountIDs []int64) (map[int64][]*service.Group, map[int64][]int64, map[int64][]service.AccountGroup, error) {
	groupsByAccount := make(map[int64][]*service.Group)
	groupIDsByAccount := make(map[int64][]int64)
	accountGroupsByAccount := make(map[int64][]service.AccountGroup)

	if len(accountIDs) == 0 {
		return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
REDACTED

	entries, err := r.client.AccountGroup.Query().
		Where(dbaccountgroup.AccountIDIn(accountIDs...)).
		WithGroup().
		Order(dbaccountgroup.ByAccountID(), dbaccountgroup.ByPriority()).
		All(ctx)
	if err != nil {
		return nil, nil, nil, err
REDACTED

	for _, ag := range entries {
		groupSvc := groupEntityToService(ag.Edges.Group)
		agSvc := service.AccountGroup{
			AccountID: ag.AccountID,
			GroupID:   ag.GroupID,
			Priority:  ag.Priority,
			CreatedAt: ag.CreatedAt,
			Group:     groupSvc,
	REDACTED
		accountGroupsByAccount[ag.AccountID] = append(accountGroupsByAccount[ag.AccountID], agSvc)
		groupIDsByAccount[ag.AccountID] = append(groupIDsByAccount[ag.AccountID], ag.GroupID)
		if groupSvc != nil {
			groupsByAccount[ag.AccountID] = append(groupsByAccount[ag.AccountID], groupSvc)
	REDACTED
REDACTED

	return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
REDACTED

func accountEntityToService(m *dbent.Account) *service.Account {
	if m == nil {
		return nil
REDACTED

	return &service.Account{
		ID:                  m.ID,
		Name:                m.Name,
		Notes:               m.Notes,
		Platform:            m.Platform,
		Type:                m.Type,
		Credentials:         copyJSONMap(m.Credentials),
		Extra:               copyJSONMap(m.Extra),
		ProxyID:             m.ProxyID,
		Concurrency:         m.Concurrency,
		Priority:            m.Priority,
		Status:              m.Status,
		ErrorMessage:        derefString(m.ErrorMessage),
		LastUsedAt:          m.LastUsedAt,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
		Schedulable:         m.Schedulable,
		RateLimitedAt:       m.RateLimitedAt,
		RateLimitResetAt:    m.RateLimitResetAt,
		OverloadUntil:       m.OverloadUntil,
		SessionWindowStart:  m.SessionWindowStart,
		SessionWindowEnd:    m.SessionWindowEnd,
		SessionWindowStatus: derefString(m.SessionWindowStatus),
REDACTED
REDACTED

func normalizeJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{REDACTED
REDACTED
	return in
REDACTED

func copyJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
REDACTED
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
REDACTED
	return out
REDACTED

func joinClauses(clauses []string, sep string) string {
	if len(clauses) == 0 {
		return ""
REDACTED
	out := clauses[0]
	for i := 1; i < len(clauses); i++ {
		out += sep + clauses[i]
REDACTED
	return out
REDACTED

func itoa(v int) string {
	return strconv.Itoa(v)
REDACTED
