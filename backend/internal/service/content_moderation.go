package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	ContentModerationModeOff      = "off"
	ContentModerationModeObserve  = "observe"
	ContentModerationModePreBlock = "pre_block"

	contentModerationAPIKeysModeAppend  = "append"
	contentModerationAPIKeysModeReplace = "replace"

	ContentModerationActionAllow        = "allow"
	ContentModerationActionBlock        = "block"
	ContentModerationActionHashBlock    = "hash_block"
	ContentModerationActionKeywordBlock = "keyword_block"
	ContentModerationActionError        = "error"

	contentModerationKeywordCategory = "keyword"

	ContentModerationKeywordModeKeywordOnly   = "keyword_only"
	ContentModerationKeywordModeKeywordAndAPI = "keyword_and_api"
	ContentModerationKeywordModeAPIOnly       = "api_only"

	ContentModerationModelFilterAll     = "all"
	ContentModerationModelFilterInclude = "include"
	ContentModerationModelFilterExclude = "exclude"

	ContentModerationProtocolAnthropicMessages = "anthropic_messages"
	ContentModerationProtocolOpenAIResponses   = "openai_responses"
	ContentModerationProtocolOpenAIChat        = "openai_chat_completions"
	ContentModerationProtocolGemini            = "gemini"
	ContentModerationProtocolOpenAIImages      = "openai_images"

	defaultContentModerationBaseURL   = "https://api.openai.com"
	defaultContentModerationModel     = "omni-moderation-latest"
	defaultContentModerationTimeoutMS = 3000
	maxContentModerationTimeoutMS     = 30000
	maxModerationInputRunes           = 12000
	maxModerationExcerptRunes         = 240

	defaultContentModerationWorkerCount          = 4
	maxContentModerationWorkerCount              = 32
	defaultContentModerationQueueSize            = 32768
	maxContentModerationQueueSize                = 100000
	defaultContentModerationBanThreshold         = 10
	defaultContentModerationViolationWindowHours = 720
	defaultContentModerationBlockHTTPStatus      = http.StatusForbidden
	defaultContentModerationBlockMessage         = "内容审计命中风险规则，请调整输入后重试"
	defaultContentModerationRetryCount           = 2
	maxContentModerationRetryCount               = 5
	defaultContentModerationHitRetentionDays     = 180
	defaultContentModerationNonHitRetentionDays  = 3
	maxContentModerationRetentionDays            = 3650
	maxContentModerationNonHitRetentionDays      = 3
	contentModerationKeyRateLimitFreezeDuration  = time.Minute
	contentModerationKeyAuthFreezeDuration       = 10 * time.Minute
	contentModerationKeyHTTPErrorFreezeDuration  = 10 * time.Second
	maxContentModerationInputImages              = 1
	maxContentModerationTestImages               = maxContentModerationInputImages
	maxContentModerationTestImageBytes           = 8 * 1024 * 1024
	maxContentModerationTestImageDataURLBytes    = 12 * 1024 * 1024
	maxContentModerationBlockedKeywords          = 10000
	maxContentModerationBlockedKeywordRunes      = 200
	maxContentModerationModelFilterModels        = 1000
	maxContentModerationModelFilterRunes         = 200

	contentModerationCleanupInterval = 24 * time.Hour
	contentModerationCleanupTimeout  = 30 * time.Minute
	contentModerationCleanupDelay    = 5 * time.Minute
)

