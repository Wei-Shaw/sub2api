package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// GeminiCachedContentNamePrefix 是缓存资源名前缀，网关与上游 AI Studio 一致。
	GeminiCachedContentNamePrefix = "cachedContents/"
	// GeminiCachedContentNotFoundMessage 与上游对不存在、已过期或无权访问的缓存返回的文案一致。
	GeminiCachedContentNotFoundMessage = "CachedContent not found (or permission denied)"
	// GeminiCachedContentNotFoundResponse 是网关对引用不存在缓存统一返回的 403 响应体（与 AI Studio 上游一致）。
	GeminiCachedContentNotFoundResponse = `{"error":{"code":403,"message":"CachedContent not found (or permission denied)","status":"PERMISSION_DENIED"}}`

	geminiCachedContentDefaultTTL          = time.Hour
	geminiCachedContentDefaultMaxTTL       = 24 * time.Hour
	geminiCachedContentDisplayNameMaxRunes = 128
	geminiCachedContentUpstreamTagPrefix   = "s2a-"
	geminiCachedContentResponseMaxBytes    = 4 << 20
	geminiCachedContentUpstreamTimeout     = 5 * time.Minute
	geminiCachedContentOfficialHost        = "generativelanguage.googleapis.com"

	// GeminiCachedContentListDefaultPageSize / GeminiCachedContentListMaxPageSize 约束 list 分页。
	GeminiCachedContentListDefaultPageSize = 50
	GeminiCachedContentListMaxPageSize     = 1000
)

var (
	ErrGeminiCachedContentNotFound = infraerrors.New(http.StatusNotFound, "GEMINI_CACHED_CONTENT_NOT_FOUND", "gemini cached content not found")
	ErrGeminiCachedContentExists   = infraerrors.New(http.StatusConflict, "GEMINI_CACHED_CONTENT_EXISTS", "gemini cached content already exists")

	geminiCachedContentPublicIDPattern  = regexp.MustCompile(`^[a-z0-9]{1,64}$`)
	geminiCachedContentAIStudioName     = regexp.MustCompile(`^cachedContents/[A-Za-z0-9_-]+$`)
	geminiCachedContentVertexName       = regexp.MustCompile(`^projects/[A-Za-z0-9_.:-]+/locations/([a-z0-9-]+)/cachedContents/[A-Za-z0-9_-]+$`)
	geminiCachedContentPublicIDEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)
)

