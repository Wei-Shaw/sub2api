package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/pagination"
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

	// Gateway protocol identifiers as set by the core GatewayPipeline
	// (backend/internal/gateway/request.go). The host forwards these raw
	// strings on ContentCheckRequest.Protocol; they differ from the plugin's
	// own ContentModerationProtocol* canonical values above, so
	// gatewayProtocolToModeration maps between them. Keep in sync with the
	// gateway's Protocol* constants.
	gatewayProtocolAnthropic          = "anthropic"
	gatewayProtocolAnthropicViaOpenAI = "anthropic_via_openai"
	gatewayProtocolChatCompletions    = "chat_completions"
	gatewayProtocolResponses          = "responses"
	gatewayProtocolOpenAI             = "openai"
	gatewayProtocolGemini             = "gemini"
	gatewayProtocolImages             = "images"

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

	// contentModerationHTTPStatusOverloaded is the non-standard 529 "Site is
	// overloaded" status some upstreams return; it has no net/http constant.
	// Treated like 429 for key-freeze purposes.
	contentModerationHTTPStatusOverloaded = 529

	// contentModerationKeyErrorTextMaxRunes caps the per-key LastError text
	// stored in the in-memory health snapshot.
	contentModerationKeyErrorTextMaxRunes     = 180
	maxContentModerationInputImages           = 1
	maxContentModerationTestImages            = maxContentModerationInputImages
	maxContentModerationTestImageBytes        = 8 * 1024 * 1024
	maxContentModerationTestImageDataURLBytes = 12 * 1024 * 1024
	maxContentModerationBlockedKeywords       = 10000
	maxContentModerationBlockedKeywordRunes   = 200
	maxContentModerationModelFilterModels     = 1000
	maxContentModerationModelFilterRunes      = 200

	contentModerationCleanupInterval = 24 * time.Hour
	contentModerationCleanupTimeout  = 30 * time.Minute
	contentModerationCleanupDelay    = 5 * time.Minute

	// SettingKeyModerationConfig is the plugin-scoped settings key under
	// which the full ContentModerationConfig JSON blob is stored. It
	// replaces the core's SettingKeyContentModerationConfig. The plugin's
	// enabled state (formerly the separate risk_control_enabled global
	// setting) is now carried by ContentModerationConfig.Enabled.
	SettingKeyModerationConfig = "moderation_config"
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
}

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
	}
}

func ContentModerationCategories() []string {
	out := make([]string, len(contentModerationCategoryOrder))
	copy(out, contentModerationCategoryOrder)
	return out
}

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
}

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
}

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
}

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
}

type TestContentModerationAPIKeysInput struct {
	APIKeys   []string `json:"api_keys"`
	BaseURL   string   `json:"base_url"`
	Model     string   `json:"model"`
	TimeoutMS int      `json:"timeout_ms"`
	Prompt    string   `json:"prompt"`
	Images    []string `json:"images"`
}

type TestContentModerationAPIKeysResult struct {
	Items       []ContentModerationAPIKeyStatus   `json:"items"`
	AuditResult *ContentModerationTestAuditResult `json:"audit_result,omitempty"`
	ImageCount  int                               `json:"image_count"`
}

type ContentModerationTestAuditResult struct {
	Flagged         bool               `json:"flagged"`
	HighestCategory string             `json:"highest_category"`
	HighestScore    float64            `json:"highest_score"`
	CompositeScore  float64            `json:"composite_score"`
	CategoryScores  map[string]float64 `json:"category_scores"`
	Thresholds      map[string]float64 `json:"thresholds"`
}

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
}

type ContentModerationModelFilter struct {
	Type   string   `json:"type"`
	Models []string `json:"models"`
}

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
}

type ContentModerationInput struct {
	Text   string
	Images []string
}

func (in *ContentModerationInput) Normalize() {
	if in == nil {
		return
	}
	in.Text = trimRunes(normalizeContentModerationText(in.Text), maxModerationInputRunes)
	in.Images = normalizeModerationImages(in.Images)
}

func (in ContentModerationInput) IsEmpty() bool {
	return strings.TrimSpace(in.Text) == "" && len(in.Images) == 0
}

func (in ContentModerationInput) ModerationInput() any {
	images := limitContentModerationImages(in.Images)
	if len(images) == 0 {
		return in.Text
	}
	parts := make([]moderationAPIInputPart, 0, len(images)+1)
	if strings.TrimSpace(in.Text) != "" {
		parts = append(parts, moderationAPIInputPart{Type: "text", Text: in.Text})
	}
	for _, image := range images {
		parts = append(parts, moderationAPIInputPart{
			Type:     "image_url",
			ImageURL: &moderationAPIImageURLRef{URL: image},
		})
	}
	return parts
}

func (in ContentModerationInput) ExcerptText() string {
	return in.Text
}

func (in ContentModerationInput) Hash() string {
	h := sha256.New()
	_, _ = h.Write([]byte("text:"))
	_, _ = h.Write([]byte(in.Text))
	for _, image := range in.Images {
		imageHash := sha256.Sum256([]byte(image))
		_, _ = h.Write([]byte("\nimage:"))
		_, _ = h.Write([]byte(hex.EncodeToString(imageHash[:])))
	}
	return hex.EncodeToString(h.Sum(nil))
}

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
}

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
}

type ContentModerationLogFilter struct {
	Pagination pagination.PaginationParams
	Result     string
	GroupID    *int64
	Endpoint   string
	Search     string
	From       *time.Time
	To         *time.Time
}

type ContentModerationCleanupResult struct {
	DeletedHit    int64     `json:"deleted_hit"`
	DeletedNonHit int64     `json:"deleted_non_hit"`
	FinishedAt    time.Time `json:"finished_at"`
}

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
}

type ContentModerationUnbanUserResult struct {
	UserID int64  `json:"user_id"`
	Status string `json:"status"`
}

type ContentModerationDeleteHashResult struct {
	InputHash string `json:"input_hash"`
	Deleted   bool   `json:"deleted"`
}

type ContentModerationClearHashesResult struct {
	Deleted int64 `json:"deleted"`
}

// Repository is the SQL persistence surface the service depends on. It is
// implemented in the repository package on top of the SDK's *sql.DB.
type Repository interface {
	CreateLog(ctx context.Context, log *ContentModerationLog) error
	ListLogs(ctx context.Context, filter ContentModerationLogFilter) ([]ContentModerationLog, *pagination.PaginationResult, error)
	CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time) (int, error)
	CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*ContentModerationCleanupResult, error)
	GroupExists(ctx context.Context, groupID int64) (bool, error)
}

// HashCache is the flagged-input-hash cache surface. It is implemented in the
// repository package on top of the SDK's Redis client.
type HashCache interface {
	RecordFlaggedInputHash(ctx context.Context, inputHash string) error
	HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	ClearFlaggedInputHashes(ctx context.Context) (int64, error)
	CountFlaggedInputHashes(ctx context.Context) (int64, error)
}
