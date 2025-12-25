package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
REDACTED

func NewUserRepository(db *gorm.DB) service.UserRepository {
	return &userRepository{db: dbREDACTED
REDACTED

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	return translatePersistenceError(err, nil, service.ErrEmailExists)
REDACTED

func (r *userRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	return &user, nil
REDACTED

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	return &user, nil
REDACTED

func (r *userRepository) Update(ctx context.Context, user *model.User) error {
	err := r.db.WithContext(ctx).Save(user).Error
	return translatePersistenceError(err, nil, service.ErrEmailExists)
REDACTED

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.User{REDACTED, id).Error
REDACTED

func (r *userRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.User, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

// ListWithFilters lists users with optional filtering by status, role, and search query
func (r *userRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, role, search string) ([]model.User, *pagination.PaginationResult, error) {
	var users []model.User
	var total int64

	db := r.db.WithContext(ctx).Model(&model.User{REDACTED)

	// Apply filters
	if status != "" {
		db = db.Where("status = ?", status)
REDACTED
	if role != "" {
		db = db.Where("role = ?", role)
REDACTED
	if search != "" {
		searchPattern := "%" + search + "%"
		db = db.Where(
			"email ILIKE ? OR username ILIKE ? OR wechat ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
REDACTED

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	// Query users with pagination (reuse the same db with filters applied)
	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&users).Error; err != nil {
		return nil, nil, err
REDACTED

	// Batch load subscriptions for all users (avoid N+1)
	if len(users) > 0 {
		userIDs := make([]int64, len(users))
		userMap := make(map[int64]*model.User, len(users))
		for i := range users {
			userIDs[i] = users[i].ID
			userMap[users[i].ID] = &users[i]
	REDACTED

		// Query active subscriptions with groups in one query
		var subscriptions []model.UserSubscription
		if err := r.db.WithContext(ctx).
			Preload("Group").
			Where("user_id IN ? AND status = ?", userIDs, model.SubscriptionStatusActive).
			Find(&subscriptions).Error; err != nil {
			return nil, nil, err
	REDACTED

		// Associate subscriptions with users
		for i := range subscriptions {
			if user, ok := userMap[subscriptions[i].UserID]; ok {
				user.Subscriptions = append(user.Subscriptions, subscriptions[i])
		REDACTED
	REDACTED
REDACTED

	pages := int(total) / params.Limit()
	if int(total)%params.Limit() > 0 {
		pages++
REDACTED

	return users, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.Limit(),
		Pages:    pages,
REDACTED, nil
REDACTED

func (r *userRepository) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	return r.db.WithContext(ctx).Model(&model.User{REDACTED).Where("id = ?", id).
		Update("balance", gorm.Expr("balance + ?", amount)).Error
REDACTED

// DeductBalance 扣减用户余额，仅当余额充足时执行
func (r *userRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	result := r.db.WithContext(ctx).Model(&model.User{REDACTED).
		Where("id = ? AND balance >= ?", id, amount).
		Update("balance", gorm.Expr("balance - ?", amount))
	if result.Error != nil {
		return result.Error
REDACTED
	if result.RowsAffected == 0 {
		return service.ErrInsufficientBalance
REDACTED
	return nil
REDACTED

func (r *userRepository) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	return r.db.WithContext(ctx).Model(&model.User{REDACTED).Where("id = ?", id).
		Update("concurrency", gorm.Expr("concurrency + ?", amount)).Error
REDACTED

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{REDACTED).Where("email = ?", email).Count(&count).Error
	return count > 0, err
REDACTED

// RemoveGroupFromAllowedGroups 从所有用户的 allowed_groups 数组中移除指定的分组ID
// 使用 PostgreSQL 的 array_remove 函数
func (r *userRepository) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.User{REDACTED).
		Where("? = ANY(allowed_groups)", groupID).
		Update("allowed_groups", gorm.Expr("array_remove(allowed_groups, ?)", groupID))
	return result.RowsAffected, result.Error
REDACTED

// GetFirstAdmin 获取第一个管理员用户（用于 Admin API Key 认证）
func (r *userRepository) GetFirstAdmin(ctx context.Context) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("role = ? AND status = ?", model.RoleAdmin, model.StatusActive).
		Order("id ASC").
		First(&user).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
REDACTED
	return &user, nil
REDACTED