// GeminiCachedContent 是网关对外暴露的显式缓存与上游资源、账号、归属的绑定。
type GeminiCachedContent struct {
	ID       int64
	PublicID string
	UserID   int64
	APIKeyID int64
	GroupID  int64
	// AccountID 是持有上游缓存的账号；引用缓存的请求只能发往该账号。
	AccountID    int64
	UpstreamName string
	// Model 是客户端视角的模型名（渠道映射后，不含 models/ 前缀），引用缓存的请求模型必须与之字面一致。
	Model string
	// UpstreamModel 是创建时发给上游的模型资源名。
	UpstreamModel   string
	DisplayName     string
	TotalTokenCount int64
	ExpireTime      time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// PublicName 返回客户端可见的缓存资源名。
func (c *GeminiCachedContent) PublicName() string {
	return GeminiCachedContentNamePrefix + c.PublicID
}

// UpstreamModelID 返回上游模型资源名的末段（裸模型名），用于计费定价查找。
func (c *GeminiCachedContent) UpstreamModelID() string {
	model := c.UpstreamModel
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	return model
}

// GeminiCachedContentRepository 持久化显式缓存绑定。所有读取都过滤已软删除的记录。
type GeminiCachedContentRepository interface {
	Create(ctx context.Context, record *GeminiCachedContent) error
	GetForOwner(ctx context.Context, apiKeyID int64, publicID string) (*GeminiCachedContent, error)
	// ListForOwner 按 id 倒序返回分组内 expire_time 晚于 activeAt 的记录；beforeID>0 时只返回 id 更小的记录。
	ListForOwner(ctx context.Context, apiKeyID, groupID int64, activeAt time.Time, beforeID int64, limit int) ([]*GeminiCachedContent, error)
	// UpdateExpireTime 原子地写入新的到期时间并返回写入前的值。
	UpdateExpireTime(ctx context.Context, id int64, expireTime time.Time) (time.Time, error)
	SoftDelete(ctx context.Context, id int64) error
}

// IsGeminiExplicitCacheAccountType 只按平台与账号类型判断是否可能支持显式缓存，适用于调度快照账号。
func IsGeminiExplicitCacheAccountType(account *Account) bool {
	if account == nil || account.Platform != PlatformGemini {
		return false
	}
	return account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount
}

// IsGeminiExplicitCacheAccount 判断完整账号是否支持显式缓存：官方地址的 AI Studio API Key 账号、
// 声明上游支持显式缓存的自定义地址 API Key 账号，或 Vertex service account 账号。
func IsGeminiExplicitCacheAccount(account *Account) bool {
	if !IsGeminiExplicitCacheAccountType(account) {
		return false
	}
	if account.Type == AccountTypeServiceAccount {
		return true
	}
	if strings.TrimSpace(account.GetCredential("api_key")) == "" {
		return false
	}
	return isOfficialGeminiAPIBaseURL(account.GetCredential("base_url")) || account.GeminiExplicitCacheUpstreamEnabled()
}

const (
	geminiExplicitCacheUpstreamKey       = "explicit_cache_upstream"
	geminiExplicitCacheUpstreamMaxTTLKey = "explicit_cache_upstream_max_ttl_seconds"
)

// GeminiExplicitCacheUpstreamEnabled 判断 API Key 账号是否声明其上游实现了显式缓存接口，
// 并保证缓存只在创建它的上游账号上使用（本系统同版本网关）。
func (a *Account) GeminiExplicitCacheUpstreamEnabled() bool {
	if a == nil || a.Type != AccountTypeAPIKey || a.Credentials == nil {
		return false
	}
	enabled, _ := a.Credentials[geminiExplicitCacheUpstreamKey].(bool)
	return enabled
}

// GeminiExplicitCacheUpstreamMaxTTL 返回上游网关允许的缓存最长有效期；0 表示未配置。
func (a *Account) GeminiExplicitCacheUpstreamMaxTTL() time.Duration {
	if !a.GeminiExplicitCacheUpstreamEnabled() {
		return 0
	}
	var seconds float64
	switch v := a.Credentials[geminiExplicitCacheUpstreamMaxTTLKey].(type) {
	case float64:
		seconds = v
	case int:
		seconds = float64(v)
	case int64:
		seconds = float64(v)
	case json.Number:
		seconds, _ = v.Float64()
	case string:
		seconds, _ = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// geminiCachedContentAPIBase 返回 API Key 账号缓存接口的上游根地址。
func geminiCachedContentAPIBase(account *Account) string {
	if isOfficialGeminiAPIBaseURL(account.GetCredential("base_url")) {
		return geminicli.AIStudioBaseURL
	}
	return account.GetGeminiBaseURL(geminicli.AIStudioBaseURL)
}

func isOfficialGeminiAPIBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	path := strings.TrimRight(u.Path, "/")
	return strings.EqualFold(u.Scheme, "https") &&
		strings.EqualFold(u.Hostname(), geminiCachedContentOfficialHost) &&
		u.Port() == "" && path == "" && u.RawQuery == ""
}

// NewGeminiCachedContentPublicID 生成 40 位小写字母数字的缓存 ID。
func NewGeminiCachedContentPublicID() (string, error) {
	var b [25]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(geminiCachedContentPublicIDEncoding.EncodeToString(b[:])), nil
}

// ParseGeminiCachedContentPublicID 从 "cachedContents/{id}" 或裸 id 中解析缓存 ID。
func ParseGeminiCachedContentPublicID(name string) (string, bool) {
	id := strings.TrimPrefix(strings.TrimSpace(name), GeminiCachedContentNamePrefix)
	if !geminiCachedContentPublicIDPattern.MatchString(id) {
		return "", false
	}
	return id, true
}

type geminiCachedContentBoundCtxKey struct{}

// WithGeminiCachedContentBound 标记请求引用了显式缓存并已硬绑定到持有账号。
func WithGeminiCachedContentBound(ctx context.Context) context.Context {
	return context.WithValue(ctx, geminiCachedContentBoundCtxKey{}, true)
}

func geminiCachedContentBoundFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	bound, _ := ctx.Value(geminiCachedContentBoundCtxKey{}).(bool)
	return bound
}

// isGeminiBoundCacheUpstreamPassthrough 判断绑定缓存的请求在上游网关账号上收到的错误是否只反映
// 上游持有缓存的那个账号的状态（429 / 5xx），不应据此限流或重试本账号。
func isGeminiBoundCacheUpstreamPassthrough(ctx context.Context, account *Account, statusCode int) bool {
	if statusCode != http.StatusTooManyRequests && statusCode < http.StatusInternalServerError {
		return false
	}
	return account.GeminiExplicitCacheUpstreamEnabled() && geminiCachedContentBoundFromContext(ctx)
}

// GeminiCachedContentReference 是请求体中引用显式缓存的位置。
type GeminiCachedContentReference struct {
	Name  string
	Paths []string
}

var geminiCachedContentReferencePaths = []string{"cachedContent", "generateContentRequest.cachedContent"}

// FindGeminiCachedContentReference 返回 generateContent / countTokens 请求体中引用的缓存名；未引用时返回 nil。
// 引用键重复、使用 snake_case 写法或多个位置引用不同缓存时返回错误，保证校验与改写的引用就是发往上游的引用。
func FindGeminiCachedContentReference(body []byte) (*GeminiCachedContentReference, error) {
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return nil, nil
	}
	if err := checkGeminiCachedContentReferenceKeys(root, "cachedContent", "generateContentRequest"); err != nil {
		return nil, err
	}
	if nested := root.Get("generateContentRequest"); nested.IsObject() {
		if err := checkGeminiCachedContentReferenceKeys(nested, "cachedContent"); err != nil {
			return nil, err
		}
	}
	var ref *GeminiCachedContentReference
	for _, path := range geminiCachedContentReferencePaths {
		value := gjson.GetBytes(body, path)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		if value.Type != gjson.String {
			return nil, errors.New("cachedContent must be a string")
		}
		name := strings.TrimSpace(value.String())
		if name == "" {
			continue
		}
		if ref == nil {
			ref = &GeminiCachedContentReference{Name: name}
		} else if ref.Name != name {
			return nil, errors.New("conflicting cachedContent references")
		}
		ref.Paths = append(ref.Paths, path)
	}
	return ref, nil
}

