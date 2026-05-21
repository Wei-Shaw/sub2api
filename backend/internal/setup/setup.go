package setup

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

// Config paths
const (
	ConfigFileName             = "config.yaml"
	InstallLockFile            = ".installed"
	defaultUserConcurrency     = 5
	simpleModeAdminConcurrency = 30
)

func setupDefaultAdminConcurrency() int {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_MODE")), config.RunModeSimple) {
		return simpleModeAdminConcurrency
	}
	return defaultUserConcurrency
}

func isContainerRuntime() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Fallback check for environments where /.dockerenv is not present.
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		text := string(data)
		if strings.Contains(text, "docker") || strings.Contains(text, "containerd") || strings.Contains(text, "kubepods") {
			return true
		}
	}

	return false
}

// GetDataDir returns the data directory for storing config and lock files.
// Priority: DATA_DIR env > /app/data (Docker) > current directory (non-Docker only)
func GetDataDir() string {
	// Check DATA_DIR environment variable first
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}

	// In container runtime, always use /app/data and do not fall back to ".".
	// This avoids misleading errors like "open ./config.yaml" when volume permissions are wrong.
	dockerDataDir := "/app/data"
	if isContainerRuntime() {
		if info, err := os.Stat(dockerDataDir); err != nil || !info.IsDir() {
			_, _ = fmt.Fprintf(os.Stderr, "[setup] ERROR: %s is missing in container runtime. Please mount/create %s with correct permissions.\n", dockerDataDir, dockerDataDir)
			return dockerDataDir
		}

		testFile := dockerDataDir + "/.write_test"
		if f, err := os.Create(testFile); err == nil {
			_ = f.Close()
			_ = os.Remove(testFile)
			return dockerDataDir
		}

		_, _ = fmt.Fprintf(os.Stderr, "[setup] ERROR: %s is not writable in container runtime. Please fix volume permissions (expected app UID/GID writable).\n", dockerDataDir)
		return dockerDataDir
	}

	// Non-container fallback: prefer /app/data if it exists and is writable.
	if info, err := os.Stat(dockerDataDir); err == nil && info.IsDir() {
		testFile := dockerDataDir + "/.write_test"
		if f, err := os.Create(testFile); err == nil {
			_ = f.Close()
			_ = os.Remove(testFile)
			return dockerDataDir
		}
	}

	// Default to current directory
	return "."
}

// GetConfigFilePath returns the full path to config.yaml
func GetConfigFilePath() string {
	return GetDataDir() + "/" + ConfigFileName
}

// GetInstallLockPath returns the full path to .installed lock file
func GetInstallLockPath() string {
	return GetDataDir() + "/" + InstallLockFile
}

// SetupConfig holds the setup configuration
type SetupConfig struct {
	Database DatabaseConfig `json:"database" yaml:"database"`
	Redis    RedisConfig    `json:"redis" yaml:"redis"`
	Admin    AdminConfig    `json:"admin" yaml:"-"` // Not stored in config file
	Server   ServerConfig   `json:"server" yaml:"server"`
	JWT      JWTConfig      `json:"jwt" yaml:"jwt"`
	Timezone string         `json:"timezone" yaml:"timezone"` // e.g. "Asia/Shanghai", "UTC"
}

type DatabaseConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	DBName   string `json:"dbname" yaml:"dbname"`
	SSLMode  string `json:"sslmode" yaml:"sslmode"`
}

type RedisConfig struct {
	Host      string `json:"host" yaml:"host"`
	Port      int    `json:"port" yaml:"port"`
	Password  string `json:"password" yaml:"password"`
	DB        int    `json:"db" yaml:"db"`
	EnableTLS bool   `json:"enable_tls" yaml:"enable_tls"`
}

type AdminConfig struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ServerConfig struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
	Mode string `json:"mode" yaml:"mode"`
}

type JWTConfig struct {
	Secret     string `json:"secret" yaml:"secret"`
	ExpireHour int    `json:"expire_hour" yaml:"expire_hour"`
}

const (
	adminBootstrapReasonEmptyDatabase          = "empty_database"
	adminBootstrapReasonAdminExists            = "admin_exists"
	adminBootstrapReasonUsersExistWithoutAdmin = "users_exist_without_admin"
)

type adminBootstrapDecision struct {
	shouldCreate bool
	reason       string
}

