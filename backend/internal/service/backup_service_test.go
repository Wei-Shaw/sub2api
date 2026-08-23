//go:build unit

package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ─── Mocks ───

type mockSettingRepo struct {
	mu            sync.Mutex
	data          map[string]string
	getValueErr   error
	getValueCalls int
}

func newMockSettingRepo() *mockSettingRepo {
	return &mockSettingRepo{data: make(map[string]string)}
}

func (m *mockSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: v}, nil
}

func (m *mockSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getValueCalls++
	if m.getValueErr != nil {
		return "", m.getValueErr
	}
	v, ok := m.data[key]
	if !ok {
		return "", nil
	}
	return v, nil
}

func (m *mockSettingRepo) Set(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]string)
	for _, k := range keys {
		if v, ok := m.data[k]; ok {
			result[k] = v
		}
	}
	return result, nil
}

func (m *mockSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range settings {
		m.data[k] = v
	}
	return nil
}

func (m *mockSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]string, len(m.data))
	for k, v := range m.data {
		result[k] = v
	}
	return result, nil
}

func (m *mockSettingRepo) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// plainEncryptor 仅做 base64-like 包装，用于测试
type plainEncryptor struct{}

func (e *plainEncryptor) Encrypt(plaintext string) (string, error) {
	return "ENC:" + plaintext, nil
}

func (e *plainEncryptor) Decrypt(ciphertext string) (string, error) {
	if strings.HasPrefix(ciphertext, "ENC:") {
		return strings.TrimPrefix(ciphertext, "ENC:"), nil
	}
	return ciphertext, fmt.Errorf("not encrypted")
}

type mockDumper struct {
	dumpData []byte
	dumpErr  error
	restored []byte
	restErr  error
}

func (m *mockDumper) Dump(_ context.Context) (io.ReadCloser, error) {
	if m.dumpErr != nil {
		return nil, m.dumpErr
	}
	return io.NopCloser(bytes.NewReader(m.dumpData)), nil
}

func (m *mockDumper) Restore(_ context.Context, data io.Reader) error {
	if m.restErr != nil {
		return m.restErr
	}
	d, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	m.restored = d
	return nil
}

// blockingDumper 可控延迟的 dumper，用于测试异步行为
type blockingDumper struct {
	blockCh chan struct{}
	data    []byte
	restErr error
}

func (d *blockingDumper) Dump(ctx context.Context) (io.ReadCloser, error) {
	select {
	case <-d.blockCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return io.NopCloser(bytes.NewReader(d.data)), nil
}

func (d *blockingDumper) Restore(_ context.Context, data io.Reader) error {
	if d.restErr != nil {
		return d.restErr
	}
	_, _ = io.ReadAll(data)
	return nil
}

type mockObjectStore struct {
	objects          map[string][]byte
	mu               sync.Mutex
	failUploadFileAt int
	uploadFileCalls  int
	deletedKeys      []string
	failDeleteKeys   map[string]error
}

type cancelingUploadFailureStore struct {
	*mockObjectStore
	cancel context.CancelFunc
}

func (m *cancelingUploadFailureStore) UploadFile(_ context.Context, key string, filePath string, _ string) (int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return 0, readErr
	}
	if closeErr != nil {
		return 0, closeErr
	}

	m.mu.Lock()
	m.objects[key] = data
	m.mu.Unlock()
	m.cancel()
	return 0, fmt.Errorf("injected upload failure after object landed")
}

func (m *cancelingUploadFailureStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.mockObjectStore.Delete(ctx, key)
}

func newMockObjectStore() *mockObjectStore {
	return &mockObjectStore{objects: make(map[string][]byte), failDeleteKeys: make(map[string]error)}
}

func (m *mockObjectStore) Upload(_ context.Context, key string, body io.Reader, _ string) (int64, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	m.objects[key] = data
	m.mu.Unlock()
	return int64(len(data)), nil
}

