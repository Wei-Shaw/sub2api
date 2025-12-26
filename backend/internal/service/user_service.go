package service

import (
	"context"
	"fmt"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/infrastructure/errors"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound      = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	ErrPasswordIncorrect = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
	ErrInsufficientPerms = infraerrors.Forbidden("INSUFFICIENT_PERMISSIONS", "insufficient permissions")
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetFirstAdmin(ctx context.Context) (*model.User, error)
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]model.User, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, role, search string) ([]model.User, *pagination.PaginationResult, error)

	UpdateBalance(ctx context.Context, id int64, amount float64) error
	DeductBalance(ctx context.Context, id int64, amount float64) error
	UpdateConcurrency(ctx context.Context, id int64, amount int) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error)
REDACTED

// UpdateProfileRequest 更新用户资料请求
type UpdateProfileRequest struct {
	Email       *string `json:"email"`
	Username    *string `json:"username"`
	Wechat      *string `json:"wechat"`
	Concurrency *int    `json:"concurrency"`
REDACTED

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
REDACTED

// UserService 用户服务
type UserService struct {
	userRepo UserRepository
REDACTED

// NewUserService 创建用户服务实例
func NewUserService(userRepo UserRepository) *UserService {
	return &UserService{
		userRepo: userRepo,
REDACTED
REDACTED

// GetFirstAdmin 获取首个管理员用户（用于 Admin API Key 认证）
func (s *UserService) GetFirstAdmin(ctx context.Context) (*model.User, error) {
	admin, err := s.userRepo.GetFirstAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first admin: %w", err)
REDACTED
	return admin, nil
REDACTED

// GetProfile 获取用户资料
func (s *UserService) GetProfile(ctx context.Context, userID int64) (*model.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	return user, nil
REDACTED

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*model.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED

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

	if req.Wechat != nil {
		user.Wechat = *req.Wechat
REDACTED

	if req.Concurrency != nil {
		user.Concurrency = *req.Concurrency
REDACTED

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
REDACTED

	return user, nil
REDACTED

// ChangePassword 修改密码
func (s *UserService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
REDACTED

	// 验证当前密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrPasswordIncorrect
REDACTED

	// 生成新密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
REDACTED

	user.PasswordHash = string(hashedPassword)

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
REDACTED

	return nil
REDACTED

// GetByID 根据ID获取用户（管理员功能）
func (s *UserService) GetByID(ctx context.Context, id int64) (*model.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED
	return user, nil
REDACTED

// List 获取用户列表（管理员功能）
func (s *UserService) List(ctx context.Context, params pagination.PaginationParams) ([]model.User, *pagination.PaginationResult, error) {
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

	return nil
REDACTED

// Delete 删除用户（管理员功能）
func (s *UserService) Delete(ctx context.Context, userID int64) error {
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
REDACTED
	return nil
REDACTED
