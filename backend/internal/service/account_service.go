package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"gorm.io/gorm"
)

var (
	ErrAccountNotFound = errors.New("account not found")
)

type AccountRepository interface {
	Create(ctx context.Context, account *model.Account) error
	GetByID(ctx context.Context, id int64) (*model.Account, error)
	// GetByCRSAccountID finds an account previously synced from CRS.
	// Returns (nil, nil) if not found.
	GetByCRSAccountID(ctx context.Context, crsAccountID string) (*model.Account, error)
	Update(ctx context.Context, account *model.Account) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]model.Account, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string) ([]model.Account, *pagination.PaginationResult, error)
	ListByGroup(ctx context.Context, groupID int64) ([]model.Account, error)
	ListActive(ctx context.Context) ([]model.Account, error)
	ListByPlatform(ctx context.Context, platform string) ([]model.Account, error)

	UpdateLastUsed(ctx context.Context, id int64) error
	SetError(ctx context.Context, id int64, errorMsg string) error
	SetSchedulable(ctx context.Context, id int64, schedulable bool) error
	BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error

	ListSchedulable(ctx context.Context) ([]model.Account, error)
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]model.Account, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]model.Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]model.Account, error)

	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	SetOverloaded(ctx context.Context, id int64, until time.Time) error
	ClearRateLimit(ctx context.Context, id int64) error
	UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error)
REDACTED

