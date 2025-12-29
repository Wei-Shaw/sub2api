package infrastructure

import (
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"

	entsql "entgo.io/ent/dialect/sql"
)

// ProviderSet 是基础设施层的 Wire 依赖提供者集合。
//
// Wire 是 Google 开发的编译时依赖注入工具。ProviderSet 将相关的依赖提供函数
// 组织在一起，便于在应用程序启动时自动组装依赖关系。
//
// 包含的提供者：
//   - ProvideEnt: 提供 Ent ORM 客户端
//   - ProvideSQLDB: 提供底层 SQL 数据库连接
//   - ProvideRedis: 提供 Redis 客户端
var ProviderSet = wire.NewSet(
	ProvideEnt,
	ProvideSQLDB,
	ProvideRedis,
)

// ProvideEnt 为依赖注入提供 Ent 客户端。
//
// 该函数是 InitEnt 的包装器，符合 Wire 的依赖提供函数签名要求。
// Wire 会在编译时分析依赖关系，自动生成初始化代码。
//
// 依赖：config.Config
// 提供：*ent.Client
func ProvideEnt(cfg *config.Config) (*ent.Client, error) {
	client, _, err := InitEnt(cfg)
	return client, err
REDACTED

// ProvideSQLDB 从 Ent 客户端提取底层的 *sql.DB 连接。
//
// 某些 Repository 需要直接执行原生 SQL（如复杂的批量更新、聚合查询），
// 此时需要访问底层的 sql.DB 而不是通过 Ent ORM。
//
// 设计说明：
//   - Ent 底层使用 sql.DB，通过 Driver 接口可以访问
//   - 这种设计允许在同一事务中混用 Ent 和原生 SQL
//
// 依赖：*ent.Client
// 提供：*sql.DB
func ProvideSQLDB(client *ent.Client) (*sql.DB, error) {
	if client == nil {
		return nil, errors.New("nil ent client")
REDACTED
	// 从 Ent 客户端获取底层驱动
	drv, ok := client.Driver().(*entsql.Driver)
	if !ok {
		return nil, errors.New("ent driver does not expose *sql.DB")
REDACTED
	// 返回驱动持有的 sql.DB 实例
	return drv.DB(), nil
REDACTED

// ProvideRedis 为依赖注入提供 Redis 客户端。
//
// Redis 用于：
//   - 分布式锁（如并发控制）
//   - 缓存（如用户会话、API 响应缓存）
//   - 速率限制
//   - 实时统计数据
//
// 依赖：config.Config
// 提供：*redis.Client
func ProvideRedis(cfg *config.Config) *redis.Client {
	return InitRedis(cfg)
REDACTED
