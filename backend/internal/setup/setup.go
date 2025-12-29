package setup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/infrastructure"
	"github.com/Wei-Shaw/sub2api/internal/service"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
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

// TestDatabaseConnection tests the database connection and creates database if not exists
func TestDatabaseConnection(cfg *DatabaseConfig) error {
	// First, connect to the default 'postgres' database to check/create target database
	defaultDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=postgres sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", defaultDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
REDACTED

	defer func() {
		if db == nil {
			return
	REDACTED
		if err := db.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
	REDACTED
REDACTED()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
REDACTED

	// Check if target database exists
	var exists bool
	row := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", cfg.DBName)
	if err := row.Scan(&exists); err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
REDACTED

	// Create database if not exists
	if !exists {
		// 注意：数据库名不能参数化，依赖前置输入校验保障安全。
		// Note: Database names cannot be parameterized, but we've already validated cfg.DBName
		// in the handler using validateDBName() which only allows [a-zA-Z][a-zA-Z0-9_]*
		_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", cfg.DBName))
		if err != nil {
			return fmt.Errorf("failed to create database '%s': %w", cfg.DBName, err)
	REDACTED
		log.Printf("Database '%s' created successfully", cfg.DBName)
REDACTED

	// Now connect to the target database to verify
	if err := db.Close(); err != nil {
		log.Printf("failed to close postgres connection: %v", err)
REDACTED
	db = nil

	targetDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	targetDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to database '%s': %w", cfg.DBName, err)
REDACTED

	defer func() {
		if err := targetDB.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
	REDACTED
REDACTED()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := targetDB.PingContext(ctx2); err != nil {
		return fmt.Errorf("ping target database failed: %w", err)
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
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Printf("failed to close redis client: %v", err)
	REDACTED
REDACTED()

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
		secret, err := generateSecret(32)
		if err != nil {
			return fmt.Errorf("failed to generate jwt secret: %w", err)
	REDACTED
		cfg.JWT.Secret = secret
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

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
REDACTED

	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
	REDACTED
REDACTED()

	return infrastructure.ApplyMigrations(context.Background(), db)
REDACTED

func createAdminUser(cfg *SetupConfig) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
REDACTED

	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
	REDACTED
REDACTED()

	// 使用超时上下文避免安装流程因数据库异常而长时间阻塞。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if admin already exists
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users WHERE role = $1", service.RoleAdmin).Scan(&count); err != nil {
		return err
REDACTED
	if count > 0 {
		return nil // Admin already exists
REDACTED

	admin := &service.User{
		Email:       cfg.Admin.Email,
		Role:        service.RoleAdmin,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: 5,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
REDACTED

	if err := admin.SetPassword(cfg.Admin.Password); err != nil {
		return err
REDACTED

	_, err = db.ExecContext(
		ctx,
		`INSERT INTO users (email, password_hash, role, balance, concurrency, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		admin.Email,
		admin.PasswordHash,
		admin.Role,
		admin.Balance,
		admin.Concurrency,
		admin.Status,
		admin.CreatedAt,
		admin.UpdatedAt,
	)
	return err
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
			UserConcurrency int     `yaml:"user_concurrency"`
			UserBalance     float64 `yaml:"user_balance"`
			ApiKeyPrefix    string  `yaml:"api_key_prefix"`
			RateMultiplier  float64 `yaml:"rate_multiplier"`
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
			UserConcurrency int     `yaml:"user_concurrency"`
			UserBalance     float64 `yaml:"user_balance"`
			ApiKeyPrefix    string  `yaml:"api_key_prefix"`
			RateMultiplier  float64 `yaml:"rate_multiplier"`
	REDACTED{
			UserConcurrency: 5,
			UserBalance:     0,
			ApiKeyPrefix:    "sk-",
			RateMultiplier:  1.0,
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

func generateSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(bytes), nil
REDACTED

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
		secret, err := generateSecret(32)
		if err != nil {
			return fmt.Errorf("failed to generate jwt secret: %w", err)
	REDACTED
		cfg.JWT.Secret = secret
		log.Println("Generated JWT secret automatically")
REDACTED

	// Generate admin password if not provided
	if cfg.Admin.Password == "" {
		password, err := generateSecret(16)
		if err != nil {
			return fmt.Errorf("failed to generate admin password: %w", err)
	REDACTED
		cfg.Admin.Password = password
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
