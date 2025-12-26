package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

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

func (r *groupRepository) Create(ctx context.Context, group *service.Group) error {
	m := groupModelFromService(group)
	err := r.db.WithContext(ctx).Create(m).Error
	if err == nil {
		applyGroupModelToService(group, m)
REDACTED
	return translatePersistenceError(err, nil, service.ErrGroupExists)
REDACTED

func (r *groupRepository) GetByID(ctx context.Context, id int64) (*service.Group, error) {
	var m groupModel
	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrGroupNotFound, nil)
REDACTED
	group := groupModelToService(&m)
	count, _ := r.GetAccountCount(ctx, group.ID)
	group.AccountCount = count
	return group, nil
REDACTED

func (r *groupRepository) Update(ctx context.Context, group *service.Group) error {
	m := groupModelFromService(group)
	err := r.db.WithContext(ctx).Save(m).Error
	if err == nil {
		applyGroupModelToService(group, m)
REDACTED
	return err
REDACTED

func (r *groupRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&groupModel{REDACTED, id).Error
REDACTED

func (r *groupRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", nil)
REDACTED

// ListWithFilters lists groups with optional filtering by platform, status, and is_exclusive
func (r *groupRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status string, isExclusive *bool) ([]service.Group, *pagination.PaginationResult, error) {
	var groups []groupModel
	var total int64

	db := r.db.WithContext(ctx).Model(&groupModel{REDACTED)

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

	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *groupModelToService(&groups[i]))
REDACTED

	// 获取每个分组的账号数量
	for i := range outGroups {
		count, _ := r.GetAccountCount(ctx, outGroups[i].ID)
		outGroups[i].AccountCount = count
REDACTED

	return outGroups, paginationResultFromTotal(total, params), nil
REDACTED

func (r *groupRepository) ListActive(ctx context.Context) ([]service.Group, error) {
	var groups []groupModel
	err := r.db.WithContext(ctx).Where("status = ?", service.StatusActive).Order("id ASC").Find(&groups).Error
	if err != nil {
		return nil, err
REDACTED
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *groupModelToService(&groups[i]))
REDACTED
	// 获取每个分组的账号数量
	for i := range outGroups {
		count, _ := r.GetAccountCount(ctx, outGroups[i].ID)
		outGroups[i].AccountCount = count
REDACTED
	return outGroups, nil
REDACTED

func (r *groupRepository) ListActiveByPlatform(ctx context.Context, platform string) ([]service.Group, error) {
	var groups []groupModel
	err := r.db.WithContext(ctx).Where("status = ? AND platform = ?", service.StatusActive, platform).Order("id ASC").Find(&groups).Error
	if err != nil {
		return nil, err
REDACTED
	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *groupModelToService(&groups[i]))
REDACTED
	// 获取每个分组的账号数量
	for i := range outGroups {
		count, _ := r.GetAccountCount(ctx, outGroups[i].ID)
		outGroups[i].AccountCount = count
REDACTED
	return outGroups, nil
REDACTED

func (r *groupRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&groupModel{REDACTED).Where("name = ?", name).Count(&count).Error
	return count > 0, err
REDACTED

func (r *groupRepository) GetAccountCount(ctx context.Context, groupID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("account_groups").Where("group_id = ?", groupID).Count(&count).Error
	return count, err
REDACTED

// DeleteAccountGroupsByGroupID 删除分组与账号的关联关系
func (r *groupRepository) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	result := r.db.WithContext(ctx).Exec("DELETE FROM account_groups WHERE group_id = ?", groupID)
	return result.RowsAffected, result.Error
REDACTED

func (r *groupRepository) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	group, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	var affectedUserIDs []int64
	if group.IsSubscriptionType() {
		if err := r.db.WithContext(ctx).
			Table("user_subscriptions").
			Where("group_id = ?", id).
			Pluck("user_id", &affectedUserIDs).Error; err != nil {
			return nil, err
	REDACTED
REDACTED

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 删除订阅类型分组的订阅记录
		if group.IsSubscriptionType() {
			if err := tx.Exec("DELETE FROM user_subscriptions WHERE group_id = ?", id).Error; err != nil {
				return err
		REDACTED
	REDACTED

		// 2. 将 api_keys 中绑定该分组的 group_id 设为 nil
		if err := tx.Exec("UPDATE api_keys SET group_id = NULL WHERE group_id = ?", id).Error; err != nil {
			return err
	REDACTED

		// 3. 从 users.allowed_groups 数组中移除该分组 ID
		if err := tx.Exec(
			"UPDATE users SET allowed_groups = array_remove(allowed_groups, ?) WHERE ? = ANY(allowed_groups)",
			id, id,
		).Error; err != nil {
			return err
	REDACTED

		// 4. 删除 account_groups 中间表的数据
		if err := tx.Exec("DELETE FROM account_groups WHERE group_id = ?", id).Error; err != nil {
			return err
	REDACTED

		// 5. 删除分组本身（带锁，避免并发写）
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"REDACTED).Delete(&groupModel{REDACTED, id).Error; err != nil {
			return err
	REDACTED

		return nil
REDACTED)
	if err != nil {
		return nil, err
REDACTED

	return affectedUserIDs, nil
REDACTED

type groupModel struct {
	ID             int64   `gorm:"primaryKey"`
	Name           string  `gorm:"uniqueIndex;size:100;not null"`
	Description    string  `gorm:"type:text"`
	Platform       string  `gorm:"size:50;default:anthropic;not null"`
	RateMultiplier float64 `gorm:"type:decimal(10,4);default:1.0;not null"`
	IsExclusive    bool    `gorm:"default:false;not null"`
	Status         string  `gorm:"size:20;default:active;not null"`

	SubscriptionType string   `gorm:"size:20;default:standard;not null"`
	DailyLimitUSD    *float64 `gorm:"type:decimal(20,8)"`
	WeeklyLimitUSD   *float64 `gorm:"type:decimal(20,8)"`
	MonthlyLimitUSD  *float64 `gorm:"type:decimal(20,8)"`

	CreatedAt time.Time      `gorm:"not null"`
	UpdatedAt time.Time      `gorm:"not null"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
REDACTED

func (groupModel) TableName() string { return "groups" REDACTED

func groupModelToService(m *groupModel) *service.Group {
	if m == nil {
		return nil
REDACTED
	return &service.Group{
		ID:               m.ID,
		Name:             m.Name,
		Description:      m.Description,
		Platform:         m.Platform,
		RateMultiplier:   m.RateMultiplier,
		IsExclusive:      m.IsExclusive,
		Status:           m.Status,
		SubscriptionType: m.SubscriptionType,
		DailyLimitUSD:    m.DailyLimitUSD,
		WeeklyLimitUSD:   m.WeeklyLimitUSD,
		MonthlyLimitUSD:  m.MonthlyLimitUSD,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
REDACTED
REDACTED

func groupModelFromService(sg *service.Group) *groupModel {
	if sg == nil {
		return nil
REDACTED
	return &groupModel{
		ID:               sg.ID,
		Name:             sg.Name,
		Description:      sg.Description,
		Platform:         sg.Platform,
		RateMultiplier:   sg.RateMultiplier,
		IsExclusive:      sg.IsExclusive,
		Status:           sg.Status,
		SubscriptionType: sg.SubscriptionType,
		DailyLimitUSD:    sg.DailyLimitUSD,
		WeeklyLimitUSD:   sg.WeeklyLimitUSD,
		MonthlyLimitUSD:  sg.MonthlyLimitUSD,
		CreatedAt:        sg.CreatedAt,
		UpdatedAt:        sg.UpdatedAt,
REDACTED
REDACTED

func applyGroupModelToService(group *service.Group, m *groupModel) {
	if group == nil || m == nil {
		return
REDACTED
	group.ID = m.ID
	group.CreatedAt = m.CreatedAt
	group.UpdatedAt = m.UpdatedAt
REDACTED