// checkGeminiCachedContentReferenceKeys 拒绝对象中重复出现的 uniqueKeys 与 snake_case 写法的缓存引用键。
func checkGeminiCachedContentReferenceKeys(obj gjson.Result, uniqueKeys ...string) error {
	seen := make(map[string]bool, len(uniqueKeys))
	var err error
	obj.ForEach(func(key, value gjson.Result) bool {
		name := key.String()
		switch {
		case name == "cached_content":
			err = errors.New("cached_content is not supported, use cachedContent")
		case name == "generate_content_request" && (value.Get("cachedContent").Exists() || value.Get("cached_content").Exists()):
			err = errors.New("generate_content_request is not supported with cachedContent, use generateContentRequest")
		case slices.Contains(uniqueKeys, name):
			if seen[name] {
				err = fmt.Errorf("duplicate %s field", name)
			}
			seen[name] = true
		}
		return err == nil
	})
	return err
}

// RewriteGeminiCachedContentReference 把请求体中的缓存引用改写为上游资源名。
func RewriteGeminiCachedContentReference(body []byte, ref *GeminiCachedContentReference, upstreamName string) ([]byte, error) {
	if ref == nil {
		return body, nil
	}
	out := body
	for _, path := range ref.Paths {
		next, err := sjson.SetBytes(out, path, upstreamName)
		if err != nil {
			return nil, err
		}
		out = next
	}
	return out, nil
}

// GeminiCachedContentCreateRequest 是校验后的创建请求。
type GeminiCachedContentCreateRequest struct {
	// Model 为客户端请求的模型名（不含 models/ 前缀）。
	Model       string
	TTL         time.Duration
	DisplayName string
	// ttlExplicit 表示客户端显式指定了 ttl 或 expireTime。
	ttlExplicit bool
	payload     map[string]any
}

// TTLSeconds 返回发往上游的整秒 TTL。
func (r *GeminiCachedContentCreateRequest) TTLSeconds() int64 {
	return int64(math.Ceil(r.TTL.Seconds()))
}

// TTLForAccount 返回在账号上创建缓存使用的整秒有效期。账号上限小于请求有效期时：
// 未显式指定有效期的请求按账号上限创建，显式指定的返回 false（该账号不可用）。
func (r *GeminiCachedContentCreateRequest) TTLForAccount(account *Account) (time.Duration, bool) {
	ttl := time.Duration(r.TTLSeconds()) * time.Second
	limit := account.GeminiExplicitCacheUpstreamMaxTTL()
	if limit <= 0 || ttl <= limit {
		return ttl, true
	}
	if r.ttlExplicit {
		return 0, false
	}
	return limit, true
}

