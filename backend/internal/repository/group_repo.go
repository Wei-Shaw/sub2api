package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type groupRepository struct {
	db *gorm.DB
REDACTED

func NewGroupRepository(db *gorm.DB) service.GroupRepository {
	return &groupRepository{db: dbREDACTED
REDACTED

func (r *groupRepository) Create(ctx context.Context, group *model.Group) error {
	err := r.db.WithContext(ctx).Create(group).Error
	return translatePersistenceError(err, nil, service.ErrGroupExists)
REDACTED

func (r *groupRepository) GetByID(ctx context.Context, id int64) (*model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).First(&group, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
REDACTED
	return &group, nil
REDACTED

func (r *groupRepository) Update(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Save(group).Error
REDACTED

func (r *groupRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Group{REDACTED, id).Error
REDACTED

func (r *groupRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.Group, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", nil)
REDACTED

// ListWithFilters lists groups with optional filtering by platform, status, and is_exclusive
func (r *groupRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status string, isExclusive *bool) ([]model.Group, *pagination.PaginationResult, error) {
	var groups []model.Group
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Group{REDACTED)

	// Apply filters
	if platform != "" {
		db = db.Where("platform = ?", platform)
REDACTED
	if status != "" {
		db = db.Where("status = ?", status)
REDACTED
	if isExclusive != nil {
		db = db.Where("is_exclusive = ?", *isExclusive)
REDACTED

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, nil, err
REDACTED

	// 获取每个分组的账号数量
	for i := range groups {
		count, _ := r.GetAccountCount(ctx, groups[i].ID)
		groups[i].AccountCount = count
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return groups, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *groupRepository) ListActive(ctx context.Context) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).Where("status = ?", model.StatusActive).Order("id ASC").Find(&groups).Error
	if err != nil {
		return nil, err
REDACTED
	// 获取每个分组的账号数量
	for i := range groups {
		count, _ := r.GetAccountCount(ctx, groups[i].ID)
		groups[i].AccountCount = count
REDACTED
	return groups, nil
REDACTED

func (r *groupRepository) ListActiveByPlatform(ctx context.Context, platform string) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).Where("status = ? AND platform = ?", model.StatusActive, platform).Order("id ASC").Find(&groups).Error
	if err != nil {
		return nil, err
REDACTED
	// 获取每个分组的账号数量
	for i := range groups {
		count, _ := r.GetAccountCount(ctx, groups[i].ID)
		groups[i].AccountCount = count
REDACTED
	return groups, nil
REDACTED

func (r *groupRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Group{REDACTED).Where("name = ?", name).Count(&count).Error
	return count > 0, err
REDACTED

func (r *groupRepository) GetAccountCount(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AccountGroup{REDACTED).Where("group_id = ?", groupID).Count(&count).Error
	return count, err
REDACTED

// DeleteAccountGroupsByGroupID 删除分组与账号的关联关系
func (r *groupRepository) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Where("group_id = ?", groupID).Delete(&model.AccountGroup{REDACTED)
	return result.RowsAffected, result.Error
REDACTED

func (r *groupRepository) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	group, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	var affectedUserIDs []int64
	if group.IsSubscriptionType() {
		var subscriptions []model.UserSubscription
		if err := r.db.WithContext(ctx).
			Model(&model.UserSubscription{REDACTED).
			Where("group_id = ?", id).
			Select("user_id").
			Find(&subscriptions).Error; err != nil {
			return nil, err
	REDACTED
		for _, sub := range subscriptions {
			affectedUserIDs = append(affectedUserIDs, sub.UserID)
	REDACTED
REDACTED

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 删除订阅类型分组的订阅记录
		if group.IsSubscriptionType() {
			if err := tx.Where("group_id = ?", id).Delete(&model.UserSubscription{REDACTED).Error; err != nil {
				return err
		REDACTED
	REDACTED

		// 2. 将 api_keys 中绑定该分组的 group_id 设为 nil
		if err := tx.Model(&model.ApiKey{REDACTED).Where("group_id = ?", id).Update("group_id", nil).Error; err != nil {
			return err
	REDACTED

		// 3. 从 users.allowed_groups 数组中移除该分组 ID
		if err := tx.Model(&model.User{REDACTED).
			Where("? = ANY(allowed_groups)", id).
			Update("allowed_groups", gorm.Expr("array_remove(allowed_groups, ?)", id)).Error; err != nil {
			return err
	REDACTED

		// 4. 删除 account_groups 中间表的数据
		if err := tx.Where("group_id = ?", id).Delete(&model.AccountGroup{REDACTED).Error; err != nil {
			return err
	REDACTED

		// 5. 删除分组本身（带锁，避免并发写）
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"REDACTED).Delete(&model.Group{REDACTED, id).Error; err != nil {
			return err
	REDACTED

		return nil
REDACTED)
	if err != nil {
		return nil, err
REDACTED

	return affectedUserIDs, nil
REDACTED
