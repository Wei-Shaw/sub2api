package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"gorm.io/gorm"
)

var (
	ErrGroupNotFound = errors.New("group not found")
	ErrGroupExists   = errors.New("group name already exists")
)

type GroupRepository interface {
	Create(ctx context.Context, group *model.Group) error
	GetByID(ctx context.Context, id int64) (*model.Group, error)
	Update(ctx context.Context, group *model.Group) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]model.Group, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status string, isExclusive *bool) ([]model.Group, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]model.Group, error)
	ListActiveByPlatform(ctx context.Context, platform string) ([]model.Group, error)

	ExistsByName(ctx context.Context, name string) (bool, error)
	GetAccountCount(ctx context.Context, groupID int64) (int64, error)
	DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error)

	DB() *gorm.DB
REDACTED

// CreateGroupRequest 创建分组请求
type CreateGroupRequest struct {
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	RateMultiplier float64 `json:"rate_multiplier"`
	IsExclusive    bool    `json:"is_exclusive"`
REDACTED

// UpdateGroupRequest 更新分组请求
type UpdateGroupRequest struct {
	Name           *string  `json:"name"`
	Description    *string  `json:"description"`
	RateMultiplier *float64 `json:"rate_multiplier"`
	IsExclusive    *bool    `json:"is_exclusive"`
	Status         *string  `json:"status"`
REDACTED

// GroupService 分组管理服务
type GroupService struct {
	groupRepo GroupRepository
REDACTED

// NewGroupService 创建分组服务实例
func NewGroupService(groupRepo GroupRepository) *GroupService {
	return &GroupService{
		groupRepo: groupRepo,
REDACTED
REDACTED

// Create 创建分组
func (s *GroupService) Create(ctx context.Context, req CreateGroupRequest) (*model.Group, error) {
	// 检查名称是否已存在
	exists, err := s.groupRepo.ExistsByName(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("check group exists: %w", err)
REDACTED
	if exists {
		return nil, ErrGroupExists
REDACTED

	// 创建分组
	group := &model.Group{
		Name:           req.Name,
		Description:    req.Description,
		RateMultiplier: req.RateMultiplier,
		IsExclusive:    req.IsExclusive,
		Status:         model.StatusActive,
REDACTED

	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
REDACTED

	return group, nil
REDACTED

// GetByID 根据ID获取分组
func (s *GroupService) GetByID(ctx context.Context, id int64) (*model.Group, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
	REDACTED
		return nil, fmt.Errorf("get group: %w", err)
REDACTED
	return group, nil
REDACTED

// List 获取分组列表
func (s *GroupService) List(ctx context.Context, params pagination.PaginationParams) ([]model.Group, *pagination.PaginationResult, error) {
	groups, pagination, err := s.groupRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list groups: %w", err)
REDACTED
	return groups, pagination, nil
REDACTED

// ListActive 获取活跃分组列表
func (s *GroupService) ListActive(ctx context.Context) ([]model.Group, error) {
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
REDACTED
	return groups, nil
REDACTED

// Update 更新分组
func (s *GroupService) Update(ctx context.Context, id int64, req UpdateGroupRequest) (*model.Group, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
	REDACTED
		return nil, fmt.Errorf("get group: %w", err)
REDACTED

	// 更新字段
	if req.Name != nil && *req.Name != group.Name {
		// 检查新名称是否已存在
		exists, err := s.groupRepo.ExistsByName(ctx, *req.Name)
		if err != nil {
			return nil, fmt.Errorf("check group exists: %w", err)
	REDACTED
		if exists {
			return nil, ErrGroupExists
	REDACTED
		group.Name = *req.Name
REDACTED

	if req.Description != nil {
		group.Description = *req.Description
REDACTED

	if req.RateMultiplier != nil {
		group.RateMultiplier = *req.RateMultiplier
REDACTED

	if req.IsExclusive != nil {
		group.IsExclusive = *req.IsExclusive
REDACTED

	if req.Status != nil {
		group.Status = *req.Status
REDACTED

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, fmt.Errorf("update group: %w", err)
REDACTED

	return group, nil
REDACTED

// Delete 删除分组
func (s *GroupService) Delete(ctx context.Context, id int64) error {
	// 检查分组是否存在
	_, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGroupNotFound
	REDACTED
		return fmt.Errorf("get group: %w", err)
REDACTED

	if err := s.groupRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete group: %w", err)
REDACTED

	return nil
REDACTED

// GetStats 获取分组统计信息
func (s *GroupService) GetStats(ctx context.Context, id int64) (map[string]any, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
	REDACTED
		return nil, fmt.Errorf("get group: %w", err)
REDACTED

	// 获取账号数量
	accountCount, err := s.groupRepo.GetAccountCount(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get account count: %w", err)
REDACTED

	stats := map[string]any{
		"id":              group.ID,
		"name":            group.Name,
		"rate_multiplier": group.RateMultiplier,
		"is_exclusive":    group.IsExclusive,
		"status":          group.Status,
		"account_count":   accountCount,
REDACTED

	return stats, nil
REDACTED
