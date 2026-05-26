package repository

import (
	"crypto/tls"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/redis/go-redis/v9"
)

// InitRedis 初始化 Redis 客户端
//
// 性能优化说明：
// 原实现使用 go-redis 默认配置，未设置连接池和超时参数：
// 1. 默认连接池大小可能不足以支撑高并发
// 2. 无超时控制可能导致慢操作阻塞
//
// 新实现支持可配置的连接池和超时参数：
// 1. PoolSize: 控制最大并发连接数（默认 128）
// 2. MinIdleConns: 保持最小空闲连接，减少冷启动延迟（默认 10）
// 3. DialTimeout/ReadTimeout/WriteTimeout: 精确控制各阶段超时
func InitRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(buildRedisOptions(cfg))
}

// buildRedisOptions 构建 Redis 连接选项
// 从配置文件读取连接池和超时参数，支持生产环境调优
func buildRedisOptions(cfg *config.Config) *redis.Options {
	opts := &redis.Options{
		Addr:         cfg.Redis.Address(),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  time.Duration(cfg.Redis.DialTimeoutSeconds) * time.Second,  // 建连超时
		ReadTimeout:  time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second,  // 读取超时
		WriteTimeout: time.Duration(cfg.Redis.WriteTimeoutSeconds) * time.Second, // 写入超时
		PoolSize:     cfg.Redis.PoolSize,                                         // 连接池大小
		MinIdleConns: cfg.Redis.MinIdleConns,                                     // 最小空闲连接
	}

	if cfg.Redis.EnableTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: cfg.Redis.Host,
		}
	}

	return opts
}