func (m *mockObjectStore) UploadFile(ctx context.Context, key string, filePath string, contentType string) (int64, error) {
	m.mu.Lock()
	m.uploadFileCalls++
	call := m.uploadFileCalls
	failAt := m.failUploadFileAt
	m.mu.Unlock()
	if failAt > 0 && call == failAt {
		return 0, fmt.Errorf("injected upload failure at call %d", call)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	return m.Upload(ctx, key, file, contentType)
}

func (m *mockObjectStore) Download(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	data, ok := m.objects[key]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("not found: %s", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockObjectStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	m.deletedKeys = append(m.deletedKeys, key)
	if err, ok := m.failDeleteKeys[key]; ok {
		m.mu.Unlock()
		return err
	}
	delete(m.objects, key)
	m.mu.Unlock()
	return nil
}

func (m *mockObjectStore) PresignURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://presigned.example.com/" + key, nil
}

func (m *mockObjectStore) HeadBucket(_ context.Context) error {
	return nil
}

func newTestBackupService(repo *mockSettingRepo, dumper DBDumper, store *mockObjectStore) *BackupService {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:   "localhost",
			Port:   5432,
			User:   "test",
			DBName: "testdb",
		},
		// A fixed encryption key is the supported production posture: persisting
		// an S3 secret requires it (#4524).
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	}
	factory := func(_ context.Context, _ *BackupS3Config) (BackupObjectStore, error) {
		return store, nil
	}
	return NewBackupService(repo, cfg, &plainEncryptor{}, factory, dumper)
}

// newTestBackupServiceEphemeralKey mirrors a deployment that never set
// TOTP_ENCRYPTION_KEY, so the secret encryption key is auto-generated.
func newTestBackupServiceEphemeralKey(repo *mockSettingRepo) *BackupService {
	cfg := &config.Config{
		Database: config.DatabaseConfig{Host: "localhost", Port: 5432, User: "test", DBName: "testdb"},
		Totp:     config.TotpConfig{EncryptionKeyConfigured: false},
	}
	factory := func(_ context.Context, _ *BackupS3Config) (BackupObjectStore, error) {
		return newMockObjectStore(), nil
	}
	return NewBackupService(repo, cfg, &plainEncryptor{}, factory, &mockDumper{})
}

func seedS3Config(t *testing.T, repo *mockSettingRepo) {
	t.Helper()
	cfg := BackupS3Config{
		Bucket:          "test-bucket",
		AccessKeyID:     "AKID",
		SecretAccessKey: "ENC:secret123",
		Prefix:          "backups",
	}
	data, _ := json.Marshal(cfg)
	require.NoError(t, repo.Set(context.Background(), settingKeyBackupS3Config, string(data)))
}

// ─── Tests ───

func TestBackupService_S3ConfigEncryption(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	// 保存配置 -> SecretAccessKey 应被加密
	_, err := svc.UpdateS3Config(context.Background(), BackupS3Config{
		Bucket:          "my-bucket",
		AccessKeyID:     "AKID",
		SecretAccessKey: "my-secret",
		Prefix:          "backups",
	})
	require.NoError(t, err)

	// 直接读取数据库中存储的值，应该是加密后的
	raw, _ := repo.GetValue(context.Background(), settingKeyBackupS3Config)
	var stored BackupS3Config
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	require.Equal(t, "ENC:my-secret", stored.SecretAccessKey)

	// 通过 GetS3Config 获取应该脱敏
	cfg, err := svc.GetS3Config(context.Background())
	require.NoError(t, err)
	require.Empty(t, cfg.SecretAccessKey)
	require.Equal(t, "my-bucket", cfg.Bucket)

	// loadS3Config 内部应解密
	internal, err := svc.loadS3Config(context.Background())
	require.NoError(t, err)
	require.Equal(t, "my-secret", internal.SecretAccessKey)
}

func TestBackupService_S3ConfigKeepExistingSecret(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	// 先保存一个有 secret 的配置
	_, err := svc.UpdateS3Config(context.Background(), BackupS3Config{
		Bucket:          "my-bucket",
		AccessKeyID:     "AKID",
		SecretAccessKey: "original-secret",
	})
	require.NoError(t, err)

	// 再更新时不提供 secret，应保留原值
	_, err = svc.UpdateS3Config(context.Background(), BackupS3Config{
		Bucket:      "my-bucket",
		AccessKeyID: "AKID-NEW",
	})
	require.NoError(t, err)

	internal, err := svc.loadS3Config(context.Background())
	require.NoError(t, err)
	require.Equal(t, "original-secret", internal.SecretAccessKey)
	require.Equal(t, "AKID-NEW", internal.AccessKeyID)
}

func TestBackupService_UpdateS3Config_RejectsEphemeralKey(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupServiceEphemeralKey(repo)

	// 提供新 secret 但密钥为自动生成 -> 必须拒绝，避免重启后无法解密（#4524）。
	_, err := svc.UpdateS3Config(context.Background(), BackupS3Config{
		Bucket:          "my-bucket",
		AccessKeyID:     "AKID",
		SecretAccessKey: "my-secret",
		Prefix:          "backups",
	})
	require.ErrorIs(t, err, ErrSecretEncryptionKeyNotConfigured)

	// 不应写入任何配置。
	raw, _ := repo.GetValue(context.Background(), settingKeyBackupS3Config)
	require.Empty(t, raw)
}

