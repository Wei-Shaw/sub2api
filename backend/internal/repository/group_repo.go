package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type GroupRepository struct {
	db *gorm.DB
REDACTED

func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: dbREDACTED
REDACTED

func (r *GroupRepository) Create(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Create(group).Error
REDACTED

func (r *GroupRepository) GetByID(ctx context.Context, id int64) (*model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).First(&group, id).Error
	if err != nil {
		return nil, err
REDACTED
	return &group, nil
REDACTED

func (r *GroupRepository) Update(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Save(group).Error
REDACTED

func (r *GroupRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Group{REDACTED, id).Error
REDACTED

func (r *GroupRepository) List(ctx context.Context, params pagination.PaginationParams) ([]model.Group, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", nil)
REDACTED

// ListWithFilters lists groups with optional filtering by platform, status, and is_exclusive
func (r *GroupRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status string, isExclusive *bool) ([]model.Group, *pagination.PaginationResult, error) {
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

func (r *GroupRepository) ListActive(ctx context.Context) ([]model.Group, error) {
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

func (r *GroupRepository) ListActiveByPlatform(ctx context.Context, platform string) ([]model.Group, error) {
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

func (r *GroupRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Group{REDACTED).Where("name = ?", name).Count(&count).Error
	return count > 0, err
REDACTED

func (r *GroupRepository) GetAccountCount(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AccountGroup{REDACTED).Where("group_id = ?", groupID).Count(&count).Error
	return count, err
REDACTED

// DeleteAccountGroupsByGroupID 删除分组与账号的关联关系
func (r *GroupRepository) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Where("group_id = ?", groupID).Delete(&model.AccountGroup{REDACTED)
	return result.RowsAffected, result.Error
REDACTED

// DB 返回底层数据库连接，用于事务处理
func (r *GroupRepository) DB() *gorm.DB {
	return r.db
REDACTED