// ParseGeminiCachedContentCreateRequest 校验创建请求。未指定有效期时使用上游默认的 1 小时（不超过 maxTTL）。
// 数字按原文保留，重新编码后发往上游的内容与请求一致。
func ParseGeminiCachedContentCreateRequest(body []byte, now time.Time, maxTTL time.Duration) (*GeminiCachedContentCreateRequest, error) {
	payload, err := decodeGeminiCachedContentPayload(body)
	if err != nil {
		return nil, err
	}
	rawModel, _ := payload["model"].(string)
	model := strings.TrimPrefix(strings.TrimSpace(rawModel), "models/")
	if model == "" {
		return nil, errors.New("model is required")
	}
	if !IsSafeGeminiModelPathSegment(model) {
		return nil, errors.New("model name must be in the format 'models/{model_name}'")
	}

	ttl, err := parseGeminiCachedContentExpiration(payload, now)
	if err != nil {
		return nil, err
	}
	ttlExplicit := ttl > 0
	if ttl == 0 {
		ttl = geminiCachedContentDefaultTTL
		if maxTTL > 0 && ttl > maxTTL {
			ttl = maxTTL
		}
	}
	if maxTTL > 0 && ttl > maxTTL {
		return nil, fmt.Errorf("ttl exceeds the maximum allowed %ds", int64(maxTTL.Seconds()))
	}

	displayName := ""
	if raw, ok := payload["displayName"]; ok && raw != nil {
		s, ok := raw.(string)
		if !ok {
			return nil, errors.New("displayName must be a string")
		}
		if utf8.RuneCountInString(s) > geminiCachedContentDisplayNameMaxRunes {
			return nil, fmt.Errorf("displayName must be at most %d characters", geminiCachedContentDisplayNameMaxRunes)
		}
		displayName = s
	}

	for _, key := range []string{"name", "model", "displayName", "ttl", "expireTime", "createTime", "updateTime", "usageMetadata"} {
		delete(payload, key)
	}
	return &GeminiCachedContentCreateRequest{Model: model, TTL: ttl, DisplayName: displayName, ttlExplicit: ttlExplicit, payload: payload}, nil
}

// ParseGeminiCachedContentPatchRequest 校验更新请求，只允许修改有效期，返回新的 TTL（自 now 起算）。
func ParseGeminiCachedContentPatchRequest(body []byte, updateMask string, now time.Time, maxTTL time.Duration) (time.Duration, error) {
	payload, err := decodeGeminiCachedContentPayload(body)
	if err != nil {
		return 0, err
	}
	for key := range payload {
		switch key {
		case "ttl", "expireTime", "name":
		default:
			return 0, errors.New("can not update immutable fields (model or display_name)")
		}
	}
	for _, field := range strings.Split(updateMask, ",") {
		switch strings.TrimSpace(field) {
		case "", "ttl", "expireTime", "expire_time":
		default:
			return 0, errors.New("can not update immutable fields (model or display_name)")
		}
	}
	ttl, err := parseGeminiCachedContentExpiration(payload, now)
	if err != nil {
		return 0, err
	}
	if ttl == 0 {
		return 0, errors.New("ttl or expireTime is required")
	}
	if maxTTL > 0 && ttl > maxTTL {
		return 0, fmt.Errorf("ttl exceeds the maximum allowed %ds", int64(maxTTL.Seconds()))
	}
	return ttl, nil
}

// geminiCachedContentFieldAliases 是缓存元数据字段的 snake_case 写法到 camelCase 的映射。
var geminiCachedContentFieldAliases = map[string]string{
	"display_name":   "displayName",
	"expire_time":    "expireTime",
	"create_time":    "createTime",
	"update_time":    "updateTime",
	"usage_metadata": "usageMetadata",
}

