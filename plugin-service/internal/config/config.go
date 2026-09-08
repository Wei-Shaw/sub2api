package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr string
	Database   DatabaseConfig
	MinIO      MinIOConfig
}

type MinIOConfig struct {
	Enabled   bool
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type DatabaseConfig struct {
	Enabled  bool
	URL      string
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func MustLoad() Config {
	loadDotEnvIfPresent()
	port := envInt("PLUGIN_SERVER_PORT", 8091)
	return Config{
		ListenAddr: ":" + strconv.Itoa(port),
		Database:   loadDatabaseConfig(),
		MinIO:      loadMinIOConfig(),
	}
}

func loadMinIOConfig() MinIOConfig {
	config := MinIOConfig{
		Endpoint:  strings.TrimSpace(os.Getenv("MINIO_ENDPOINT")),
		AccessKey: firstNonEmptyEnv("MINIO_ACCESS_KEY", "MINIO_ROOT_USER"),
		SecretKey: firstNonEmptyEnv("MINIO_SECRET_KEY", "MINIO_ROOT_PASSWORD"),
		Bucket:    strings.TrimSpace(os.Getenv("MINIO_BUCKET")),
	}
	configured := 0
	for _, value := range []string{config.Endpoint, config.AccessKey, config.SecretKey, config.Bucket} {
		if value != "" {
			configured++
		}
	}
	if configured == 0 {
		return config
	}
	if configured != 4 {
		panic("incomplete MinIO configuration: endpoint, access key, secret key, and bucket are required")
	}
	useSSL, err := strconv.ParseBool(envString("MINIO_USE_SSL", "false"))
	if err != nil {
		panic("invalid MINIO_USE_SSL value")
	}
	config.Enabled = true
	config.UseSSL = useSSL
	return config
}

func (d DatabaseConfig) DSN() string {
	if strings.TrimSpace(d.URL) != "" {
		return strings.TrimSpace(d.URL)
	}
	sslMode := strings.TrimSpace(d.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	if d.Password == "" {
		return fmt.Sprintf(
			"host=%s port=%d user=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.User, d.DBName, sslMode,
		)
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, sslMode,
	)
}

func loadDatabaseConfig() DatabaseConfig {
	if url := strings.TrimSpace(os.Getenv("DATABASE_URL")); url != "" {
		return DatabaseConfig{
			Enabled: true,
			URL:     url,
		}
	}

	host := strings.TrimSpace(os.Getenv("DATABASE_HOST"))
	if host == "" {
		host = envString("POSTGRES_HOST", "")
	}
	user := envString("DATABASE_USER", envString("POSTGRES_USER", "postgres"))
	password := firstNonEmptyEnv("DATABASE_PASSWORD", "POSTGRES_PASSWORD")
	dbName := envString("DATABASE_DBNAME", envString("POSTGRES_DB", "sub2api"))
	hasSharedPostgresConfig := host != "" || password != "" || os.Getenv("POSTGRES_USER") != "" || os.Getenv("POSTGRES_DB") != ""
	if !hasSharedPostgresConfig {
		return DatabaseConfig{}
	}
	if host == "" {
		host = "127.0.0.1"
	}

	return DatabaseConfig{
		Enabled:  true,
		Host:     host,
		Port:     firstPositiveIntEnv(5432, "DATABASE_PORT", "POSTGRES_PORT"),
		User:     user,
		Password: password,
		DBName:   dbName,
		SSLMode:  envString("DATABASE_SSLMODE", "disable"),
	}
}

func loadDotEnvIfPresent() {
	for _, candidate := range []string{
		filepath.Join("..", ".env"),
		filepath.Join("..", "deploy", ".env"),
	} {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		applyDotEnvFile(candidate)
	}
}

func applyDotEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func firstPositiveIntEnv(fallback int, keys ...string) int {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
