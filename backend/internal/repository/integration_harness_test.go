//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	mysqlDriver "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	redisclient "github.com/redis/go-redis/v9"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/alicebob/miniredis/v2"
)

const (
	redisImageTag    = "redis:8.4-alpine"
	postgresImageTag = "postgres:18.1-alpine3.23"
)

var (
	integrationDB        *sql.DB
	integrationEntClient *dbent.Client
	integrationRedis     *redisclient.Client

	// integrationMiniRedis 非 nil 表示 integrationRedis 是 miniredis 实例
	// （Docker 不可用时的退化路径）。miniredis 的 TTL 不随墙钟推进，只能通过
	// FastForward 前进，依赖键自然过期的测试需要据此做时间快进。
	integrationMiniRedis *miniredis.Miniredis

	redisNamespaceSeq uint64
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	if err := timezone.Init("UTC"); err != nil {
		log.Printf("failed to init timezone: %v", err)
		os.Exit(1)
	}

	// SUB2API_TEST_DB=mysql 时使用外部 MySQL 测试库（SUB2API_TEST_MYSQL_DSN），
	// 用于 MySQL 方言的仓库层集成验证；默认 postgres 走 testcontainers。
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SUB2API_TEST_DB")), "mysql") {
		code := runMySQLHarness(ctx, m)
		os.Exit(code)
	}

	if !dockerIsAvailable(ctx) {
		// In CI we expect Docker to be available so integration tests should fail loudly.
		if os.Getenv("CI") != "" {
			log.Printf("docker is not available (CI=true); failing integration tests")
			os.Exit(1)
		}
		log.Printf("docker is not available; skipping integration tests (start Docker to enable)")
		os.Exit(0)
	}

	postgresImage := selectDockerImage(ctx, postgresImageTag)
	pgContainer, err := tcpostgres.Run(
		ctx,
		postgresImage,
		tcpostgres.WithDatabase("sub2api_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Printf("failed to start postgres container: %v", err)
		os.Exit(1)
	}
	defer func() { _ = pgContainer.Terminate(ctx) }()

	redisContainer, err := tcredis.Run(
		ctx,
		redisImageTag,
	)
	if err != nil {
		log.Printf("failed to start redis container: %v", err)
		os.Exit(1)
	}
	defer func() { _ = redisContainer.Terminate(ctx) }()

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	if err != nil {
		log.Printf("failed to get postgres dsn: %v", err)
		os.Exit(1)
	}

	integrationDB, err = openSQLWithRetry(ctx, dsn, 30*time.Second)
	if err != nil {
		log.Printf("failed to open sql db: %v", err)
		os.Exit(1)
	}
	if err := ApplyMigrations(ctx, integrationDB); err != nil {
		log.Printf("failed to apply db migrations: %v", err)
		os.Exit(1)
	}

	// 创建 ent client 用于集成测试
	drv := entsql.OpenDB(dialect.Postgres, integrationDB)
	integrationEntClient = dbent.NewClient(dbent.Driver(drv))

	integrationRedis, err = newRedisClientFromContainer(ctx, redisContainer)
	if err != nil {
		log.Printf("failed to connect redis container: %v", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = integrationEntClient.Close()
	_ = integrationRedis.Close()
	_ = integrationDB.Close()

	os.Exit(code)
}

// dialRedisReachable 探测指定 Redis 地址是否可连（空地址返回 false）。
func dialRedisReachable(ctx context.Context, addr string) bool {
	if addr == "" {
		return false
	}
	d := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// newRedisClientFromContainer 通过 redis 容器的 ConnectionString 构造客户端。
// （Host+MappedPort 组合依赖 testcontainers 核心库的 Port 类型细节，
// 不同版本方法集不同；ConnectionString 是模块层稳定 API。）
func newRedisClientFromContainer(ctx context.Context, c *tcredis.RedisContainer) (*redisclient.Client, error) {
	connStr, err := c.ConnectionString(ctx)
	if err != nil {
		return nil, err
	}
	addr := strings.TrimPrefix(strings.TrimPrefix(connStr, "redis://"), "rediss://")
	client := redisclient.NewClient(&redisclient.Options{Addr: addr, DB: 0})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func dockerIsAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

// runMySQLHarness 在外部 MySQL 测试库上运行整个集成测试套件。
//
// 环境变量：
//   - SUB2API_TEST_MYSQL_DSN（必填）：例如 debuguser:pass@tcp(localhost:3306)/sub2api_test?parseTime=true
//
// 行为：清空目标库全部对象 → 应用全部 MySQL 方言迁移 → 设置进程方言 → 构造 ent client。
// Redis：优先 Docker 容器；Docker 不可用时退化为 miniredis（进程内实现，
// 满足仓库层测试对 Redis 客户端的需求，个别依赖真实 Redis 行为的测试若失败属预期）。
// 注意：会销毁目标库数据，仅限专用测试库使用。
func runMySQLHarness(ctx context.Context, m *testing.M) int {
	dsn := strings.TrimSpace(os.Getenv("SUB2API_TEST_MYSQL_DSN"))
	if dsn == "" {
		log.Printf("SUB2API_TEST_DB=mysql requires SUB2API_TEST_MYSQL_DSN")
		os.Exit(1)
	}

	cfg, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		log.Printf("invalid SUB2API_TEST_MYSQL_DSN: %v", err)
		os.Exit(1)
	}
	if cfg.DBName == "" {
		log.Printf("SUB2API_TEST_MYSQL_DSN must include a database name")
		os.Exit(1)
	}
	cfg.MultiStatements = true
	cfg.ParseTime = true
	if cfg.Collation == "" || cfg.Collation == "utf8mb4_general_ci" {
		cfg.Collation = "utf8mb4_0900_ai_ci"
	}
	// 测试统一 UTC（与 postgres 路径 TimeZone=UTC 对齐）。
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}

	integrationDB, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Printf("failed to open mysql: %v", err)
		os.Exit(1)
	}
	if err := pingWithTimeout(ctx, integrationDB, 10*time.Second); err != nil {
		log.Printf("failed to ping mysql: %v", err)
		os.Exit(1)
	}

	if err := resetMySQLSchema(ctx, integrationDB); err != nil {
		log.Printf("failed to reset mysql schema: %v", err)
		os.Exit(1)
	}
	if _, err := integrationDB.ExecContext(ctx, schemaMigrationsTableDDLMySQL); err != nil {
		log.Printf("failed to create schema_migrations: %v", err)
		os.Exit(1)
	}
	// 测试库可能无 SUPER 权限（binlog 开启时 CREATE TRIGGER 报 1419），
	// 容错跳过触发器 DDL 以完成 schema 引导；触发器行为在该环境不验证。
	// 必须在应用迁移之前设置。
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SUB2API_TEST_MYSQL_STRICT_TRIGGERS")), "true") {
		mysqlTriggerFailureTolerant = false
	} else {
		mysqlTriggerFailureTolerant = true
	}
	if err := applyMigrationsFSWithDialect(ctx, integrationDB, migrationsForDialect(dialect.MySQL), dialect.MySQL); err != nil {
		log.Printf("failed to apply mysql migrations: %v", err)
		os.Exit(1)
	}

	SetDialect(dialect.MySQL)
	drv := entsql.OpenDB(dialect.MySQL, integrationDB)
	integrationEntClient = dbent.NewClient(dbent.Driver(drv))

	var miniRedis *miniredis.Miniredis
	// Redis 来源优先级：Docker 容器 > 显式环境变量 > 本机 6379 > miniredis 兜底。
	// （miniredis 的 TTL 不随墙钟推进、个别命令行为与真机有差，真 Redis 优先。）
	switch {
	case dockerIsAvailable(ctx):
		redisContainer, err := tcredis.Run(ctx, redisImageTag)
		if err != nil {
			log.Printf("failed to start redis container: %v", err)
			os.Exit(1)
		}
		defer func() { _ = redisContainer.Terminate(ctx) }()
		integrationRedis, err = newRedisClientFromContainer(ctx, redisContainer)
		if err != nil {
			log.Printf("failed to connect redis container: %v", err)
			os.Exit(1)
		}
	case dialRedisReachable(ctx, strings.TrimSpace(os.Getenv("SUB2API_TEST_REDIS_ADDR"))):
		integrationRedis = redisclient.NewClient(&redisclient.Options{
			Addr: strings.TrimSpace(os.Getenv("SUB2API_TEST_REDIS_ADDR")),
			DB:   0,
		})
	case dialRedisReachable(ctx, "127.0.0.1:6379"):
		log.Printf("using local redis at 127.0.0.1:6379 for integration tests")
		integrationRedis = redisclient.NewClient(&redisclient.Options{
			Addr: "127.0.0.1:6379",
			DB:   0,
		})
	default:
		log.Printf("docker/redis unavailable; using miniredis for integration tests")
		mini, miniErr := miniredis.Run()
		if miniErr != nil {
			log.Printf("failed to start miniredis: %v", miniErr)
			os.Exit(1)
		}
		miniRedis = mini
		integrationMiniRedis = mini
		integrationRedis = redisclient.NewClient(&redisclient.Options{
			Addr: miniRedis.Addr(),
			DB:   0,
		})
	}
	if err := integrationRedis.Ping(ctx).Err(); err != nil {
		log.Printf("failed to ping redis: %v", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = integrationEntClient.Close()
	_ = integrationRedis.Close()
	_ = integrationDB.Close()
	return code
}

// selectDockerImage resolves the container image for the harness.
//
// SUB2API_TEST_POSTGRES_IMAGE overrides the PostgreSQL image so the suite can be
// run against the oldest documented-supported server, not just the newest. That
// matters for SQL that behaves differently across major versions: jsonpath
// .datetime() only accepts the ISO-8601 "Z" designator from PostgreSQL 17 on, so
// a suite pinned to 18 cannot observe breakage on 14-16.
//
//	SUB2API_TEST_POSTGRES_IMAGE=postgres:15-alpine go test -tags integration ./internal/repository/
func selectDockerImage(ctx context.Context, preferred string) string {
	if override := strings.TrimSpace(os.Getenv("SUB2API_TEST_POSTGRES_IMAGE")); override != "" &&
		strings.HasPrefix(preferred, "postgres:") {
		return override
	}
	if dockerImageExists(ctx, preferred) {
		return preferred
	}

	return preferred
}

func dockerImageExists(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	cmd.Env = os.Environ()
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func openSQLWithRetry(ctx context.Context, dsn string, timeout time.Duration) (*sql.DB, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			lastErr = err
			time.Sleep(250 * time.Millisecond)
			continue
		}

		if err := pingWithTimeout(ctx, db, 2*time.Second); err != nil {
			lastErr = err
			_ = db.Close()
			time.Sleep(250 * time.Millisecond)
			continue
		}

		return db, nil
	}

	return nil, fmt.Errorf("db not ready after %s: %w", timeout, lastErr)
}

func pingWithTimeout(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.PingContext(pingCtx)
}

func testTx(t *testing.T) *sql.Tx {
	t.Helper()

	tx, err := integrationDB.BeginTx(context.Background(), nil)
	require.NoError(t, err, "begin tx")
	t.Cleanup(func() {
		_ = tx.Rollback()
	})
	return tx
}

// testEntClient 返回全局的 ent client，用于测试需要内部管理事务的代码（如 Create/Update 方法）。
// 注意：此 client 的操作会真正写入数据库，测试结束后不会自动回滚。
func testEntClient(t *testing.T) *dbent.Client {
	t.Helper()
	return integrationEntClient
}

// testEntTx 返回一个 ent 事务，用于需要事务隔离的测试。
// 测试结束后会自动回滚，不会影响数据库状态。
func testEntTx(t *testing.T) *dbent.Tx {
	t.Helper()

	tx, err := integrationEntClient.Tx(context.Background())
	require.NoError(t, err, "begin ent tx")
	t.Cleanup(func() {
		_ = tx.Rollback()
	})
	return tx
}

// testEntSQLTx 已弃用：不要在新测试中使用此函数。
// 基于 *sql.Tx 创建的 ent client 在调用 client.Tx() 时会 panic。
// 对于需要测试内部使用事务的代码，请使用 testEntClient。
// 对于需要事务隔离的测试，请使用 testEntTx。
//
// Deprecated: Use testEntClient or testEntTx instead.
func testEntSQLTx(t *testing.T) (*dbent.Client, *sql.Tx) {
	t.Helper()

	// 直接失败，避免旧测试误用导致的事务嵌套 panic。
	t.Fatalf("testEntSQLTx 已弃用：请使用 testEntClient 或 testEntTx")
	return nil, nil
}

func testRedis(t *testing.T) *redisclient.Client {
	t.Helper()

	prefix := fmt.Sprintf(
		"it:%s:%d:%d:",
		sanitizeRedisNamespace(t.Name()),
		time.Now().UnixNano(),
		atomic.AddUint64(&redisNamespaceSeq, 1),
	)

	opts := *integrationRedis.Options()
	rdb := redisclient.NewClient(&opts)
	rdb.AddHook(prefixHook{prefix: prefix})

	t.Cleanup(func() {
		ctx := context.Background()

		var cursor uint64
		for {
			keys, nextCursor, err := integrationRedis.Scan(ctx, cursor, prefix+"*", 500).Result()
			require.NoError(t, err, "scan redis keys for cleanup")
			if len(keys) > 0 {
				require.NoError(t, integrationRedis.Unlink(ctx, keys...).Err(), "unlink redis keys for cleanup")
			}

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}

		_ = rdb.Close()
	})

	return rdb
}

func assertTTLWithin(t *testing.T, ttl time.Duration, min, max time.Duration) {
	t.Helper()
	require.GreaterOrEqual(t, ttl, min, "ttl should be >= min")
	require.LessOrEqual(t, ttl, max, "ttl should be <= max")
}

func sanitizeRedisNamespace(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

type prefixHook struct {
	prefix string
}

func (h prefixHook) DialHook(next redisclient.DialHook) redisclient.DialHook { return next }

func (h prefixHook) ProcessHook(next redisclient.ProcessHook) redisclient.ProcessHook {
	return func(ctx context.Context, cmd redisclient.Cmder) error {
		h.prefixCmd(cmd)
		return next(ctx, cmd)
	}
}

func (h prefixHook) ProcessPipelineHook(next redisclient.ProcessPipelineHook) redisclient.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redisclient.Cmder) error {
		for _, cmd := range cmds {
			h.prefixCmd(cmd)
		}
		return next(ctx, cmds)
	}
}