func TestBackupService_UpdateS3Config_NoSecretAllowedWithEphemeralKey(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupServiceEphemeralKey(repo)

	// 不含 secret 的更新（如只改 bucket）不触碰加密路径，应放行。
	_, err := svc.UpdateS3Config(context.Background(), BackupS3Config{
		Bucket:      "my-bucket",
		AccessKeyID: "AKID",
	})
	require.NoError(t, err)
}

func TestBackupService_EncryptionKeyConfigured(t *testing.T) {
	repo := newMockSettingRepo()
	require.True(t, newTestBackupService(repo, &mockDumper{}, newMockObjectStore()).EncryptionKeyConfigured())
	require.False(t, newTestBackupServiceEphemeralKey(repo).EncryptionKeyConfigured())
}

func TestBackupService_SaveRecordConcurrency(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	var wg sync.WaitGroup
	n := 20
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			record := &BackupRecord{
				ID:        fmt.Sprintf("rec-%d", idx),
				Status:    "completed",
				StartedAt: time.Now().Format(time.RFC3339),
			}
			_ = svc.saveRecord(context.Background(), record)
		}(i)
	}
	wg.Wait()

	records, err := svc.loadRecords(context.Background())
	require.NoError(t, err)
	require.Len(t, records, n)
}

func TestBackupService_LoadRecords_Empty(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	records, err := svc.loadRecords(context.Background())
	require.NoError(t, err)
	require.Nil(t, records) // 无数据时返回 nil
}

func TestBackupService_LoadRecords_Corrupted(t *testing.T) {
	repo := newMockSettingRepo()
	_ = repo.Set(context.Background(), settingKeyBackupRecords, "not valid json{{{")
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	records, err := svc.loadRecords(context.Background())
	require.Error(t, err) // 损坏数据应返回错误
	require.Nil(t, records)
}

func TestBackupService_CreateBackup_Streaming(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumpContent := "-- PostgreSQL dump\nCREATE TABLE test (id int);\n"
	dumper := &mockDumper{dumpData: []byte(dumpContent)}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	record, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.NoError(t, err)
	require.Equal(t, "completed", record.Status)
	require.Greater(t, record.SizeBytes, int64(0))
	require.NotEmpty(t, record.S3Key)

	// 验证 S3 上确实有文件
	store.mu.Lock()
	require.Len(t, store.objects, 1)
	store.mu.Unlock()
}

func TestBackupService_CreateBackup_SplitsCompressedArchive(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	dumpContent := entropyBackupFixture(512)
	dumper := &mockDumper{dumpData: dumpContent}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)
	svc.partSizeBytes = 32

	record, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.NoError(t, err)
	require.Equal(t, "completed", record.Status)
	require.Greater(t, len(record.Parts), 1)
	require.Empty(t, record.S3Key)

	var compressed bytes.Buffer
	store.mu.Lock()
	for _, part := range record.Parts {
		data, ok := store.objects[part.S3Key]
		require.True(t, ok)
		require.LessOrEqual(t, len(data), 32)
		compressed.Write(data)
	}
	store.mu.Unlock()

	gzReader, err := gzip.NewReader(bytes.NewReader(compressed.Bytes()))
	require.NoError(t, err)
	decompressed, err := io.ReadAll(gzReader)
	require.NoError(t, err)
	require.NoError(t, gzReader.Close())
	require.Equal(t, dumpContent, decompressed)
}

func TestBackupService_StartBackup_SplitsCompressedArchive(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{dumpData: entropyBackupFixture(512)}, store)
	svc.partSizeBytes = 32

	record, err := svc.StartBackup(context.Background(), "manual", 14)
	require.NoError(t, err)
	svc.wg.Wait()

	final, err := svc.GetBackupRecord(context.Background(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", final.Status)
	require.Greater(t, len(final.Parts), 1)
	require.Empty(t, final.S3Key)
}

func TestBackupService_StartBackup_UploadFailureCleansUploadedParts(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	store.failUploadFileAt = 2
	svc := newTestBackupService(repo, &mockDumper{dumpData: entropyBackupFixture(512)}, store)
	svc.partSizeBytes = 32

	record, err := svc.StartBackup(context.Background(), "manual", 14)
	require.NoError(t, err)
	svc.wg.Wait()

	final, err := svc.GetBackupRecord(context.Background(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", final.Status)
	require.NotEmpty(t, final.Parts)
	store.mu.Lock()
	deletedKeys := append([]string(nil), store.deletedKeys...)
	store.mu.Unlock()
	for _, part := range final.Parts {
		require.Contains(t, deletedKeys, part.S3Key)
	}
}

func TestBackupService_UploadFailureCleanupUsesDetachedContext(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	svc.partSizeBytes = 4

	archive, err := os.CreateTemp("", "backup-upload-context-*.gz")
	require.NoError(t, err)
	archivePath := archive.Name()
	defer func() { _ = os.Remove(archivePath) }()
	_, err = archive.Write([]byte("0123456789"))
	require.NoError(t, err)
	require.NoError(t, archive.Close())

	ctx, cancel := context.WithCancel(context.Background())
	store := &cancelingUploadFailureStore{
		mockObjectStore: newMockObjectStore(),
		cancel:          cancel,
	}
	record := &BackupRecord{ID: "cancel-cleanup", S3Key: "backups/cancel-cleanup.sql.gz"}

	err = svc.uploadBackupArchive(ctx, record, store, &BackupS3Config{Prefix: "backups"}, archivePath)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "context canceled")

	store.mu.Lock()
	defer store.mu.Unlock()
	for _, part := range record.Parts {
		require.Contains(t, store.deletedKeys, part.S3Key)
		require.NotContains(t, store.objects, part.S3Key)
	}
}

func entropyBackupFixture(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i*31 + 17) % 251)
	}
	return data
}

