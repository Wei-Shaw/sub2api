package repository

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// MaxExpiresAt is the maximum allowed expiration date for subscriptions (year 2099)
// This prevents time.Time JSON serialization errors (RFC 3339 requires year <= 9999)
var maxExpiresAt = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

// AutoMigrate runs schema migrations for all repository persistence models.
// Persistence models are defined within individual `*_repo.go` files.
// runMode: "standard" or "simple" - determines whether to create default groups
func AutoMigrate(db *gorm.DB, runMode string) error {
	err := db.AutoMigrate(
		&userModel{REDACTED,
		&apiKeyModel{REDACTED,
		&groupModel{REDACTED,
		&accountModel{REDACTED,
		&accountGroupModel{REDACTED,
		&proxyModel{REDACTED,
		&redeemCodeModel{REDACTED,
		&usageLogModel{REDACTED,
		&settingModel{REDACTED,
		&userSubscriptionModel{REDACTED,
	)
	if err != nil {
		return err
REDACTED

	// 创建默认分组(简易模式支持)
	if err := ensureDefaultGroups(db, runMode); err != nil {
		return err
REDACTED

	// 修复无效的过期时间（年份超过 2099 会导致 JSON 序列化失败）
	return fixInvalidExpiresAt(db)
REDACTED

// fixInvalidExpiresAt 修复 user_subscriptions 表中无效的过期时间
func fixInvalidExpiresAt(db *gorm.DB) error {
	result := db.Model(&userSubscriptionModel{REDACTED).
		Where("expires_at > ?", maxExpiresAt).
		Update("expires_at", maxExpiresAt)
	if result.Error != nil {
		return result.Error
REDACTED
	if result.RowsAffected > 0 {
		log.Printf("[AutoMigrate] Fixed %d subscriptions with invalid expires_at (year > 2099)", result.RowsAffected)
REDACTED
	return nil
REDACTED

// ensureDefaultGroups 确保默认分组存在(简易模式支持)
// 为每个平台创建一个默认分组,配置最大权限以确保简易模式下不受限制
// runMode: "standard" 时跳过创建, "simple" 时创建/恢复默认分组
func ensureDefaultGroups(db *gorm.DB, runMode string) error {
	// 标准版不创建默认分组
	if runMode == "standard" {
		return nil
REDACTED

	defaultGroups := []struct {
		name        string
		platform    string
		description string
REDACTED{
		{
			name:        "anthropic-default",
			platform:    "anthropic",
			description: "Default group for Anthropic accounts (Simple Mode)",
	REDACTED,
		{
			name:        "openai-default",
			platform:    "openai",
			description: "Default group for OpenAI accounts (Simple Mode)",
	REDACTED,
		{
			name:        "gemini-default",
			platform:    "gemini",
			description: "Default group for Gemini accounts (Simple Mode)",
	REDACTED,
REDACTED

	for _, dg := range defaultGroups {
		// 步骤1: 检查是否有软删除的记录
		var softDeletedCount int64
		if err := db.Unscoped().Model(&groupModel{REDACTED).
			Where("name = ? AND deleted_at IS NOT NULL", dg.name).
			Count(&softDeletedCount).Error; err != nil {
			return err
	REDACTED

		if softDeletedCount > 0 {
			// 恢复软删除的记录
			if err := db.Unscoped().Model(&groupModel{REDACTED).
				Where("name = ?", dg.name).
				Update("deleted_at", nil).Error; err != nil {
				log.Printf("[AutoMigrate] Failed to restore default group %s: %v", dg.name, err)
				return err
		REDACTED
			log.Printf("[AutoMigrate] Restored default group: %s (platform: %s)", dg.name, dg.platform)
			continue
	REDACTED

		// 步骤2: 检查是否有活跃记录
		var activeCount int64
		if err := db.Model(&groupModel{REDACTED).Where("name = ?", dg.name).Count(&activeCount).Error; err != nil {
			return err
	REDACTED

		if activeCount == 0 {
			// 创建新分组
			group := &groupModel{
				Name:             dg.name,
				Description:      dg.description,
				Platform:         dg.platform,
				RateMultiplier:   1.0,
				IsExclusive:      false,
				Status:           "active",
				SubscriptionType: "standard",
		REDACTED
			if err := db.Create(group).Error; err != nil {
				log.Printf("[AutoMigrate] Failed to create default group %s: %v", dg.name, err)
				return err
		REDACTED
			log.Printf("[AutoMigrate] Created default group: %s (platform: %s)", dg.name, dg.platform)
	REDACTED
REDACTED

	return nil
REDACTED