// AccountBulkUpdate describes the fields that can be updated in a bulk operation.
// Nil pointers mean "do not change".
type AccountBulkUpdate struct {
	Name        *string
	ProxyID     *int64
	Concurrency *int
	Priority    *int
	Status      *string
	Credentials map[string]any
	Extra       map[string]any
REDACTED

// CreateAccountRequest 创建账号请求
type CreateAccountRequest struct {
	Name        string         `json:"name"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	ProxyID     *int64         `json:"proxy_id"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
	GroupIDs    []int64        `json:"group_ids"`
REDACTED

// UpdateAccountRequest 更新账号请求
type UpdateAccountRequest struct {
	Name        *string         `json:"name"`
	Credentials *map[string]any `json:"credentials"`
	Extra       *map[string]any `json:"extra"`
	ProxyID     *int64          `json:"proxy_id"`
	Concurrency *int            `json:"concurrency"`
	Priority    *int            `json:"priority"`
	Status      *string         `json:"status"`
	GroupIDs    *[]int64        `json:"group_ids"`
REDACTED

// AccountService 账号管理服务
type AccountService struct {
	accountRepo AccountRepository
	groupRepo   GroupRepository
REDACTED

// NewAccountService 创建账号服务实例
func NewAccountService(accountRepo AccountRepository, groupRepo GroupRepository) *AccountService {
	return &AccountService{
		accountRepo: accountRepo,
		groupRepo:   groupRepo,
REDACTED
REDACTED

// Create 创建账号
func (s *AccountService) Create(ctx context.Context, req CreateAccountRequest) (*model.Account, error) {
	// 验证分组是否存在（如果指定了分组）
	if len(req.GroupIDs) > 0 {
		for _, groupID := range req.GroupIDs {
			_, err := s.groupRepo.GetByID(ctx, groupID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("group %d not found", groupID)
			REDACTED
				return nil, fmt.Errorf("get group: %w", err)
		REDACTED
	REDACTED
REDACTED

	// 创建账号
	account := &model.Account{
		Name:        req.Name,
		Platform:    req.Platform,
		Type:        req.Type,
		Credentials: req.Credentials,
		Extra:       req.Extra,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		Status:      model.StatusActive,
REDACTED

	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
REDACTED

	// 绑定分组
	if len(req.GroupIDs) > 0 {
		if err := s.accountRepo.BindGroups(ctx, account.ID, req.GroupIDs); err != nil {
			return nil, fmt.Errorf("bind groups: %w", err)
	REDACTED
REDACTED

	return account, nil
REDACTED

// GetByID 根据ID获取账号
func (s *AccountService) GetByID(ctx context.Context, id int64) (*model.Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
	REDACTED
		return nil, fmt.Errorf("get account: %w", err)
REDACTED
	return account, nil
REDACTED

// List 获取账号列表
func (s *AccountService) List(ctx context.Context, params pagination.PaginationParams) ([]model.Account, *pagination.PaginationResult, error) {
	accounts, pagination, err := s.accountRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list accounts: %w", err)
REDACTED
	return accounts, pagination, nil
REDACTED

// ListByPlatform 根据平台获取账号列表
func (s *AccountService) ListByPlatform(ctx context.Context, platform string) ([]model.Account, error) {
	accounts, err := s.accountRepo.ListByPlatform(ctx, platform)
	if err != nil {
		return nil, fmt.Errorf("list accounts by platform: %w", err)
REDACTED
	return accounts, nil
REDACTED

// ListByGroup 根据分组获取账号列表
func (s *AccountService) ListByGroup(ctx context.Context, groupID int64) ([]model.Account, error) {
	accounts, err := s.accountRepo.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list accounts by group: %w", err)
REDACTED
	return accounts, nil
REDACTED

// Update 更新账号
func (s *AccountService) Update(ctx context.Context, id int64, req UpdateAccountRequest) (*model.Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
	REDACTED
		return nil, fmt.Errorf("get account: %w", err)
REDACTED

	// 更新字段
	if req.Name != nil {
		account.Name = *req.Name
REDACTED

	if req.Credentials != nil {
		account.Credentials = *req.Credentials
REDACTED

	if req.Extra != nil {
		account.Extra = *req.Extra
REDACTED

	if req.ProxyID != nil {
		account.ProxyID = req.ProxyID
REDACTED

	if req.Concurrency != nil {
		account.Concurrency = *req.Concurrency
REDACTED

	if req.Priority != nil {
		account.Priority = *req.Priority
REDACTED

	if req.Status != nil {
		account.Status = *req.Status
REDACTED

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("update account: %w", err)
REDACTED

	// 更新分组绑定
	if req.GroupIDs != nil {
		// 验证分组是否存在
		for _, groupID := range *req.GroupIDs {
			_, err := s.groupRepo.GetByID(ctx, groupID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("group %d not found", groupID)
			REDACTED
				return nil, fmt.Errorf("get group: %w", err)
		REDACTED
	REDACTED

		if err := s.accountRepo.BindGroups(ctx, account.ID, *req.GroupIDs); err != nil {
			return nil, fmt.Errorf("bind groups: %w", err)
	REDACTED
REDACTED

	return account, nil
REDACTED

// Delete 删除账号
func (s *AccountService) Delete(ctx context.Context, id int64) error {
	// 检查账号是否存在
	_, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
	REDACTED
		return fmt.Errorf("get account: %w", err)
REDACTED

	if err := s.accountRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete account: %w", err)
REDACTED

	return nil
REDACTED

// UpdateStatus 更新账号状态
func (s *AccountService) UpdateStatus(ctx context.Context, id int64, status string, errorMessage string) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
	REDACTED
		return fmt.Errorf("get account: %w", err)
REDACTED

	account.Status = status
	account.ErrorMessage = errorMessage

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return fmt.Errorf("update account: %w", err)
REDACTED

	return nil
REDACTED

// UpdateLastUsed 更新最后使用时间
func (s *AccountService) UpdateLastUsed(ctx context.Context, id int64) error {
	if err := s.accountRepo.UpdateLastUsed(ctx, id); err != nil {
		return fmt.Errorf("update last used: %w", err)
REDACTED
	return nil
REDACTED

// GetCredential 获取账号凭证（安全访问）
func (s *AccountService) GetCredential(ctx context.Context, id int64, key string) (string, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrAccountNotFound
	REDACTED
		return "", fmt.Errorf("get account: %w", err)
REDACTED

	return account.GetCredential(key), nil
REDACTED

// TestCredentials 测试账号凭证是否有效（需要实现具体平台的测试逻辑）
func (s *AccountService) TestCredentials(ctx context.Context, id int64) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
	REDACTED
		return fmt.Errorf("get account: %w", err)
REDACTED

	// 根据平台执行不同的测试逻辑
	switch account.Platform {
	case model.PlatformAnthropic:
		// TODO: 测试Anthropic API凭证
		return nil
	case model.PlatformOpenAI:
		// TODO: 测试OpenAI API凭证
		return nil
	case model.PlatformGemini:
		// TODO: 测试Gemini API凭证
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", account.Platform)
REDACTED
REDACTED