func decideAdminBootstrap(totalUsers, adminUsers int64) adminBootstrapDecision {
	if adminUsers > 0 {
		return adminBootstrapDecision{
			shouldCreate: false,
			reason:       adminBootstrapReasonAdminExists,
		}
	}
	if totalUsers > 0 {
		return adminBootstrapDecision{
			shouldCreate: false,
			reason:       adminBootstrapReasonUsersExistWithoutAdmin,
		}
	}
	return adminBootstrapDecision{
		shouldCreate: true,
		reason:       adminBootstrapReasonEmptyDatabase,
	}
}

// NeedsSetup checks if the system needs initial setup
// Uses multiple checks to prevent attackers from forcing re-setup by deleting config
func NeedsSetup() bool {
	// Check 1: Config file must not exist
	if _, err := os.Stat(GetConfigFilePath()); !os.IsNotExist(err) {
		return false // Config exists, no setup needed
	}

	// Check 2: Installation lock file (harder to bypass)
	if _, err := os.Stat(GetInstallLockPath()); !os.IsNotExist(err) {
		return false // Lock file exists, already installed
	}

	return true
}

// TestDatabaseConnection tests the database connection and creates database if not exists
func TestDatabaseConnection(cfg *DatabaseConfig) error {
	// First, connect to the default 'postgres' database to check/create target database
	defaultDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", defaultDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	defer func() {
		if db == nil {
			return
		}
		if err := db.Close(); err != nil {
			logger.LegacyPrintf("setup", "failed to close postgres connection: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Check if target database exists
	var exists bool
	row := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", cfg.DBName)
	if err := row.Scan(&exists); err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}

	// Create database if not exists
	if !exists {
		// 注意：数据库名不能参数化，依赖前置输入校验保障安全。
		// Note: Database names cannot be parameterized, but we've already validated cfg.DBName
		// in the handler using validateDBName() which only allows [a-zA-Z][a-zA-Z0-9_]*
		_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", cfg.DBName))
		if err != nil {
			return fmt.Errorf("failed to create database '%s': %w", cfg.DBName, err)
		}
		logger.LegacyPrintf("setup", "Database '%s' created successfully", cfg.DBName)
	}

	// Now connect to the target database to verify
	if err := db.Close(); err != nil {
		logger.LegacyPrintf("setup", "failed to close postgres connection: %v", err)
	}
	db = nil

	targetDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	targetDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to database '%s': %w", cfg.DBName, err)
	}

	defer func() {
		if err := targetDB.Close(); err != nil {
			logger.LegacyPrintf("setup", "failed to close postgres connection: %v", err)
		}
	}()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := targetDB.PingContext(ctx2); err != nil {
		return fmt.Errorf("ping target database failed: %w", err)
	}

	return nil
}

// TestRedisConnection tests the Redis connection
func TestRedisConnection(cfg *RedisConfig) error {
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	if cfg.EnableTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: cfg.Host,
		}
	}

	rdb := redis.NewClient(opts)
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.LegacyPrintf("setup", "failed to close redis client: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

// Install performs the installation with the given configuration
func Install(cfg *SetupConfig) error {
	// Security check: prevent re-installation if already installed
	if !NeedsSetup() {
		return fmt.Errorf("system is already installed, re-installation is not allowed")
	}

	// Generate JWT secret if not provided
	if cfg.JWT.Secret == "" {
		secret, err := generateSecret(32)
		if err != nil {
			return fmt.Errorf("failed to generate jwt secret: %w", err)
		}
		cfg.JWT.Secret = secret
		logger.LegacyPrintf("setup", "%s", "Warning: JWT secret auto-generated. Consider setting a fixed secret for production.")
	}

	// Test connections
	if err := TestDatabaseConnection(&cfg.Database); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	if err := TestRedisConnection(&cfg.Redis); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}

	// Initialize database
	if err := initializeDatabase(cfg); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}

	// Create admin user (only when database is empty and no admin exists).
	if _, _, err := createAdminUser(cfg); err != nil {
		return fmt.Errorf("admin user creation failed: %w", err)
	}

	// Write config file
	if err := writeConfigFile(cfg); err != nil {
		return fmt.Errorf("config file creation failed: %w", err)
	}

	// Create installation lock file to prevent re-setup attacks
	if err := createInstallLock(); err != nil {
		return fmt.Errorf("failed to create install lock: %w", err)
	}

	return nil
}

// createInstallLock creates a lock file to prevent re-installation attacks
func createInstallLock() error {
	content := fmt.Sprintf("installed_at=%s\n", time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(GetInstallLockPath(), []byte(content), 0400) // Read-only for owner
}

func initializeDatabase(cfg *SetupConfig) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	defer func() {
		if err := db.Close(); err != nil {
			logger.LegacyPrintf("setup", "failed to close postgres connection: %v", err)
		}
	}()

	migrationCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return repository.ApplyMigrations(migrationCtx, db)
}