func TestBackupService_CreateBackup_DumpFailure(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumper := &mockDumper{dumpErr: fmt.Errorf("pg_dump failed")}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	record, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.Error(t, err)
	require.Equal(t, "failed", record.Status)
	require.Contains(t, record.ErrorMsg, "pg_dump")
}

func TestBackupService_CreateBackup_NoS3Config(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	_, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.ErrorIs(t, err, ErrBackupS3NotConfigured)
}

func TestBackupService_CreateBackup_ConcurrentBlocked(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	// 使用一个慢速 dumper 来模拟正在进行的备份
	dumper := &mockDumper{dumpData: []byte("data")}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	// 手动设置 backingUp 标志
	svc.opMu.Lock()
	svc.backingUp = true
	svc.opMu.Unlock()

	_, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.ErrorIs(t, err, ErrBackupInProgress)
}

// TestBackupService_RunScheduledBackup_LeaderElection verifies the scheduled
// backup is gated by a cross-instance leader lock: a non-leader instance skips
// the dump entirely so a clustered deployment does not run N identical backups
// against the same database, while the leader runs it and releases the lock
// afterward. Manual backups (CreateBackup/StartBackup) are intentionally left
// ungated and are covered by the other tests.
func TestBackupService_RunScheduledBackup_LeaderElection(t *testing.T) {
	t.Run("non-leader skips", func(t *testing.T) {
		repo := newMockSettingRepo()
		seedS3Config(t, repo)
		store := newMockObjectStore()
		svc := newTestBackupService(repo, &mockDumper{dumpData: []byte("data")}, store)

		// A peer already owns the lock, so this instance is not the leader.
		cache := &fakeLeaderLockCache{}
		peerRelease, ok := tryAcquireSingletonLeaderLock(context.Background(), cache, nil, backupScheduledLeaderLockKey, "peer", time.Minute)
		require.True(t, ok)
		defer peerRelease()

		svc.SetLeaderLock(cache, nil)
		svc.runScheduledBackup()

		store.mu.Lock()
		require.Empty(t, store.objects, "non-leader must not upload a backup")
		store.mu.Unlock()

		records, err := svc.ListBackups(context.Background())
		require.NoError(t, err)
		require.Empty(t, records, "non-leader must not create a backup record")
		require.Equal(t, "peer", cache.heldBy(backupScheduledLeaderLockKey), "peer keeps the lock")
	})

	t.Run("leader runs and releases", func(t *testing.T) {
		repo := newMockSettingRepo()
		seedS3Config(t, repo)
		store := newMockObjectStore()
		svc := newTestBackupService(repo, &mockDumper{dumpData: []byte("-- dump\n")}, store)

		cache := &fakeLeaderLockCache{}
		svc.SetLeaderLock(cache, nil)
		svc.runScheduledBackup()

		records, err := svc.ListBackups(context.Background())
		require.NoError(t, err)
		require.Len(t, records, 1, "leader creates exactly one backup record")
		require.Equal(t, "completed", records[0].Status)
		require.Empty(t, cache.heldBy(backupScheduledLeaderLockKey), "leader releases the lock when done")
	})
}

func TestBackupService_RestoreBackup_Streaming(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumpContent := "-- PostgreSQL dump\nCREATE TABLE test (id int);\n"
	dumper := &mockDumper{dumpData: []byte(dumpContent)}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	// 先创建一个备份
	record, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.NoError(t, err)

	// 恢复
	err = svc.RestoreBackup(context.Background(), record.ID)
	require.NoError(t, err)

	// 验证 psql 收到的数据是否与原始 dump 内容一致
	require.Equal(t, dumpContent, string(dumper.restored))
}