func (h prefixHook) prefixCmd(cmd redisclient.Cmder) {
	args := cmd.Args()
	if len(args) < 2 {
		return
	}

	prefixOne := func(i int) {
		if i < 0 || i >= len(args) {
			return
		}

		switch v := args[i].(type) {
		case string:
			if v != "" && !strings.HasPrefix(v, h.prefix) {
				args[i] = h.prefix + v
			}
		case []byte:
			s := string(v)
			if s != "" && !strings.HasPrefix(s, h.prefix) {
				args[i] = []byte(h.prefix + s)
			}
		}
	}

	switch strings.ToLower(cmd.Name()) {
	case "get", "set", "setnx", "setex", "psetex", "incr", "decr", "incrby", "expire", "pexpire", "ttl", "pttl",
		"hgetall", "hget", "hset", "hdel", "hincrbyfloat", "exists",
		"zadd", "zcard", "zrange", "zrangebyscore", "zrem", "zremrangebyscore", "zrevrange", "zrevrangebyscore", "zscore":
		prefixOne(1)
	case "mget":
		for i := 1; i < len(args); i++ {
			prefixOne(i)
		}
	case "del", "unlink":
		for i := 1; i < len(args); i++ {
			prefixOne(i)
		}
	case "eval", "evalsha", "eval_ro", "evalsha_ro":
		if len(args) < 3 {
			return
		}
		numKeys, err := strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil || numKeys <= 0 {
			return
		}
		for i := 0; i < numKeys && 3+i < len(args); i++ {
			prefixOne(3 + i)
		}
	case "scan":
		for i := 2; i+1 < len(args); i++ {
			if strings.EqualFold(fmt.Sprint(args[i]), "match") {
				prefixOne(i + 1)
				break
			}
		}
	}
}