func createAdminUser(cfg *SetupConfig) (bool, string, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return false, "", err
	}

	defer func() {
		if err := db.Close(); err != nil {
			logger.LegacyPrintf("setup", "failed to close postgres connection: %v", err)
		}
	}()

	// 使用超时上下文避免安装流程因数据库异常而长时间阻塞。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var totalUsers int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users").Scan(&totalUsers); err != nil {
		return false, "", err
	}
	var adminUsers int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users WHERE role = $1", service.RoleAdmin).Scan(&adminUsers); err != nil {
		return false, "", err
	}
	decision := decideAdminBootstrap(totalUsers, adminUsers)
	if !decision.shouldCreate {
		return false, decision.reason, nil
	}

	if strings.TrimSpace(cfg.Admin.Password) == "" {
		password, genErr := generateSecret(16)
		if genErr != nil {
			return false, "", fmt.Errorf("failed to generate admin password: %w", genErr)
		}
		cfg.Admin.Password = password
		fmt.Printf("Generated admin password (one-time): %s\n", cfg.Admin.Password)
		fmt.Println("IMPORTANT: Save this password! It will not be shown again.")
	}

	admin := &service.User{
		Email:       cfg.Admin.Email,
		Role:        service.RoleAdmin,
		Status:      service.StatusActive,
		Balance:     0,
		Concurrency: setupDefaultAdminConcurrency(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := admin.SetPassword(cfg.Admin.Password); err != nil {
		return false, "", err
	}

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
	if err != nil {
		return false, "", err
	}
	return true, decision.reason, nil
}

func writeConfigFile(cfg *SetupConfig) error {
	// Ensure timezone has a default value
	tz := cfg.Timezone
	if tz == "" {
		tz = "Asia/Shanghai"
	}

	// Prepare config for YAML (exclude sensitive data and admin config)
	yamlConfig := struct {
		Server   ServerConfig   `yaml:"server"`
		Database DatabaseConfig `yaml:"database"`
		Redis    RedisConfig    `yaml:"redis"`
		JWT      struct {
			Secret     string `yaml:"secret"`
			ExpireHour int    `yaml:"expire_hour"`
		} `yaml:"jwt"`
		Default struct {
			UserConcurrency int     `yaml:"user_concurrency"`
			UserBalance     float64 `yaml:"user_balance"`
			APIKeyPrefix    string  `yaml:"api_key_prefix"`
			RateMultiplier  float64 `yaml:"rate_multiplier"`
		} `yaml:"default"`
		RateLimit struct {
			RequestsPerMinute int `yaml:"requests_per_minute"`
			BurstSize         int `yaml:"burst_size"`
		} `yaml:"rate_limit"`
		Timezone string `yaml:"timezone"`
	}{
		Server:   cfg.Server,
		Database: cfg.Database,
		Redis:    cfg.Redis,
		JWT: struct {
			Secret     string `yaml:"secret"`
			ExpireHour int    `yaml:"expire_hour"`
		}{
			Secret:     cfg.JWT.Secret,
			ExpireHour: cfg.JWT.ExpireHour,
		},
		Default: struct {
			UserConcurrency int     `yaml:"user_concurrency"`
			UserBalance     float64 `yaml:"user_balance"`
			APIKeyPrefix    string  `yaml:"api_key_prefix"`
			RateMultiplier  float64 `yaml:"rate_multiplier"`
		}{
			UserConcurrency: defaultUserConcurrency,
			UserBalance:     0,
			APIKeyPrefix:    "sk-",
			RateMultiplier:  1.0,
		},
		RateLimit: struct {
			RequestsPerMinute int `yaml:"requests_per_minute"`
			BurstSize         int `yaml:"burst_size"`
		}{
			RequestsPerMinute: 60,
			BurstSize:         10,
		},
		Timezone: tz,
	}

	data, err := yaml.Marshal(&yamlConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(GetConfigFilePath(), data, 0600)
}

func generateSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// =============================================================================
// Auto Setup for Docker Deployment
// =============================================================================

// AutoSetupEnabled checks if auto setup is enabled via environment variable
func AutoSetupEnabled() bool {
	val := os.Getenv("AUTO_SETUP")
	return val == "true" || val == "1" || val == "yes"
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// getEnvIntOrDefault gets environment variable as int or returns default value
func getEnvIntOrDefault(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultValue
}

// AutoSetupFromEnv performs automatic setup using environment variables
// This is designed for Docker deployment where all config is passed via env vars
func AutoSetupFromEnv() error {
	logger.LegacyPrintf("setup", "%s", "Auto setup enabled, configuring from environment variables...")
	logger.LegacyPrintf("setup", "Data directory: %s", GetDataDir())

	// Get timezone from TZ or TIMEZONE env var (TZ is standard for Docker)
	tz := getEnvOrDefault("TZ", "")
	if tz == "" {
		tz = getEnvOrDefault("TIMEZONE", "Asia/Shanghai")
	}

	// Build config from environment variables
	cfg := &SetupConfig{
		Database: DatabaseConfig{
			Host:     getEnvOrDefault("DATABASE_HOST", "localhost"),
			Port:     getEnvIntOrDefault("DATABASE_PORT", 5432),
			User:     getEnvOrDefault("DATABASE_USER", "postgres"),
			Password: getEnvOrDefault("DATABASE_PASSWORD", ""),
			DBName:   getEnvOrDefault("DATABASE_DBNAME", "sub2api"),
			SSLMode:  getEnvOrDefault("DATABASE_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:      getEnvOrDefault("REDIS_HOST", "localhost"),
			Port:      getEnvIntOrDefault("REDIS_PORT", 6379),
			Password:  getEnvOrDefault("REDIS_PASSWORD", ""),
			DB:        getEnvIntOrDefault("REDIS_DB", 0),
			EnableTLS: getEnvOrDefault("REDIS_ENABLE_TLS", "false") == "true",
		},
		Admin: AdminConfig{
			Email:    getEnvOrDefault("ADMIN_EMAIL", "admin@sub2api.local"),
			Password: getEnvOrDefault("ADMIN_PASSWORD", ""),
		},
		Server: ServerConfig{
			Host: getEnvOrDefault("SERVER_HOST", "0.0.0.0"),
			Port: getEnvIntOrDefault("SERVER_PORT", 8080),
			Mode: getEnvOrDefault("SERVER_MODE", "release"),
		},
		JWT: JWTConfig{
			Secret:     getEnvOrDefault("JWT_SECRET", ""),
			ExpireHour: getEnvIntOrDefault("JWT_EXPIRE_HOUR", 24),
		},
		Timezone: tz,
	}

	// Generate JWT secret if not provided
	if cfg.JWT.Secret == "" {
		secret, err := generateSecret(32)
		if err != nil {
			return fmt.Errorf("failed to generate jwt secret: %w", err)
		}
		cfg.JWT.Secret = secret
		logger.LegacyPrintf("setup", "%s", "Warning: JWT secret auto-generated. Consider setting a fixed secret for production.")
	}

	// Test database connection
	logger.LegacyPrintf("setup", "%s", "Testing database connection...")
	if err := TestDatabaseConnection(&cfg.Database); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	logger.LegacyPrintf("setup", "%s", "Database connection successful")

	// Test Redis connection
	logger.LegacyPrintf("setup", "%s", "Testing Redis connection...")
	if err := TestRedisConnection(&cfg.Redis); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}
	logger.LegacyPrintf("setup", "%s", "Redis connection successful")

	// Initialize database
	logger.LegacyPrintf("setup", "%s", "Initializing database...")
	if err := initializeDatabase(cfg); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}
	logger.LegacyPrintf("setup", "%s", "Database initialized successfully")

	// Create admin user
	logger.LegacyPrintf("setup", "%s", "Creating admin user...")
	created, reason, err := createAdminUser(cfg)
	if err != nil {
		return fmt.Errorf("admin user creation failed: %w", err)
	}
	if created {
		logger.LegacyPrintf("setup", "Admin user created: %s", cfg.Admin.Email)
	} else {
		switch reason {
		case adminBootstrapReasonAdminExists:
			logger.LegacyPrintf("setup", "%s", "Admin user already exists, skipping admin bootstrap")
		case adminBootstrapReasonUsersExistWithoutAdmin:
			logger.LegacyPrintf("setup", "%s", "Database already has user data; skipping auto admin bootstrap to avoid password overwrite")
		default:
			logger.LegacyPrintf("setup", "%s", "Admin bootstrap skipped")
		}
	}

	// Write config file
	logger.LegacyPrintf("setup", "%s", "Writing configuration file...")
	if err := writeConfigFile(cfg); err != nil {
		return fmt.Errorf("config file creation failed: %w", err)
	}
	logger.LegacyPrintf("setup", "%s", "Configuration file created")

	// Create installation lock file
	if err := createInstallLock(); err != nil {
		return fmt.Errorf("failed to create install lock: %w", err)
	}
	logger.LegacyPrintf("setup", "%s", "Installation lock created")

	logger.LegacyPrintf("setup", "%s", "Auto setup completed successfully!")
	return nil
}
