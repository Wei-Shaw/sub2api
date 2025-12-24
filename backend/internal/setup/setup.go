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

	"sub2api/internal/model"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
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

// NeedsSetup checks if the system needs initial setup
// Uses multiple checks to prevent attackers from forcing re-setup by deleting config
func NeedsSetup() bool {
	// Check 1: Config file must not exist
	if _, err := os.Stat(ConfigFile); !os.IsNotExist(err) {
		return false // Config exists, no setup needed
	}

	// Check 2: Installation lock file (harder to bypass)
	lockFile := ".installed"
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		return false // Lock file exists, already installed
	}

	return true
}

// TestDatabaseConnection tests the database connection and creates database if not exists
func TestDatabaseConnection(cfg *DatabaseConfig) error {
	// First, connect to the default 'postgres' database to check/create target database
	defaultDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=postgres sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(defaultDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get db instance: %w", err)
	}
	defer func() {
		if sqlDB == nil {
			return
		}
		if err := sqlDB.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Check if target database exists
	var exists bool
	row := sqlDB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", cfg.DBName)
	if err := row.Scan(&exists); err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}

	// Create database if not exists
	if !exists {
		// Note: Database names cannot be parameterized, but we've already validated cfg.DBName
		// in the handler using validateDBName() which only allows [a-zA-Z][a-zA-Z0-9_]*
		_, err := sqlDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", cfg.DBName))
		if err != nil {
			return fmt.Errorf("failed to create database '%s': %w", cfg.DBName, err)
		}
		log.Printf("Database '%s' created successfully", cfg.DBName)
	}

	// Now connect to the target database to verify
	if err := sqlDB.Close(); err != nil {
		log.Printf("failed to close postgres connection: %v", err)
	}
	sqlDB = nil

	targetDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	targetDB, err := gorm.Open(postgres.Open(targetDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database '%s': %w", cfg.DBName, err)
	}

	targetSqlDB, err := targetDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get target db instance: %w", err)
	}
	defer func() {
		if err := targetSqlDB.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
		}
	}()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := targetSqlDB.PingContext(ctx2); err != nil {
		return fmt.Errorf("ping target database failed: %w", err)
	}

	return nil
}

// TestRedisConnection tests the Redis connection
func TestRedisConnection(cfg *RedisConfig) error {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Printf("failed to close redis client: %v", err)
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

	// Create admin user
	if err := createAdminUser(cfg); err != nil {
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
	lockFile := ".installed"
	content := fmt.Sprintf("installed_at=%s\n", time.Now().UTC().Format(time.RFC3339))
	return os.WriteFile(lockFile, []byte(content), 0400) // Read-only for owner
}

func initializeDatabase(cfg *SetupConfig) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
		}
	}()

	// 使用 model 包的 AutoMigrate，确保模型定义统一
	return model.AutoMigrate(db)
}

func createAdminUser(cfg *SetupConfig) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("failed to close postgres connection: %v", err)
		}
	}()

	// Check if admin already exists
	var count int64
	db.Model(&model.User{}).Where("role = ?", "admin").Count(&count)
	if count > 0 {
		return nil // Admin already exists
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.Admin.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Create admin user
	admin := &model.User{
		Email:        cfg.Admin.Email,
		PasswordHash: string(hashedPassword),
		Role:         model.RoleAdmin,
		Status:       model.StatusActive,
		Balance:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	return db.Create(admin).Error
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
			GroupID uint `yaml:"group_id"`
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
			GroupID uint `yaml:"group_id"`
		}{
			GroupID: 1,
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

	return os.WriteFile(ConfigFile, data, 0600)
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
	log.Println("Auto setup enabled, configuring from environment variables...")

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
			Host:     getEnvOrDefault("REDIS_HOST", "localhost"),
			Port:     getEnvIntOrDefault("REDIS_PORT", 6379),
			Password: getEnvOrDefault("REDIS_PASSWORD", ""),
			DB:       getEnvIntOrDefault("REDIS_DB", 0),
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
		log.Println("Generated JWT secret automatically")
	}

	if cfg.Admin.Password == "" {
		// 自动安装必须显式提供管理员密码，避免弱口令或空口令。
		return fmt.Errorf("admin password must be provided when AUTO_SETUP is enabled")
	}

	// Test database connection
	log.Println("Testing database connection...")
	if err := TestDatabaseConnection(&cfg.Database); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	log.Println("Database connection successful")

	// Test Redis connection
	log.Println("Testing Redis connection...")
	if err := TestRedisConnection(&cfg.Redis); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}
	log.Println("Redis connection successful")

	// Initialize database
	log.Println("Initializing database...")
	if err := initializeDatabase(cfg); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}
	log.Println("Database initialized successfully")

	// Create admin user
	log.Println("Creating admin user...")
	if err := createAdminUser(cfg); err != nil {
		return fmt.Errorf("admin user creation failed: %w", err)
	}
	log.Printf("Admin user created: %s", cfg.Admin.Email)

	// Write config file
	log.Println("Writing configuration file...")
	if err := writeConfigFile(cfg); err != nil {
		return fmt.Errorf("config file creation failed: %w", err)
	}
	log.Println("Configuration file created")

	// Create installation lock file
	if err := createInstallLock(); err != nil {
		return fmt.Errorf("failed to create install lock: %w", err)
	}
	log.Println("Installation lock created")

	log.Println("Auto setup completed successfully!")
	return nil
}