func TestBackupService_RestoreBackup_SplitParts(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	dumpContent := entropyBackupFixture(512)
	dumper := &mockDumper{}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	compressed := gzipBackupBytes(t, dumpContent)
	parts := splitBackupBytes(compressed, 11)
	recordParts := make([]BackupPart, 0, len(parts))
	for i, data := range parts {
		key := fmt.Sprintf("backups/split-1/payload.part-%06d", i+1)
		store.objects[key] = data
		recordParts = append(recordParts, BackupPart{
			Index:     i + 1,
			S3Key:     key,
			SizeBytes: int64(len(data)),
			SHA256:    fmt.Sprintf("%x", sha256.Sum256(data)),
		})
	}
	record := &BackupRecord{
		ID:        "split-1",
		Status:    "completed",
		Parts:     recordParts,
		SizeBytes: int64(len(compressed)),
	}
	require.NoError(t, svc.saveRecord(context.Background(), record))

	require.NoError(t, svc.RestoreBackup(context.Background(), record.ID))
	require.Equal(t, dumpContent, dumper.restored)
}

func TestBackupService_RestoreBackup_SplitPartsMissingPartDoesNotRestore(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	dumpContent := entropyBackupFixture(256)
	dumper := &mockDumper{}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	compressed := gzipBackupBytes(t, dumpContent)
	parts := splitBackupBytes(compressed, 11)
	recordParts := make([]BackupPart, 0, len(parts))
	for i, data := range parts {
		key := fmt.Sprintf("backups/split-missing/payload.part-%06d", i+1)
		store.objects[key] = data
		recordParts = append(recordParts, BackupPart{
			Index:     i + 1,
			S3Key:     key,
			SizeBytes: int64(len(data)),
			SHA256:    fmt.Sprintf("%x", sha256.Sum256(data)),
		})
	}
	delete(store.objects, recordParts[1].S3Key)
	record := &BackupRecord{ID: "split-missing", Status: "completed", Parts: recordParts}
	require.NoError(t, svc.saveRecord(context.Background(), record))

	require.Error(t, svc.RestoreBackup(context.Background(), record.ID))
	require.Empty(t, dumper.restored)
}