// decodeGeminiCachedContentPayload 解码缓存请求体（数字保留原文），并把 snake_case 写法的元数据字段改为 camelCase；
// 同一字段两种写法同时出现时报错。
func decodeGeminiCachedContentPayload(body []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil || payload == nil {
		return nil, errors.New("request body must be a JSON object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("request body must be a JSON object")
	}
	for alias, canonical := range geminiCachedContentFieldAliases {
		value, ok := payload[alias]
		if !ok {
			continue
		}
		if _, dup := payload[canonical]; dup {
			return nil, fmt.Errorf("only one of %s or %s may be set", canonical, alias)
		}
		delete(payload, alias)
		payload[canonical] = value
	}
	return payload, nil
}

// parseGeminiCachedContentExpiration 解析 ttl / expireTime（二选一），未设置时返回 0；结果向上取整到秒，与发往上游的 ttl 一致。
func parseGeminiCachedContentExpiration(payload map[string]any, now time.Time) (time.Duration, error) {
	rawTTL, hasTTL := payload["ttl"]
	rawExpire, hasExpire := payload["expireTime"]
	hasTTL = hasTTL && rawTTL != nil
	hasExpire = hasExpire && rawExpire != nil
	if hasTTL && hasExpire {
		return 0, errors.New("only one of ttl or expireTime may be set")
	}
	if hasTTL {
		s, ok := rawTTL.(string)
		if !ok {
			return 0, errors.New("ttl must be a duration string such as \"3600s\"")
		}
		return parseGoogleDurationSeconds(s)
	}
	if hasExpire {
		s, ok := rawExpire.(string)
		if !ok {
			return 0, errors.New("expireTime must be an RFC 3339 timestamp")
		}
		expire, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
		if err != nil {
			return 0, errors.New("expireTime must be an RFC 3339 timestamp")
		}
		ttl := expire.Sub(now)
		if ttl <= 0 {
			return 0, errors.New("expireTime must be in the future")
		}
		return time.Duration(math.Ceil(ttl.Seconds())) * time.Second, nil
	}
	return 0, nil
}

func parseGoogleDurationSeconds(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasSuffix(raw, "s") {
		return 0, errors.New("ttl must be a duration string such as \"3600s\"")
	}
	seconds, err := strconv.ParseFloat(strings.TrimSuffix(raw, "s"), 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, errors.New("ttl must be a duration string such as \"3600s\"")
	}
	if seconds <= 0 {
		return 0, errors.New("ttl must be positive")
	}
	if seconds > float64(math.MaxInt64/int64(time.Second)) {
		return 0, errors.New("ttl is too large")
	}
	return time.Duration(math.Ceil(seconds)) * time.Second, nil
}

// GeminiCachedContentUpstream 是上游缓存资源的元数据。
type GeminiCachedContentUpstream struct {
	Name            string
	TotalTokenCount int64
	ExpireTime      time.Time
}

// GeminiCachedContentUpstreamError 是上游返回的非 2xx 响应。
type GeminiCachedContentUpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *GeminiCachedContentUpstreamError) Error() string {
	return fmt.Sprintf("gemini cached content upstream error: %d", e.StatusCode)
}

// NotFound 判断上游是否报告缓存不存在（含已过期、已删除）。
func (e *GeminiCachedContentUpstreamError) NotFound() bool {
	return e != nil && isGeminiCachedContentNotFound(e.StatusCode, e.Body)
}

// GeminiCachedContentMalformedResponseError 表示上游返回 2xx 但响应体不可用；Name 非空时上游资源已经创建。
type GeminiCachedContentMalformedResponseError struct {
	Name string
	Err  error
}

func (e *GeminiCachedContentMalformedResponseError) Error() string {
	return "gemini cached content upstream returned an unusable response: " + e.Err.Error()
}

func (e *GeminiCachedContentMalformedResponseError) Unwrap() error {
	return e.Err
}

// RetryableOnOtherAccount 判断创建失败是否值得换一个账号重试（账号凭据、配额或上游可用性问题）。
func (e *GeminiCachedContentUpstreamError) RetryableOnOtherAccount() bool {
	if e == nil {
		return false
	}
	switch e.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	default:
		return e.StatusCode >= 500
	}
}

// GeminiCachedContentService 管理显式缓存的上游资源与网关绑定。
type GeminiCachedContentService struct {
	repo   GeminiCachedContentRepository
	compat *GeminiMessagesCompatService
	cfg    *config.Config
	now    func() time.Time
}

func NewGeminiCachedContentService(repo GeminiCachedContentRepository, compat *GeminiMessagesCompatService, cfg *config.Config) *GeminiCachedContentService {
	return &GeminiCachedContentService{repo: repo, compat: compat, cfg: cfg, now: time.Now}
}

// Now 返回服务时钟。
func (s *GeminiCachedContentService) Now() time.Time {
	return s.now()
}

// MaxTTL 返回单次创建或延长允许的最长有效期。
func (s *GeminiCachedContentService) MaxTTL() time.Duration {
	if s.cfg != nil && s.cfg.Gateway.GeminiCachedContentMaxTTLSeconds > 0 {
		return time.Duration(s.cfg.Gateway.GeminiCachedContentMaxTTLSeconds) * time.Second
	}
	return geminiCachedContentDefaultMaxTTL
}

