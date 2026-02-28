package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/userallowedgroup"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userRepository struct {
	client *dbent.Client
	sql    sqlExecutor
REDACTED

func NewUserRepository(client *dbent.Client, sqlDB *sql.DB) service.UserRepository {
	return newUserRepositoryWithSQL(client, sqlDB)
REDACTED

func newUserRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *userRepository {
	return &userRepository{client: client, sql: sqlqREDACTED
REDACTED

func (r *userRepository) Create(ctx context.Context, userIn *service.User) error {
	if userIn == nil {
		return nil
REDACTED

	// 统一使用 ent 的事务：保证用户与允许分组的更新原子化，
	// 并避免基于 *sql.Tx 手动构造 ent client 导致的 ExecQuerier 断言错误。
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
REDACTED

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() REDACTED()
		txClient = tx.Client()
REDACTED else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client 并由调用方负责提交/回滚。
		txClient = r.client
REDACTED

	created, err := txClient.User.Create().
		SetEmail(userIn.Email).
		SetUsername(userIn.Username).
		SetNotes(userIn.Notes).
		SetPasswordHash(userIn.PasswordHash).
		SetRole(userIn.Role).
		SetBalance(userIn.Balance).
		SetConcurrency(userIn.Concurrency).
		SetStatus(userIn.Status).
		SetSoraStorageQuotaBytes(userIn.SoraStorageQuotaBytes).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrEmailExists)
REDACTED

	if err := r.syncUserAllowedGroupsWithClient(ctx, txClient, created.ID, userIn.AllowedGroups); err != nil {
		return err
REDACTED

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
	REDACTED
REDACTED

	applyUserEntityToService(userIn, created)
	return nil
REDACTED

func (r *userRepository) GetByID(ctx context.Context, id int64) (*service.User, error) {
	m, err := r.client.User.Query().Where(dbuser.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{idREDACTED)
	if err != nil {
		return nil, err
REDACTED
	if v, ok := groups[id]; ok {
		out.AllowedGroups = v
REDACTED
	return out, nil
REDACTED

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	m, err := r.client.User.Query().Where(dbuser.EmailEQ(email)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{m.IDREDACTED)
	if err != nil {
		return nil, err
REDACTED
	if v, ok := groups[m.ID]; ok {
		out.AllowedGroups = v
REDACTED
	return out, nil
REDACTED

func (r *userRepository) Update(ctx context.Context, userIn *service.User) error {
	if userIn == nil {
		return nil
REDACTED

	// 使用 ent 事务包裹用户更新与 allowed_groups 同步，避免跨层事务不一致。
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
REDACTED

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() REDACTED()
		txClient = tx.Client()
REDACTED else {
		// 已处于外部事务中（ErrTxStarted），复用当前 client 并由调用方负责提交/回滚。
		txClient = r.client
REDACTED

	updated, err := txClient.User.UpdateOneID(userIn.ID).
		SetEmail(userIn.Email).
		SetUsername(userIn.Username).
		SetNotes(userIn.Notes).
		SetPasswordHash(userIn.PasswordHash).
		SetRole(userIn.Role).
		SetBalance(userIn.Balance).
		SetConcurrency(userIn.Concurrency).
		SetStatus(userIn.Status).
		SetSoraStorageQuotaBytes(userIn.SoraStorageQuotaBytes).
		SetSoraStorageUsedBytes(userIn.SoraStorageUsedBytes).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, service.ErrEmailExists)
REDACTED

	if err := r.syncUserAllowedGroupsWithClient(ctx, txClient, updated.ID, userIn.AllowedGroups); err != nil {
		return err
REDACTED

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
	REDACTED
REDACTED

	userIn.UpdatedAt = updated.UpdatedAt
	return nil
REDACTED

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	affected, err := r.client.User.Delete().Where(dbuser.IDEQ(id)).Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	if affected == 0 {
		return service.ErrUserNotFound
REDACTED
	return nil
REDACTED

func (r *userRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, service.UserListFilters{REDACTED)
REDACTED

func (r *userRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	q := r.client.User.Query()

	if filters.Status != "" {
		q = q.Where(dbuser.StatusEQ(filters.Status))
REDACTED
	if filters.Role != "" {
		q = q.Where(dbuser.RoleEQ(filters.Role))
REDACTED
	if filters.Search != "" {
		q = q.Where(
			dbuser.Or(
				dbuser.EmailContainsFold(filters.Search),
				dbuser.UsernameContainsFold(filters.Search),
				dbuser.NotesContainsFold(filters.Search),
				dbuser.HasAPIKeysWith(apikey.KeyContainsFold(filters.Search)),
			),
		)
REDACTED

	// If attribute filters are specified, we need to filter by user IDs first
	var allowedUserIDs []int64
	if len(filters.Attributes) > 0 {
		var attrErr error
		allowedUserIDs, attrErr = r.filterUsersByAttributes(ctx, filters.Attributes)
		if attrErr != nil {
			return nil, nil, attrErr
	REDACTED
		if len(allowedUserIDs) == 0 {
			// No users match the attribute filters
			return []service.User{REDACTED, paginationResultFromTotal(0, params), nil
	REDACTED
		q = q.Where(dbuser.IDIn(allowedUserIDs...))
REDACTED

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	users, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(dbuser.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	outUsers := make([]service.User, 0, len(users))
	if len(users) == 0 {
		return outUsers, paginationResultFromTotal(int64(total), params), nil
REDACTED

	userIDs := make([]int64, 0, len(users))
	userMap := make(map[int64]*service.User, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
		u := userEntityToService(users[i])
		outUsers = append(outUsers, *u)
		userMap[u.ID] = &outUsers[len(outUsers)-1]
REDACTED

	// Batch load active subscriptions with groups to avoid N+1.
	subs, err := r.client.UserSubscription.Query().
		Where(
			usersubscription.UserIDIn(userIDs...),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
		).
		WithGroup().
		All(ctx)
	if err != nil {
		return nil, nil, err
REDACTED

	for i := range subs {
		if u, ok := userMap[subs[i].UserID]; ok {
			u.Subscriptions = append(u.Subscriptions, *userSubscriptionEntityToService(subs[i]))
	REDACTED
REDACTED

	allowedGroupsByUser, err := r.loadAllowedGroups(ctx, userIDs)
	if err != nil {
		return nil, nil, err
REDACTED
	for id, u := range userMap {
		if groups, ok := allowedGroupsByUser[id]; ok {
			u.AllowedGroups = groups
	REDACTED
REDACTED

	return outUsers, paginationResultFromTotal(int64(total), params), nil
REDACTED

// filterUsersByAttributes returns user IDs that match ALL the given attribute filters
func (r *userRepository) filterUsersByAttributes(ctx context.Context, attrs map[int64]string) ([]int64, error) {
	if len(attrs) == 0 {
		return nil, nil
REDACTED

	if r.sql == nil {
		return nil, fmt.Errorf("sql executor is not configured")
REDACTED

	clauses := make([]string, 0, len(attrs))
	args := make([]any, 0, len(attrs)*2+1)
	argIndex := 1
	for attrID, value := range attrs {
		clauses = append(clauses, fmt.Sprintf("(attribute_id = $%d AND value ILIKE $%d)", argIndex, argIndex+1))
		args = append(args, attrID, "%"+value+"%")
		argIndex += 2
REDACTED

	query := fmt.Sprintf(
		`SELECT user_id
		 FROM user_attribute_values
		 WHERE %s
		 GROUP BY user_id
		 HAVING COUNT(DISTINCT attribute_id) = $%d`,
		strings.Join(clauses, " OR "),
		argIndex,
	)
	args = append(args, len(attrs))

	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()

	result := make([]int64, 0)
	for rows.Next() {
		var userID int64
		if scanErr := rows.Scan(&userID); scanErr != nil {
			return nil, scanErr
	REDACTED
		result = append(result, userID)
REDACTED
	if err := rows.Err(); err != nil {
		return nil, err
REDACTED
	return result, nil
REDACTED

func (r *userRepository) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.Update().Where(dbuser.IDEQ(id)).AddBalance(amount).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	if n == 0 {
		return service.ErrUserNotFound
REDACTED
	return nil
REDACTED

// DeductBalance 扣除用户余额
// 透支策略：允许余额变为负数，确保当前请求能够完成
// 中间件会阻止余额 <= 0 的用户发起后续请求
func (r *userRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.Update().
		Where(dbuser.IDEQ(id)).
		AddBalance(-amount).
		Save(ctx)
	if err != nil {
		return err
REDACTED
	if n == 0 {
		return service.ErrUserNotFound
REDACTED
	return nil
REDACTED

func (r *userRepository) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.Update().Where(dbuser.IDEQ(id)).AddConcurrency(amount).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	if n == 0 {
		return service.ErrUserNotFound
REDACTED
	return nil
REDACTED

// AddSoraStorageUsageWithQuota 原子累加 Sora 存储用量，并在有配额时校验不超额。
func (r *userRepository) AddSoraStorageUsageWithQuota(ctx context.Context, userID int64, deltaBytes int64, effectiveQuota int64) (int64, error) {
	if deltaBytes <= 0 {
		user, err := r.GetByID(ctx, userID)
		if err != nil {
			return 0, err
	REDACTED
		return user.SoraStorageUsedBytes, nil
REDACTED
	var newUsed int64
	err := scanSingleRow(ctx, r.sql, `
		UPDATE users
		SET sora_storage_used_bytes = sora_storage_used_bytes + $2
		WHERE id = $1
		  AND ($3 = 0 OR sora_storage_used_bytes + $2 <= $3)
		RETURNING sora_storage_used_bytes
	`, []any{userID, deltaBytes, effectiveQuotaREDACTED, &newUsed)
	if err == nil {
		return newUsed, nil
REDACTED
	if errors.Is(err, sql.ErrNoRows) {
		// 区分用户不存在和配额冲突
		exists, existsErr := r.client.User.Query().Where(dbuser.IDEQ(userID)).Exist(ctx)
		if existsErr != nil {
			return 0, existsErr
	REDACTED
		if !exists {
			return 0, service.ErrUserNotFound
	REDACTED
		return 0, service.ErrSoraStorageQuotaExceeded
REDACTED
	return 0, err
REDACTED

// ReleaseSoraStorageUsageAtomic 原子释放 Sora 存储用量，并保证不低于 0。
func (r *userRepository) ReleaseSoraStorageUsageAtomic(ctx context.Context, userID int64, deltaBytes int64) (int64, error) {
	if deltaBytes <= 0 {
		user, err := r.GetByID(ctx, userID)
		if err != nil {
			return 0, err
	REDACTED
		return user.SoraStorageUsedBytes, nil
REDACTED
	var newUsed int64
	err := scanSingleRow(ctx, r.sql, `
		UPDATE users
		SET sora_storage_used_bytes = GREATEST(sora_storage_used_bytes - $2, 0)
		WHERE id = $1
		RETURNING sora_storage_used_bytes
	`, []any{userID, deltaBytesREDACTED, &newUsed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, service.ErrUserNotFound
	REDACTED
		return 0, err
REDACTED
	return newUsed, nil
REDACTED

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.client.User.Query().Where(dbuser.EmailEQ(email)).Exist(ctx)
REDACTED

func (r *userRepository) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	client := clientFromContext(ctx, r.client)
	return client.UserAllowedGroup.Create().
		SetUserID(userID).
		SetGroupID(groupID).
		OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
		DoNothing().
		Exec(ctx)
REDACTED

func (r *userRepository) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	// 仅操作 user_allowed_groups 联接表，legacy users.allowed_groups 列已弃用。
	affected, err := r.client.UserAllowedGroup.Delete().
		Where(userallowedgroup.GroupIDEQ(groupID)).
		Exec(ctx)
	if err != nil {
		return 0, err
REDACTED
	return int64(affected), nil
REDACTED

func (r *userRepository) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	m, err := r.client.User.Query().
		Where(
			dbuser.RoleEQ(service.RoleAdmin),
			dbuser.StatusEQ(service.StatusActive),
		).
		Order(dbent.Asc(dbuser.FieldID)).
		First(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED

	out := userEntityToService(m)
	groups, err := r.loadAllowedGroups(ctx, []int64{m.IDREDACTED)
	if err != nil {
		return nil, err
REDACTED
	if v, ok := groups[m.ID]; ok {
		out.AllowedGroups = v
REDACTED
	return out, nil
REDACTED

func (r *userRepository) loadAllowedGroups(ctx context.Context, userIDs []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
REDACTED

	rows, err := r.client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDIn(userIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
REDACTED

	for i := range rows {
		out[rows[i].UserID] = append(out[rows[i].UserID], rows[i].GroupID)
REDACTED

	for userID := range out {
		sort.Slice(out[userID], func(i, j int) bool { return out[userID][i] < out[userID][j] REDACTED)
REDACTED

	return out, nil
REDACTED

// syncUserAllowedGroupsWithClient 在 ent client/事务内同步用户允许分组：
// 仅操作 user_allowed_groups 联接表，legacy users.allowed_groups 列已弃用。
func (r *userRepository) syncUserAllowedGroupsWithClient(ctx context.Context, client *dbent.Client, userID int64, groupIDs []int64) error {
	if client == nil {
		return nil
REDACTED

	// Keep join table as the source of truth for reads.
	if _, err := client.UserAllowedGroup.Delete().Where(userallowedgroup.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
REDACTED

	unique := make(map[int64]struct{REDACTED, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
	REDACTED
		unique[id] = struct{REDACTED{REDACTED
REDACTED

	if len(unique) > 0 {
		creates := make([]*dbent.UserAllowedGroupCreate, 0, len(unique))
		for groupID := range unique {
			creates = append(creates, client.UserAllowedGroup.Create().SetUserID(userID).SetGroupID(groupID))
	REDACTED
		if err := client.UserAllowedGroup.
			CreateBulk(creates...).
			OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
			DoNothing().
			Exec(ctx); err != nil {
			return err
	REDACTED
REDACTED

	return nil
REDACTED

func applyUserEntityToService(dst *service.User, src *dbent.User) {
	if dst == nil || src == nil {
		return
REDACTED
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
REDACTED

// UpdateTotpSecret 更新用户的 TOTP 加密密钥
func (r *userRepository) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	client := clientFromContext(ctx, r.client)
	update := client.User.UpdateOneID(userID)
	if encryptedSecret == nil {
		update = update.ClearTotpSecretEncrypted()
REDACTED else {
		update = update.SetTotpSecretEncrypted(*encryptedSecret)
REDACTED
	_, err := update.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	return nil
REDACTED

// EnableTotp 启用用户的 TOTP 双因素认证
func (r *userRepository) EnableTotp(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).
		SetTotpEnabled(true).
		SetTotpEnabledAt(time.Now()).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	return nil
REDACTED

// DisableTotp 禁用用户的 TOTP 双因素认证
func (r *userRepository) DisableTotp(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).
		SetTotpEnabled(false).
		ClearTotpEnabledAt().
		ClearTotpSecretEncrypted().
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	return nil
REDACTED
