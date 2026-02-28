package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// SoraQuotaService 管理 Sora 用户存储配额。
// 配额优先级：用户级 → 分组级 → 系统默认值。
type SoraQuotaService struct {
	userRepo       UserRepository
	groupRepo      GroupRepository
	settingService *SettingService
REDACTED

// NewSoraQuotaService 创建配额服务实例。
func NewSoraQuotaService(
	userRepo UserRepository,
	groupRepo GroupRepository,
	settingService *SettingService,
) *SoraQuotaService {
	return &SoraQuotaService{
		userRepo:       userRepo,
		groupRepo:      groupRepo,
		settingService: settingService,
REDACTED
REDACTED

// QuotaInfo 返回给客户端的配额信息。
type QuotaInfo struct {
	QuotaBytes     int64  `json:"quota_bytes"`      // 总配额（0 表示无限制）
	UsedBytes      int64  `json:"used_bytes"`       // 已使用
	AvailableBytes int64  `json:"available_bytes"`  // 剩余可用（无限制时为 0）
	QuotaSource    string `json:"quota_source"`     // 配额来源：user / group / system / unlimited
	Source         string `json:"source,omitempty"` // 兼容旧字段
REDACTED

// ErrSoraStorageQuotaExceeded 表示配额不足。
var ErrSoraStorageQuotaExceeded = errors.New("sora storage quota exceeded")

// QuotaExceededError 包含配额不足的上下文信息。
type QuotaExceededError struct {
	QuotaBytes int64
	UsedBytes  int64
REDACTED

func (e *QuotaExceededError) Error() string {
	if e == nil {
		return "存储配额不足"
REDACTED
	return fmt.Sprintf("存储配额不足（已用 %d / 配额 %d 字节）", e.UsedBytes, e.QuotaBytes)
REDACTED

type soraQuotaAtomicUserRepository interface {
	AddSoraStorageUsageWithQuota(ctx context.Context, userID int64, deltaBytes int64, effectiveQuota int64) (int64, error)
	ReleaseSoraStorageUsageAtomic(ctx context.Context, userID int64, deltaBytes int64) (int64, error)
REDACTED

// GetQuota 获取用户的存储配额信息。
// 优先级：用户级 > 用户所属分组级 > 系统默认值。
func (s *SoraQuotaService) GetQuota(ctx context.Context, userID int64) (*QuotaInfo, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED

	info := &QuotaInfo{
		UsedBytes: user.SoraStorageUsedBytes,
REDACTED

	// 1. 用户级配额
	if user.SoraStorageQuotaBytes > 0 {
		info.QuotaBytes = user.SoraStorageQuotaBytes
		info.QuotaSource = "user"
		info.Source = info.QuotaSource
		info.AvailableBytes = calcAvailableBytes(info.QuotaBytes, info.UsedBytes)
		return info, nil
REDACTED

	// 2. 分组级配额（取用户可用分组中最大的配额）
	if len(user.AllowedGroups) > 0 {
		var maxGroupQuota int64
		for _, gid := range user.AllowedGroups {
			group, err := s.groupRepo.GetByID(ctx, gid)
			if err != nil {
				continue
		REDACTED
			if group.SoraStorageQuotaBytes > maxGroupQuota {
				maxGroupQuota = group.SoraStorageQuotaBytes
		REDACTED
	REDACTED
		if maxGroupQuota > 0 {
			info.QuotaBytes = maxGroupQuota
			info.QuotaSource = "group"
			info.Source = info.QuotaSource
			info.AvailableBytes = calcAvailableBytes(info.QuotaBytes, info.UsedBytes)
			return info, nil
	REDACTED
REDACTED

	// 3. 系统默认值
	defaultQuota := s.getSystemDefaultQuota(ctx)
	if defaultQuota > 0 {
		info.QuotaBytes = defaultQuota
		info.QuotaSource = "system"
		info.Source = info.QuotaSource
		info.AvailableBytes = calcAvailableBytes(info.QuotaBytes, info.UsedBytes)
		return info, nil
REDACTED

	// 无配额限制
	info.QuotaSource = "unlimited"
	info.Source = info.QuotaSource
	info.AvailableBytes = 0
	return info, nil
REDACTED

// CheckQuota 检查用户是否有足够的存储配额。
// 返回 nil 表示配额充足或无限制。
func (s *SoraQuotaService) CheckQuota(ctx context.Context, userID int64, additionalBytes int64) error {
	quota, err := s.GetQuota(ctx, userID)
	if err != nil {
		return err
REDACTED
	// 0 表示无限制
	if quota.QuotaBytes == 0 {
		return nil
REDACTED
	if quota.UsedBytes+additionalBytes > quota.QuotaBytes {
		return &QuotaExceededError{
			QuotaBytes: quota.QuotaBytes,
			UsedBytes:  quota.UsedBytes,
	REDACTED
REDACTED
	return nil
REDACTED

// AddUsage 原子累加用量（上传成功后调用）。
func (s *SoraQuotaService) AddUsage(ctx context.Context, userID int64, bytes int64) error {
	if bytes <= 0 {
		return nil
REDACTED

	quota, err := s.GetQuota(ctx, userID)
	if err != nil {
		return err
REDACTED

	if quota.QuotaBytes > 0 && quota.UsedBytes+bytes > quota.QuotaBytes {
		return &QuotaExceededError{
			QuotaBytes: quota.QuotaBytes,
			UsedBytes:  quota.UsedBytes,
	REDACTED
REDACTED

	if repo, ok := s.userRepo.(soraQuotaAtomicUserRepository); ok {
		newUsed, err := repo.AddSoraStorageUsageWithQuota(ctx, userID, bytes, quota.QuotaBytes)
		if err != nil {
			if errors.Is(err, ErrSoraStorageQuotaExceeded) {
				return &QuotaExceededError{
					QuotaBytes: quota.QuotaBytes,
					UsedBytes:  quota.UsedBytes,
			REDACTED
		REDACTED
			return fmt.Errorf("update user quota usage (atomic): %w", err)
	REDACTED
		logger.LegacyPrintf("service.sora_quota", "[SoraQuota] 累加用量 user=%d +%d total=%d", userID, bytes, newUsed)
		return nil
REDACTED

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user for quota update: %w", err)
REDACTED
	user.SoraStorageUsedBytes += bytes
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user quota usage: %w", err)
REDACTED
	logger.LegacyPrintf("service.sora_quota", "[SoraQuota] 累加用量 user=%d +%d total=%d", userID, bytes, user.SoraStorageUsedBytes)
	return nil
REDACTED

// ReleaseUsage 释放用量（删除文件后调用）。
func (s *SoraQuotaService) ReleaseUsage(ctx context.Context, userID int64, bytes int64) error {
	if bytes <= 0 {
		return nil
REDACTED

	if repo, ok := s.userRepo.(soraQuotaAtomicUserRepository); ok {
		newUsed, err := repo.ReleaseSoraStorageUsageAtomic(ctx, userID, bytes)
		if err != nil {
			return fmt.Errorf("update user quota release (atomic): %w", err)
	REDACTED
		logger.LegacyPrintf("service.sora_quota", "[SoraQuota] 释放用量 user=%d -%d total=%d", userID, bytes, newUsed)
		return nil
REDACTED

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user for quota release: %w", err)
REDACTED
	user.SoraStorageUsedBytes -= bytes
	if user.SoraStorageUsedBytes < 0 {
		user.SoraStorageUsedBytes = 0
REDACTED
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user quota release: %w", err)
REDACTED
	logger.LegacyPrintf("service.sora_quota", "[SoraQuota] 释放用量 user=%d -%d total=%d", userID, bytes, user.SoraStorageUsedBytes)
	return nil
REDACTED

func calcAvailableBytes(quotaBytes, usedBytes int64) int64 {
	if quotaBytes <= 0 {
		return 0
REDACTED
	if usedBytes >= quotaBytes {
		return 0
REDACTED
	return quotaBytes - usedBytes
REDACTED

func (s *SoraQuotaService) getSystemDefaultQuota(ctx context.Context) int64 {
	if s.settingService == nil {
		return 0
REDACTED
	settings, err := s.settingService.GetSoraS3Settings(ctx)
	if err != nil {
		return 0
REDACTED
	return settings.DefaultStorageQuotaBytes
REDACTED

// GetQuotaFromSettings 从系统设置获取默认配额（供外部使用）。
func (s *SoraQuotaService) GetQuotaFromSettings(ctx context.Context) int64 {
	return s.getSystemDefaultQuota(ctx)
REDACTED

// SetUserQuota 设置用户级配额（管理员操作）。
func SetUserSoraQuota(ctx context.Context, userRepo UserRepository, userID int64, quotaBytes int64) error {
	user, err := userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
REDACTED
	user.SoraStorageQuotaBytes = quotaBytes
	return userRepo.Update(ctx, user)
REDACTED

// ParseQuotaBytes 解析配额字符串为字节数。
func ParseQuotaBytes(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
REDACTED