// MaxTTLForAccount 返回在账号上可设置的最长有效期：全局上限与上游网关上限取较小值。
func (s *GeminiCachedContentService) MaxTTLForAccount(account *Account) time.Duration {
	limit := s.MaxTTL()
	if upstream := account.GeminiExplicitCacheUpstreamMaxTTL(); upstream > 0 && upstream < limit {
		return upstream
	}
	return limit
}

// GeminiCachedContentStorageTokenHours 返回 tokens 个缓存 token 存储 duration 的存储量（token·小时）。
func GeminiCachedContentStorageTokenHours(tokens int64, duration time.Duration) float64 {
	if tokens <= 0 || duration <= 0 {
		return 0
	}
	return float64(tokens) * duration.Hours()
}

// ClampGeminiCachedContentExpire 返回本地记录与计费采用的到期时间：上游回报的到期时间不晚于 limit，缺失时取 limit。
func ClampGeminiCachedContentExpire(upstream, limit time.Time) time.Time {
	if upstream.IsZero() || upstream.After(limit) {
		return limit
	}
	return upstream
}

// Resolve 返回 API Key 名下仍有效的缓存；不存在、不在当前分组或已过期时返回 ErrGeminiCachedContentNotFound。
func (s *GeminiCachedContentService) Resolve(ctx context.Context, apiKeyID int64, groupID *int64, name string) (*GeminiCachedContent, error) {
	publicID, ok := ParseGeminiCachedContentPublicID(name)
	if !ok {
		return nil, ErrGeminiCachedContentNotFound
	}
	record, err := s.repo.GetForOwner(ctx, apiKeyID, publicID)
	if err != nil {
		return nil, err
	}
	if groupID == nil || record.GroupID != *groupID || !record.ExpireTime.After(s.now()) {
		return nil, ErrGeminiCachedContentNotFound
	}
	return record, nil
}

func (s *GeminiCachedContentService) Save(ctx context.Context, record *GeminiCachedContent) error {
	return s.repo.Create(ctx, record)
}

func (s *GeminiCachedContentService) List(ctx context.Context, apiKeyID, groupID int64, beforeID int64, limit int) ([]*GeminiCachedContent, error) {
	return s.repo.ListForOwner(ctx, apiKeyID, groupID, s.now(), beforeID, limit)
}

// UpdateExpireTime 更新到期时间并返回更新前库中的到期时间（并发更新按提交顺序各自拿到自己的前值）。
func (s *GeminiCachedContentService) UpdateExpireTime(ctx context.Context, record *GeminiCachedContent, expireTime time.Time) (time.Time, error) {
	previous, err := s.repo.UpdateExpireTime(ctx, record.ID, expireTime)
	if err != nil {
		return time.Time{}, err
	}
	record.ExpireTime = expireTime
	record.UpdatedAt = s.now()
	return previous, nil
}

func (s *GeminiCachedContentService) Forget(ctx context.Context, record *GeminiCachedContent) error {
	return s.repo.SoftDelete(ctx, record.ID)
}