// IntegrationRedisSuite provides a base suite for Redis integration tests.
// Embedding suites should call SetupTest to initialize ctx and rdb.
type IntegrationRedisSuite struct {
	suite.Suite
	ctx context.Context
	rdb *redisclient.Client
}

// SetupTest initializes ctx and rdb for each test method.
func (s *IntegrationRedisSuite) SetupTest() {
	s.ctx = context.Background()
	s.rdb = testRedis(s.T())
}

// RequireNoError is a convenience method wrapping require.NoError with s.T().
func (s *IntegrationRedisSuite) RequireNoError(err error, msgAndArgs ...any) {
	s.T().Helper()
	require.NoError(s.T(), err, msgAndArgs...)
}

// AssertTTLWithin asserts that ttl is within [min, max].
func (s *IntegrationRedisSuite) AssertTTLWithin(ttl, min, max time.Duration) {
	s.T().Helper()
	assertTTLWithin(s.T(), ttl, min, max)
}

// IntegrationDBSuite provides a base suite for DB integration tests.
// Embedding suites should call SetupTest to initialize ctx and client.
type IntegrationDBSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	tx     *dbent.Tx
}

// SetupTest initializes ctx and client for each test method.
func (s *IntegrationDBSuite) SetupTest() {
	s.ctx = context.Background()
	// 统一使用 ent.Tx，确保每个测试都有独立事务并自动回滚。
	tx := testEntTx(s.T())
	s.tx = tx
	s.client = tx.Client()
}

// RequireNoError is a convenience method wrapping require.NoError with s.T().
func (s *IntegrationDBSuite) RequireNoError(err error, msgAndArgs ...any) {
	s.T().Helper()
	require.NoError(s.T(), err, msgAndArgs...)
}
