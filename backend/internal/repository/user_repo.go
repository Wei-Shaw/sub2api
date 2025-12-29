package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/userallowedgroup"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
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
		SetWechat(userIn.Wechat).
		SetNotes(userIn.Notes).
		SetPasswordHash(userIn.PasswordHash).
		SetRole(userIn.Role).
		SetBalance(userIn.Balance).
		SetConcurrency(userIn.Concurrency).
		SetStatus(userIn.Status).
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
	if err == nil {
		if v, ok := groups[id]; ok {
			out.AllowedGroups = v
	REDACTED
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
	if err == nil {
		if v, ok := groups[m.ID]; ok {
			out.AllowedGroups = v
	REDACTED
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
		SetWechat(userIn.Wechat).
		SetNotes(userIn.Notes).
		SetPasswordHash(userIn.PasswordHash).
		SetRole(userIn.Role).
		SetBalance(userIn.Balance).
		SetConcurrency(userIn.Concurrency).
		SetStatus(userIn.Status).
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
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

func (r *userRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, role, search string) ([]service.User, *pagination.PaginationResult, error) {
	q := r.client.User.Query()

	if status != "" {
		q = q.Where(dbuser.StatusEQ(status))
REDACTED
	if role != "" {
		q = q.Where(dbuser.RoleEQ(role))
REDACTED
	if search != "" {
		q = q.Where(
			dbuser.Or(
				dbuser.EmailContainsFold(search),
				dbuser.UsernameContainsFold(search),
				dbuser.WechatContainsFold(search),
			),
		)
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
	if err == nil {
		for id, u := range userMap {
			if groups, ok := allowedGroupsByUser[id]; ok {
				u.AllowedGroups = groups
		REDACTED
	REDACTED
REDACTED

	return outUsers, paginationResultFromTotal(int64(total), params), nil
REDACTED

func (r *userRepository) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	_, err := r.client.User.Update().Where(dbuser.IDEQ(id)).AddBalance(amount).Save(ctx)
	return err
REDACTED

func (r *userRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	n, err := r.client.User.Update().
		Where(dbuser.IDEQ(id), dbuser.BalanceGTE(amount)).
		AddBalance(-amount).
		Save(ctx)
	if err != nil {
		return err
REDACTED
	if n == 0 {
		return service.ErrInsufficientBalance
REDACTED
	return nil
REDACTED

func (r *userRepository) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	_, err := r.client.User.Update().Where(dbuser.IDEQ(id)).AddConcurrency(amount).Save(ctx)
	return err
REDACTED

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return r.client.User.Query().Where(dbuser.EmailEQ(email)).Exist(ctx)
REDACTED

func (r *userRepository) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	exec := r.sql
	if exec == nil {
		// 未注入 sqlExecutor 时，退回到 ent client 的 ExecContext（支持事务）。
		exec = r.client
REDACTED

	joinAffected, err := r.client.UserAllowedGroup.Delete().
		Where(userallowedgroup.GroupIDEQ(groupID)).
		Exec(ctx)
	if err != nil {
		return 0, err
REDACTED

	arrayRes, err := exec.ExecContext(
		ctx,
		"UPDATE users SET allowed_groups = array_remove(allowed_groups, $1), updated_at = NOW() WHERE $1 = ANY(allowed_groups)",
		groupID,
	)
	if err != nil {
		return 0, err
REDACTED
	arrayAffected, _ := arrayRes.RowsAffected()

	if int64(joinAffected) > arrayAffected {
		return int64(joinAffected), nil
REDACTED
	return arrayAffected, nil
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
	if err == nil {
		if v, ok := groups[m.ID]; ok {
			out.AllowedGroups = v
	REDACTED
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
// 1) 以 user_allowed_groups 为读写源，确保新旧逻辑一致；
// 2) 额外更新 users.allowed_groups（历史字段）以保持兼容。
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

	legacyGroups := make([]int64, 0, len(unique))
	if len(unique) > 0 {
		creates := make([]*dbent.UserAllowedGroupCreate, 0, len(unique))
		for groupID := range unique {
			creates = append(creates, client.UserAllowedGroup.Create().SetUserID(userID).SetGroupID(groupID))
			legacyGroups = append(legacyGroups, groupID)
	REDACTED
		if err := client.UserAllowedGroup.
			CreateBulk(creates...).
			OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
			DoNothing().
			Exec(ctx); err != nil {
			return err
	REDACTED
REDACTED

	// Phase 1 兼容：保持 users.allowed_groups（数组字段）同步，避免旧查询路径读取到过期数据。
	var legacy any
	if len(legacyGroups) > 0 {
		sort.Slice(legacyGroups, func(i, j int) bool { return legacyGroups[i] < legacyGroups[j] REDACTED)
		legacy = pq.Array(legacyGroups)
REDACTED
	if _, err := client.ExecContext(ctx, "UPDATE users SET allowed_groups = $1::bigint[] WHERE id = $2", legacy, userID); err != nil {
		return err
REDACTED

	return nil
REDACTED

func (r *userRepository) syncUserAllowedGroups(ctx context.Context, client *dbent.Client, exec sqlExecutor, userID int64, groupIDs []int64) error {
	if client == nil || exec == nil {
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

	legacyGroups := make([]int64, 0, len(unique))
	if len(unique) > 0 {
		creates := make([]*dbent.UserAllowedGroupCreate, 0, len(unique))
		for groupID := range unique {
			creates = append(creates, client.UserAllowedGroup.Create().SetUserID(userID).SetGroupID(groupID))
			legacyGroups = append(legacyGroups, groupID)
	REDACTED
		if err := client.UserAllowedGroup.
			CreateBulk(creates...).
			OnConflictColumns(userallowedgroup.FieldUserID, userallowedgroup.FieldGroupID).
			DoNothing().
			Exec(ctx); err != nil {
			return err
	REDACTED
REDACTED

	// Phase 1 compatibility: keep legacy users.allowed_groups array updated for existing raw SQL paths.
	var legacy any
	if len(legacyGroups) > 0 {
		sort.Slice(legacyGroups, func(i, j int) bool { return legacyGroups[i] < legacyGroups[j] REDACTED)
		legacy = pq.Array(legacyGroups)
REDACTED
	if _, err := exec.ExecContext(ctx, "UPDATE users SET allowed_groups = $1::bigint[] WHERE id = $2", legacy, userID); err != nil {
		return err
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