// BoundAccount 返回持有缓存的账号；账号已删除或不再支持显式缓存时返回 nil。
func (s *GeminiCachedContentService) BoundAccount(ctx context.Context, record *GeminiCachedContent) (*Account, error) {
	if s.compat == nil || s.compat.accountRepo == nil {
		return nil, errors.New("account repository not configured")
	}
	account, err := s.compat.accountRepo.GetByID(ctx, record.AccountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if account == nil || !IsGeminiExplicitCacheAccount(account) {
		return nil, nil
	}
	return account, nil
}

// GeminiCachedContentUpstreamModelResource 返回创建缓存时发往上游的模型资源名。
func GeminiCachedContentUpstreamModelResource(account *Account, mappedModel string) (string, error) {
	switch account.Type {
	case AccountTypeAPIKey:
		return "models/" + mappedModel, nil
	case AccountTypeServiceAccount:
		projectID := strings.TrimSpace(account.VertexProjectID())
		if projectID == "" {
			return "", errors.New("vertex project_id is required")
		}
		location := strings.TrimSpace(account.VertexLocation(mappedModel))
		if location == "" {
			location = vertexDefaultLocation
		}
		if !vertexLocationPattern.MatchString(location) {
			return "", fmt.Errorf("invalid vertex location: %s", location)
		}
		return fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s", projectID, location, mappedModel), nil
	default:
		return "", fmt.Errorf("account type %s does not support explicit context caching", account.Type)
	}
}

// CreateUpstream 在账号上创建上游缓存。
func (s *GeminiCachedContentService) CreateUpstream(ctx context.Context, account *Account, req *GeminiCachedContentCreateRequest, mappedModel, publicID string, ttl time.Duration) (*GeminiCachedContentUpstream, error) {
	upstreamModel, err := GeminiCachedContentUpstreamModelResource(account, mappedModel)
	if err != nil {
		return nil, err
	}
	payload := make(map[string]any, len(req.payload)+3)
	for k, v := range req.payload {
		payload[k] = v
	}
	payload["model"] = upstreamModel
	payload["ttl"] = fmt.Sprintf("%ds", int64(math.Ceil(ttl.Seconds())))
	payload["displayName"] = geminiCachedContentUpstreamTagPrefix + publicID
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	base, err := s.apiBase(account)
	if err != nil {
		return nil, err
	}
	collectionURL, err := geminiCachedContentCollectionURL(account, base, mappedModel)
	if err != nil {
		return nil, err
	}
	respBody, err := s.doUpstream(ctx, account, http.MethodPost, collectionURL, body)
	if err != nil {
		return nil, err
	}
	upstream, err := parseGeminiCachedContentUpstream(respBody)
	if err != nil {
		return nil, err
	}
	if upstream.TotalTokenCount <= 0 {
		return nil, &GeminiCachedContentMalformedResponseError{Name: upstream.Name, Err: errors.New("missing usageMetadata.totalTokenCount")}
	}
	return upstream, nil
}

// PatchUpstream 把上游缓存的有效期改为自现在起 ttl。
func (s *GeminiCachedContentService) PatchUpstream(ctx context.Context, account *Account, record *GeminiCachedContent, ttl time.Duration) (*GeminiCachedContentUpstream, error) {
	base, err := s.apiBase(account)
	if err != nil {
		return nil, err
	}
	resourceURL, err := geminiCachedContentResourceURL(account, base, record.UpstreamName)
	if err != nil {
		return nil, err
	}
	seconds := int64(math.Ceil(ttl.Seconds()))
	body, _ := json.Marshal(map[string]any{"ttl": fmt.Sprintf("%ds", seconds)})
	respBody, err := s.doUpstream(ctx, account, http.MethodPatch, resourceURL+"?updateMask=ttl", body)
	if err != nil {
		return nil, err
	}
	out := &GeminiCachedContentUpstream{Name: record.UpstreamName}
	if raw := gjson.GetBytes(respBody, "expireTime").String(); raw != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			out.ExpireTime = t
		}
	}
	return out, nil
}

// DeleteUpstream 删除上游缓存。
func (s *GeminiCachedContentService) DeleteUpstream(ctx context.Context, account *Account, record *GeminiCachedContent) error {
	base, err := s.apiBase(account)
	if err != nil {
		return err
	}
	resourceURL, err := geminiCachedContentResourceURL(account, base, record.UpstreamName)
	if err != nil {
		return err
	}
	_, err = s.doUpstream(ctx, account, http.MethodDelete, resourceURL, nil)
	return err
}

// doUpstream 调用上游缓存接口并返回 2xx 响应体。上游调用与客户端连接脱钩：客户端断开不会中途取消
// 已发出的创建 / 修改，调用方据结果完成落库、计费或清理。
func (s *GeminiCachedContentService) doUpstream(ctx context.Context, account *Account, method, target string, body []byte) ([]byte, error) {
	if s.compat == nil || s.compat.httpUpstream == nil {
		return nil, errors.New("gemini upstream client not configured")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), geminiCachedContentUpstreamTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch account.Type {
	case AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, errors.New("gemini api_key not configured")
		}
		req.Header.Set("x-goog-api-key", apiKey)
	case AccountTypeServiceAccount:
		if s.compat.tokenProvider == nil {
			return nil, errors.New("gemini token provider not configured")
		}
		accessToken, err := s.compat.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	default:
		return nil, fmt.Errorf("account type %s does not support explicit context caching", account.Type)
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.compat.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, geminiCachedContentResponseMaxBytes))
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if err != nil {
		if success {
			return nil, &GeminiCachedContentMalformedResponseError{Err: err}
		}
		return nil, err
	}
	if !success {
		return nil, &GeminiCachedContentUpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}
	return respBody, nil
}

// apiBase 返回 API Key 账号经地址校验后的上游根地址；Vertex 账号返回空串（地址由区域决定）。
func (s *GeminiCachedContentService) apiBase(account *Account) (string, error) {
	if account.Type != AccountTypeAPIKey {
		return "", nil
	}
	if s.compat == nil {
		return "", errors.New("gemini upstream client not configured")
	}
	return s.compat.validateUpstreamBaseURL(geminiCachedContentAPIBase(account))
}

