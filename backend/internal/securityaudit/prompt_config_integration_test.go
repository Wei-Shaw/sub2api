package securityaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const promptAuditRedisTestEnv = "PROMPT_AUDIT_TEST_REDIS_ADDR"

type postgresPromptAuditSettingRepository struct{ db *sql.DB REDACTED

func (r postgresPromptAuditSettingRepository) Get(ctx context.Context, key string) (*service.Setting, error) {
	var value string
	var updated time.Time
	err := r.db.QueryRowContext(ctx, `SELECT value,updated_at FROM settings WHERE key=$1`, key).Scan(&value, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSettingNotFound
REDACTED
	if err != nil {
		return nil, err
REDACTED
	return &service.Setting{Key: key, Value: value, UpdatedAt: updatedREDACTED, nil
REDACTED

func (r postgresPromptAuditSettingRepository) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
REDACTED
	return setting.Value, nil
REDACTED

func (r postgresPromptAuditSettingRepository) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW())
		ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=EXCLUDED.updated_at`, key, value)
	return err
REDACTED

func (r postgresPromptAuditSettingRepository) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = ""
REDACTED
	rows, err := r.db.QueryContext(ctx, `SELECT key,value FROM settings WHERE key=ANY($1)`, pq.Array(keys))
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
	REDACTED
		result[key] = value
REDACTED
	return result, rows.Err()
REDACTED

func (r postgresPromptAuditSettingRepository) SetMultiple(ctx context.Context, values map[string]string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
REDACTED
	defer func() { _ = tx.Rollback() REDACTED()
	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW())
			ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=EXCLUDED.updated_at`, key, value); err != nil {
			return err
	REDACTED
REDACTED
	return tx.Commit()
REDACTED

func (r postgresPromptAuditSettingRepository) GetAll(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = rows.Close() REDACTED()
	result := map[string]string{REDACTED
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
	REDACTED
		result[key] = value
REDACTED
	return result, rows.Err()
REDACTED

func (r postgresPromptAuditSettingRepository) Delete(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM settings WHERE key=$1`, key)
	return err
REDACTED

func promptAuditTestEncryptor(t *testing.T) service.SecretEncryptor {
REDACTED
	encryptor, err := repository.NewAESEncryptor(&config.Config{Totp: config.TotpConfig{EncryptionKey: strings.Repeat("42", 32)REDACTEDREDACTED)
REDACTED
	return encryptor
REDACTED

func promptAuditUpdateRequest(version int64, workerCount int, token string) UpdateConfigRequest {
	return UpdateConfigRequest{
		ExpectedConfigVersion: version, Enabled: true, BlockingEnabled: false, StorePassEvents: false,
		Strategy: "priority", WorkerCount: workerCount, QueueCapacity: 64, Scanners: []string{"pii", "jailbreak"REDACTED,
		AllGroups: true, Endpoints: []UpdateEndpoint{{
			ID: "guard-one", Name: "Guard One", Protocol: "openai_compatible",
			BaseURL: "http://127.0.0.1:18080", Model: "", Token: token,
			TimeoutMS: 1000, InputLimit: 1024, Enabled: true,
REDACTED
REDACTED
REDACTED

func waitForConfigVersion(t *testing.T, manager *ConfigManager, version int64, timeout time.Duration) {
REDACTED
	require.Eventually(t, func() bool {
		active, ok := manager.Active()
		return ok && active.ConfigVersion == version
REDACTED, timeout, 20*time.Millisecond)
REDACTED

func TestPromptAuditConfigCASSecretRoundTripInvalidationAndTTL(t *testing.T) {
	redisAddress := strings.TrimSpace(os.Getenv(promptAuditRedisTestEnv))
	if redisAddress == "" {
		t.Skip(promptAuditRedisTestEnv + " is not set")
REDACTED
	db := openPromptAuditIntegrationDB(t)
	settingRepo := postgresPromptAuditSettingRepository{db: dbREDACTED
	require.NoError(t, settingRepo.Set(context.Background(), SettingKeyRiskControl, "true"))
	encryptor := promptAuditTestEncryptor(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddressREDACTED)
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) REDACTED)
	require.NoError(t, redisClient.Ping(context.Background()).Err())

	managerOne := NewConfigManager(db, settingRepo, redisClient, encryptor)
	managerTwo := NewConfigManager(db, settingRepo, redisClient, encryptor)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, managerOne.Start(ctx))
	require.NoError(t, managerTwo.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, managerOne.Shutdown(context.Background()))
		require.NoError(t, managerTwo.Shutdown(context.Background()))
REDACTED)
	require.Eventually(t, func() bool {
		return redisClient.PubSubNumSub(context.Background(), ConfigInvalidationChannel).Val()[ConfigInvalidationChannel] >= 2
REDACTED, 2*time.Second, 20*time.Millisecond)

	const canary = "GUARD_TOKEN_CANARY_SECRET_4_CONFIG"
	public, err := managerOne.Save(context.Background(), promptAuditUpdateRequest(1, 1, canary), 101)
REDACTED
	require.Equal(t, int64(2), public.ConfigVersion)
	require.True(t, public.Endpoints[0].HasToken)
	publicJSON, err := json.Marshal(public)
REDACTED
	require.NotContains(t, string(publicJSON), canary)
	waitForConfigVersion(t, managerTwo, 2, 2*time.Second)

	raw, err := settingRepo.GetValue(context.Background(), SettingKeyPromptAuditConfig)
REDACTED
	require.NotContains(t, raw, canary)
	stored, err := ParseStorageConfig(raw)
REDACTED
	require.NotEmpty(t, stored.Endpoints[0].TokenCiphertext)
	plain, err := encryptor.Decrypt(stored.Endpoints[0].TokenCiphertext)
REDACTED
	require.Equal(t, canary, plain)
	require.NotContains(t, stored.ChangeSummary, canary)
	require.NotContains(t, stored.ChangeSummary, stored.Endpoints[0].BaseURL)

	type saveResult struct {
		config PublicConfig
		err    error
REDACTED
	start := make(chan struct{REDACTED)
	results := make(chan saveResult, 2)
	var wg sync.WaitGroup
	for index, manager := range []*ConfigManager{managerOne, managerTwoREDACTED {
		wg.Add(1)
		go func(index int, manager *ConfigManager) {
			defer wg.Done()
			<-start
			cfg, saveErr := manager.Save(context.Background(), promptAuditUpdateRequest(2, index+2, ""), int64(201+index))
			results <- saveResult{config: cfg, err: saveErrREDACTED
	REDACTED(index, manager)
REDACTED
	close(start)
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for result := range results {
		if result.err == nil {
			succeeded++
			require.Equal(t, int64(3), result.config.ConfigVersion)
			continue
	REDACTED
		conflicted++
		require.Equal(t, ErrorCodeConfigConflict, infraerrors.Reason(result.err))
REDACTED
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, conflicted)
	waitForConfigVersion(t, managerOne, 3, 2*time.Second)
	waitForConfigVersion(t, managerTwo, 3, 2*time.Second)

	// A manager without Redis subscriptions must still converge through the
	// bounded five-second refresh loop.
	ttlManager := NewConfigManager(db, settingRepo, nil, encryptor)
	require.NoError(t, ttlManager.Start(ctx))
	t.Cleanup(func() { require.NoError(t, ttlManager.Shutdown(context.Background())) REDACTED)
	waitForConfigVersion(t, ttlManager, 3, time.Second)
	updated, err := managerOne.Save(context.Background(), promptAuditUpdateRequest(3, 5, ""), 301)
REDACTED
	require.Equal(t, int64(4), updated.ConfigVersion)
	waitForConfigVersion(t, ttlManager, 4, 7*time.Second)

	// Redis publication failure is observable degradation, not a rollback of a
	// successfully committed PostgreSQL config.
	deadRedis := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: 0, DialTimeout: 30 * time.Millisecond, ReadTimeout: 30 * time.Millisecond, WriteTimeout: 30 * time.MillisecondREDACTED)
	t.Cleanup(func() { _ = deadRedis.Close() REDACTED)
	degraded := NewConfigManager(db, settingRepo, deadRedis, encryptor)
	require.NoError(t, degraded.Reload(context.Background()))
	degradedSaved, err := degraded.Save(context.Background(), promptAuditUpdateRequest(4, 6, ""), 401)
REDACTED
	require.Equal(t, int64(5), degradedSaved.ConfigVersion)
	active, ok := degraded.Active()
	require.True(t, ok)
	require.Equal(t, int64(5), active.ConfigVersion)
REDACTED
