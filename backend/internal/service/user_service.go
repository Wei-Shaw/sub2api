package service

import (
	"context"
	"fmt"
	"log"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrUserNotFound      = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	ErrPasswordIncorrect = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
	ErrInsufficientPerms = infraerrors.Forbidden("INSUFFICIENT_PERMISSIONS", "insufficient permissions")
)

// UserListFilters contains all filter options for listing users
type UserListFilters struct {
	Status     string           // User status filter
	Role       string           // User role filter
	Search     string           // Search in email, username
	Attributes map[int64]string // Custom attribute filters: attributeID -> value
	// IncludeSubscriptions controls whether ListWithFilters should load active subscriptions.
	// For large datasets this can be expensive; admin list pages should enable it on demand.
	// nil means not specified (default: load subscriptions for backward compatibility).
	IncludeSubscriptions *bool
REDACTED

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetFirstAdmin(ctx context.Context) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error)

	UpdateBalance(ctx context.Context, id int64, amount float64) error
	DeductBalance(ctx context.Context, id int64, amount float64) error
	UpdateConcurrency(ctx context.Context, id int64, amount int) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error)
	// AddGroupToAllowedGroups 将指定分组增量添加到用户的 allowed_groups（幂等，冲突忽略）
	AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error

	// TOTP 双因素认证
	UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error
	EnableTotp(ctx context.Context, userID int64) error
	DisableTotp(ctx context.Context, userID int64) error
REDACTED

// UpdateProfileRequest 更新用户资料请求
type UpdateProfileRequest struct {
	Email       *string `json:"email"`
	Username    *string `json:"username"`
	Concurrency *int    `json:"concurrency"`
REDACTED

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
REDACTED

// UserService 用户服务
type UserService struct {
	userRepo             UserRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCache         BillingCache
REDACTED

// NewUserService 创建用户服务实例
func NewUserService(userRepo UserRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCache BillingCache) *UserService {
	return &UserService{
		userRepo:             userRepo,
		authCacheInvalidator: authCacheInvalidator,
		billingCache:         billingCache,
REDACTED
REDACTED

// GetFirstAdmin 获取首个管理员用户（用于 Admin API Key 认证）
func (s *UserService) GetFirstAdmin(ctx context.Context) (*User, error) {
	admin, err := s.userRepo.GetFirstAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first admin: %w", err)
REDACTED
	return admin, nil
REDACTED

// GetProfile 获取用户资料
func (s *UserService) GetProfile(ctx context.Context, userID int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	return user, nil
REDACTED

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	oldConcurrency := user.Concurrency

	// 更新字段
	if req.Email != nil {
		// 检查新邮箱是否已被使用
		exists, err := s.userRepo.ExistsByEmail(ctx, *req.Email)
		if err != nil {
			return nil, fmt.Errorf("check email exists: %w", err)
	REDACTED
		if exists && *req.Email != user.Email {
			return nil, ErrEmailExists
	REDACTED
		user.Email = *req.Email
REDACTED

	if req.Username != nil {
		user.Username = *req.Username
REDACTED

	if req.Concurrency != nil {
		user.Concurrency = *req.Concurrency
REDACTED

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
REDACTED
	if s.authCacheInvalidator != nil && user.Concurrency != oldConcurrency {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED

	return user, nil
REDACTED

// ChangePassword 修改密码
// Security: Increments TokenVersion to invalidate all existing JWT tokens
func (s *UserService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
REDACTED

	// 验证当前密码
	if !user.CheckPassword(req.CurrentPassword) {
		return ErrPasswordIncorrect
REDACTED

	if err := user.SetPassword(req.NewPassword); err != nil {
		return fmt.Errorf("set password: %w", err)
REDACTED

	// Increment TokenVersion to invalidate all existing tokens
	// This ensures that any tokens issued before the password change become invalid
	user.TokenVersion++

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
REDACTED

	return nil
REDACTED

// GetByID 根据ID获取用户（管理员功能）
func (s *UserService) GetByID(ctx context.Context, id int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	return user, nil
REDACTED

// List 获取用户列表（管理员功能）
func (s *UserService) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	users, pagination, err := s.userRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
REDACTED
	return users, pagination, nil
REDACTED

// UpdateBalance 更新用户余额（管理员功能）
func (s *UserService) UpdateBalance(ctx context.Context, userID int64, amount float64) error {
	if err := s.userRepo.UpdateBalance(ctx, userID, amount); err != nil {
		return fmt.Errorf("update balance: %w", err)
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	if s.billingCache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCache.InvalidateUserBalance(cacheCtx, userID); err != nil {
				log.Printf("invalidate user balance cache failed: user_id=%d err=%v", userID, err)
		REDACTED
	REDACTED()
REDACTED
	return nil
REDACTED

// UpdateConcurrency 更新用户并发数（管理员功能）
func (s *UserService) UpdateConcurrency(ctx context.Context, userID int64, concurrency int) error {
	if err := s.userRepo.UpdateConcurrency(ctx, userID, concurrency); err != nil {
		return fmt.Errorf("update concurrency: %w", err)
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	return nil
REDACTED

// UpdateStatus 更新用户状态（管理员功能）
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
REDACTED

	user.Status = status

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED

	return nil
REDACTED

// Delete 删除用户（管理员功能）
func (s *UserService) Delete(ctx context.Context, userID int64) error {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
REDACTED
	return nil
REDACTED
