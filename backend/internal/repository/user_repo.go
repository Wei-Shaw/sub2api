package repository

import (
	"context"
	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
REDACTED

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: dbREDACTED
REDACTED

func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
REDACTED

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
REDACTED
	return &user, nil
REDACTED

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
REDACTED
	return &user, nil
REDACTED

func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
REDACTED

func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.User{REDACTED, id).Error
REDACTED

func (r *UserRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.User, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
REDACTED

// ListWithFilters lists users with optional filtering by status, role, and search query
func (r *UserRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, role, search string) ([]model.User, *pagination.PaginationResult, error) {
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
		db = db.Where("email ILIKE ?", searchPattern)
REDACTED

	if err := db.Count(&total).Error; err != nil {
		return nil, nil, err
REDACTED

	if err := db.Offset(params.Offset()).Limit(params.Limit()).Order("id DESC").Find(&users).Error; err != nil {
		return nil, nil, err
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

func (r *UserRepository) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	return r.db.WithContext(ctx).Model(&model.User{REDACTED).Where("id = ?", id).
		Update("balance", gorm.Expr("balance + ?", amount)).Error
REDACTED

// DeductBalance 扣减用户余额，仅当余额充足时执行
func (r *UserRepository) DeductBalance(ctx context.Context, id int64, amount float64) error {
	result := r.db.WithContext(ctx).Model(&model.User{REDACTED).
		Where("id = ? AND balance >= ?", id, amount).
		Update("balance", gorm.Expr("balance - ?", amount))
	if result.Error != nil {
		return result.Error
REDACTED
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound // 余额不足或用户不存在
REDACTED
	return nil
REDACTED

func (r *UserRepository) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	return r.db.WithContext(ctx).Model(&model.User{REDACTED).Where("id = ?", id).
		Update("concurrency", gorm.Expr("concurrency + ?", amount)).Error
REDACTED

func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{REDACTED).Where("email = ?", email).Count(&count).Error
	return count > 0, err
REDACTED

// RemoveGroupFromAllowedGroups 从所有用户的 allowed_groups 数组中移除指定的分组ID
// 使用 PostgreSQL 的 array_remove 函数
func (r *UserRepository) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.User{REDACTED).
		Where("? = ANY(allowed_groups)", groupID).
		Update("allowed_groups", gorm.Expr("array_remove(allowed_groups, ?)", groupID))
	return result.RowsAffected, result.Error
REDACTED
