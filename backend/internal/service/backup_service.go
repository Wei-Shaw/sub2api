package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	settingKeyBackupS3Config = "backup_s3_config"
	settingKeyBackupSchedule = "backup_schedule"
	settingKeyBackupRecords  = "backup_records"

	maxBackupRecords = 100
)

var (
	ErrBackupS3NotConfigured = infraerrors.BadRequest("BACKUP_S3_NOT_CONFIGURED", "backup S3 storage is not configured")
	ErrBackupNotFound        = infraerrors.NotFound("BACKUP_NOT_FOUND", "backup record not found")
	ErrBackupInProgress      = infraerrors.Conflict("BACKUP_IN_PROGRESS", "a backup is already in progress")
	ErrRestoreInProgress     = infraerrors.Conflict("RESTORE_IN_PROGRESS", "a restore is already in progress")
)

// BackupS3Config S3 兼容存储配置（支持 Cloudflare R2）
type BackupS3Config struct {
	Endpoint        string `json:"endpoint"` // e.g. https://<account_id>.r2.cloudflarestorage.com
	Region          string `json:"region"`   // R2 用 "auto"
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key,omitempty"` //nolint:revive // field name follows AWS convention
	Prefix          string `json:"prefix"`                      // S3 key 前缀，如 "backups/"
	ForcePathStyle  bool   `json:"force_path_style"`
REDACTED

// IsConfigured 检查必要字段是否已配置
func (c *BackupS3Config) IsConfigured() bool {
	return c.Bucket != "" && c.AccessKeyID != "" && c.SecretAccessKey != ""
REDACTED

// BackupScheduleConfig 定时备份配置
type BackupScheduleConfig struct {
	Enabled     bool   `json:"enabled"`
	CronExpr    string `json:"cron_expr"`    // cron 表达式，如 "0 2 * * *" 每天凌晨2点
	RetainDays  int    `json:"retain_days"`  // 备份文件过期天数，默认14，0=不自动清理
	RetainCount int    `json:"retain_count"` // 最多保留份数，0=不限制
REDACTED

// BackupRecord 备份记录
type BackupRecord struct {
	ID          string `json:"id"`
	Status      string `json:"status"`      // pending, running, completed, failed
	BackupType  string `json:"backup_type"` // postgres
	FileName    string `json:"file_name"`
	S3Key       string `json:"s3_key"`
	SizeBytes   int64  `json:"size_bytes"`
	TriggeredBy string `json:"triggered_by"` // manual, scheduled
	ErrorMsg    string `json:"error_message,omitempty"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"` // 过期时间
REDACTED

// BackupService 数据库备份恢复服务
type BackupService struct {
	settingRepo SettingRepository
	dbCfg       *config.DatabaseConfig

	mu        sync.Mutex
	s3Client  *s3.Client
	s3Cfg     *BackupS3Config
	backingUp bool
	restoring bool

	cronMu      sync.Mutex
	cronSched   *cron.Cron
	cronEntryID cron.EntryID
REDACTED

func NewBackupService(settingRepo SettingRepository, cfg *config.Config) *BackupService {
	svc := &BackupService{
		settingRepo: settingRepo,
		dbCfg:       &cfg.Database,
REDACTED
	return svc
REDACTED

// Start 启动定时备份调度器
func (s *BackupService) Start() {
	s.cronSched = cron.New()
	s.cronSched.Start()

	// 加载已有的定时配置
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schedule, err := s.GetSchedule(ctx)
	if err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 加载定时备份配置失败: %v", err)
		return
REDACTED
	if schedule.Enabled && schedule.CronExpr != "" {
		if err := s.applyCronSchedule(schedule); err != nil {
			logger.LegacyPrintf("service.backup", "[Backup] 应用定时备份配置失败: %v", err)
	REDACTED
REDACTED
REDACTED

// Stop 停止定时备份
func (s *BackupService) Stop() {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()
	if s.cronSched != nil {
		s.cronSched.Stop()
REDACTED
REDACTED

// ─── S3 配置管理 ───

func (s *BackupService) GetS3Config(ctx context.Context) (*BackupS3Config, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyBackupS3Config)
	if err != nil || raw == "" {
		return &BackupS3Config{REDACTED, nil
REDACTED
	var cfg BackupS3Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return &BackupS3Config{REDACTED, nil
REDACTED
	// 脱敏返回
	cfg.SecretAccessKey = ""
	return &cfg, nil
REDACTED

func (s *BackupService) UpdateS3Config(ctx context.Context, cfg BackupS3Config) (*BackupS3Config, error) {
	// 如果没提供 secret，保留原有值
	if cfg.SecretAccessKey == "" {
		old, _ := s.loadS3Config(ctx)
		if old != nil {
			cfg.SecretAccessKey = old.SecretAccessKey
	REDACTED
REDACTED

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal s3 config: %w", err)
REDACTED
	if err := s.settingRepo.Set(ctx, settingKeyBackupS3Config, string(data)); err != nil {
		return nil, fmt.Errorf("save s3 config: %w", err)
REDACTED

	// 清除缓存的 S3 客户端
	s.mu.Lock()
	s.s3Client = nil
	s.s3Cfg = nil
	s.mu.Unlock()

	cfg.SecretAccessKey = ""
	return &cfg, nil
REDACTED

func (s *BackupService) TestS3Connection(ctx context.Context, cfg BackupS3Config) error {
	// 如果没提供 secret，用已保存的
	if cfg.SecretAccessKey == "" {
		old, _ := s.loadS3Config(ctx)
		if old != nil {
			cfg.SecretAccessKey = old.SecretAccessKey
	REDACTED
REDACTED

	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return fmt.Errorf("incomplete S3 config: bucket, access_key_id, secret_access_key are required")
REDACTED

	client, err := s.buildS3Client(ctx, &cfg)
	if err != nil {
		return err
REDACTED
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &cfg.Bucket,
REDACTED)
	if err != nil {
		return fmt.Errorf("S3 HeadBucket failed: %w", err)
REDACTED
	return nil
REDACTED

// ─── 定时备份管理 ───

func (s *BackupService) GetSchedule(ctx context.Context) (*BackupScheduleConfig, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyBackupSchedule)
	if err != nil || raw == "" {
		return &BackupScheduleConfig{REDACTED, nil
REDACTED
	var cfg BackupScheduleConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return &BackupScheduleConfig{REDACTED, nil
REDACTED
	return &cfg, nil
REDACTED

func (s *BackupService) UpdateSchedule(ctx context.Context, cfg BackupScheduleConfig) (*BackupScheduleConfig, error) {
	if cfg.Enabled && cfg.CronExpr == "" {
		return nil, infraerrors.BadRequest("INVALID_CRON", "cron expression is required when schedule is enabled")
REDACTED
	// 验证 cron 表达式
	if cfg.CronExpr != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(cfg.CronExpr); err != nil {
			return nil, infraerrors.BadRequest("INVALID_CRON", fmt.Sprintf("invalid cron expression: %v", err))
	REDACTED
REDACTED

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal schedule config: %w", err)
REDACTED
	if err := s.settingRepo.Set(ctx, settingKeyBackupSchedule, string(data)); err != nil {
		return nil, fmt.Errorf("save schedule config: %w", err)
REDACTED

	// 应用或停止定时任务
	if cfg.Enabled {
		if err := s.applyCronSchedule(&cfg); err != nil {
			return nil, err
	REDACTED
REDACTED else {
		s.removeCronSchedule()
REDACTED

	return &cfg, nil
REDACTED

func (s *BackupService) applyCronSchedule(cfg *BackupScheduleConfig) error {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	if s.cronSched == nil {
		return fmt.Errorf("cron scheduler not initialized")
REDACTED

	// 移除旧任务
	if s.cronEntryID != 0 {
		s.cronSched.Remove(s.cronEntryID)
		s.cronEntryID = 0
REDACTED

	entryID, err := s.cronSched.AddFunc(cfg.CronExpr, func() {
		s.runScheduledBackup()
REDACTED)
	if err != nil {
		return infraerrors.BadRequest("INVALID_CRON", fmt.Sprintf("failed to schedule: %v", err))
REDACTED
	s.cronEntryID = entryID
	logger.LegacyPrintf("service.backup", "[Backup] 定时备份已启用: %s", cfg.CronExpr)
	return nil
REDACTED

func (s *BackupService) removeCronSchedule() {
	s.cronMu.Lock()
	defer s.cronMu.Unlock()
	if s.cronSched != nil && s.cronEntryID != 0 {
		s.cronSched.Remove(s.cronEntryID)
		s.cronEntryID = 0
		logger.LegacyPrintf("service.backup", "[Backup] 定时备份已停用")
REDACTED
REDACTED

func (s *BackupService) runScheduledBackup() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// 读取定时备份配置中的过期天数
	schedule, _ := s.GetSchedule(ctx)
	expireDays := 14 // 默认14天过期
	if schedule != nil && schedule.RetainDays > 0 {
		expireDays = schedule.RetainDays
REDACTED

	logger.LegacyPrintf("service.backup", "[Backup] 开始执行定时备份, 过期天数: %d", expireDays)
	record, err := s.CreateBackup(ctx, "scheduled", expireDays)
	if err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 定时备份失败: %v", err)
		return
REDACTED
	logger.LegacyPrintf("service.backup", "[Backup] 定时备份完成: id=%s size=%d", record.ID, record.SizeBytes)

	// 清理过期备份（复用已加载的 schedule）
	if schedule == nil {
		return
REDACTED
	if err := s.cleanupOldBackups(ctx, schedule); err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 清理过期备份失败: %v", err)
REDACTED
REDACTED

// ─── 备份/恢复核心 ───

// CreateBackup 创建全量数据库备份并上传到 S3
// expireDays: 备份过期天数，0=永不过期，默认14天
func (s *BackupService) CreateBackup(ctx context.Context, triggeredBy string, expireDays int) (*BackupRecord, error) {
	s.mu.Lock()
	if s.backingUp {
		s.mu.Unlock()
		return nil, ErrBackupInProgress
REDACTED
	s.backingUp = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.backingUp = false
		s.mu.Unlock()
REDACTED()

	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return nil, err
REDACTED
	if s3Cfg == nil || !s3Cfg.IsConfigured() {
		return nil, ErrBackupS3NotConfigured
REDACTED

	client, err := s.getOrCreateS3Client(ctx, s3Cfg)
	if err != nil {
		return nil, fmt.Errorf("init S3 client: %w", err)
REDACTED

	now := time.Now()
	backupID := uuid.New().String()[:8]
	fileName := fmt.Sprintf("%s_%s.sql.gz", s.dbCfg.DBName, now.Format("20060102_150405"))
	s3Key := s.buildS3Key(s3Cfg, fileName)

	var expiresAt string
	if expireDays > 0 {
		expiresAt = now.AddDate(0, 0, expireDays).Format(time.RFC3339)
REDACTED

	record := &BackupRecord{
		ID:          backupID,
		Status:      "running",
		BackupType:  "postgres",
		FileName:    fileName,
		S3Key:       s3Key,
		TriggeredBy: triggeredBy,
		StartedAt:   now.Format(time.RFC3339),
		ExpiresAt:   expiresAt,
REDACTED

	// 执行全量 pg_dump
	dumpData, err := s.pgDump(ctx)
	if err != nil {
		record.Status = "failed"
		record.ErrorMsg = fmt.Sprintf("pg_dump failed: %v", err)
		record.FinishedAt = time.Now().Format(time.RFC3339)
		_ = s.saveRecord(ctx, record)
		return record, fmt.Errorf("pg_dump: %w", err)
REDACTED

	// gzip 压缩
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	if _, err := gzWriter.Write(dumpData); err != nil {
		record.Status = "failed"
		record.ErrorMsg = fmt.Sprintf("gzip failed: %v", err)
		record.FinishedAt = time.Now().Format(time.RFC3339)
		_ = s.saveRecord(ctx, record)
		return record, fmt.Errorf("gzip: %w", err)
REDACTED
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
REDACTED

	record.SizeBytes = int64(compressed.Len())

	// 上传到 S3
	contentType := "application/gzip"
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s3Cfg.Bucket,
		Key:         &s3Key,
		Body:        bytes.NewReader(compressed.Bytes()),
		ContentType: &contentType,
REDACTED)
	if err != nil {
		record.Status = "failed"
		record.ErrorMsg = fmt.Sprintf("S3 upload failed: %v", err)
		record.FinishedAt = time.Now().Format(time.RFC3339)
		_ = s.saveRecord(ctx, record)
		return record, fmt.Errorf("s3 upload: %w", err)
REDACTED

	record.Status = "completed"
	record.FinishedAt = time.Now().Format(time.RFC3339)
	if err := s.saveRecord(ctx, record); err != nil {
		logger.LegacyPrintf("service.backup", "[Backup] 保存备份记录失败: %v", err)
REDACTED

	return record, nil
REDACTED

// RestoreBackup 从 S3 下载备份并恢复到数据库
func (s *BackupService) RestoreBackup(ctx context.Context, backupID string) error {
	s.mu.Lock()
	if s.restoring {
		s.mu.Unlock()
		return ErrRestoreInProgress
REDACTED
	s.restoring = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.restoring = false
		s.mu.Unlock()
REDACTED()

	record, err := s.GetBackupRecord(ctx, backupID)
	if err != nil {
		return err
REDACTED
	if record.Status != "completed" {
		return infraerrors.BadRequest("BACKUP_NOT_COMPLETED", "can only restore from a completed backup")
REDACTED

	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return err
REDACTED
	client, err := s.getOrCreateS3Client(ctx, s3Cfg)
	if err != nil {
		return fmt.Errorf("init S3 client: %w", err)
REDACTED

	// 从 S3 下载
	result, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s3Cfg.Bucket,
		Key:    &record.S3Key,
REDACTED)
	if err != nil {
		return fmt.Errorf("S3 download failed: %w", err)
REDACTED
	defer func() { _ = result.Body.Close() REDACTED()

	// 解压 gzip
	gzReader, err := gzip.NewReader(result.Body)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
REDACTED
	defer func() { _ = gzReader.Close() REDACTED()

	sqlData, err := io.ReadAll(gzReader)
	if err != nil {
		return fmt.Errorf("read backup data: %w", err)
REDACTED

	// 执行 psql 恢复
	if err := s.pgRestore(ctx, sqlData); err != nil {
		return fmt.Errorf("pg restore: %w", err)
REDACTED

	return nil
REDACTED

// ─── 备份记录管理 ───

func (s *BackupService) ListBackups(ctx context.Context) ([]BackupRecord, error) {
	records, err := s.loadRecords(ctx)
	if err != nil {
		return nil, err
REDACTED
	// 倒序返回（最新在前）
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt > records[j].StartedAt
REDACTED)
	return records, nil
REDACTED

func (s *BackupService) GetBackupRecord(ctx context.Context, backupID string) (*BackupRecord, error) {
	records, err := s.loadRecords(ctx)
	if err != nil {
		return nil, err
REDACTED
	for i := range records {
		if records[i].ID == backupID {
			return &records[i], nil
	REDACTED
REDACTED
	return nil, ErrBackupNotFound
REDACTED

func (s *BackupService) DeleteBackup(ctx context.Context, backupID string) error {
	records, err := s.loadRecords(ctx)
	if err != nil {
		return err
REDACTED

	var found *BackupRecord
	var remaining []BackupRecord
	for i := range records {
		if records[i].ID == backupID {
			found = &records[i]
	REDACTED else {
			remaining = append(remaining, records[i])
	REDACTED
REDACTED
	if found == nil {
		return ErrBackupNotFound
REDACTED

	// 从 S3 删除
	if found.S3Key != "" && found.Status == "completed" {
		s3Cfg, err := s.loadS3Config(ctx)
		if err == nil && s3Cfg != nil && s3Cfg.IsConfigured() {
			client, err := s.getOrCreateS3Client(ctx, s3Cfg)
			if err == nil {
				_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: &s3Cfg.Bucket,
					Key:    &found.S3Key,
			REDACTED)
		REDACTED
	REDACTED
REDACTED

	return s.saveRecords(ctx, remaining)
REDACTED

// GetBackupDownloadURL 获取备份文件预签名下载 URL
func (s *BackupService) GetBackupDownloadURL(ctx context.Context, backupID string) (string, error) {
	record, err := s.GetBackupRecord(ctx, backupID)
	if err != nil {
		return "", err
REDACTED
	if record.Status != "completed" {
		return "", infraerrors.BadRequest("BACKUP_NOT_COMPLETED", "backup is not completed")
REDACTED

	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return "", err
REDACTED
	client, err := s.getOrCreateS3Client(ctx, s3Cfg)
	if err != nil {
		return "", err
REDACTED

	presignClient := s3.NewPresignClient(client)
	result, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &s3Cfg.Bucket,
		Key:    &record.S3Key,
REDACTED, s3.WithPresignExpires(1*time.Hour))
	if err != nil {
		return "", fmt.Errorf("presign url: %w", err)
REDACTED
	return result.URL, nil
REDACTED

// ─── 内部方法 ───

func (s *BackupService) loadS3Config(ctx context.Context) (*BackupS3Config, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyBackupS3Config)
	if err != nil || raw == "" {
		return nil, nil
REDACTED
	var cfg BackupS3Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, nil
REDACTED
	return &cfg, nil
REDACTED

func (s *BackupService) buildS3Client(ctx context.Context, cfg *BackupS3Config) (*s3.Client, error) {
	region := cfg.Region
	if region == "" {
		region = "auto" // Cloudflare R2 默认 region
REDACTED

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
REDACTED

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = &cfg.Endpoint
	REDACTED
		if cfg.ForcePathStyle {
			o.UsePathStyle = true
	REDACTED
		o.APIOptions = append(o.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
REDACTED)
	return client, nil
REDACTED

func (s *BackupService) getOrCreateS3Client(ctx context.Context, cfg *BackupS3Config) (*s3.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.s3Client != nil && s.s3Cfg != nil {
		return s.s3Client, nil
REDACTED

	if cfg == nil {
		return nil, ErrBackupS3NotConfigured
REDACTED

	client, err := s.buildS3Client(ctx, cfg)
	if err != nil {
		return nil, err
REDACTED
	s.s3Client = client
	s.s3Cfg = cfg
	return client, nil
REDACTED

func (s *BackupService) buildS3Key(cfg *BackupS3Config, fileName string) string {
	prefix := strings.TrimRight(cfg.Prefix, "/")
	if prefix == "" {
		prefix = "backups"
REDACTED
	return fmt.Sprintf("%s/%s/%s", prefix, time.Now().Format("2006/01/02"), fileName)
REDACTED

func (s *BackupService) pgDump(ctx context.Context) ([]byte, error) {
	args := []string{
		"-h", s.dbCfg.Host,
		"-p", fmt.Sprintf("%d", s.dbCfg.Port),
		"-U", s.dbCfg.User,
		"-d", s.dbCfg.DBName,
		"--no-owner",
		"--no-acl",
		"--clean",
		"--if-exists",
REDACTED

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	if s.dbCfg.Password != "" {
		cmd.Env = append(cmd.Environ(), "PGPASSWORD="+s.dbCfg.Password)
REDACTED
	if s.dbCfg.SSLMode != "" {
		cmd.Env = append(cmd.Environ(), "PGSSLMODE="+s.dbCfg.SSLMode)
REDACTED

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%v: %s", err, stderr.String())
REDACTED
	return stdout.Bytes(), nil
REDACTED

func (s *BackupService) pgRestore(ctx context.Context, sqlData []byte) error {
	args := []string{
		"-h", s.dbCfg.Host,
		"-p", fmt.Sprintf("%d", s.dbCfg.Port),
		"-U", s.dbCfg.User,
		"-d", s.dbCfg.DBName,
		"--single-transaction",
REDACTED

	cmd := exec.CommandContext(ctx, "psql", args...)
	if s.dbCfg.Password != "" {
		cmd.Env = append(cmd.Environ(), "PGPASSWORD="+s.dbCfg.Password)
REDACTED
	if s.dbCfg.SSLMode != "" {
		cmd.Env = append(cmd.Environ(), "PGSSLMODE="+s.dbCfg.SSLMode)
REDACTED

	cmd.Stdin = bytes.NewReader(sqlData)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, stderr.String())
REDACTED
	return nil
REDACTED

func (s *BackupService) loadRecords(ctx context.Context) ([]BackupRecord, error) {
	raw, err := s.settingRepo.GetValue(ctx, settingKeyBackupRecords)
	if err != nil || raw == "" {
		return nil, nil
REDACTED
	var records []BackupRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, nil
REDACTED
	return records, nil
REDACTED

func (s *BackupService) saveRecords(ctx context.Context, records []BackupRecord) error {
	data, err := json.Marshal(records)
	if err != nil {
		return err
REDACTED
	return s.settingRepo.Set(ctx, settingKeyBackupRecords, string(data))
REDACTED

func (s *BackupService) saveRecord(ctx context.Context, record *BackupRecord) error {
	records, _ := s.loadRecords(ctx)

	// 更新已有记录或追加
	found := false
	for i := range records {
		if records[i].ID == record.ID {
			records[i] = *record
			found = true
			break
	REDACTED
REDACTED
	if !found {
		records = append(records, *record)
REDACTED

	// 限制记录数量
	if len(records) > maxBackupRecords {
		records = records[len(records)-maxBackupRecords:]
REDACTED

	return s.saveRecords(ctx, records)
REDACTED

func (s *BackupService) cleanupOldBackups(ctx context.Context, schedule *BackupScheduleConfig) error {
	if schedule == nil {
		return nil
REDACTED

	records, err := s.loadRecords(ctx)
	if err != nil {
		return err
REDACTED

	// 按时间倒序
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt > records[j].StartedAt
REDACTED)

	var toDelete []BackupRecord
	var toKeep []BackupRecord

	for i, r := range records {
		shouldDelete := false

		// 按保留份数清理
		if schedule.RetainCount > 0 && i >= schedule.RetainCount {
			shouldDelete = true
	REDACTED

		// 按保留天数清理
		if schedule.RetainDays > 0 && r.StartedAt != "" {
			startedAt, err := time.Parse(time.RFC3339, r.StartedAt)
			if err == nil && time.Since(startedAt) > time.Duration(schedule.RetainDays)*24*time.Hour {
				shouldDelete = true
		REDACTED
	REDACTED

		if shouldDelete && r.Status == "completed" {
			toDelete = append(toDelete, r)
	REDACTED else {
			toKeep = append(toKeep, r)
	REDACTED
REDACTED

	// 删除 S3 上的文件
	for _, r := range toDelete {
		if r.S3Key != "" {
			_ = s.deleteS3Object(ctx, r.S3Key)
	REDACTED
REDACTED

	if len(toDelete) > 0 {
		logger.LegacyPrintf("service.backup", "[Backup] 自动清理了 %d 个过期备份", len(toDelete))
		return s.saveRecords(ctx, toKeep)
REDACTED
	return nil
REDACTED

func (s *BackupService) deleteS3Object(ctx context.Context, key string) error {
	s3Cfg, err := s.loadS3Config(ctx)
	if err != nil || s3Cfg == nil {
		return nil
REDACTED
	client, err := s.getOrCreateS3Client(ctx, s3Cfg)
	if err != nil {
		return err
REDACTED
	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s3Cfg.Bucket,
		Key:    &key,
REDACTED)
	return err
REDACTED
