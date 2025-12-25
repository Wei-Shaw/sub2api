package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type accountRepository struct {
	db *gorm.DB
REDACTED

func NewAccountRepository(db *gorm.DB) service.AccountRepository {
	return &accountRepository{db: dbREDACTED
REDACTED

func (r *accountRepository) Create(ctx context.Context, account *model.Account) error {
	return r.db.WithContext(ctx).Create(account).Error
REDACTED

func (r *accountRepository) GetByID(ctx context.Context, id int64) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).Preload("Proxy").Preload("AccountGroups.Group").First(&account, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
REDACTED
	// 填充 GroupIDs 和 Groups 虚拟字段
	account.GroupIDs = make([]int64, 0, len(account.AccountGroups))
	account.Groups = make([]*model.Group, 0, len(account.AccountGroups))
	for _, ag := range account.AccountGroups {
		account.GroupIDs = append(account.GroupIDs, ag.GroupID)
		if ag.Group != nil {
			account.Groups = append(account.Groups, ag.Group)
	REDACTED
REDACTED
	return &account, nil
REDACTED

func (r *accountRepository) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*model.Account, error) {
	if crsAccountID == "" {
		return nil, nil
REDACTED

	var account model.Account
	err := r.db.WithContext(ctx).Where("extra->>'crs_account_id' = ?", crsAccountID).First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
	REDACTED
		return nil, err
REDACTED
	return &account, nil
REDACTED

func (r *accountRepository) Update(ctx context.Context, account *model.Account) error {
	return r.db.WithContext(ctx).Save(account).Error
REDACTED

func (r *accountRepository) Delete(ctx context.Context, id int64) error {
	// 先删除账号与分组的绑定关系
	if err := r.db.WithContext(ctx).Where("account_id = ?", id).Delete(&model.AccountGroup{REDACTED).Error; err != nil {
		return err
REDACTED
	// 再删除账号
	return r.db.WithContext(ctx).Delete(&model.Account{REDACTED, id).Error
REDACTED

func (r *accountRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.Account, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", "")
REDACTED

// ListWithFilters lists accounts with optional filtering by platform, type, status, and search query
func (r *accountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string) ([]model.Account, *pagination.PaginationResult, error) {
	var accounts []model.Account
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Account{REDACTED)

	// Apply filters
	if platform != "" {
		db = db.Where("platform = ?", platform)
REDACTED
	if accountType != "" {
		db = db.Where("type = ?", accountType)
REDACTED
	if status != "" {
		db = db.Where("status = ?", status)
REDACTED
	if search != "" {
		searchPattern := "%" + search + "%"
		db = db.Where("name ILIKE ?", searchPattern)
REDACTED

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Preload("Proxy").Preload("AccountGroups.Group").Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&accounts).Error; err != nil {
		return nil, nil, err
REDACTED

	// 填充每个 Account 的虚拟字段（GroupIDs 和 Groups）
	for i := range accounts {
		accounts[i].GroupIDs = make([]int64, 0, len(accounts[i].AccountGroups))
		accounts[i].Groups = make([]*model.Group, 0, len(accounts[i].AccountGroups))
		for _, ag := range accounts[i].AccountGroups {
			accounts[i].GroupIDs = append(accounts[i].GroupIDs, ag.GroupID)
			if ag.Group != nil {
				accounts[i].Groups = append(accounts[i].Groups, ag.Group)
		REDACTED
	REDACTED
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return accounts, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *accountRepository) ListByGroup(ctx context.Context, groupID int64) ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.account_id = accounts.id").
		Where("account_groups.group_id = ? AND accounts.status = ?", groupID, model.StatusActive).
		Preload("Proxy").
		Order("account_groups.priority ASC, accounts.priority ASC").
		Find(&accounts).Error
	return accounts, err
REDACTED

func (r *accountRepository) ListActive(ctx context.Context) ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.WithContext(ctx).
		Where("status = ?", model.StatusActive).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
REDACTED

func (r *accountRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).Update("last_used_at", now).Error
REDACTED

func (r *accountRepository) SetError(ctx context.Context, id int64, errorMsg string) error {
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).
		Updates(map[string]any{
			"status":        model.StatusError,
			"error_message": errorMsg,
	REDACTED).Error
REDACTED

func (r *accountRepository) AddToGroup(ctx context.Context, accountID, groupID int64, priority int) error {
	ag := &model.AccountGroup{
		AccountID: accountID,
		GroupID:   groupID,
		Priority:  priority,
REDACTED
	return r.db.WithContext(ctx).Create(ag).Error
REDACTED

func (r *accountRepository) RemoveFromGroup(ctx context.Context, accountID, groupID int64) error {
	return r.db.WithContext(ctx).Where("account_id = ? AND group_id = ?", accountID, groupID).
		Delete(&model.AccountGroup{REDACTED).Error
REDACTED

func (r *accountRepository) GetGroups(ctx context.Context, accountID int64) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.group_id = groups.id").
		Where("account_groups.account_id = ?", accountID).
		Find(&groups).Error
	return groups, err
REDACTED

func (r *accountRepository) ListByPlatform(ctx context.Context, platform string) ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.WithContext(ctx).
		Where("platform = ? AND status = ?", platform, model.StatusActive).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
REDACTED

func (r *accountRepository) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	// 删除现有绑定
	if err := r.db.WithContext(ctx).Where("account_id = ?", accountID).Delete(&model.AccountGroup{REDACTED).Error; err != nil {
		return err
REDACTED

	// 添加新绑定
	if len(groupIDs) > 0 {
		accountGroups := make([]model.AccountGroup, 0, len(groupIDs))
		for i, groupID := range groupIDs {
			accountGroups = append(accountGroups, model.AccountGroup{
				AccountID: accountID,
				GroupID:   groupID,
				Priority:  i + 1, // 使用索引作为优先级
		REDACTED)
	REDACTED
		return r.db.WithContext(ctx).Create(&accountGroups).Error
REDACTED

	return nil
REDACTED

// ListSchedulable 获取所有可调度的账号
func (r *accountRepository) ListSchedulable(ctx context.Context) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Where("status = ? AND schedulable = ?", model.StatusActive, true).
		Where("(overload_until IS NULL OR overload_until <= ?)", now).
		Where("(rate_limit_reset_at IS NULL OR rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
REDACTED

// ListSchedulableByGroupID 按组获取可调度的账号
func (r *accountRepository) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.account_id = accounts.id").
		Where("account_groups.group_id = ?", groupID).
		Where("accounts.status = ? AND accounts.schedulable = ?", model.StatusActive, true).
		Where("(accounts.overload_until IS NULL OR accounts.overload_until <= ?)", now).
		Where("(accounts.rate_limit_reset_at IS NULL OR accounts.rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("account_groups.priority ASC, accounts.priority ASC").
		Find(&accounts).Error
	return accounts, err
REDACTED

// ListSchedulableByPlatform 按平台获取可调度的账号
func (r *accountRepository) ListSchedulableByPlatform(ctx context.Context, platform string) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Where("platform = ?", platform).
		Where("status = ? AND schedulable = ?", model.StatusActive, true).
		Where("(overload_until IS NULL OR overload_until <= ?)", now).
		Where("(rate_limit_reset_at IS NULL OR rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("priority ASC").
		Find(&accounts).Error
	return accounts, err
REDACTED

// ListSchedulableByGroupIDAndPlatform 按组和平台获取可调度的账号
func (r *accountRepository) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]model.Account, error) {
	var accounts []model.Account
	now := time.Now()
	err := r.db.WithContext(ctx).
		Joins("JOIN account_groups ON account_groups.account_id = accounts.id").
		Where("account_groups.group_id = ?", groupID).
		Where("accounts.platform = ?", platform).
		Where("accounts.status = ? AND accounts.schedulable = ?", model.StatusActive, true).
		Where("(accounts.overload_until IS NULL OR accounts.overload_until <= ?)", now).
		Where("(accounts.rate_limit_reset_at IS NULL OR accounts.rate_limit_reset_at <= ?)", now).
		Preload("Proxy").
		Order("account_groups.priority ASC, accounts.priority ASC").
		Find(&accounts).Error
	return accounts, err
REDACTED

// SetRateLimited 标记账号为限流状态(429)
func (r *accountRepository) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).
		Updates(map[string]any{
			"rate_limited_at":     now,
			"rate_limit_reset_at": resetAt,
	REDACTED).Error
REDACTED

// SetOverloaded 标记账号为过载状态(529)
func (r *accountRepository) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).
		Update("overload_until", until).Error
REDACTED

// ClearRateLimit 清除账号的限流状态
func (r *accountRepository) ClearRateLimit(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).
		Updates(map[string]any{
			"rate_limited_at":     nil,
			"rate_limit_reset_at": nil,
			"overload_until":      nil,
	REDACTED).Error
REDACTED

// UpdateSessionWindow 更新账号的5小时时间窗口信息
func (r *accountRepository) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	updates := map[string]any{
		"session_window_status": status,
REDACTED
	if start != nil {
		updates["session_window_start"] = start
REDACTED
	if end != nil {
		updates["session_window_end"] = end
REDACTED
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).Updates(updates).Error
REDACTED

// SetSchedulable 设置账号的调度开关
func (r *accountRepository) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).
		Update("schedulable", schedulable).Error
REDACTED

// UpdateExtra updates specific fields in account's Extra JSONB field
// It merges the updates into existing Extra data without overwriting other fields
func (r *accountRepository) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
REDACTED

	// Get current account to preserve existing Extra data
	var account model.Account
	if err := r.db.WithContext(ctx).Select("extra").Where("id = ?", id).First(&account).Error; err != nil {
		return err
REDACTED

	// Initialize Extra if nil
	if account.Extra == nil {
		account.Extra = make(model.JSONB)
REDACTED

	// Merge updates into existing Extra
	for k, v := range updates {
		account.Extra[k] = v
REDACTED

	// Save updated Extra
	return r.db.WithContext(ctx).Model(&model.Account{REDACTED).Where("id = ?", id).
		Update("extra", account.Extra).Error
REDACTED

// BulkUpdate updates multiple accounts with the provided fields.
// It merges credentials/extra JSONB fields instead of overwriting them.
func (r *accountRepository) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
REDACTED

	updateMap := map[string]any{REDACTED

	if updates.Name != nil {
		updateMap["name"] = *updates.Name
REDACTED
	if updates.ProxyID != nil {
		updateMap["proxy_id"] = updates.ProxyID
REDACTED
	if updates.Concurrency != nil {
		updateMap["concurrency"] = *updates.Concurrency
REDACTED
	if updates.Priority != nil {
		updateMap["priority"] = *updates.Priority
REDACTED
	if updates.Status != nil {
		updateMap["status"] = *updates.Status
REDACTED
	if len(updates.Credentials) > 0 {
		updateMap["credentials"] = gorm.Expr("COALESCE(credentials,'{REDACTED') || ?", updates.Credentials)
REDACTED
	if len(updates.Extra) > 0 {
		updateMap["extra"] = gorm.Expr("COALESCE(extra,'{REDACTED') || ?", updates.Extra)
REDACTED

	if len(updateMap) == 0 {
		return 0, nil
REDACTED

	result := r.db.WithContext(ctx).
		Model(&model.Account{REDACTED).
		Where("id IN ?", ids).
		Clauses(clause.Returning{REDACTED).
		Updates(updateMap)

	return result.RowsAffected, result.Error
REDACTED
