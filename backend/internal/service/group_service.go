package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"

	"gorm.io/gorm"
)

var (
	ErrGroupNotFound = errors.New("group not found")
	ErrGroupExists   = errors.New("group name already exists")
)

// CreateGroupRequest 创建分组请求
type CreateGroupRequest struct {
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	RateMultiplier float64 `json:"rate_multiplier"`
	IsExclusive    bool    `json:"is_exclusive"`
}

// UpdateGroupRequest 更新分组请求
type UpdateGroupRequest struct {
	Name           *string  `json:"name"`
	Description    *string  `json:"description"`
	RateMultiplier *float64 `json:"rate_multiplier"`
	IsExclusive    *bool    `json:"is_exclusive"`
	Status         *string  `json:"status"`
}

// GroupService 分组管理服务
type GroupService struct {
	groupRepo ports.GroupRepository
}

// NewGroupService 创建分组服务实例
func NewGroupService(groupRepo ports.GroupRepository) *GroupService {
	return &GroupService{
		groupRepo: groupRepo,
	}
}

// Create 创建分组
func (s *GroupService) Create(ctx context.Context, req CreateGroupRequest) (*model.Group, error) {
	// 检查名称是否已存在
	exists, err := s.groupRepo.ExistsByName(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("check group exists: %w", err)
	}
	if exists {
		return nil, ErrGroupExists
	}

	// 创建分组
	group := &model.Group{
		Name:           req.Name,
		Description:    req.Description,
		RateMultiplier: req.RateMultiplier,
		IsExclusive:    req.IsExclusive,
		Status:         model.StatusActive,
	}

	if err := s.groupRepo.Create(ctx, group); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}

	return group, nil
}

// GetByID 根据ID获取分组
func (s *GroupService) GetByID(ctx context.Context, id int64) (*model.Group, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}
	return group, nil
}

// List 获取分组列表
func (s *GroupService) List(ctx context.Context, params pagination.PaginationParams) ([]model.Group, *pagination.PaginationResult, error) {
	groups, pagination, err := s.groupRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list groups: %w", err)
	}
	return groups, pagination, nil
}

// ListActive 获取活跃分组列表
func (s *GroupService) ListActive(ctx context.Context) ([]model.Group, error) {
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	return groups, nil
}

// Update 更新分组
func (s *GroupService) Update(ctx context.Context, id int64, req UpdateGroupRequest) (*model.Group, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}

	// 更新字段
	if req.Name != nil && *req.Name != group.Name {
		// 检查新名称是否已存在
		exists, err := s.groupRepo.ExistsByName(ctx, *req.Name)
		if err != nil {
			return nil, fmt.Errorf("check group exists: %w", err)
		}
		if exists {
			return nil, ErrGroupExists
		}
		group.Name = *req.Name
	}

	if req.Description != nil {
		group.Description = *req.Description
	}

	if req.RateMultiplier != nil {
		group.RateMultiplier = *req.RateMultiplier
	}

	if req.IsExclusive != nil {
		group.IsExclusive = *req.IsExclusive
	}

	if req.Status != nil {
		group.Status = *req.Status
	}

	if err := s.groupRepo.Update(ctx, group); err != nil {
		return nil, fmt.Errorf("update group: %w", err)
	}

	return group, nil
}

// Delete 删除分组
func (s *GroupService) Delete(ctx context.Context, id int64) error {
	// 检查分组是否存在
	_, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGroupNotFound
		}
		return fmt.Errorf("get group: %w", err)
	}

	if err := s.groupRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}

	return nil
}

// GetStats 获取分组统计信息
func (s *GroupService) GetStats(ctx context.Context, id int64) (map[string]any, error) {
	group, err := s.groupRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}

	// 获取账号数量
	accountCount, err := s.groupRepo.GetAccountCount(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get account count: %w", err)
	}

	stats := map[string]any{
		"id":              group.ID,
		"name":            group.Name,
		"rate_multiplier": group.RateMultiplier,
		"is_exclusive":    group.IsExclusive,
		"status":          group.Status,
		"account_count":   accountCount,
	}

	return stats, nil
}
