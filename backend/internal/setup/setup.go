package setup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gopkg.in/yaml.v3"
)

// Config paths
const (
	ConfigFile = "config.yaml"
	EnvFile    = ".env"
)

// SetupConfig holds the setup configuration
type SetupConfig struct {
	Database DatabaseConfig `json:"database" yaml:"database"`
	Redis    RedisConfig    `json:"redis" yaml:"redis"`
	Admin    AdminConfig    `json:"admin" yaml:"-"` // Not stored in config file
	Server   ServerConfig   `json:"server" yaml:"server"`
	JWT      JWTConfig      `json:"jwt" yaml:"jwt"`
	Timezone string         `json:"timezone" yaml:"timezone"` // e.g. "Asia/Shanghai", "UTC"
REDACTED

type DatabaseConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	DBName   string `json:"dbname" yaml:"dbname"`
	SSLMode  string `json:"sslmode" yaml:"sslmode"`
REDACTED

type RedisConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
REDACTED

type AdminConfig struct {
	Email    string `json:"email"`
	Password string `json:"password"`
REDACTED

type ServerConfig struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
	Mode string `json:"mode" yaml:"mode"`
REDACTED

type JWTConfig struct {
	Secret     string `json:"secret" yaml:"secret"`
	ExpireHour int    `json:"expire_hour" yaml:"expire_hour"`
REDACTED

// NeedsSetup checks if the system needs initial setup
// Uses multiple checks to prevent attackers from forcing re-setup by deleting config
func NeedsSetup() bool {
	// Check 1: Config file must not exist
	if _, err := os.Stat(ConfigFile); !os.IsNotExist(err) {
		return false // Config exists, no setup needed
REDACTED

	// Check 2: Installation lock file (harder to bypass)
	lockFile := ".installed"
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		return false // Lock file exists, already installed
REDACTED

	return true
REDACTED

// TestDatabaseConnection tests the database connection
func TestDatabaseConnection(cfg *DatabaseConfig) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{REDACTED)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
REDACTED

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get db instance: %w", err)
REDACTED
	defer sqlDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
REDACTED

	return nil
REDACTED

// TestRedisConnection tests the Redis connection
func TestRedisConnection(cfg *RedisConfig) error {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
REDACTED)
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping failed: %w", err)
REDACTED

	return nil
REDACTED

// Install performs the installation with the given configuration
func Install(cfg *SetupConfig) error {
	// Security check: prevent re-installation if already installed
	if !NeedsSetup() {
		return fmt.Errorf("system is already installed, re-installation is not allowed")
REDACTED

	// Generate JWT secret if not provided
	if cfg.JWT.Secret == "" {
		cfg.JWT.Secret = generateSecret(32)
REDACTED

	// Test connections
	if err := TestDatabaseConnection(&cfg.Database); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
REDACTED

	if err := TestRedisConnection(&cfg.Redis); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
REDACTED

	// Initialize database
	if err := initializeDatabase(cfg); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
REDACTED

	// Create admin user
	if err := createAdminUser(cfg); err != nil {
		return fmt.Errorf("admin user creation failed: %w", err)
REDACTED

	// Write config file
	if err := writeConfigFile(cfg); err != nil {
		return fmt.Errorf("config file creation failed: %w", err)
REDACTED

	// Create installation lock file to prevent re-setup attacks
	if err := createInstallLock(); err != nil {
		return fmt.Errorf("failed to create install lock: %w", err)
REDACTED

	return nil
REDACTED

// createInstallLock creates a lock file to prevent re-installation attacks
func createInstallLock() error {
	lockFile := ".installed"
	content := fmt.Sprintf("installed_at=%s\n", time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(lockFile, []byte(content), 0400) // Read-only for owner
REDACTED

func initializeDatabase(cfg *SetupConfig) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{REDACTED)
	if err != nil {
		return err
REDACTED

	sqlDB, err := db.DB()
	if err != nil {
		return err
REDACTED
	defer sqlDB.Close()

	// Run auto-migration for all models
	return db.AutoMigrate(
		&User{REDACTED,
		&Group{REDACTED,
		&APIKey{REDACTED,
		&Account{REDACTED,
		&Proxy{REDACTED,
		&RedeemCode{REDACTED,
		&UsageLog{REDACTED,
		&UserSubscription{REDACTED,
		&Setting{REDACTED,
	)
REDACTED

func createAdminUser(cfg *SetupConfig) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{REDACTED)
	if err != nil {
		return err
REDACTED

	sqlDB, err := db.DB()
	if err != nil {
		return err
REDACTED
	defer sqlDB.Close()

	// Check if admin already exists
	var count int64
	db.Model(&User{REDACTED).Where("role = ?", "admin").Count(&count)
	if count > 0 {
		return nil // Admin already exists
REDACTED

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
REDACTED

	// Create admin user
	admin := &User{
		Email:        cfg.Admin.Email,
		PasswordHash: string(hashedPassword),
		Role:         "admin",
		Status:       "active",
		Balance:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
REDACTED

	return db.Create(admin).Error
REDACTED

func writeConfigFile(cfg *SetupConfig) error {
	// Ensure timezone has a default value
	tz := cfg.Timezone
	if tz == "" {
		tz = "Asia/Shanghai"
REDACTED

	// Prepare config for YAML (exclude sensitive data and admin config)
	yamlConfig := struct {
		Server   ServerConfig   `yaml:"server"`
		Database DatabaseConfig `yaml:"database"`
		Redis    RedisConfig    `yaml:"redis"`
		JWT      struct {
			Secret     string `yaml:"secret"`
			ExpireHour int    `yaml:"expire_hour"`
	REDACTED `yaml:"jwt"`
		Default struct {
			GroupID uint `yaml:"group_id"`
	REDACTED `yaml:"default"`
		RateLimit struct {
			RequestsPerMinute int `yaml:"requests_per_minute"`
			BurstSize         int `yaml:"burst_size"`
	REDACTED `yaml:"rate_limit"`
		Timezone string `yaml:"timezone"`
REDACTED{
		Server:   cfg.Server,
		Database: cfg.Database,
		Redis:    cfg.Redis,
		JWT: struct {
			Secret     string `yaml:"secret"`
			ExpireHour int    `yaml:"expire_hour"`
	REDACTED{
			Secret:     cfg.JWT.Secret,
			ExpireHour: cfg.JWT.ExpireHour,
	REDACTED,
		Default: struct {
			GroupID uint `yaml:"group_id"`
	REDACTED{
			GroupID: 1,
	REDACTED,
		RateLimit: struct {
			RequestsPerMinute int `yaml:"requests_per_minute"`
			BurstSize         int `yaml:"burst_size"`
	REDACTED{
			RequestsPerMinute: 60,
			BurstSize:         10,
	REDACTED,
		Timezone: tz,
REDACTED

	data, err := yaml.Marshal(&yamlConfig)
	if err != nil {
		return err
REDACTED

	return os.WriteFile(ConfigFile, data, 0600)
REDACTED

func generateSecret(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
REDACTED

// Minimal model definitions for migration (to avoid circular import)
type User struct {
	ID           uint      `gorm:"primaryKey"`
	Email        string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	Role         string    `gorm:"default:user"`
	Status       string    `gorm:"default:active"`
	Balance      float64   `gorm:"default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
REDACTED

type Group struct {
	ID             uint    `gorm:"primaryKey"`
	Name           string  `gorm:"uniqueIndex;not null"`
	Description    string  `gorm:"type:text"`
	RateMultiplier float64 `gorm:"default:1.0"`
	IsExclusive    bool    `gorm:"default:false"`
	Priority       int     `gorm:"default:0"`
	Status         string  `gorm:"default:active"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
REDACTED

type APIKey struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index;not null"`
	Key       string `gorm:"uniqueIndex;not null"`
	Name      string
	GroupID   *uint
	Status    string `gorm:"default:active"`
	CreatedAt time.Time
	UpdatedAt time.Time
REDACTED

type Account struct {
	ID          uint   `gorm:"primaryKey"`
	Platform    string `gorm:"not null"`
	Type        string `gorm:"not null"`
	Credentials string `gorm:"type:text"`
	Status      string `gorm:"default:active"`
	Priority    int    `gorm:"default:0"`
	ProxyID     *uint
	CreatedAt   time.Time
	UpdatedAt   time.Time
REDACTED

type Proxy struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"not null"`
	Protocol  string `gorm:"not null"`
	Host      string `gorm:"not null"`
	Port      int    `gorm:"not null"`
	Username  string
	Password  string
	Status    string `gorm:"default:active"`
	CreatedAt time.Time
	UpdatedAt time.Time
REDACTED

type RedeemCode struct {
	ID        uint    `gorm:"primaryKey"`
	Code      string  `gorm:"uniqueIndex;not null"`
	Value     float64 `gorm:"not null"`
	Status    string  `gorm:"default:unused"`
	UsedBy    *uint
	UsedAt    *time.Time
	ExpiresAt *time.Time
	CreatedAt time.Time
REDACTED

type UsageLog struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       uint   `gorm:"index"`
	APIKeyID     uint   `gorm:"index"`
	AccountID    *uint  `gorm:"index"`
	Model        string `gorm:"index"`
	InputTokens  int
	OutputTokens int
	Cost         float64
	CreatedAt    time.Time
REDACTED

type UserSubscription struct {
	ID        uint `gorm:"primaryKey"`
	UserID    uint `gorm:"index;not null"`
	GroupID   uint `gorm:"index;not null"`
	Quota     int64
	Used      int64 `gorm:"default:0"`
	Status    string
	ExpiresAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
REDACTED

type Setting struct {
	ID        uint   `gorm:"primaryKey"`
	Key       string `gorm:"uniqueIndex;not null"`
	Value     string `gorm:"type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
REDACTED

func (User) TableName() string             { return "users" REDACTED
func (Group) TableName() string            { return "groups" REDACTED
func (APIKey) TableName() string           { return "api_keys" REDACTED
func (Account) TableName() string          { return "accounts" REDACTED
func (Proxy) TableName() string            { return "proxies" REDACTED
func (RedeemCode) TableName() string       { return "redeem_codes" REDACTED
func (UsageLog) TableName() string         { return "usage_logs" REDACTED
func (UserSubscription) TableName() string { return "user_subscriptions" REDACTED
func (Setting) TableName() string          { return "settings" REDACTED

// =============================================================================
// Auto Setup for Docker Deployment
// =============================================================================

// AutoSetupEnabled checks if auto setup is enabled via environment variable
func AutoSetupEnabled() bool {
	val := os.Getenv("AUTO_SETUP")
	return val == "true" || val == "1" || val == "yes"
REDACTED

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
REDACTED
	return defaultValue
REDACTED

// getEnvIntOrDefault gets environment variable as int or returns default value
func getEnvIntOrDefault(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
	REDACTED
REDACTED
	return defaultValue
REDACTED

// AutoSetupFromEnv performs automatic setup using environment variables
// This is designed for Docker deployment where all config is passed via env vars
func AutoSetupFromEnv() error {
	log.Println("Auto setup enabled, configuring from environment variables...")

	// Get timezone from TZ or TIMEZONE env var (TZ is standard for Docker)
	tz := getEnvOrDefault("TZ", "")
	if tz == "" {
		tz = getEnvOrDefault("TIMEZONE", "Asia/Shanghai")
REDACTED

	// Build config from environment variables
	cfg := &SetupConfig{
		Database: DatabaseConfig{
			Host:     getEnvOrDefault("DATABASE_HOST", "localhost"),
			Port:     getEnvIntOrDefault("DATABASE_PORT", 5432),
			User:     getEnvOrDefault("DATABASE_USER", "postgres"),
			Password: getEnvOrDefault("DATABASE_PASSWORD", ""),
			DBName:   getEnvOrDefault("DATABASE_DBNAME", "sub2api"),
			SSLMode:  getEnvOrDefault("DATABASE_SSLMODE", "disable"),
	REDACTED,
		Redis: RedisConfig{
			Host:     getEnvOrDefault("REDIS_HOST", "localhost"),
			Port:     getEnvIntOrDefault("REDIS_PORT", 6379),
			Password: getEnvOrDefault("REDIS_PASSWORD", ""),
			DB:       getEnvIntOrDefault("REDIS_DB", 0),
	REDACTED,
		Admin: AdminConfig{
			Email:    getEnvOrDefault("ADMIN_EMAIL", "admin@sub2api.local"),
			Password: getEnvOrDefault("ADMIN_PASSWORD", ""),
	REDACTED,
		Server: ServerConfig{
			Host: getEnvOrDefault("SERVER_HOST", "0.0.0.0"),
			Port: getEnvIntOrDefault("SERVER_PORT", 8080),
			Mode: getEnvOrDefault("SERVER_MODE", "release"),
	REDACTED,
		JWT: JWTConfig{
			Secret:     getEnvOrDefault("JWT_SECRET", ""),
			ExpireHour: getEnvIntOrDefault("JWT_EXPIRE_HOUR", 24),
	REDACTED,
		Timezone: tz,
REDACTED

	// Generate JWT secret if not provided
	if cfg.JWT.Secret == "" {
		cfg.JWT.Secret = generateSecret(32)
		log.Println("Generated JWT secret automatically")
REDACTED

	// Generate admin password if not provided
	if cfg.Admin.Password == "" {
		cfg.Admin.Password = generateSecret(16)
		log.Printf("Generated admin password: %s", cfg.Admin.Password)
		log.Println("IMPORTANT: Save this password! It will not be shown again.")
REDACTED

	// Test database connection
	log.Println("Testing database connection...")
	if err := TestDatabaseConnection(&cfg.Database); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
REDACTED
	log.Println("Database connection successful")

	// Test Redis connection
	log.Println("Testing Redis connection...")
	if err := TestRedisConnection(&cfg.Redis); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
REDACTED
	log.Println("Redis connection successful")

	// Initialize database
	log.Println("Initializing database...")
	if err := initializeDatabase(cfg); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
REDACTED
	log.Println("Database initialized successfully")

	// Create admin user
	log.Println("Creating admin user...")
	if err := createAdminUser(cfg); err != nil {
		return fmt.Errorf("admin user creation failed: %w", err)
REDACTED
	log.Printf("Admin user created: %s", cfg.Admin.Email)

	// Write config file
	log.Println("Writing configuration file...")
	if err := writeConfigFile(cfg); err != nil {
		return fmt.Errorf("config file creation failed: %w", err)
REDACTED
	log.Println("Configuration file created")

	// Create installation lock file
	if err := createInstallLock(); err != nil {
		return fmt.Errorf("failed to create install lock: %w", err)
REDACTED
	log.Println("Installation lock created")

	log.Println("Auto setup completed successfully!")
	return nil
REDACTED