func parseGeminiCachedContentUpstream(body []byte) (*GeminiCachedContentUpstream, error) {
	name := strings.TrimSpace(gjson.GetBytes(body, "name").String())
	if name == "" {
		return nil, &GeminiCachedContentMalformedResponseError{Err: errors.New("missing name")}
	}
	if !geminiCachedContentAIStudioName.MatchString(name) && !geminiCachedContentVertexName.MatchString(name) {
		return nil, &GeminiCachedContentMalformedResponseError{Err: fmt.Errorf("unexpected name %q", name)}
	}
	out := &GeminiCachedContentUpstream{
		Name:            name,
		TotalTokenCount: gjson.GetBytes(body, "usageMetadata.totalTokenCount").Int(),
	}
	if raw := gjson.GetBytes(body, "expireTime").String(); raw != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			out.ExpireTime = t
		}
	}
	return out, nil
}

func geminiCachedContentCollectionURL(account *Account, base, mappedModel string) (string, error) {
	switch account.Type {
	case AccountTypeAPIKey:
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base == "" {
			return "", errors.New("gemini base url is required")
		}
		return base + "/v1beta/cachedContents", nil
	case AccountTypeServiceAccount:
		projectID := strings.TrimSpace(account.VertexProjectID())
		if projectID == "" {
			return "", errors.New("vertex project_id is required")
		}
		location := strings.TrimSpace(account.VertexLocation(mappedModel))
		if location == "" {
			location = vertexDefaultLocation
		}
		if !vertexLocationPattern.MatchString(location) {
			return "", fmt.Errorf("invalid vertex location: %s", location)
		}
		return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/cachedContents",
			geminiCachedContentVertexHost(location), url.PathEscape(projectID), url.PathEscape(location)), nil
	default:
		return "", fmt.Errorf("account type %s does not support explicit context caching", account.Type)
	}
}

func geminiCachedContentResourceURL(account *Account, base, upstreamName string) (string, error) {
	switch account.Type {
	case AccountTypeAPIKey:
		if !geminiCachedContentAIStudioName.MatchString(upstreamName) {
			return "", fmt.Errorf("invalid upstream cached content name: %s", upstreamName)
		}
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if base == "" {
			return "", errors.New("gemini base url is required")
		}
		return base + "/v1beta/" + upstreamName, nil
	case AccountTypeServiceAccount:
		m := geminiCachedContentVertexName.FindStringSubmatch(upstreamName)
		if m == nil {
			return "", fmt.Errorf("invalid upstream cached content name: %s", upstreamName)
		}
		return fmt.Sprintf("https://%s/v1/%s", geminiCachedContentVertexHost(m[1]), upstreamName), nil
	default:
		return "", fmt.Errorf("account type %s does not support explicit context caching", account.Type)
	}
}

func geminiCachedContentVertexHost(location string) string {
	if location == "global" {
		return "aiplatform.googleapis.com"
	}
	return location + "-aiplatform.googleapis.com"
}

// GeminiCachedContentView 返回客户端可见的缓存元数据。
func GeminiCachedContentView(record *GeminiCachedContent) map[string]any {
	view := map[string]any{
		"name":          record.PublicName(),
		"model":         "models/" + record.Model,
		"usageMetadata": map[string]any{"totalTokenCount": record.TotalTokenCount},
		"createTime":    record.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updateTime":    record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"expireTime":    record.ExpireTime.UTC().Format(time.RFC3339Nano),
	}
	if record.DisplayName != "" {
		view["displayName"] = record.DisplayName
	}
	return view
}

// GeminiExplicitCacheExclusions 返回显式缓存请求在 gemini 分组内需要排除的可调度账号。
// boundAccountID > 0 时只保留该账号；否则排除平台或类型不支持显式缓存的账号
// （调度快照不含 base_url，官方地址判定在选中后对完整账号进行）。
func (s *GatewayService) GeminiExplicitCacheExclusions(ctx context.Context, groupID *int64, boundAccountID int64) (map[int64]struct{}, error) {
	accounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformGemini, false)
	if err != nil {
		return nil, err
	}
	excluded := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		keep := IsGeminiExplicitCacheAccountType(account)
		if boundAccountID > 0 {
			keep = account.ID == boundAccountID
		}
		if !keep {
			excluded[account.ID] = struct{}{}
		}
	}
	return excluded, nil
}