var contentModerationCategoryOrder = []string{
	"harassment",
	"harassment/threatening",
	"hate",
	"hate/threatening",
	"illicit",
	"illicit/violent",
	"self-harm",
	"self-harm/intent",
	"self-harm/instructions",
	"sexual",
	"sexual/minors",
	"violence",
	"violence/graphic",
REDACTED

func ContentModerationDefaultThresholds() map[string]float64 {
	return map[string]float64{
		"harassment":             0.98,
		"harassment/threatening": 0.90,
		"hate":                   0.65,
		"hate/threatening":       0.65,
		"illicit":                0.95,
		"illicit/violent":        0.95,
		"self-harm":              0.65,
		"self-harm/intent":       0.85,
		"self-harm/instructions": 0.65,
		"sexual":                 0.65,
		"sexual/minors":          0.65,
		"violence":               0.95,
		"violence/graphic":       0.95,
REDACTED
REDACTED

func ContentModerationCategories() []string {
	out := make([]string, len(contentModerationCategoryOrder))
	copy(out, contentModerationCategoryOrder)
	return out
REDACTED

type ContentModerationConfig struct {
	Enabled              bool                         `json:"enabled"`
	Mode                 string                       `json:"mode"`
	BaseURL              string                       `json:"base_url"`
	Model                string                       `json:"model"`
	APIKey               string                       `json:"api_key,omitempty"`
	APIKeys              []string                     `json:"api_keys,omitempty"`
	TimeoutMS            int                          `json:"timeout_ms"`
	SampleRate           int                          `json:"sample_rate"`
	AllGroups            bool                         `json:"all_groups"`
	GroupIDs             []int64                      `json:"group_ids"`
	RecordNonHits        bool                         `json:"record_non_hits"`
	Thresholds           map[string]float64           `json:"thresholds"`
	WorkerCount          int                          `json:"worker_count"`
	QueueSize            int                          `json:"queue_size"`
	BlockStatus          int                          `json:"block_status"`
	BlockMessage         string                       `json:"block_message"`
	EmailOnHit           bool                         `json:"email_on_hit"`
	AutoBanEnabled       bool                         `json:"auto_ban_enabled"`
	BanThreshold         int                          `json:"ban_threshold"`
	ViolationWindowHours int                          `json:"violation_window_hours"`
	RetryCount           int                          `json:"retry_count"`
	HitRetentionDays     int                          `json:"hit_retention_days"`
	NonHitRetentionDays  int                          `json:"non_hit_retention_days"`
	PreHashCheckEnabled  bool                         `json:"pre_hash_check_enabled"`
	BlockedKeywords      []string                     `json:"blocked_keywords"`
	KeywordBlockingMode  string                       `json:"keyword_blocking_mode"`
	ModelFilter          ContentModerationModelFilter `json:"model_filter"`
REDACTED

type ContentModerationConfigView struct {
	Enabled              bool                            `json:"enabled"`
	Mode                 string                          `json:"mode"`
	BaseURL              string                          `json:"base_url"`
	Model                string                          `json:"model"`
	APIKeyConfigured     bool                            `json:"api_key_configured"`
	APIKeyMasked         string                          `json:"api_key_masked"`
	APIKeyCount          int                             `json:"api_key_count"`
	APIKeyMasks          []string                        `json:"api_key_masks"`
	APIKeyStatuses       []ContentModerationAPIKeyStatus `json:"api_key_statuses"`
	TimeoutMS            int                             `json:"timeout_ms"`
	SampleRate           int                             `json:"sample_rate"`
	AllGroups            bool                            `json:"all_groups"`
	GroupIDs             []int64                         `json:"group_ids"`
	RecordNonHits        bool                            `json:"record_non_hits"`
	Thresholds           map[string]float64              `json:"thresholds"`
	WorkerCount          int                             `json:"worker_count"`
	QueueSize            int                             `json:"queue_size"`
	BlockStatus          int                             `json:"block_status"`
	BlockMessage         string                          `json:"block_message"`
	EmailOnHit           bool                            `json:"email_on_hit"`
	AutoBanEnabled       bool                            `json:"auto_ban_enabled"`
	BanThreshold         int                             `json:"ban_threshold"`
	ViolationWindowHours int                             `json:"violation_window_hours"`
	RetryCount           int                             `json:"retry_count"`
	HitRetentionDays     int                             `json:"hit_retention_days"`
	NonHitRetentionDays  int                             `json:"non_hit_retention_days"`
	PreHashCheckEnabled  bool                            `json:"pre_hash_check_enabled"`
	BlockedKeywords      []string                        `json:"blocked_keywords"`
	KeywordBlockingMode  string                          `json:"keyword_blocking_mode"`
	ModelFilter          ContentModerationModelFilter    `json:"model_filter"`
REDACTED

type ContentModerationAPIKeyStatus struct {
	Index          int        `json:"index"`
	KeyHash        string     `json:"key_hash"`
	Masked         string     `json:"masked"`
	Status         string     `json:"status"`
	FailureCount   int        `json:"failure_count"`
	SuccessCount   int64      `json:"success_count"`
	LastError      string     `json:"last_error"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	FrozenUntil    *time.Time `json:"frozen_until,omitempty"`
	LastLatencyMS  int        `json:"last_latency_ms"`
	LastHTTPStatus int        `json:"last_http_status"`
	LastTested     bool       `json:"last_tested"`
	Configured     bool       `json:"configured"`
REDACTED

type ContentModerationAPIKeyLoad struct {
	Index          int    `json:"index"`
	KeyHash        string `json:"key_hash"`
	Masked         string `json:"masked"`
	Status         string `json:"status"`
	Active         int64  `json:"active"`
	Total          int64  `json:"total"`
	Success        int64  `json:"success"`
	Errors         int64  `json:"errors"`
	AvgLatencyMS   int64  `json:"avg_latency_ms"`
	LastLatencyMS  int    `json:"last_latency_ms"`
	LastHTTPStatus int    `json:"last_http_status"`
REDACTED

type TestContentModerationAPIKeysInput struct {
	APIKeys   []string `json:"api_keys"`
	BaseURL   string   `json:"base_url"`
	Model     string   `json:"model"`
	TimeoutMS int      `json:"timeout_ms"`
	Prompt    string   `json:"prompt"`
	Images    []string `json:"images"`
REDACTED

type TestContentModerationAPIKeysResult struct {
	Items       []ContentModerationAPIKeyStatus   `json:"items"`
	AuditResult *ContentModerationTestAuditResult `json:"audit_result,omitempty"`
	ImageCount  int                               `json:"image_count"`
REDACTED

type ContentModerationTestAuditResult struct {
	Flagged         bool               `json:"flagged"`
	HighestCategory string             `json:"highest_category"`
	HighestScore    float64            `json:"highest_score"`
	CompositeScore  float64            `json:"composite_score"`
	CategoryScores  map[string]float64 `json:"category_scores"`
	Thresholds      map[string]float64 `json:"thresholds"`
REDACTED

type UpdateContentModerationConfigInput struct {
	Enabled              *bool                         `json:"enabled"`
	Mode                 *string                       `json:"mode"`
	BaseURL              *string                       `json:"base_url"`
	Model                *string                       `json:"model"`
	APIKey               *string                       `json:"api_key"`
	APIKeys              *[]string                     `json:"api_keys"`
	APIKeysMode          string                        `json:"api_keys_mode"`
	DeleteAPIKeyHashes   *[]string                     `json:"delete_api_key_hashes"`
	ClearAPIKey          bool                          `json:"clear_api_key"`
	TimeoutMS            *int                          `json:"timeout_ms"`
	SampleRate           *int                          `json:"sample_rate"`
	AllGroups            *bool                         `json:"all_groups"`
	GroupIDs             *[]int64                      `json:"group_ids"`
	RecordNonHits        *bool                         `json:"record_non_hits"`
	Thresholds           *map[string]float64           `json:"thresholds"`
	WorkerCount          *int                          `json:"worker_count"`
	QueueSize            *int                          `json:"queue_size"`
	BlockStatus          *int                          `json:"block_status"`
	BlockMessage         *string                       `json:"block_message"`
	EmailOnHit           *bool                         `json:"email_on_hit"`
	AutoBanEnabled       *bool                         `json:"auto_ban_enabled"`
	BanThreshold         *int                          `json:"ban_threshold"`
	ViolationWindowHours *int                          `json:"violation_window_hours"`
	RetryCount           *int                          `json:"retry_count"`
	HitRetentionDays     *int                          `json:"hit_retention_days"`
	NonHitRetentionDays  *int                          `json:"non_hit_retention_days"`
	PreHashCheckEnabled  *bool                         `json:"pre_hash_check_enabled"`
	BlockedKeywords      *[]string                     `json:"blocked_keywords"`
	KeywordBlockingMode  *string                       `json:"keyword_blocking_mode"`
	ModelFilter          *ContentModerationModelFilter `json:"model_filter"`
REDACTED

type ContentModerationModelFilter struct {
	Type   string   `json:"type"`
	Models []string `json:"models"`
REDACTED

type ContentModerationCheckInput struct {
	RequestID  string
	UserID     int64
	UserEmail  string
	APIKeyID   int64
	APIKeyName string
	GroupID    *int64
	GroupName  string
	Endpoint   string
	Provider   string
	Model      string
	Protocol   string
	Body       []byte
REDACTED

type ContentModerationInput struct {
	Text   string
	Images []string
REDACTED

func (in *ContentModerationInput) Normalize() {
	if in == nil {
		return
REDACTED
	in.Text = trimRunes(normalizeContentModerationText(in.Text), maxModerationInputRunes)
	in.Images = normalizeModerationImages(in.Images)
REDACTED

func (in ContentModerationInput) IsEmpty() bool {
	return strings.TrimSpace(in.Text) == "" && len(in.Images) == 0
REDACTED

func (in ContentModerationInput) ModerationInput() any {
	images := limitContentModerationImages(in.Images)
	if len(images) == 0 {
		return in.Text
REDACTED
	parts := make([]moderationAPIInputPart, 0, len(images)+1)
	if strings.TrimSpace(in.Text) != "" {
		parts = append(parts, moderationAPIInputPart{Type: "text", Text: in.TextREDACTED)
REDACTED
	for _, image := range images {
		parts = append(parts, moderationAPIInputPart{
			Type:     "image_url",
			ImageURL: &moderationAPIImageURLRef{URL: imageREDACTED,
	REDACTED)
REDACTED
	return parts
REDACTED

func (in ContentModerationInput) ExcerptText() string {
	return in.Text
REDACTED

func (in ContentModerationInput) Hash() string {
	h := sha256.New()
	_, _ = h.Write([]byte("text:"))
	_, _ = h.Write([]byte(in.Text))
	for _, image := range in.Images {
		imageHash := sha256.Sum256([]byte(image))
		_, _ = h.Write([]byte("\nimage:"))
		_, _ = h.Write([]byte(hex.EncodeToString(imageHash[:])))
REDACTED
	return hex.EncodeToString(h.Sum(nil))
REDACTED

type ContentModerationDecision struct {
	Allowed         bool               `json:"allowed"`
	Blocked         bool               `json:"blocked"`
	Flagged         bool               `json:"flagged"`
	Message         string             `json:"message"`
	StatusCode      int                `json:"status_code"`
	InputHash       string             `json:"input_hash,omitempty"`
	HighestCategory string             `json:"highest_category"`
	HighestScore    float64            `json:"highest_score"`
	CategoryScores  map[string]float64 `json:"category_scores"`
	Action          string             `json:"action"`
REDACTED

type ContentModerationLog struct {
	ID                int64              `json:"id"`
	RequestID         string             `json:"request_id"`
	UserID            *int64             `json:"user_id,omitempty"`
	UserEmail         string             `json:"user_email"`
	APIKeyID          *int64             `json:"api_key_id,omitempty"`
	APIKeyName        string             `json:"api_key_name"`
	GroupID           *int64             `json:"group_id,omitempty"`
	GroupName         string             `json:"group_name"`
	Endpoint          string             `json:"endpoint"`
	Provider          string             `json:"provider"`
	Model             string             `json:"model"`
	Mode              string             `json:"mode"`
	Action            string             `json:"action"`
	Flagged           bool               `json:"flagged"`
	HighestCategory   string             `json:"highest_category"`
	HighestScore      float64            `json:"highest_score"`
	CategoryScores    map[string]float64 `json:"category_scores"`
	ThresholdSnapshot map[string]float64 `json:"threshold_snapshot"`
	InputExcerpt      string             `json:"input_excerpt"`
	UpstreamLatencyMS *int               `json:"upstream_latency_ms,omitempty"`
	Error             string             `json:"error"`
	ViolationCount    int                `json:"violation_count"`
	AutoBanned        bool               `json:"auto_banned"`
	EmailSent         bool               `json:"email_sent"`
	UserStatus        string             `json:"user_status"`
	QueueDelayMS      *int               `json:"queue_delay_ms,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
REDACTED

type ContentModerationLogFilter struct {
	Pagination pagination.PaginationParams
	Result     string
	GroupID    *int64
	Endpoint   string
	Search     string
	From       *time.Time
	To         *time.Time
REDACTED

type ContentModerationCleanupResult struct {
	DeletedHit    int64     `json:"deleted_hit"`
	DeletedNonHit int64     `json:"deleted_non_hit"`
	FinishedAt    time.Time `json:"finished_at"`
REDACTED

type ContentModerationRuntimeStatus struct {
	Enabled                      bool                            `json:"enabled"`
	RiskControlEnabled           bool                            `json:"risk_control_enabled"`
	Mode                         string                          `json:"mode"`
	WorkerCount                  int                             `json:"worker_count"`
	MaxWorkers                   int                             `json:"max_workers"`
	ActiveWorkers                int                             `json:"active_workers"`
	IdleWorkers                  int                             `json:"idle_workers"`
	QueueSize                    int                             `json:"queue_size"`
	QueueLength                  int                             `json:"queue_length"`
	QueueUsagePercent            float64                         `json:"queue_usage_percent"`
	Enqueued                     int64                           `json:"enqueued"`
	Dropped                      int64                           `json:"dropped"`
	Processed                    int64                           `json:"processed"`
	Errors                       int64                           `json:"errors"`
	PreBlockActive               int                             `json:"pre_block_active"`
	PreBlockChecked              int64                           `json:"pre_block_checked"`
	PreBlockAllowed              int64                           `json:"pre_block_allowed"`
	PreBlockBlocked              int64                           `json:"pre_block_blocked"`
	PreBlockErrors               int64                           `json:"pre_block_errors"`
	PreBlockAvgLatencyMS         int64                           `json:"pre_block_avg_latency_ms"`
	PreBlockAPIKeyActive         int64                           `json:"pre_block_api_key_active"`
	PreBlockAPIKeyAvailableCount int64                           `json:"pre_block_api_key_available_count"`
	PreBlockAPIKeyTotalCalls     int64                           `json:"pre_block_api_key_total_calls"`
	PreBlockAPIKeyLoads          []ContentModerationAPIKeyLoad   `json:"pre_block_api_key_loads"`
	APIKeyStatuses               []ContentModerationAPIKeyStatus `json:"api_key_statuses"`
	FlaggedHashCount             int64                           `json:"flagged_hash_count"`
	LastCleanupAt                *time.Time                      `json:"last_cleanup_at,omitempty"`
	LastCleanupDeletedHit        int64                           `json:"last_cleanup_deleted_hit"`
	LastCleanupDeletedNonHit     int64                           `json:"last_cleanup_deleted_non_hit"`
REDACTED

type ContentModerationUnbanUserResult struct {
	UserID int64  `json:"user_id"`
	Status string `json:"status"`
REDACTED

type ContentModerationDeleteHashResult struct {
	InputHash string `json:"input_hash"`
	Deleted   bool   `json:"deleted"`
REDACTED

type ContentModerationClearHashesResult struct {
	Deleted int64 `json:"deleted"`
REDACTED

type ContentModerationRepository interface {
	CreateLog(ctx context.Context, log *ContentModerationLog) error
	ListLogs(ctx context.Context, filter ContentModerationLogFilter) ([]ContentModerationLog, *pagination.PaginationResult, error)
	CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time) (int, error)
	CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*ContentModerationCleanupResult, error)
REDACTED

type ContentModerationHashCache interface {
	RecordFlaggedInputHash(ctx context.Context, inputHash string) error
	HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	ClearFlaggedInputHashes(ctx context.Context) (int64, error)
	CountFlaggedInputHashes(ctx context.Context) (int64, error)
REDACTED

type ContentModerationService struct {
	settingRepo              SettingRepository
	repo                     ContentModerationRepository
	hashCache                ContentModerationHashCache
	groupRepo                GroupRepository
	userRepo                 UserRepository
	authCacheInvalidator     APIKeyAuthCacheInvalidator
	emailService             *EmailService
	httpClient               *http.Client
	asyncQueue               chan contentModerationTask
	workerCount              int
	apiKeyCursor             atomic.Uint64
	asyncActive              atomic.Int64
	asyncEnqueued            atomic.Int64
	asyncDropped             atomic.Int64
	asyncProcessed           atomic.Int64
	asyncErrors              atomic.Int64
	preBlockActive           atomic.Int64
	preBlockChecked          atomic.Int64
	preBlockAllowed          atomic.Int64
	preBlockBlocked          atomic.Int64
	preBlockErrors           atomic.Int64
	preBlockLatencyTotalMS   atomic.Int64
	lastCleanupUnix          atomic.Int64
	lastCleanupDeletedHit    atomic.Int64
	lastCleanupDeletedNonHit atomic.Int64
	keyHealthMu              sync.Mutex
	keyHealth                map[string]*contentModerationKeyHealth
REDACTED

type contentModerationTask struct {
	input            ContentModerationCheckInput
	content          ContentModerationInput
	inputHash        string
	log              *ContentModerationLog
	config           *ContentModerationConfig
	recordHash       bool
	applySideEffects bool
	enqueuedAt       time.Time
REDACTED

type contentModerationKeyHealth struct {
	Hash           string
	Masked         string
	FailureCount   int
	SuccessCount   int64
	LastError      string
	LastCheckedAt  time.Time
	FrozenUntil    time.Time
	LastLatencyMS  int
	LastHTTPStatus int
	LastTested     bool
	SyncActive     int64
	SyncTotal      int64
	SyncSuccess    int64
	SyncErrors     int64
	SyncLatencyMS  int64
REDACTED

func NewContentModerationService(
	settingRepo SettingRepository,
	repo ContentModerationRepository,
	hashCache ContentModerationHashCache,
	groupRepo GroupRepository,
	userRepo UserRepository,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	emailService *EmailService,
) *ContentModerationService {
	svc := &ContentModerationService{
		settingRepo:          settingRepo,
		repo:                 repo,
		hashCache:            hashCache,
		groupRepo:            groupRepo,
		userRepo:             userRepo,
		authCacheInvalidator: authCacheInvalidator,
		emailService:         emailService,
		httpClient:           &http.Client{REDACTED,
		workerCount:          maxContentModerationWorkerCount,
		asyncQueue:           make(chan contentModerationTask, maxContentModerationQueueSize),
		keyHealth:            make(map[string]*contentModerationKeyHealth),
REDACTED
	if settingRepo != nil && repo != nil {
		for i := 0; i < svc.workerCount; i++ {
			go svc.worker(i)
	REDACTED
		go svc.cleanupWorker()
REDACTED
	return svc
REDACTED

func (s *ContentModerationService) GetConfig(ctx context.Context) (*ContentModerationConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	return s.configView(cfg), nil
REDACTED

func (s *ContentModerationService) UpdateConfig(ctx context.Context, input UpdateContentModerationConfigInput) (*ContentModerationConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	if input.Enabled != nil {
		cfg.Enabled = *input.Enabled
REDACTED
	if input.Mode != nil {
		cfg.Mode = strings.TrimSpace(*input.Mode)
REDACTED
	if input.BaseURL != nil {
		cfg.BaseURL = strings.TrimSpace(*input.BaseURL)
REDACTED
	if input.Model != nil {
		cfg.Model = strings.TrimSpace(*input.Model)
REDACTED
	if input.TimeoutMS != nil {
		cfg.TimeoutMS = *input.TimeoutMS
REDACTED
	if input.SampleRate != nil {
		cfg.SampleRate = *input.SampleRate
REDACTED
	if input.WorkerCount != nil {
		cfg.WorkerCount = *input.WorkerCount
REDACTED
	if input.QueueSize != nil {
		cfg.QueueSize = *input.QueueSize
REDACTED
	if input.BlockStatus != nil {
		cfg.BlockStatus = *input.BlockStatus
REDACTED
	if input.BlockMessage != nil {
		cfg.BlockMessage = strings.TrimSpace(*input.BlockMessage)
REDACTED
	if input.EmailOnHit != nil {
		cfg.EmailOnHit = *input.EmailOnHit
REDACTED
	if input.AutoBanEnabled != nil {
		cfg.AutoBanEnabled = *input.AutoBanEnabled
REDACTED
	if input.BanThreshold != nil {
		cfg.BanThreshold = *input.BanThreshold
REDACTED
	if input.ViolationWindowHours != nil {
		cfg.ViolationWindowHours = *input.ViolationWindowHours
REDACTED
	if input.RetryCount != nil {
		cfg.RetryCount = *input.RetryCount
REDACTED
	if input.HitRetentionDays != nil {
		cfg.HitRetentionDays = *input.HitRetentionDays
REDACTED
	if input.NonHitRetentionDays != nil {
		cfg.NonHitRetentionDays = *input.NonHitRetentionDays
REDACTED
	if input.PreHashCheckEnabled != nil {
		cfg.PreHashCheckEnabled = *input.PreHashCheckEnabled
REDACTED
	if input.BlockedKeywords != nil {
		cfg.BlockedKeywords = normalizeBlockedKeywords(*input.BlockedKeywords)
REDACTED
	if input.KeywordBlockingMode != nil {
		cfg.KeywordBlockingMode = strings.TrimSpace(*input.KeywordBlockingMode)
REDACTED
	if input.ModelFilter != nil {
		cfg.ModelFilter = *input.ModelFilter
REDACTED
	if input.AllGroups != nil {
		cfg.AllGroups = *input.AllGroups
REDACTED
	if input.GroupIDs != nil {
		cfg.GroupIDs = normalizeInt64IDs(*input.GroupIDs)
REDACTED
	if input.RecordNonHits != nil {
		cfg.RecordNonHits = *input.RecordNonHits
REDACTED
	if input.Thresholds != nil {
		cfg.Thresholds = mergeContentModerationThresholds(ContentModerationDefaultThresholds(), *input.Thresholds)
REDACTED
	if input.ClearAPIKey {
		cfg.APIKey = ""
		cfg.APIKeys = []string{REDACTED
REDACTED else {
		apiKeysMode := normalizeContentModerationAPIKeysMode(input.APIKeysMode)
		if input.DeleteAPIKeyHashes != nil && apiKeysMode != contentModerationAPIKeysModeReplace {
			cfg.APIKeys = deleteModerationAPIKeysByHash(cfg.apiKeys(), *input.DeleteAPIKeyHashes)
			cfg.APIKey = ""
	REDACTED
		if input.APIKeys != nil {
			if apiKeysMode == contentModerationAPIKeysModeReplace {
				cfg.APIKeys = normalizeModerationAPIKeys(*input.APIKeys)
		REDACTED else {
				cfg.APIKeys = normalizeModerationAPIKeys(append(cfg.apiKeys(), *input.APIKeys...))
		REDACTED
			cfg.APIKey = ""
	REDACTED
		if input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
			cfg.APIKeys = normalizeModerationAPIKeys(append(cfg.APIKeys, *input.APIKey))
			cfg.APIKey = ""
	REDACTED
REDACTED
	if err := s.validateConfig(ctx, cfg); err != nil {
		return nil, err
REDACTED
	cfg.normalize()
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal content moderation config: %w", err)
REDACTED
	if err := s.settingRepo.Set(ctx, SettingKeyContentModerationConfig, string(raw)); err != nil {
		return nil, fmt.Errorf("save content moderation config: %w", err)
REDACTED
	return s.configView(cfg), nil
REDACTED

func (s *ContentModerationService) TestAPIKeys(ctx context.Context, input TestContentModerationAPIKeysInput) (*TestContentModerationAPIKeysResult, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	keys := normalizeModerationAPIKeys(input.APIKeys)
	configured := false
	if len(keys) == 0 {
		keys = cfg.apiKeys()
		configured = true
REDACTED
	if strings.TrimSpace(input.BaseURL) != "" {
		cfg.BaseURL = input.BaseURL
REDACTED
	if strings.TrimSpace(input.Model) != "" {
		cfg.Model = input.Model
REDACTED
	if input.TimeoutMS > 0 {
		cfg.TimeoutMS = input.TimeoutMS
REDACTED
	cfg.normalize()
	testInput, imageCount, err := buildModerationTestInput(input.Prompt, input.Images)
	if err != nil {
		return nil, err
REDACTED
	auditOnly := contentModerationTestHasAuditInput(input.Prompt, input.Images)
	if configured && auditOnly {
		key, ok := s.nextUsableAPIKey(cfg)
		if !ok {
			return &TestContentModerationAPIKeysResult{
				Items:      s.apiKeyStatuses(keys),
				ImageCount: imageCount,
		REDACTED, nil
	REDACTED
		keys = []string{keyREDACTED
REDACTED
	if len(keys) == 0 {
		return &TestContentModerationAPIKeysResult{Items: []ContentModerationAPIKeyStatus{REDACTED, ImageCount: imageCountREDACTED, nil
REDACTED
	items := make([]ContentModerationAPIKeyStatus, 0, len(keys))
	var auditResult *ContentModerationTestAuditResult
	for idx, key := range keys {
		start := time.Now()
		httpStatus := 0
		result, err := s.callModerationOnceWithInput(ctx, cfg, key, testInput, &httpStatus)
		latency := int(time.Since(start).Milliseconds())
		keyHash := moderationAPIKeyHash(key)
		if err != nil {
			s.markAPIKeyError(key, err.Error(), latency, httpStatus)
	REDACTED else {
			s.markAPIKeySuccess(key, latency, httpStatus)
			if auditResult == nil {
				auditResult = buildContentModerationTestAuditResult(result, cfg.Thresholds)
		REDACTED
	REDACTED
		status := s.apiKeyStatusForHash(idx, keyHash, maskSecretTail(key), configured)
		status.LastTested = true
		items = append(items, status)
REDACTED
	return &TestContentModerationAPIKeysResult{Items: items, AuditResult: auditResult, ImageCount: imageCountREDACTED, nil
REDACTED

func (s *ContentModerationService) Check(ctx context.Context, input ContentModerationCheckInput) (*ContentModerationDecision, error) {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllowREDACTED
	if s == nil || s.settingRepo == nil || s.repo == nil {
		slog.Info("content_moderation.skip_unavailable",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
REDACTED
	if !s.isRiskControlEnabled(ctx) {
		slog.Info("content_moderation.skip_feature_disabled",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
REDACTED
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("content_moderation.skip_config_load_failed",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"error", err)
		return allow, nil
REDACTED
	inGroupScope := cfg.includesGroup(input.GroupID)
	inModelScope := cfg.includesModel(input.Model)
	slog.Info("content_moderation.config_loaded",
		"user_id", input.UserID,
		"api_key_id", input.APIKeyID,
		"group_id", contentModerationLogGroupID(input.GroupID),
		"group_name", input.GroupName,
		"endpoint", input.Endpoint,
		"provider", input.Provider,
		"protocol", input.Protocol,
		"model", input.Model,
		"enabled", cfg.Enabled,
		"mode", cfg.Mode,
		"all_groups", cfg.AllGroups,
		"configured_group_ids", cfg.GroupIDs,
		"in_group_scope", inGroupScope,
		"model_filter_type", cfg.ModelFilter.Type,
		"configured_models", cfg.ModelFilter.Models,
		"in_model_scope", inModelScope,
		"sample_rate", cfg.SampleRate,
		"api_key_count", len(cfg.apiKeys()),
		"pre_hash_check_enabled", cfg.PreHashCheckEnabled,
		"record_non_hits", cfg.RecordNonHits)
	if !cfg.Enabled {
		slog.Info("content_moderation.skip_config_disabled",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
REDACTED
	if cfg.Mode == ContentModerationModeOff {
		slog.Info("content_moderation.skip_mode_off",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
REDACTED
	if !inGroupScope {
		slog.Info("content_moderation.skip_group_out_of_scope",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"group_name", input.GroupName,
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"all_groups", cfg.AllGroups,
			"configured_group_ids", cfg.GroupIDs)
		return allow, nil
REDACTED
	if !inModelScope {
		slog.Info("content_moderation.skip_model_out_of_scope",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"group_name", input.GroupName,
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"model", input.Model,
			"model_filter_type", cfg.ModelFilter.Type,
			"configured_models", cfg.ModelFilter.Models)
		return allow, nil
REDACTED
	content := ExtractContentModerationInput(input.Protocol, input.Body)
	if content.IsEmpty() {
		slog.Info("content_moderation.skip_empty_input",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"body_bytes", len(input.Body))
		return allow, nil
REDACTED
	content.Normalize()
	slog.Info("content_moderation.input_extracted",
		"user_id", input.UserID,
		"api_key_id", input.APIKeyID,
		"group_id", contentModerationLogGroupID(input.GroupID),
		"endpoint", input.Endpoint,
		"protocol", input.Protocol,
		"text_runes", len([]rune(content.Text)),
		"image_count", len(content.Images))
	hashText := content.Hash()
	if cfg.Mode == ContentModerationModePreBlock {
		if cfg.KeywordBlockingMode != ContentModerationKeywordModeAPIOnly && len(cfg.BlockedKeywords) > 0 {
			if keyword, hit := matchBlockedKeyword(content.Text, cfg.BlockedKeywords); hit {
				s.recordPreBlockSyncMetric(0, ContentModerationActionKeywordBlock)
				slog.Info("content_moderation.keyword_block",
					"user_id", input.UserID,
					"api_key_id", input.APIKeyID,
					"group_id", contentModerationLogGroupID(input.GroupID),
					"endpoint", input.Endpoint,
					"protocol", input.Protocol,
					"keyword_blocking_mode", cfg.KeywordBlockingMode,
					"keyword", keyword)
				scores := map[string]float64{contentModerationKeywordCategory: 1.0REDACTED
				log := s.buildLog(input, cfg, ContentModerationActionKeywordBlock, true, contentModerationKeywordCategory, 1.0, scores, content.ExcerptText(), nil, nil, "")
				s.enqueueRecord(input, cfg, log, hashText, false, true)
				return &ContentModerationDecision{
					Allowed:         false,
					Blocked:         true,
					Flagged:         true,
					Message:         cfg.BlockMessage,
					StatusCode:      cfg.BlockStatus,
					HighestCategory: contentModerationKeywordCategory,
					HighestScore:    1.0,
					CategoryScores:  scores,
					Action:          ContentModerationActionKeywordBlock,
			REDACTED, nil
		REDACTED
	REDACTED
		if cfg.KeywordBlockingMode == ContentModerationKeywordModeKeywordOnly {
			s.recordPreBlockSyncMetric(0, ContentModerationActionAllow)
			slog.Info("content_moderation.skip_api_keyword_only",
				"user_id", input.UserID,
				"api_key_id", input.APIKeyID,
				"group_id", contentModerationLogGroupID(input.GroupID),
				"endpoint", input.Endpoint,
				"protocol", input.Protocol)
			return allow, nil
	REDACTED
REDACTED
	if cfg.PreHashCheckEnabled && s.hashCache != nil {
		matched, err := s.hashCache.HasFlaggedInputHash(ctx, hashText)
		if err != nil {
			slog.Warn("content_moderation.hash_check_failed", "user_id", input.UserID, "endpoint", input.Endpoint, "error", err)
	REDACTED
		if matched {
			if cfg.Mode == ContentModerationModePreBlock {
				s.recordPreBlockSyncMetric(0, ContentModerationActionHashBlock)
		REDACTED
			slog.Info("content_moderation.hash_block",
				"user_id", input.UserID,
				"api_key_id", input.APIKeyID,
				"group_id", contentModerationLogGroupID(input.GroupID),
				"endpoint", input.Endpoint,
				"protocol", input.Protocol,
				"input_hash", hashText)
			message := cfg.BlockMessage
			if message != "" {
				message = fmt.Sprintf("%s（hash: %s）", message, hashText)
		REDACTED
			scores := map[string]float64{"hash": 1.0REDACTED
			log := s.buildLog(input, cfg, ContentModerationActionHashBlock, true, "hash", 1.0, scores, content.ExcerptText(), nil, nil, "")
			s.enqueueRecord(input, cfg, log, hashText, false, false)
			return &ContentModerationDecision{
				Allowed:    false,
				Blocked:    true,
				Flagged:    true,
				Message:    message,
				StatusCode: cfg.BlockStatus,
				InputHash:  hashText,
				Action:     ContentModerationActionHashBlock,
		REDACTED, nil
	REDACTED
REDACTED
	if !cfg.shouldSample(hashText) {
		if cfg.Mode == ContentModerationModePreBlock {
			s.recordPreBlockSyncMetric(0, ContentModerationActionAllow)
	REDACTED
		slog.Info("content_moderation.skip_sample_rate",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"sample_rate", cfg.SampleRate)
		return allow, nil
REDACTED
	if len(cfg.apiKeys()) == 0 {
		if cfg.Mode == ContentModerationModePreBlock {
			s.recordPreBlockSyncMetric(0, ContentModerationActionError)
	REDACTED
		slog.Warn("content_moderation.skip_no_audit_api_keys",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol)
		return allow, nil
REDACTED
	if cfg.Mode == ContentModerationModeObserve {
		slog.Info("content_moderation.enqueue_observe",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"queue_len", len(s.asyncQueue))
		s.enqueueAsync(input, cfg, content, hashText)
		return allow, nil
REDACTED

	return s.checkSync(ctx, input, cfg, content, hashText, nil, true), nil
REDACTED

func (s *ContentModerationService) checkSync(ctx context.Context, input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string, queueDelay *int, allowBlock bool) *ContentModerationDecision {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllowREDACTED
	trackPreBlock := queueDelay == nil && allowBlock && cfg != nil && cfg.Mode == ContentModerationModePreBlock
	if trackPreBlock {
		s.preBlockActive.Add(1)
		defer s.preBlockActive.Add(-1)
REDACTED
	start := time.Now()
	result, err := s.callModeration(ctx, cfg, content.ModerationInput(), trackPreBlock)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		if trackPreBlock {
			s.recordPreBlockSyncMetric(latency, ContentModerationActionError)
	REDACTED
		slog.Warn("content_moderation.audit_api_failed",
			"user_id", input.UserID,
			"api_key_id", input.APIKeyID,
			"group_id", contentModerationLogGroupID(input.GroupID),
			"endpoint", input.Endpoint,
			"protocol", input.Protocol,
			"mode", cfg.Mode,
			"allow_block", allowBlock,
			"queue_delay_ms", queueDelay,
			"latency_ms", latency,
			"error", err)
		if queueDelay != nil {
			s.asyncErrors.Add(1)
	REDACTED
		if cfg.RecordNonHits {
			log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, nil, content.ExcerptText(), &latency, queueDelay, err.Error())
			_ = s.repo.CreateLog(ctx, log)
	REDACTED
		return allow
REDACTED

	flagged, highestCategory, highestScore := evaluateModerationScores(result.CategoryScores, cfg.Thresholds)
	action := ContentModerationActionAllow
	blocked := false
	if allowBlock && flagged && cfg.Mode == ContentModerationModePreBlock {
		action = ContentModerationActionBlock
		blocked = true
REDACTED
	if trackPreBlock {
		s.recordPreBlockSyncMetric(latency, action)
REDACTED
	slog.Info("content_moderation.audit_result",
		"user_id", input.UserID,
		"api_key_id", input.APIKeyID,
		"group_id", contentModerationLogGroupID(input.GroupID),
		"group_name", input.GroupName,
		"endpoint", input.Endpoint,
		"protocol", input.Protocol,
		"mode", cfg.Mode,
		"allow_block", allowBlock,
		"flagged", flagged,
		"blocked", blocked,
		"action", action,
		"highest_category", highestCategory,
		"highest_score", highestScore,
		"latency_ms", latency,
		"queue_delay_ms", queueDelay)
	if flagged || cfg.RecordNonHits {
		log := s.buildLog(input, cfg, action, flagged, highestCategory, highestScore, result.CategoryScores, content.ExcerptText(), &latency, queueDelay, "")
		if queueDelay == nil && cfg.Mode == ContentModerationModePreBlock {
			s.enqueueRecord(input, cfg, log, hashText, flagged, flagged)
	REDACTED else {
			s.persistContentModerationLog(ctx, cfg, log, hashText, flagged, flagged)
	REDACTED
REDACTED
	if blocked {
		return &ContentModerationDecision{
			Allowed:         false,
			Blocked:         true,
			Flagged:         true,
			Message:         cfg.BlockMessage,
			StatusCode:      cfg.BlockStatus,
			HighestCategory: highestCategory,
			HighestScore:    highestScore,
			CategoryScores:  result.CategoryScores,
			Action:          action,
	REDACTED
REDACTED
	return &ContentModerationDecision{
		Allowed:         true,
		Flagged:         flagged,
		Message:         "",
		HighestCategory: highestCategory,
		HighestScore:    highestScore,
		CategoryScores:  result.CategoryScores,
		Action:          action,
REDACTED
REDACTED

func (s *ContentModerationService) recordPreBlockSyncMetric(latencyMS int, action string) {
	if s == nil {
		return
REDACTED
	s.preBlockChecked.Add(1)
	if latencyMS < 0 {
		latencyMS = 0
REDACTED
	s.preBlockLatencyTotalMS.Add(int64(latencyMS))
	switch action {
	case ContentModerationActionBlock, ContentModerationActionHashBlock, ContentModerationActionKeywordBlock:
		s.preBlockBlocked.Add(1)
	case ContentModerationActionError:
		s.preBlockErrors.Add(1)
	default:
		s.preBlockAllowed.Add(1)
REDACTED
REDACTED

func (s *ContentModerationService) enqueueAsync(input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string) {
	if s == nil || s.asyncQueue == nil {
		return
REDACTED
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
REDACTED
	if len(s.asyncQueue) >= queueSize {
		slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "queue_size", queueSize)
		s.asyncDropped.Add(1)
		return
REDACTED
	task := contentModerationTask{
		input:      input,
		content:    content,
		inputHash:  hashText,
		enqueuedAt: time.Now(),
REDACTED
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
	default:
		slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint)
		s.asyncDropped.Add(1)
REDACTED
REDACTED

func (s *ContentModerationService) enqueueRecord(input ContentModerationCheckInput, cfg *ContentModerationConfig, log *ContentModerationLog, inputHash string, recordHash bool, applySideEffects bool) {
	if s == nil || s.asyncQueue == nil || log == nil {
		return
REDACTED
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
REDACTED
	if len(s.asyncQueue) >= queueSize {
		slog.Warn("content_moderation.record_queue_full",
			"user_id", input.UserID,
			"endpoint", input.Endpoint,
			"action", log.Action,
			"queue_size", queueSize)
		s.asyncDropped.Add(1)
		return
REDACTED
	task := contentModerationTask{
		input:            input,
		inputHash:        inputHash,
		log:              log,
		config:           cloneContentModerationConfig(cfg),
		recordHash:       recordHash,
		applySideEffects: applySideEffects,
		enqueuedAt:       time.Now(),
REDACTED
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
	default:
		slog.Warn("content_moderation.record_queue_full",
			"user_id", input.UserID,
			"endpoint", input.Endpoint,
			"action", log.Action)
		s.asyncDropped.Add(1)
REDACTED
REDACTED

func (s *ContentModerationService) worker(id int) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), maxContentModerationTimeoutMS*time.Millisecond+10*time.Second)
		cfg, err := s.loadConfig(ctx)
		if err != nil || id >= cfg.WorkerCount {
			cancel()
			time.Sleep(time.Second)
			continue
	REDACTED
		task, ok := s.dequeueAsyncTask(ctx, time.Second)
		if !ok {
			cancel()
			continue
	REDACTED
		func() {
			defer cancel()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("content_moderation.worker_panic", "worker_id", id, "recover", r)
			REDACTED
		REDACTED()
			if task.log != nil {
				s.asyncActive.Add(1)
				defer s.asyncActive.Add(-1)
				queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
				task.log.QueueDelayMS = &queueDelay
				taskCfg := task.config
				if taskCfg == nil {
					taskCfg = cfg
			REDACTED
				s.persistContentModerationLog(ctx, taskCfg, task.log, task.inputHash, task.recordHash, task.applySideEffects)
				s.asyncProcessed.Add(1)
				return
		REDACTED
			if !cfg.Enabled || cfg.Mode == ContentModerationModeOff || len(cfg.apiKeys()) == 0 {
				return
		REDACTED
			if !cfg.includesGroup(task.input.GroupID) {
				return
		REDACTED
			if !cfg.includesModel(task.input.Model) {
				return
		REDACTED
			s.asyncActive.Add(1)
			defer s.asyncActive.Add(-1)
			queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
			_ = s.checkSync(ctx, task.input, cfg, task.content, task.inputHash, &queueDelay, false)
			s.asyncProcessed.Add(1)
	REDACTED()
REDACTED
REDACTED

func (s *ContentModerationService) dequeueAsyncTask(ctx context.Context, idleWait time.Duration) (contentModerationTask, bool) {
	var zero contentModerationTask
	if s == nil || s.asyncQueue == nil {
		return zero, false
REDACTED
	if idleWait <= 0 {
		idleWait = time.Second
REDACTED
	timer := time.NewTimer(idleWait)
	defer timer.Stop()
	select {
	case task, ok := <-s.asyncQueue:
		return task, ok
	case <-ctx.Done():
		return zero, false
	case <-timer.C:
		return zero, false
REDACTED
REDACTED

func (s *ContentModerationService) ListLogs(ctx context.Context, filter ContentModerationLogFilter) ([]ContentModerationLog, *pagination.PaginationResult, error) {
	if filter.Pagination.Page <= 0 {
		filter.Pagination.Page = 1
REDACTED
	if filter.Pagination.PageSize <= 0 {
		filter.Pagination.PageSize = 20
REDACTED
	if filter.Pagination.PageSize > 100 {
		filter.Pagination.PageSize = 100
REDACTED
	if filter.Pagination.SortOrder == "" {
		filter.Pagination.SortOrder = pagination.SortOrderDesc
REDACTED
	return s.repo.ListLogs(ctx, filter)
REDACTED

func (s *ContentModerationService) UnbanUser(ctx context.Context, userID int64) (*ContentModerationUnbanUserResult, error) {
	if s == nil || s.userRepo == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_USER_REPOSITORY_UNAVAILABLE", "用户仓储不可用")
REDACTED
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "用户 ID 无效")
REDACTED
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, infraerrors.NotFound("USER_NOT_FOUND", "用户不存在")
	REDACTED
		return nil, fmt.Errorf("get content moderation unban user: %w", err)
REDACTED
	if user.Status != StatusActive {
		user.Status = StatusActive
		if err := s.userRepo.Update(ctx, user); err != nil {
			return nil, fmt.Errorf("update content moderation unban user: %w", err)
	REDACTED
REDACTED
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	return &ContentModerationUnbanUserResult{
		UserID: userID,
		Status: StatusActive,
REDACTED, nil
REDACTED

func (s *ContentModerationService) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (*ContentModerationDeleteHashResult, error) {
	inputHash = normalizeContentModerationHash(inputHash)
	if inputHash == "" {
		return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_HASH", "风险输入哈希无效")
REDACTED
	if s == nil || s.hashCache == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_HASH_CACHE_UNAVAILABLE", "内容审计哈希缓存不可用")
REDACTED
	deleted, err := s.hashCache.DeleteFlaggedInputHash(ctx, inputHash)
	if err != nil {
		return nil, fmt.Errorf("delete content moderation flagged hash: %w", err)
REDACTED
	return &ContentModerationDeleteHashResult{
		InputHash: inputHash,
		Deleted:   deleted,
REDACTED, nil
REDACTED

func (s *ContentModerationService) ClearFlaggedInputHashes(ctx context.Context) (*ContentModerationClearHashesResult, error) {
	if s == nil || s.hashCache == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_HASH_CACHE_UNAVAILABLE", "内容审计哈希缓存不可用")
REDACTED
	deleted, err := s.hashCache.ClearFlaggedInputHashes(ctx)
	if err != nil {
		return nil, fmt.Errorf("clear content moderation flagged hashes: %w", err)
REDACTED
	return &ContentModerationClearHashesResult{Deleted: deletedREDACTED, nil
REDACTED

func (s *ContentModerationService) GetStatus(ctx context.Context) (*ContentModerationRuntimeStatus, error) {
	if s == nil {
		return &ContentModerationRuntimeStatus{REDACTED, nil
REDACTED
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
REDACTED
	riskEnabled := s.isRiskControlEnabled(ctx)
	active := int(s.asyncActive.Load())
	if active < 0 {
		active = 0
REDACTED
	if active > cfg.WorkerCount {
		active = cfg.WorkerCount
REDACTED
	preBlockActive := int(s.preBlockActive.Load())
	if preBlockActive < 0 {
		preBlockActive = 0
REDACTED
	preBlockChecked := s.preBlockChecked.Load()
	preBlockAvgLatency := int64(0)
	if preBlockChecked > 0 {
		preBlockAvgLatency = s.preBlockLatencyTotalMS.Load() / preBlockChecked
REDACTED
	queueLength := 0
	if s.asyncQueue != nil {
		queueLength = len(s.asyncQueue)
REDACTED
	queueUsage := 0.0
	if cfg.QueueSize > 0 {
		queueUsage = float64(queueLength) * 100 / float64(cfg.QueueSize)
REDACTED
	var flaggedHashCount int64
	if s.hashCache != nil {
		if n, err := s.hashCache.CountFlaggedInputHashes(ctx); err == nil {
			flaggedHashCount = n
	REDACTED else {
			slog.Warn("content_moderation.hash_count_failed", "error", err)
	REDACTED
REDACTED
	var lastCleanupAt *time.Time
	if unix := s.lastCleanupUnix.Load(); unix > 0 {
		t := time.Unix(unix, 0)
		lastCleanupAt = &t
REDACTED
	return &ContentModerationRuntimeStatus{
		Enabled:                      cfg.Enabled,
		RiskControlEnabled:           riskEnabled,
		Mode:                         cfg.Mode,
		WorkerCount:                  cfg.WorkerCount,
		MaxWorkers:                   maxContentModerationWorkerCount,
		ActiveWorkers:                active,
		IdleWorkers:                  cfg.WorkerCount - active,
		QueueSize:                    cfg.QueueSize,
		QueueLength:                  queueLength,
		QueueUsagePercent:            queueUsage,
		Enqueued:                     s.asyncEnqueued.Load(),
		Dropped:                      s.asyncDropped.Load(),
		Processed:                    s.asyncProcessed.Load(),
		Errors:                       s.asyncErrors.Load(),
		PreBlockActive:               preBlockActive,
		PreBlockChecked:              preBlockChecked,
		PreBlockAllowed:              s.preBlockAllowed.Load(),
		PreBlockBlocked:              s.preBlockBlocked.Load(),
		PreBlockErrors:               s.preBlockErrors.Load(),
		PreBlockAvgLatencyMS:         preBlockAvgLatency,
		PreBlockAPIKeyActive:         s.preBlockAPIKeyActive(cfg.apiKeys()),
		PreBlockAPIKeyAvailableCount: s.preBlockAPIKeyAvailableCount(cfg.apiKeys()),
		PreBlockAPIKeyTotalCalls:     s.preBlockAPIKeyTotalCalls(cfg.apiKeys()),
		PreBlockAPIKeyLoads:          s.preBlockAPIKeyLoads(cfg.apiKeys()),
		APIKeyStatuses:               s.apiKeyStatuses(cfg.apiKeys()),
		FlaggedHashCount:             flaggedHashCount,
		LastCleanupAt:                lastCleanupAt,
		LastCleanupDeletedHit:        s.lastCleanupDeletedHit.Load(),
		LastCleanupDeletedNonHit:     s.lastCleanupDeletedNonHit.Load(),
REDACTED, nil
REDACTED

func (s *ContentModerationService) cleanupWorker() {
	timer := time.NewTimer(contentModerationCleanupDelay)
	defer timer.Stop()
	for {
		<-timer.C
		s.runCleanupOnce()
		timer.Reset(contentModerationCleanupInterval)
REDACTED
REDACTED

func (s *ContentModerationService) runCleanupOnce() {
	if s == nil || s.repo == nil || s.settingRepo == nil {
		return
REDACTED
	ctx, cancel := context.WithTimeout(context.Background(), contentModerationCleanupTimeout)
	defer cancel()
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("content_moderation.cleanup_load_config_failed", "error", err)
		return
REDACTED
	now := time.Now()
	hitBefore := now.AddDate(0, 0, -cfg.HitRetentionDays)
	nonHitBefore := now.AddDate(0, 0, -cfg.NonHitRetentionDays)
	result, err := s.repo.CleanupExpiredLogs(ctx, hitBefore, nonHitBefore)
	if err != nil {
		slog.Warn("content_moderation.cleanup_failed", "error", err)
		return
REDACTED
	if result == nil {
		return
REDACTED
	s.lastCleanupUnix.Store(result.FinishedAt.Unix())
	s.lastCleanupDeletedHit.Store(result.DeletedHit)
	s.lastCleanupDeletedNonHit.Store(result.DeletedNonHit)
REDACTED

func (s *ContentModerationService) loadConfig(ctx context.Context) (*ContentModerationConfig, error) {
	cfg := defaultContentModerationConfig()
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyContentModerationConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			cfg.normalize()
			return cfg, nil
	REDACTED
		return nil, fmt.Errorf("get content moderation config: %w", err)
REDACTED
	if strings.TrimSpace(raw) == "" {
		cfg.normalize()
		return cfg, nil
REDACTED
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_CONFIG", "内容审计配置不是有效 JSON")
REDACTED
	cfg.normalize()
	return cfg, nil
REDACTED

func (s *ContentModerationService) isRiskControlEnabled(ctx context.Context) bool {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyRiskControlEnabled)
	if err != nil {
		return false
REDACTED
	return raw == "true"
REDACTED

func (s *ContentModerationService) validateConfig(ctx context.Context, cfg *ContentModerationConfig) error {
	if cfg == nil {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_CONFIG", "内容审计配置不能为空")
REDACTED
	cfg.normalize()
	switch cfg.Mode {
	case ContentModerationModeOff, ContentModerationModeObserve, ContentModerationModePreBlock:
	default:
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_MODE", "内容审计模式无效")
REDACTED
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_BASE_URL", "OpenAI Base URL 无效")
REDACTED
	if cfg.BlockStatus < 400 || cfg.BlockStatus > 599 {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_BLOCK_STATUS", "拦截 HTTP 状态码必须在 400-599 之间")
REDACTED
	if cfg.ModelFilter.Type != ContentModerationModelFilterAll && len(cfg.ModelFilter.Models) == 0 {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_MODEL_FILTER", "指定或排除模型时至少需要配置 1 个模型")
REDACTED
	if !cfg.AllGroups && len(cfg.GroupIDs) > 0 && s.groupRepo != nil {
		for _, groupID := range cfg.GroupIDs {
			if _, err := s.groupRepo.GetByIDLite(ctx, groupID); err != nil {
				return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_GROUP", fmt.Sprintf("审计分组不存在: %d", groupID))
		REDACTED
	REDACTED
REDACTED
	return nil
REDACTED

func (s *ContentModerationService) callModeration(ctx context.Context, cfg *ContentModerationConfig, input any, trackKeyLoad ...bool) (*moderationAPIResult, error) {
	attempts := cfg.RetryCount + 1
	if attempts <= 0 {
		attempts = 1
REDACTED
	if attempts > maxContentModerationRetryCount+1 {
		attempts = maxContentModerationRetryCount + 1
REDACTED
	trackLoad := len(trackKeyLoad) > 0 && trackKeyLoad[0]
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		key, ok := s.nextUsableAPIKey(cfg)
		if !ok {
			lastErr = errors.New("no moderation api key available")
			break
	REDACTED
		if trackLoad {
			s.beginModerationAPIKeyCall(key)
	REDACTED
		start := time.Now()
		httpStatus := 0
		result, err := s.callModerationOnceWithInput(ctx, cfg, key, input, &httpStatus)
		latency := int(time.Since(start).Milliseconds())
		if err == nil {
			if trackLoad {
				s.finishModerationAPIKeyCall(key, latency, true)
		REDACTED
			s.markAPIKeySuccess(key, latency, httpStatus)
			return result, nil
	REDACTED
		if trackLoad {
			s.finishModerationAPIKeyCall(key, latency, false)
	REDACTED
		s.markAPIKeyError(key, err.Error(), latency, httpStatus)
		lastErr = err
		if httpStatus == http.StatusBadRequest {
			break
	REDACTED
		if attempt == attempts-1 {
			break
	REDACTED
		wait := time.Duration(100*(attempt+1)) * time.Millisecond
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
	REDACTED
REDACTED
	return nil, lastErr
REDACTED

func (s *ContentModerationService) callModerationOnceWithInput(ctx context.Context, cfg *ContentModerationConfig, apiKey string, input any, httpStatus *int) (*moderationAPIResult, error) {
	base := strings.TrimRight(cfg.BaseURL, "/")
	endpoint, err := url.JoinPath(base, "/v1/moderations")
	if err != nil {
		return nil, err
REDACTED
	payload := moderationAPIRequest{
		Model: cfg.Model,
		Input: input,
REDACTED
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
REDACTED
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
REDACTED
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if httpStatus != nil {
		*httpStatus = resp.StatusCode
REDACTED

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("moderation api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
REDACTED
	var out moderationAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
REDACTED
	if len(out.Results) == 0 {
		return nil, errors.New("moderation api returned empty results")
REDACTED
	return &out.Results[0], nil
REDACTED

func (s *ContentModerationService) buildLog(input ContentModerationCheckInput, cfg *ContentModerationConfig, action string, flagged bool, highestCategory string, highestScore float64, scores map[string]float64, text string, latency *int, queueDelay *int, errText string) *ContentModerationLog {
	var userID *int64
	if input.UserID > 0 {
		userID = &input.UserID
REDACTED
	var apiKeyID *int64
	if input.APIKeyID > 0 {
		apiKeyID = &input.APIKeyID
REDACTED
	return &ContentModerationLog{
		RequestID:         input.RequestID,
		UserID:            userID,
		UserEmail:         input.UserEmail,
		APIKeyID:          apiKeyID,
		APIKeyName:        input.APIKeyName,
		GroupID:           cloneInt64Ptr(input.GroupID),
		GroupName:         input.GroupName,
		Endpoint:          input.Endpoint,
		Provider:          input.Provider,
		Model:             input.Model,
		Mode:              cfg.Mode,
		Action:            action,
		Flagged:           flagged,
		HighestCategory:   highestCategory,
		HighestScore:      highestScore,
		CategoryScores:    cloneFloatMap(scores),
		ThresholdSnapshot: cloneFloatMap(cfg.Thresholds),
		InputExcerpt:      trimRunes(redactContentModerationSecrets(text), maxModerationExcerptRunes),
		UpstreamLatencyMS: latency,
		QueueDelayMS:      queueDelay,
		Error:             errText,
REDACTED
REDACTED

func (s *ContentModerationService) persistContentModerationLog(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog, hashText string, recordHash bool, applySideEffects bool) {
	if s == nil || log == nil {
		return
REDACTED
	if recordHash && s.hashCache != nil {
		if err := s.hashCache.RecordFlaggedInputHash(ctx, hashText); err != nil {
			slog.Warn("content_moderation.record_hash_failed", "user_id", contentModerationEmailUserID(log), "endpoint", log.Endpoint, "error", err)
	REDACTED
REDACTED
	autoBanJustApplied := false
	if applySideEffects {
		autoBanJustApplied = s.applyFlaggedAccountSideEffects(ctx, cfg, log)
		s.sendFlaggedNotificationSideEffects(ctx, cfg, log, autoBanJustApplied)
REDACTED
	if s.repo != nil {
		if err := s.repo.CreateLog(ctx, log); err != nil {
			slog.Warn("content_moderation.create_log_failed", "user_id", contentModerationEmailUserID(log), "endpoint", log.Endpoint, "action", log.Action, "error", err)
			return
	REDACTED
REDACTED
REDACTED

func (s *ContentModerationService) applyFlaggedAccountSideEffects(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) bool {
	if s == nil || cfg == nil || log == nil || !log.Flagged || log.UserID == nil || *log.UserID <= 0 {
		return false
REDACTED
	count := 1
	if s.repo != nil && cfg.ViolationWindowHours > 0 {
		since := time.Now().Add(-time.Duration(cfg.ViolationWindowHours) * time.Hour)
		if n, err := s.repo.CountFlaggedByUserSince(ctx, *log.UserID, since); err == nil {
			count = n + 1
	REDACTED
REDACTED
	log.ViolationCount = count
	autoBanJustApplied := false
	if cfg.AutoBanEnabled && cfg.BanThreshold > 0 && count >= cfg.BanThreshold && s.userRepo != nil {
		user, err := s.userRepo.GetByID(ctx, *log.UserID)
		if err != nil {
			slog.Warn("content_moderation.ban_get_user_failed", "user_id", *log.UserID, "error", err)
			return false
	REDACTED
		if user.IsAdmin() {
			slog.Warn("content_moderation.autoban_skipped_admin", "user_id", *log.UserID, "role", user.Role, "count", count, "threshold", cfg.BanThreshold)
			// TODO: Disable the triggering API key instead when API key mutation is available here.
			return false
	REDACTED
		if user.Status != StatusDisabled {
			user.Status = StatusDisabled
			if err := s.userRepo.Update(ctx, user); err != nil {
				slog.Warn("content_moderation.ban_update_user_failed", "user_id", *log.UserID, "error", err)
				return false
		REDACTED
			if s.authCacheInvalidator != nil {
				s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, *log.UserID)
		REDACTED
			autoBanJustApplied = true
	REDACTED
		log.AutoBanned = true
REDACTED
	return autoBanJustApplied
REDACTED

func (s *ContentModerationService) sendFlaggedNotificationSideEffects(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog, autoBanJustApplied bool) {
	if s == nil || cfg == nil || log == nil || !log.Flagged {
		return
REDACTED
	if s.emailService == nil || strings.TrimSpace(log.UserEmail) == "" {
		return
REDACTED
	emailSent := false
	if cfg.EmailOnHit {
		if err := s.sendViolationEmail(ctx, cfg, log); err != nil {
			slog.Warn("content_moderation.email_failed", "user_id", *log.UserID, "email", log.UserEmail, "error", err)
	REDACTED else {
			emailSent = true
	REDACTED
REDACTED
	if autoBanJustApplied {
		if err := s.sendAccountDisabledEmail(ctx, cfg, log); err != nil {
			slog.Warn("content_moderation.ban_email_failed", "user_id", *log.UserID, "email", log.UserEmail, "error", err)
	REDACTED else {
			emailSent = true
	REDACTED
REDACTED
	log.EmailSent = emailSent
REDACTED

func (s *ContentModerationService) sendViolationEmail(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) error {
	siteName := s.siteName(ctx)
	if s.emailService.notificationEmailService != nil {
		if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventContentModerationViolation,
			RecipientEmail: log.UserEmail,
			RecipientName:  emailRecipientName(log.UserEmail),
			UserID:         contentModerationEmailUserID(log),
			SourceType:     "content_moderation",
			SourceID:       contentModerationEmailSourceID(log),
			Variables:      contentModerationEmailVariables(log, cfg),
	REDACTED); err == nil {
			return nil
	REDACTED else {
			if !shouldFallbackNotificationEmail(err) {
				return err
		REDACTED
			slog.Warn("template content moderation violation email failed; falling back to built-in body", "log_id", log.ID, "recipient_hash", notificationEmailHash(log.UserEmail), "err", err.Error())
	REDACTED
REDACTED
	subject := fmt.Sprintf("[%s] 账户风控提醒 / Risk Control Notice", sanitizeEmailHeader(siteName))
	body := buildContentModerationViolationEmailBody(siteName, log, cfg)
	return s.emailService.SendEmail(ctx, log.UserEmail, subject, body)
REDACTED

func (s *ContentModerationService) sendAccountDisabledEmail(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) error {
	siteName := s.siteName(ctx)
	if s.emailService.notificationEmailService != nil {
		if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
			Event:          NotificationEmailEventContentModerationDisabled,
			RecipientEmail: log.UserEmail,
			RecipientName:  emailRecipientName(log.UserEmail),
			UserID:         contentModerationEmailUserID(log),
			SourceType:     "content_moderation",
			SourceID:       contentModerationEmailSourceID(log),
			Variables:      contentModerationEmailVariables(log, cfg),
	REDACTED); err == nil {
			return nil
	REDACTED else {
			if !shouldFallbackNotificationEmail(err) {
				return err
		REDACTED
			slog.Warn("template content moderation disabled email failed; falling back to built-in body", "log_id", log.ID, "recipient_hash", notificationEmailHash(log.UserEmail), "err", err.Error())
	REDACTED
REDACTED
	subject := fmt.Sprintf("[%s] 账户已被禁用 / Account Disabled", sanitizeEmailHeader(siteName))
	body := buildContentModerationAccountDisabledEmailBody(siteName, log, cfg)
	return s.emailService.SendEmail(ctx, log.UserEmail, subject, body)
REDACTED

func contentModerationEmailUserID(log *ContentModerationLog) int64 {
	if log == nil || log.UserID == nil {
		return 0
REDACTED
	return *log.UserID
REDACTED

func contentModerationEmailSourceID(log *ContentModerationLog) string {
	if log == nil || log.ID <= 0 {
		return ""
REDACTED
	return fmt.Sprintf("%d", log.ID)
REDACTED

func contentModerationEmailVariables(log *ContentModerationLog, cfg *ContentModerationConfig) map[string]string {
	variables := map[string]string{
		"triggered_at":        time.Now().UTC().Format(time.RFC3339),
		"group_name":          "-",
		"moderation_category": "-",
		"moderation_score":    "0.000",
		"violation_count":     "0",
		"ban_threshold":       "0",
REDACTED
	if log != nil {
		if !log.CreatedAt.IsZero() {
			variables["triggered_at"] = log.CreatedAt.UTC().Format(time.RFC3339)
	REDACTED
		if strings.TrimSpace(log.GroupName) != "" {
			variables["group_name"] = strings.TrimSpace(log.GroupName)
	REDACTED
		if strings.TrimSpace(log.HighestCategory) != "" {
			variables["moderation_category"] = strings.TrimSpace(log.HighestCategory)
	REDACTED
		variables["moderation_score"] = fmt.Sprintf("%.3f", log.HighestScore)
		variables["violation_count"] = fmt.Sprintf("%d", log.ViolationCount)
REDACTED
	if cfg != nil {
		variables["ban_threshold"] = fmt.Sprintf("%d", cfg.BanThreshold)
REDACTED
	return variables
REDACTED

func (s *ContentModerationService) siteName(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return "Sub2API"
REDACTED
	name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || strings.TrimSpace(name) == "" {
		return "Sub2API"
REDACTED
	return strings.TrimSpace(name)
REDACTED

func defaultContentModerationConfig() *ContentModerationConfig {
	return &ContentModerationConfig{
		Enabled:              false,
		Mode:                 ContentModerationModePreBlock,
		BaseURL:              defaultContentModerationBaseURL,
		Model:                defaultContentModerationModel,
		TimeoutMS:            defaultContentModerationTimeoutMS,
		SampleRate:           100,
		AllGroups:            true,
		GroupIDs:             []int64{REDACTED,
		RecordNonHits:        false,
		Thresholds:           ContentModerationDefaultThresholds(),
		WorkerCount:          defaultContentModerationWorkerCount,
		QueueSize:            defaultContentModerationQueueSize,
		BlockStatus:          defaultContentModerationBlockHTTPStatus,
		BlockMessage:         defaultContentModerationBlockMessage,
		EmailOnHit:           true,
		AutoBanEnabled:       true,
		BanThreshold:         defaultContentModerationBanThreshold,
		ViolationWindowHours: defaultContentModerationViolationWindowHours,
		RetryCount:           defaultContentModerationRetryCount,
		HitRetentionDays:     defaultContentModerationHitRetentionDays,
		NonHitRetentionDays:  defaultContentModerationNonHitRetentionDays,
		PreHashCheckEnabled:  false,
		BlockedKeywords:      []string{REDACTED,
		KeywordBlockingMode:  ContentModerationKeywordModeKeywordAndAPI,
		ModelFilter: ContentModerationModelFilter{
			Type:   ContentModerationModelFilterAll,
			Models: []string{REDACTED,
	REDACTED,
REDACTED
REDACTED

func cloneContentModerationConfig(cfg *ContentModerationConfig) *ContentModerationConfig {
	if cfg == nil {
		return nil
REDACTED
	clone := *cfg
	clone.APIKeys = append([]string(nil), cfg.APIKeys...)
	clone.GroupIDs = append([]int64(nil), cfg.GroupIDs...)
	clone.BlockedKeywords = append([]string(nil), cfg.BlockedKeywords...)
	clone.Thresholds = cloneFloatMap(cfg.Thresholds)
	clone.ModelFilter = ContentModerationModelFilter{
		Type:   cfg.ModelFilter.Type,
		Models: append([]string(nil), cfg.ModelFilter.Models...),
REDACTED
	return &clone
REDACTED

func (cfg *ContentModerationConfig) normalize() {
	if cfg.APIKey != "" {
		cfg.APIKeys = normalizeModerationAPIKeys(append(cfg.APIKeys, cfg.APIKey))
		cfg.APIKey = ""
REDACTED else {
		cfg.APIKeys = normalizeModerationAPIKeys(cfg.APIKeys)
REDACTED
	if cfg.Mode == "" {
		cfg.Mode = ContentModerationModePreBlock
REDACTED
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultContentModerationBaseURL
REDACTED
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.Model == "" {
		cfg.Model = defaultContentModerationModel
REDACTED
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = defaultContentModerationTimeoutMS
REDACTED
	if cfg.TimeoutMS > maxContentModerationTimeoutMS {
		cfg.TimeoutMS = maxContentModerationTimeoutMS
REDACTED
	if cfg.SampleRate < 0 {
		cfg.SampleRate = 0
REDACTED
	if cfg.SampleRate > 100 {
		cfg.SampleRate = 100
REDACTED
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = defaultContentModerationWorkerCount
REDACTED
	if cfg.WorkerCount > maxContentModerationWorkerCount {
		cfg.WorkerCount = maxContentModerationWorkerCount
REDACTED
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultContentModerationQueueSize
REDACTED
	if cfg.QueueSize > maxContentModerationQueueSize {
		cfg.QueueSize = maxContentModerationQueueSize
REDACTED
	if strings.TrimSpace(cfg.BlockMessage) == "" {
		cfg.BlockMessage = defaultContentModerationBlockMessage
REDACTED
	cfg.BlockMessage = strings.TrimSpace(cfg.BlockMessage)
	if cfg.BlockStatus <= 0 {
		cfg.BlockStatus = defaultContentModerationBlockHTTPStatus
REDACTED
	if cfg.BanThreshold <= 0 {
		cfg.BanThreshold = defaultContentModerationBanThreshold
REDACTED
	if cfg.ViolationWindowHours <= 0 {
		cfg.ViolationWindowHours = defaultContentModerationViolationWindowHours
REDACTED
	if cfg.RetryCount < 0 {
		cfg.RetryCount = 0
REDACTED
	if cfg.RetryCount > maxContentModerationRetryCount {
		cfg.RetryCount = maxContentModerationRetryCount
REDACTED
	if cfg.HitRetentionDays <= 0 {
		cfg.HitRetentionDays = defaultContentModerationHitRetentionDays
REDACTED
	if cfg.HitRetentionDays > maxContentModerationRetentionDays {
		cfg.HitRetentionDays = maxContentModerationRetentionDays
REDACTED
	if cfg.NonHitRetentionDays <= 0 {
		cfg.NonHitRetentionDays = defaultContentModerationNonHitRetentionDays
REDACTED
	if cfg.NonHitRetentionDays > maxContentModerationNonHitRetentionDays {
		cfg.NonHitRetentionDays = maxContentModerationNonHitRetentionDays
REDACTED
	cfg.GroupIDs = normalizeInt64IDs(cfg.GroupIDs)
	cfg.Thresholds = mergeContentModerationThresholds(ContentModerationDefaultThresholds(), cfg.Thresholds)
	cfg.BlockedKeywords = normalizeBlockedKeywords(cfg.BlockedKeywords)
	cfg.KeywordBlockingMode = normalizeKeywordBlockingMode(cfg.KeywordBlockingMode)
	cfg.ModelFilter = normalizeContentModerationModelFilter(cfg.ModelFilter)
REDACTED

func (cfg *ContentModerationConfig) includesGroup(groupID *int64) bool {
	if cfg.AllGroups {
		return true
REDACTED
	if groupID == nil {
		return false
REDACTED
	for _, id := range cfg.GroupIDs {
		if id == *groupID {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (cfg *ContentModerationConfig) includesModel(model string) bool {
	if cfg == nil {
		return true
REDACTED
	filter := normalizeContentModerationModelFilter(cfg.ModelFilter)
	switch filter.Type {
	case ContentModerationModelFilterInclude:
		return contentModerationModelListContains(filter.Models, model)
	case ContentModerationModelFilterExclude:
		return !contentModerationModelListContains(filter.Models, model)
	default:
		return true
REDACTED
REDACTED

func contentModerationLogGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
REDACTED
	return *groupID
REDACTED

func (cfg *ContentModerationConfig) shouldSample(hashText string) bool {
	if cfg.SampleRate >= 100 {
		return true
REDACTED
	if cfg.SampleRate <= 0 {
		return false
REDACTED
	raw, err := hex.DecodeString(hashText)
	if err != nil || len(raw) < 2 {
		return true
REDACTED
	return int(binary.BigEndian.Uint16(raw[:2])%100) < cfg.SampleRate
REDACTED

func (cfg *ContentModerationConfig) apiKeys() []string {
	if cfg == nil {
		return nil
REDACTED
	return normalizeModerationAPIKeys(cfg.APIKeys)
REDACTED

func (s *ContentModerationService) nextUsableAPIKey(cfg *ContentModerationConfig) (string, bool) {
	keys := cfg.apiKeys()
	if len(keys) == 0 {
		return "", false
REDACTED
	now := time.Now()
	for i := 0; i < len(keys); i++ {
		idx := int(s.apiKeyCursor.Add(1)-1) % len(keys)
		key := keys[idx]
		if !s.isAPIKeyFrozen(key, now) {
			return key, true
	REDACTED
REDACTED
	return "", false
REDACTED

func (s *ContentModerationService) isAPIKeyFrozen(key string, now time.Time) bool {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return false
REDACTED
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.keyHealth[hash]
	return state != nil && state.FrozenUntil.After(now)
REDACTED

func (s *ContentModerationService) beginModerationAPIKeyCall(key string) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
REDACTED
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	state.SyncActive++
REDACTED

func (s *ContentModerationService) finishModerationAPIKeyCall(key string, latencyMS int, success bool) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
REDACTED
	if latencyMS < 0 {
		latencyMS = 0
REDACTED
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	if state.SyncActive > 0 {
		state.SyncActive--
REDACTED
	state.SyncTotal++
	state.SyncLatencyMS += int64(latencyMS)
	if success {
		state.SyncSuccess++
		return
REDACTED
	state.SyncErrors++
REDACTED

func (s *ContentModerationService) markAPIKeySuccess(key string, latencyMS int, httpStatus int) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
REDACTED
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	state.FailureCount = 0
	state.SuccessCount++
	state.LastError = ""
	state.LastCheckedAt = time.Now()
	state.FrozenUntil = time.Time{REDACTED
	state.LastLatencyMS = latencyMS
	state.LastHTTPStatus = httpStatus
	state.LastTested = true
REDACTED

func (s *ContentModerationService) markAPIKeyError(key string, errText string, latencyMS int, httpStatus int) {
	hash := moderationAPIKeyHash(key)
	if hash == "" || s == nil {
		return
REDACTED
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.ensureAPIKeyHealthLocked(hash, maskSecretTail(key))
	if contentModerationFreezeDurationForHTTPStatus(httpStatus) > 0 {
		state.FailureCount++
REDACTED
	state.LastError = trimRunes(errText, 180)
	state.LastCheckedAt = time.Now()
	state.LastLatencyMS = latencyMS
	state.LastHTTPStatus = httpStatus
	state.LastTested = true
	if freezeDuration := contentModerationFreezeDurationForHTTPStatus(httpStatus); freezeDuration > 0 {
		state.FrozenUntil = time.Now().Add(freezeDuration)
REDACTED
REDACTED

func contentModerationFreezeDurationForHTTPStatus(httpStatus int) time.Duration {
	switch httpStatus {
	case 0, http.StatusBadRequest:
		return 0
	case http.StatusUnauthorized, http.StatusForbidden:
		return contentModerationKeyAuthFreezeDuration
	case http.StatusTooManyRequests, 529:
		return contentModerationKeyRateLimitFreezeDuration
	default:
		return contentModerationKeyHTTPErrorFreezeDuration
REDACTED
REDACTED

func (s *ContentModerationService) ensureAPIKeyHealthLocked(hash string, masked string) *contentModerationKeyHealth {
	if s.keyHealth == nil {
		s.keyHealth = make(map[string]*contentModerationKeyHealth)
REDACTED
	state := s.keyHealth[hash]
	if state == nil {
		state = &contentModerationKeyHealth{Hash: hashREDACTED
		s.keyHealth[hash] = state
REDACTED
	if strings.TrimSpace(masked) != "" {
		state.Masked = masked
REDACTED
	return state
REDACTED

func (s *ContentModerationService) configView(cfg *ContentModerationConfig) *ContentModerationConfigView {
	keys := cfg.apiKeys()
	masks := make([]string, 0, len(keys))
	for _, key := range keys {
		masks = append(masks, maskSecretTail(key))
REDACTED
	apiKeyMasked := ""
	if len(masks) > 0 {
		apiKeyMasked = masks[0]
REDACTED
	return &ContentModerationConfigView{
		Enabled:              cfg.Enabled,
		Mode:                 cfg.Mode,
		BaseURL:              cfg.BaseURL,
		Model:                cfg.Model,
		APIKeyConfigured:     len(keys) > 0,
		APIKeyMasked:         apiKeyMasked,
		APIKeyCount:          len(keys),
		APIKeyMasks:          masks,
		APIKeyStatuses:       s.apiKeyStatuses(keys),
		TimeoutMS:            cfg.TimeoutMS,
		SampleRate:           cfg.SampleRate,
		AllGroups:            cfg.AllGroups,
		GroupIDs:             append([]int64(nil), cfg.GroupIDs...),
		RecordNonHits:        cfg.RecordNonHits,
		Thresholds:           cloneFloatMap(cfg.Thresholds),
		WorkerCount:          cfg.WorkerCount,
		QueueSize:            cfg.QueueSize,
		BlockStatus:          cfg.BlockStatus,
		BlockMessage:         cfg.BlockMessage,
		EmailOnHit:           cfg.EmailOnHit,
		AutoBanEnabled:       cfg.AutoBanEnabled,
		BanThreshold:         cfg.BanThreshold,
		ViolationWindowHours: cfg.ViolationWindowHours,
		RetryCount:           cfg.RetryCount,
		HitRetentionDays:     cfg.HitRetentionDays,
		NonHitRetentionDays:  cfg.NonHitRetentionDays,
		PreHashCheckEnabled:  cfg.PreHashCheckEnabled,
		BlockedKeywords:      append([]string(nil), cfg.BlockedKeywords...),
		KeywordBlockingMode:  cfg.KeywordBlockingMode,
		ModelFilter:          cloneContentModerationModelFilter(cfg.ModelFilter),
REDACTED
REDACTED

func (s *ContentModerationService) apiKeyStatuses(keys []string) []ContentModerationAPIKeyStatus {
	out := make([]ContentModerationAPIKeyStatus, 0, len(keys))
	for idx, key := range keys {
		out = append(out, s.apiKeyStatusForHash(idx, moderationAPIKeyHash(key), maskSecretTail(key), true))
REDACTED
	return out
REDACTED

func (s *ContentModerationService) preBlockAPIKeyLoads(keys []string) []ContentModerationAPIKeyLoad {
	out := make([]ContentModerationAPIKeyLoad, 0, len(keys))
	for idx, key := range keys {
		out = append(out, s.preBlockAPIKeyLoadForHash(idx, moderationAPIKeyHash(key), maskSecretTail(key)))
REDACTED
	return out
REDACTED

func (s *ContentModerationService) preBlockAPIKeyActive(keys []string) int64 {
	var total int64
	for _, item := range s.preBlockAPIKeyLoads(keys) {
		total += item.Active
REDACTED
	return total
REDACTED

func (s *ContentModerationService) preBlockAPIKeyAvailableCount(keys []string) int64 {
	now := time.Now()
	var count int64
	for _, key := range keys {
		if !s.isAPIKeyFrozen(key, now) {
			count++
	REDACTED
REDACTED
	return count
REDACTED

func (s *ContentModerationService) preBlockAPIKeyTotalCalls(keys []string) int64 {
	var total int64
	for _, item := range s.preBlockAPIKeyLoads(keys) {
		total += item.Total
REDACTED
	return total
REDACTED

func (s *ContentModerationService) preBlockAPIKeyLoadForHash(index int, hash string, masked string) ContentModerationAPIKeyLoad {
	load := ContentModerationAPIKeyLoad{
		Index:   index,
		KeyHash: hash,
		Masked:  masked,
		Status:  "unknown",
REDACTED
	status := s.apiKeyStatusForHash(index, hash, masked, true)
	load.Status = status.Status
	load.LastLatencyMS = status.LastLatencyMS
	load.LastHTTPStatus = status.LastHTTPStatus
	if hash == "" || s == nil {
		return load
REDACTED
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.keyHealth[hash]
	if state == nil {
		return load
REDACTED
	load.Active = state.SyncActive
	load.Total = state.SyncTotal
	load.Success = state.SyncSuccess
	load.Errors = state.SyncErrors
	if state.SyncTotal > 0 {
		load.AvgLatencyMS = state.SyncLatencyMS / state.SyncTotal
REDACTED
	return load
REDACTED

func (s *ContentModerationService) apiKeyStatusForHash(index int, hash string, masked string, configured bool) ContentModerationAPIKeyStatus {
	status := ContentModerationAPIKeyStatus{
		Index:      index,
		KeyHash:    hash,
		Masked:     masked,
		Status:     "unknown",
		Configured: configured,
REDACTED
	if hash == "" || s == nil {
		return status
REDACTED
	now := time.Now()
	s.keyHealthMu.Lock()
	defer s.keyHealthMu.Unlock()
	state := s.keyHealth[hash]
	if state == nil {
		return status
REDACTED
	status.FailureCount = state.FailureCount
	status.SuccessCount = state.SuccessCount
	status.LastError = state.LastError
	status.LastLatencyMS = state.LastLatencyMS
	status.LastHTTPStatus = state.LastHTTPStatus
	status.LastTested = state.LastTested
	if !state.LastCheckedAt.IsZero() {
		t := state.LastCheckedAt
		status.LastCheckedAt = &t
REDACTED
	if state.FrozenUntil.After(now) {
		t := state.FrozenUntil
		status.FrozenUntil = &t
		status.Status = "frozen"
		return status
REDACTED
	if state.LastError != "" {
		status.Status = "error"
		return status
REDACTED
	if state.SuccessCount > 0 || state.LastTested {
		status.Status = "ok"
REDACTED
	return status
REDACTED

func moderationAPIKeyHash(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
REDACTED
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
REDACTED

func buildModerationTestInput(prompt string, images []string) (any, int, error) {
	prompt = trimRunes(normalizeContentModerationText(prompt), maxModerationInputRunes)
	normalizedImages := make([]string, 0, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
	REDACTED
		if len(normalizedImages) >= maxContentModerationTestImages {
			return nil, 0, infraerrors.BadRequest("TOO_MANY_MODERATION_TEST_IMAGES", fmt.Sprintf("最多上传 %d 张测试图片", maxContentModerationTestImages))
	REDACTED
		if err := validateModerationTestImageDataURL(image); err != nil {
			return nil, 0, err
	REDACTED
		normalizedImages = append(normalizedImages, image)
REDACTED
	if prompt == "" && len(normalizedImages) == 0 {
		return "hello", 0, nil
REDACTED
	if len(normalizedImages) == 0 {
		return prompt, 0, nil
REDACTED
	parts := make([]moderationAPIInputPart, 0, len(normalizedImages)+1)
	if prompt != "" {
		parts = append(parts, moderationAPIInputPart{Type: "text", Text: promptREDACTED)
REDACTED
	for _, image := range normalizedImages {
		parts = append(parts, moderationAPIInputPart{
			Type:     "image_url",
			ImageURL: &moderationAPIImageURLRef{URL: imageREDACTED,
	REDACTED)
REDACTED
	return parts, len(normalizedImages), nil
REDACTED

func contentModerationTestHasAuditInput(prompt string, images []string) bool {
	if normalizeContentModerationText(prompt) != "" {
		return true
REDACTED
	for _, image := range images {
		if strings.TrimSpace(image) != "" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func validateModerationTestImageDataURL(value string) error {
	if len(value) > maxContentModerationTestImageDataURLBytes {
		return infraerrors.BadRequest("MODERATION_TEST_IMAGE_TOO_LARGE", "测试图片不能超过 8MB")
REDACTED
	if !strings.HasPrefix(value, "data:image/") {
		return infraerrors.BadRequest("INVALID_MODERATION_TEST_IMAGE", "测试图片必须是 data:image/* base64")
REDACTED
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
		return infraerrors.BadRequest("INVALID_MODERATION_TEST_IMAGE", "测试图片必须是 base64 data URL")
REDACTED
	raw, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return infraerrors.BadRequest("INVALID_MODERATION_TEST_IMAGE", "测试图片 base64 无效")
REDACTED
	if len(raw) > maxContentModerationTestImageBytes {
		return infraerrors.BadRequest("MODERATION_TEST_IMAGE_TOO_LARGE", "测试图片不能超过 8MB")
REDACTED
	return nil
REDACTED

func buildContentModerationTestAuditResult(result *moderationAPIResult, thresholds map[string]float64) *ContentModerationTestAuditResult {
	if result == nil {
		return nil
REDACTED
	scores := make(map[string]float64, len(result.CategoryScores))
	for category, score := range result.CategoryScores {
		scores[category] = score
REDACTED
	thresholdSnapshot := mergeContentModerationThresholds(ContentModerationDefaultThresholds(), thresholds)
	flagged, highestCategory, highestScore := evaluateModerationScores(scores, thresholdSnapshot)
	compositeScore := highestScore
	return &ContentModerationTestAuditResult{
		Flagged:         flagged,
		HighestCategory: highestCategory,
		HighestScore:    highestScore,
		CompositeScore:  compositeScore,
		CategoryScores:  scores,
		Thresholds:      thresholdSnapshot,
REDACTED
REDACTED

type moderationAPIRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"`
REDACTED

type moderationAPIInputPart struct {
	Type     string                    `json:"type"`
	Text     string                    `json:"text,omitempty"`
	ImageURL *moderationAPIImageURLRef `json:"image_url,omitempty"`
REDACTED

type moderationAPIImageURLRef struct {
	URL string `json:"url"`
REDACTED

type moderationAPIResponse struct {
	Results []moderationAPIResult `json:"results"`
REDACTED

type moderationAPIResult struct {
	Flagged        bool               `json:"flagged"`
	CategoryScores map[string]float64 `json:"category_scores"`
REDACTED

func evaluateModerationScores(scores map[string]float64, thresholds map[string]float64) (bool, string, float64) {
	flagged := false
	highestCategory := ""
	highestScore := 0.0
	for _, category := range contentModerationCategoryOrder {
		score := scores[category]
		if score > highestScore || highestCategory == "" {
			highestScore = score
			highestCategory = category
	REDACTED
		if score >= thresholds[category] {
			flagged = true
	REDACTED
REDACTED
	for category, score := range scores {
		if score > highestScore || highestCategory == "" {
			highestScore = score
			highestCategory = category
	REDACTED
REDACTED
	return flagged, highestCategory, highestScore
REDACTED

func mergeContentModerationThresholds(base map[string]float64, override map[string]float64) map[string]float64 {
	out := cloneFloatMap(base)
	if out == nil {
		out = map[string]float64{REDACTED
REDACTED
	for _, category := range contentModerationCategoryOrder {
		if v, ok := override[category]; ok {
			if v < 0 {
				v = 0
		REDACTED
			if v > 1 {
				v = 1
		REDACTED
			out[category] = v
	REDACTED
REDACTED
	return out
REDACTED

func normalizeInt64IDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{REDACTED
REDACTED
	seen := make(map[int64]struct{REDACTED, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
	REDACTED
		if _, ok := seen[id]; ok {
			continue
	REDACTED
		seen[id] = struct{REDACTED{REDACTED
		out = append(out, id)
REDACTED
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] REDACTED)
	return out
REDACTED

func normalizeBlockedKeywords(in []string) []string {
	if len(in) == 0 {
		return []string{REDACTED
REDACTED
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{REDACTED, len(in))
	for _, raw := range in {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
	REDACTED
		kw = trimRunes(kw, maxContentModerationBlockedKeywordRunes)
		key := strings.ToLower(kw)
		if _, ok := seen[key]; ok {
			continue
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		out = append(out, kw)
		if len(out) >= maxContentModerationBlockedKeywords {
			break
	REDACTED
REDACTED
	return out
REDACTED

func normalizeKeywordBlockingMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ContentModerationKeywordModeKeywordOnly:
		return ContentModerationKeywordModeKeywordOnly
	case ContentModerationKeywordModeAPIOnly:
		return ContentModerationKeywordModeAPIOnly
	case ContentModerationKeywordModeKeywordAndAPI:
		return ContentModerationKeywordModeKeywordAndAPI
	default:
		return ContentModerationKeywordModeKeywordAndAPI
REDACTED
REDACTED

func normalizeContentModerationModelFilter(filter ContentModerationModelFilter) ContentModerationModelFilter {
	out := ContentModerationModelFilter{
		Type:   normalizeContentModerationModelFilterType(filter.Type),
		Models: normalizeContentModerationModelNames(filter.Models),
REDACTED
	if out.Type == ContentModerationModelFilterAll {
		out.Models = []string{REDACTED
REDACTED
	return out
REDACTED

func cloneContentModerationModelFilter(filter ContentModerationModelFilter) ContentModerationModelFilter {
	normalized := normalizeContentModerationModelFilter(filter)
	normalized.Models = append([]string(nil), normalized.Models...)
	return normalized
REDACTED

func normalizeContentModerationModelFilterType(filterType string) string {
	switch strings.ToLower(strings.TrimSpace(filterType)) {
	case ContentModerationModelFilterInclude:
		return ContentModerationModelFilterInclude
	case ContentModerationModelFilterExclude:
		return ContentModerationModelFilterExclude
	case ContentModerationModelFilterAll:
		return ContentModerationModelFilterAll
	default:
		return ContentModerationModelFilterAll
REDACTED
REDACTED

func normalizeContentModerationModelNames(models []string) []string {
	if len(models) == 0 {
		return []string{REDACTED
REDACTED
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{REDACTED, len(models))
	for _, raw := range models {
		model := trimRunes(strings.TrimSpace(raw), maxContentModerationModelFilterRunes)
		if model == "" {
			continue
	REDACTED
		key := strings.ToLower(model)
		if _, ok := seen[key]; ok {
			continue
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		out = append(out, model)
		if len(out) >= maxContentModerationModelFilterModels {
			break
	REDACTED
REDACTED
	return out
REDACTED

func contentModerationModelListContains(models []string, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
REDACTED
	for _, candidate := range models {
		if strings.ToLower(strings.TrimSpace(candidate)) == model {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func matchBlockedKeyword(text string, keywords []string) (string, bool) {
	if text == "" || len(keywords) == 0 {
		return "", false
REDACTED
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if kw == "" {
			continue
	REDACTED
		if strings.Contains(lower, strings.ToLower(kw)) {
			return kw, true
	REDACTED
REDACTED
	return "", false
REDACTED

func normalizeModerationAPIKeys(keys []string) []string {
	if len(keys) == 0 {
		return []string{REDACTED
REDACTED
	seen := make(map[string]struct{REDACTED, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
	REDACTED
		if _, ok := seen[key]; ok {
			continue
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		out = append(out, key)
REDACTED
	return out
REDACTED

func deleteModerationAPIKeysByHash(keys []string, hashes []string) []string {
	keys = normalizeModerationAPIKeys(keys)
	deleteHashes := make(map[string]struct{REDACTED, len(hashes))
	for _, hash := range hashes {
		hash = normalizeContentModerationHash(hash)
		if hash != "" {
			deleteHashes[hash] = struct{REDACTED{REDACTED
	REDACTED
REDACTED
	if len(deleteHashes) == 0 {
		return keys
REDACTED
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := deleteHashes[moderationAPIKeyHash(key)]; ok {
			continue
	REDACTED
		out = append(out, key)
REDACTED
	return out
REDACTED

func normalizeContentModerationAPIKeysMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case contentModerationAPIKeysModeReplace:
		return contentModerationAPIKeysModeReplace
	default:
		return contentModerationAPIKeysModeAppend
REDACTED
REDACTED

func normalizeContentModerationHash(inputHash string) string {
	inputHash = strings.ToLower(strings.TrimSpace(inputHash))
	if len(inputHash) != sha256.Size*2 {
		return ""
REDACTED
	if _, err := hex.DecodeString(inputHash); err != nil {
		return ""
REDACTED
	return inputHash
REDACTED

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if in == nil {
		return map[string]float64{REDACTED
REDACTED
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
REDACTED
	return out
REDACTED

func cloneInt64Ptr(in *int64) *int64 {
	if in == nil {
		return nil
REDACTED
	v := *in
	return &v
REDACTED

func trimRunes(text string, max int) string {
	if max <= 0 {
		return ""
REDACTED
	runes := []rune(text)
	if len(runes) <= max {
		return text
REDACTED
	return string(runes[:max])
REDACTED

func maskSecretTail(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
REDACTED
	if len(secret) <= 4 {
		return "****"
REDACTED
	return strings.Repeat("*", 8) + secret[len(secret)-4:]
REDACTED