func TestBackupService_DownloadBackupPartsRejectsMismatchedMetadata(t *testing.T) {
	tests := []struct {
		name string
		part BackupPart
		want string
	}{
		{
			name: "size",
			part: BackupPart{Index: 1, S3Key: "backups/mismatch/size", SizeBytes: 4},
			want: "size mismatch",
		},
		{
			name: "checksum",
			part: BackupPart{Index: 1, S3Key: "backups/mismatch/checksum", SizeBytes: 3, SHA256: "bad-checksum"},
			want: "checksum mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockSettingRepo()
			seedS3Config(t, repo)
			store := newMockObjectStore()
			store.objects[tt.part.S3Key] = []byte("abc")
			svc := newTestBackupService(repo, &mockDumper{}, store)

			_, err := svc.downloadBackupParts(context.Background(), store, []BackupPart{tt.part})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func gzipBackupBytes(t *testing.T, content []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	_, err := writer.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return out.Bytes()
}

func splitBackupBytes(data []byte, partSize int) [][]byte {
	parts := make([][]byte, 0, (len(data)+partSize-1)/partSize)
	for len(data) > 0 {
		size := partSize
		if len(data) < size {
			size = len(data)
		}
		parts = append(parts, append([]byte(nil), data[:size]...))
		data = data[size:]
	}
	return parts
}

func TestBackupService_RestoreBackup_NotCompleted(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	// 手动插入一条 failed 记录
	_ = svc.saveRecord(context.Background(), &BackupRecord{
		ID:     "fail-1",
		Status: "failed",
	})

	err := svc.RestoreBackup(context.Background(), "fail-1")
	require.Error(t, err)
}

func TestBackupService_DeleteBackup(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumpContent := "data"
	dumper := &mockDumper{dumpData: []byte(dumpContent)}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	record, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.NoError(t, err)

	// S3 中应有文件
	store.mu.Lock()
	require.Len(t, store.objects, 1)
	store.mu.Unlock()

	// 删除
	err = svc.DeleteBackup(context.Background(), record.ID)
	require.NoError(t, err)

	// S3 中文件应被删除
	store.mu.Lock()
	require.Len(t, store.objects, 0)
	store.mu.Unlock()

	// 记录应不存在
	_, err = svc.GetBackupRecord(context.Background(), record.ID)
	require.ErrorIs(t, err, ErrBackupNotFound)
}

func TestBackupService_DeleteBackup_RunningKeepsUploadObjects(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	parts := []BackupPart{
		{Index: 1, S3Key: "backups/running/payload.part-000001", SizeBytes: 3},
		{Index: 2, S3Key: "backups/running/payload.part-000002", SizeBytes: 3},
	}
	for _, part := range parts {
		store.objects[part.S3Key] = []byte("abc")
	}
	record := &BackupRecord{ID: "running-delete", Status: "running", Parts: parts}
	require.NoError(t, svc.saveRecord(context.Background(), record))

	err := svc.DeleteBackup(context.Background(), record.ID)
	require.ErrorIs(t, err, ErrBackupInProgress)
	store.mu.Lock()
	require.Empty(t, store.deletedKeys)
	for _, part := range parts {
		require.Contains(t, store.objects, part.S3Key)
	}
	store.mu.Unlock()
	got, getErr := svc.GetBackupRecord(context.Background(), record.ID)
	require.NoError(t, getErr)
	require.Equal(t, "running", got.Status)
}

func TestBackupService_GetDownloadURL(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumper := &mockDumper{dumpData: []byte("data")}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	record, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.NoError(t, err)

	download, err := svc.GetBackupDownloadURL(context.Background(), record.ID)
	require.NoError(t, err)
	require.Contains(t, download.URL, "https://presigned.example.com/")
}

func TestBackupService_DeleteBackup_SplitPartsFailureKeepsRecord(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)

	parts := []BackupPart{
		{Index: 1, S3Key: "backups/split/payload.part-000001", SizeBytes: 3},
		{Index: 2, S3Key: "backups/split/payload.part-000002", SizeBytes: 3},
		{Index: 3, S3Key: "backups/split/payload.part-000003", SizeBytes: 3},
	}
	for _, part := range parts {
		store.objects[part.S3Key] = []byte("abc")
	}
	store.failDeleteKeys[parts[1].S3Key] = fmt.Errorf("delete failed")
	record := &BackupRecord{ID: "split-delete", Status: "completed", Parts: parts}
	require.NoError(t, svc.saveRecord(context.Background(), record))

	err := svc.DeleteBackup(context.Background(), record.ID)
	require.Error(t, err)

	store.mu.Lock()
	deleted := append([]string(nil), store.deletedKeys...)
	store.mu.Unlock()
	for _, part := range parts {
		require.Contains(t, deleted, part.S3Key)
	}
	got, getErr := svc.GetBackupRecord(context.Background(), record.ID)
	require.NoError(t, getErr)
	require.Equal(t, record.ID, got.ID)
	store.mu.Lock()
	require.Contains(t, store.objects, parts[1].S3Key)
	store.mu.Unlock()
}

func TestBackupService_GetDownloadURL_SplitParts(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)

	parts := []BackupPart{
		{Index: 2, S3Key: "backups/split/payload.part-000002", SizeBytes: 7},
		{Index: 1, S3Key: "backups/split/payload.part-000001", SizeBytes: 5},
	}
	record := &BackupRecord{ID: "split-download", Status: "completed", Parts: parts}
	require.NoError(t, svc.saveRecord(context.Background(), record))

	download, err := svc.GetBackupDownloadURL(context.Background(), record.ID)
	require.NoError(t, err)
	require.Empty(t, download.URL)
	require.Len(t, download.Parts, 2)
	require.Equal(t, 1, download.Parts[0].Index)
	require.Equal(t, int64(5), download.Parts[0].SizeBytes)
	require.Equal(t, "https://presigned.example.com/backups/split/payload.part-000001", download.Parts[0].URL)
	require.Equal(t, 2, download.Parts[1].Index)
}

func TestBackupService_CleanupOldBackups_SplitParts(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	now := time.Now()
	parts := []BackupPart{
		{Index: 1, S3Key: "backups/old/payload.part-000001", SizeBytes: 3},
		{Index: 2, S3Key: "backups/old/payload.part-000002", SizeBytes: 3},
	}
	for _, part := range parts {
		store.objects[part.S3Key] = []byte("abc")
	}
	require.NoError(t, svc.saveRecord(context.Background(), &BackupRecord{
		ID:        "new",
		Status:    "completed",
		StartedAt: now.Format(time.RFC3339),
	}))
	require.NoError(t, svc.saveRecord(context.Background(), &BackupRecord{
		ID:        "old",
		Status:    "completed",
		StartedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		Parts:     parts,
	}))

	err := svc.cleanupOldBackups(context.Background(), &BackupScheduleConfig{RetainCount: 1})
	require.NoError(t, err)
	_, err = svc.GetBackupRecord(context.Background(), "old")
	require.ErrorIs(t, err, ErrBackupNotFound)
	store.mu.Lock()
	for _, part := range parts {
		require.NotContains(t, store.objects, part.S3Key)
	}
	store.mu.Unlock()
}

func TestBackupService_ListBackups_Sorted(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	now := time.Now()
	for i := 0; i < 3; i++ {
		_ = svc.saveRecord(context.Background(), &BackupRecord{
			ID:        fmt.Sprintf("rec-%d", i),
			Status:    "completed",
			StartedAt: now.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
		})
	}

	records, err := svc.ListBackups(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 3)
	// 最新在前
	require.Equal(t, "rec-2", records[0].ID)
	require.Equal(t, "rec-0", records[2].ID)
}

func TestBackupService_TestS3Connection(t *testing.T) {
	repo := newMockSettingRepo()
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)

	err := svc.TestS3Connection(context.Background(), BackupS3Config{
		Bucket:          "test",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
	})
	require.NoError(t, err)
}

