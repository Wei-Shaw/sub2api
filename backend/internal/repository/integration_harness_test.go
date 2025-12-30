//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	redisclient "github.com/redis/go-redis/v9"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	redisImageTag    = "redis:8.4-alpine"
	postgresImageTag = "postgres:18.1-alpine3.23"
)

var (
	integrationDB    *gorm.DB
	integrationRedis *redisclient.Client

	redisNamespaceSeq uint64
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	if err := timezone.Init("UTC"); err != nil {
		log.Printf("failed to init timezone: %v", err)
		os.Exit(1)
REDACTED

	if !dockerIsAvailable(ctx) {
		// In CI we expect Docker to be available so integration tests should fail loudly.
		if os.Getenv("CI") != "" {
			log.Printf("docker is not available (CI=true); failing integration tests")
			os.Exit(1)
	REDACTED
		log.Printf("docker is not available; skipping integration tests (start Docker to enable)")
		os.Exit(0)
REDACTED

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
REDACTED
	defer func() { _ = pgContainer.Terminate(ctx) REDACTED()

	redisContainer, err := tcredis.Run(
		ctx,
		redisImageTag,
	)
	if err != nil {
		log.Printf("failed to start redis container: %v", err)
		os.Exit(1)
REDACTED
	defer func() { _ = redisContainer.Terminate(ctx) REDACTED()

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	if err != nil {
		log.Printf("failed to get postgres dsn: %v", err)
		os.Exit(1)
REDACTED

	integrationDB, err = openGormWithRetry(ctx, dsn, 30*time.Second)
	if err != nil {
		log.Printf("failed to open gorm db: %v", err)
		os.Exit(1)
REDACTED
	// 使用 simple 模式以便测试默认分组功能
	if err := AutoMigrate(integrationDB, "simple"); err != nil {
		log.Printf("failed to automigrate db: %v", err)
		os.Exit(1)
REDACTED

	redisHost, err := redisContainer.Host(ctx)
	if err != nil {
		log.Printf("failed to get redis host: %v", err)
		os.Exit(1)
REDACTED
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	if err != nil {
		log.Printf("failed to get redis port: %v", err)
		os.Exit(1)
REDACTED

	integrationRedis = redisclient.NewClient(&redisclient.Options{
		Addr: fmt.Sprintf("%s:%d", redisHost, redisPort.Int()),
		DB:   0,
REDACTED)
	if err := integrationRedis.Ping(ctx).Err(); err != nil {
		log.Printf("failed to ping redis: %v", err)
		os.Exit(1)
REDACTED

	code := m.Run()

	_ = integrationRedis.Close()

	os.Exit(code)
REDACTED

func dockerIsAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Env = os.Environ()
	return cmd.Run() == nil
REDACTED

func selectDockerImage(ctx context.Context, preferred string) string {
	if dockerImageExists(ctx, preferred) {
		return preferred
REDACTED

	return preferred
REDACTED

func dockerImageExists(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	cmd.Env = os.Environ()
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
REDACTED

func openGormWithRetry(ctx context.Context, dsn string, timeout time.Duration) (*gorm.DB, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
	REDACTED)
		if err != nil {
			lastErr = err
			time.Sleep(250 * time.Millisecond)
			continue
	REDACTED

		sqlDB, err := db.DB()
		if err != nil {
			lastErr = err
			time.Sleep(250 * time.Millisecond)
			continue
	REDACTED

		if err := pingWithTimeout(ctx, sqlDB, 2*time.Second); err != nil {
			lastErr = err
			time.Sleep(250 * time.Millisecond)
			continue
	REDACTED

		return db, nil
REDACTED

	return nil, fmt.Errorf("db not ready after %s: %w", timeout, lastErr)
REDACTED

func pingWithTimeout(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.PingContext(pingCtx)
REDACTED

func testTx(t *testing.T) *gorm.DB {
REDACTED

	tx := integrationDB.Begin()
	require.NoError(t, tx.Error, "begin tx")
	t.Cleanup(func() {
		_ = tx.Rollback().Error
REDACTED)
	return tx
REDACTED

func testRedis(t *testing.T) *redisclient.Client {
REDACTED

	prefix := fmt.Sprintf(
		"it:%s:%d:%d:",
		sanitizeRedisNamespace(t.Name()),
		time.Now().UnixNano(),
		atomic.AddUint64(&redisNamespaceSeq, 1),
	)

	opts := *integrationRedis.Options()
	rdb := redisclient.NewClient(&opts)
	rdb.AddHook(prefixHook{prefix: prefixREDACTED)

	t.Cleanup(func() {
		ctx := context.Background()

		var cursor uint64
		for {
			keys, nextCursor, err := integrationRedis.Scan(ctx, cursor, prefix+"*", 500).Result()
			require.NoError(t, err, "scan redis keys for cleanup")
			if len(keys) > 0 {
				require.NoError(t, integrationRedis.Unlink(ctx, keys...).Err(), "unlink redis keys for cleanup")
		REDACTED

			cursor = nextCursor
			if cursor == 0 {
				break
		REDACTED
	REDACTED

		_ = rdb.Close()
REDACTED)

	return rdb
REDACTED

func assertTTLWithin(t *testing.T, ttl time.Duration, min, max time.Duration) {
REDACTED
	require.GreaterOrEqual(t, ttl, min, "ttl should be >= min")
	require.LessOrEqual(t, ttl, max, "ttl should be <= max")
REDACTED

func sanitizeRedisNamespace(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
REDACTED

type prefixHook struct {
	prefix string
REDACTED

func (h prefixHook) DialHook(next redisclient.DialHook) redisclient.DialHook { return next REDACTED

func (h prefixHook) ProcessHook(next redisclient.ProcessHook) redisclient.ProcessHook {
	return func(ctx context.Context, cmd redisclient.Cmder) error {
		h.prefixCmd(cmd)
		return next(ctx, cmd)
REDACTED
REDACTED

func (h prefixHook) ProcessPipelineHook(next redisclient.ProcessPipelineHook) redisclient.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redisclient.Cmder) error {
		for _, cmd := range cmds {
			h.prefixCmd(cmd)
	REDACTED
		return next(ctx, cmds)
REDACTED
REDACTED

func (h prefixHook) prefixCmd(cmd redisclient.Cmder) {
	args := cmd.Args()
	if len(args) < 2 {
		return
REDACTED

	prefixOne := func(i int) {
		if i < 0 || i >= len(args) {
			return
	REDACTED

		switch v := args[i].(type) {
		case string:
			if v != "" && !strings.HasPrefix(v, h.prefix) {
				args[i] = h.prefix + v
		REDACTED
		case []byte:
			s := string(v)
			if s != "" && !strings.HasPrefix(s, h.prefix) {
				args[i] = []byte(h.prefix + s)
		REDACTED
	REDACTED
REDACTED

	switch strings.ToLower(cmd.Name()) {
	case "get", "set", "setnx", "setex", "psetex", "incr", "decr", "incrby", "expire", "pexpire", "ttl", "pttl",
		"hgetall", "hget", "hset", "hdel", "hincrbyfloat", "exists":
		prefixOne(1)
	case "del", "unlink":
		for i := 1; i < len(args); i++ {
			prefixOne(i)
	REDACTED
	case "eval", "evalsha", "eval_ro", "evalsha_ro":
		if len(args) < 3 {
			return
	REDACTED
		numKeys, err := strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil || numKeys <= 0 {
			return
	REDACTED
		for i := 0; i < numKeys && 3+i < len(args); i++ {
			prefixOne(3 + i)
	REDACTED
	case "scan":
		for i := 2; i+1 < len(args); i++ {
			if strings.EqualFold(fmt.Sprint(args[i]), "match") {
				prefixOne(i + 1)
				break
		REDACTED
	REDACTED
REDACTED
REDACTED

// IntegrationRedisSuite provides a base suite for Redis integration tests.
// Embedding suites should call SetupTest to initialize ctx and rdb.
type IntegrationRedisSuite struct {
	suite.Suite
	ctx context.Context
	rdb *redisclient.Client
REDACTED

// SetupTest initializes ctx and rdb for each test method.
func (s *IntegrationRedisSuite) SetupTest() {
	s.ctx = context.Background()
	s.rdb = testRedis(s.T())
REDACTED

// RequireNoError is a convenience method wrapping require.NoError with s.T().
func (s *IntegrationRedisSuite) RequireNoError(err error, msgAndArgs ...any) {
	s.T().Helper()
	require.NoError(s.T(), err, msgAndArgs...)
REDACTED

// AssertTTLWithin asserts that ttl is within [min, max].
func (s *IntegrationRedisSuite) AssertTTLWithin(ttl, min, max time.Duration) {
	s.T().Helper()
	assertTTLWithin(s.T(), ttl, min, max)
REDACTED

// IntegrationDBSuite provides a base suite for DB (Gorm) integration tests.
// Embedding suites should call SetupTest to initialize ctx and db.
type IntegrationDBSuite struct {
	suite.Suite
	ctx context.Context
	db  *gorm.DB
REDACTED

// SetupTest initializes ctx and db for each test method.
func (s *IntegrationDBSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = testTx(s.T())
REDACTED

// RequireNoError is a convenience method wrapping require.NoError with s.T().
func (s *IntegrationDBSuite) RequireNoError(err error, msgAndArgs ...any) {
	s.T().Helper()
	require.NoError(s.T(), err, msgAndArgs...)
REDACTED