func TestBackupService_TestS3Connection_Incomplete(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	err := svc.TestS3Connection(context.Background(), BackupS3Config{
		Bucket: "test",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "incomplete")
}

func TestBackupService_Schedule_CronValidation(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())
	svc.cronSched = nil // 未初始化 cron

	// 启用但 cron 为空
	_, err := svc.UpdateSchedule(context.Background(), BackupScheduleConfig{
		Enabled:  true,
		CronExpr: "",
	})
	require.Error(t, err)

	// 无效的 cron 表达式
	_, err = svc.UpdateSchedule(context.Background(), BackupScheduleConfig{
		Enabled:  true,
		CronExpr: "invalid",
	})
	require.Error(t, err)
}

func TestBackupService_LoadS3Config_Corrupted(t *testing.T) {
	repo := newMockSettingRepo()
	_ = repo.Set(context.Background(), settingKeyBackupS3Config, "not json!!!!")
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	cfg, err := svc.loadS3Config(context.Background())
	require.Error(t, err)
	require.Nil(t, cfg)
}

// ─── Async Backup Tests ───

func TestStartBackup_ReturnsImmediately(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumper := &blockingDumper{blockCh: make(chan struct{}), data: []byte("data")}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	record, err := svc.StartBackup(context.Background(), "manual", 14)
	require.NoError(t, err)
	require.Equal(t, "running", record.Status)
	require.NotEmpty(t, record.ID)

	// 释放 dumper 让后台完成
	close(dumper.blockCh)
	svc.wg.Wait()

	// 验证最终状态
	final, err := svc.GetBackupRecord(context.Background(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", final.Status)
	require.Greater(t, final.SizeBytes, int64(0))
}

func TestStartBackup_ConcurrentBlocked(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumper := &blockingDumper{blockCh: make(chan struct{}), data: []byte("data")}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	// 第一次启动
	_, err := svc.StartBackup(context.Background(), "manual", 14)
	require.NoError(t, err)

	// 第二次应被阻塞
	_, err = svc.StartBackup(context.Background(), "manual", 14)
	require.ErrorIs(t, err, ErrBackupInProgress)

	close(dumper.blockCh)
	svc.wg.Wait()
}

func TestStartBackup_ShuttingDown(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	svc := newTestBackupService(repo, &mockDumper{dumpData: []byte("data")}, newMockObjectStore())

	svc.shuttingDown.Store(true)

	_, err := svc.StartBackup(context.Background(), "manual", 14)
	require.Error(t, err)
	require.Contains(t, err.Error(), "shutting down")
}

func TestRecoverStaleRecords(t *testing.T) {
	repo := newMockSettingRepo()
	svc := newTestBackupService(repo, &mockDumper{}, newMockObjectStore())

	// 模拟一条孤立的 running 记录
	_ = svc.saveRecord(context.Background(), &BackupRecord{
		ID:        "stale-1",
		Status:    "running",
		StartedAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	})
	// 模拟一条孤立的恢复中记录
	_ = svc.saveRecord(context.Background(), &BackupRecord{
		ID:            "stale-2",
		Status:        "completed",
		RestoreStatus: "running",
		StartedAt:     time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	})

	svc.recoverStaleRecords()

	r1, _ := svc.GetBackupRecord(context.Background(), "stale-1")
	require.Equal(t, "failed", r1.Status)
	require.Contains(t, r1.ErrorMsg, "server restart")

	r2, _ := svc.GetBackupRecord(context.Background(), "stale-2")
	require.Equal(t, "failed", r2.RestoreStatus)
	require.Contains(t, r2.RestoreError, "server restart")
}

func TestBackupService_RecoverStaleRecords_CleansBackupObjects(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	parts := []BackupPart{
		{Index: 1, S3Key: "backups/stale/payload.part-000001", SizeBytes: 3},
		{Index: 2, S3Key: "backups/stale/payload.part-000002", SizeBytes: 3},
	}
	for _, part := range parts {
		store.objects[part.S3Key] = []byte("abc")
	}
	require.NoError(t, svc.saveRecord(context.Background(), &BackupRecord{
		ID:        "stale-parts",
		Status:    "running",
		Parts:     parts,
		StartedAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
	}))

	svc.recoverStaleRecords()

	record, err := svc.GetBackupRecord(context.Background(), "stale-parts")
	require.NoError(t, err)
	require.Equal(t, "failed", record.Status)
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, part := range parts {
		require.Contains(t, store.deletedKeys, part.S3Key)
		require.NotContains(t, store.objects, part.S3Key)
	}
}

func TestBackupService_RecoverStaleRecords_PreservesKeysWhenCleanupFails(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	store := newMockObjectStore()
	svc := newTestBackupService(repo, &mockDumper{}, store)
	part := BackupPart{Index: 1, S3Key: "backups/stale-failed/payload.part-000001", SizeBytes: 3}
	store.objects[part.S3Key] = []byte("abc")
	store.failDeleteKeys[part.S3Key] = fmt.Errorf("delete failed")
	require.NoError(t, svc.saveRecord(context.Background(), &BackupRecord{
		ID:        "stale-cleanup-failed",
		Status:    "running",
		Parts:     []BackupPart{part},
		StartedAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
	}))

	svc.recoverStaleRecords()

	record, err := svc.GetBackupRecord(context.Background(), "stale-cleanup-failed")
	require.NoError(t, err)
	require.Equal(t, "failed", record.Status)
	require.Contains(t, record.ErrorMsg, "cleanup failed")
	require.Equal(t, part.S3Key, record.Parts[0].S3Key)
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Contains(t, store.objects, part.S3Key)
}

func TestGracefulShutdown(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumper := &blockingDumper{blockCh: make(chan struct{}), data: []byte("data")}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	_, err := svc.StartBackup(context.Background(), "manual", 14)
	require.NoError(t, err)

	// Stop 应该等待备份完成
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	// 短暂等待确认 Stop 还在等待
	select {
	case <-done:
		t.Fatal("Stop returned before backup finished")
	case <-time.After(100 * time.Millisecond):
		// 预期：Stop 还在等待
	}

	// 释放备份
	close(dumper.blockCh)

	// 现在 Stop 应该完成
	select {
	case <-done:
		// 预期
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after backup finished")
	}
}

func TestStartRestore_Async(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)

	dumpContent := "-- PostgreSQL dump\nCREATE TABLE test (id int);\n"
	dumper := &mockDumper{dumpData: []byte(dumpContent)}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	// 先创建一个备份（同步方式）
	record, err := svc.CreateBackup(context.Background(), "manual", 14)
	require.NoError(t, err)

	// 异步恢复
	restored, err := svc.StartRestore(context.Background(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "running", restored.RestoreStatus)

	svc.wg.Wait()

	// 验证最终状态
	final, err := svc.GetBackupRecord(context.Background(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", final.RestoreStatus)
}

func TestBackupService_StartRestore_SplitParts(t *testing.T) {
	repo := newMockSettingRepo()
	seedS3Config(t, repo)
	dumpContent := entropyBackupFixture(384)
	dumper := &mockDumper{}
	store := newMockObjectStore()
	svc := newTestBackupService(repo, dumper, store)

	compressed := gzipBackupBytes(t, dumpContent)
	parts := splitBackupBytes(compressed, 13)
	recordParts := make([]BackupPart, 0, len(parts))
	for i, data := range parts {
		key := fmt.Sprintf("backups/split-async/payload.part-%06d", i+1)
		store.objects[key] = data
		recordParts = append(recordParts, BackupPart{
			Index:     i + 1,
			S3Key:     key,
			SizeBytes: int64(len(data)),
			SHA256:    fmt.Sprintf("%x", sha256.Sum256(data)),
		})
	}
	record := &BackupRecord{ID: "split-async", Status: "completed", Parts: recordParts}
	require.NoError(t, svc.saveRecord(context.Background(), record))

	started, err := svc.StartRestore(context.Background(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "running", started.RestoreStatus)
	svc.wg.Wait()

	final, err := svc.GetBackupRecord(context.Background(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", final.RestoreStatus)
	require.Equal(t, dumpContent, dumper.restored)
}
