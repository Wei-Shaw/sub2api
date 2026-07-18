package middleware

import (
	"math"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// IngressRejectReason identifies expected gateway admission failures that must
// not be treated as operational request errors.
type IngressRejectReason string

const (
	IngressRejectQueryAPIKeyDeprecated  IngressRejectReason = "query_api_key_deprecated"
	IngressRejectAPIKeyRequired         IngressRejectReason = "api_key_required"
	IngressRejectInvalidAPIKey          IngressRejectReason = "invalid_api_key"
	IngressRejectAPIKeyDisabled         IngressRejectReason = "api_key_disabled"
	IngressRejectIPRestricted           IngressRejectReason = "ip_restricted"
	IngressRejectUserInactive           IngressRejectReason = "user_inactive"
	IngressRejectGroupDeleted           IngressRejectReason = "group_deleted"
	IngressRejectGroupDisabled          IngressRejectReason = "group_disabled"
	IngressRejectGroupNotAllowed        IngressRejectReason = "group_not_allowed"
	IngressRejectGroupUnassigned        IngressRejectReason = "group_unassigned"
	IngressRejectInvalidAuthRateLimited IngressRejectReason = "invalid_auth_rate_limited"
	IngressRejectAPIKeyAuthOverloaded   IngressRejectReason = "api_key_auth_overloaded"
)

const ingressRejectReasonContextKey = "ingress_reject_reason"

type IngressRejectRecorder interface {
	RecordIngressReject(reason, routeFamily, protocol, clientIP string, userID, apiKeyID int64)
REDACTED

func invalidAuthClientKey(c *gin.Context) string {
	return normalizeIngressRejectIP(SecurityClientIP(c))
REDACTED

func rejectInvalidAuthAbuse(c *gin.Context, apiKeyService interface {
	CheckInvalidAuthAbuse(string) (time.Duration, bool)
REDACTED) bool {
	if c == nil || apiKeyService == nil {
		return false
REDACTED
	retry, blocked := apiKeyService.CheckInvalidAuthAbuse(invalidAuthClientKey(c))
	if !blocked {
		return false
REDACTED
	retrySeconds := int(math.Ceil(retry.Seconds()))
	if retrySeconds < 1 {
		retrySeconds = 1
REDACTED
	c.Header("Retry-After", strconv.Itoa(retrySeconds))
	MarkIngressRejected(c, IngressRejectInvalidAuthRateLimited)
	return true
REDACTED

func recordInvalidAuthFailure(c *gin.Context, apiKeyService interface {
	RecordInvalidAuthFailure(string)
REDACTED) {
	if c == nil || apiKeyService == nil {
		return
REDACTED
	apiKeyService.RecordInvalidAuthFailure(invalidAuthClientKey(c))
REDACTED

type ingressRejectRecorderHolder struct{ recorder IngressRejectRecorder REDACTED

var activeIngressRejectRecorder atomic.Pointer[ingressRejectRecorderHolder]

func SetIngressRejectRecorder(recorder IngressRejectRecorder) {
	if recorder == nil {
		activeIngressRejectRecorder.Store(nil)
		return
REDACTED
	activeIngressRejectRecorder.Store(&ingressRejectRecorderHolder{recorder: recorderREDACTED)
REDACTED

// MarkIngressRejected marks a request as rejected before gateway admission.
func MarkIngressRejected(c *gin.Context, reason IngressRejectReason) {
	if c == nil || reason == "" {
		return
REDACTED
	c.Set(ingressRejectReasonContextKey, reason)
REDACTED

// GetIngressRejectReason returns the admission rejection reason, if any.
func GetIngressRejectReason(c *gin.Context) (IngressRejectReason, bool) {
	if c == nil {
		return "", false
REDACTED
	value, exists := c.Get(ingressRejectReasonContextKey)
	if !exists {
		return "", false
REDACTED
	reason, ok := value.(IngressRejectReason)
	return reason, ok && reason != ""
REDACTED

func recordIngressReject(c *gin.Context, reason IngressRejectReason) {
	holder := activeIngressRejectRecorder.Load()
	if holder == nil || holder.recorder == nil || c == nil || c.Request == nil {
		return
REDACTED
	routeFamily, protocol := ingressRejectRoute(c.Request.URL.Path)
	clientIP := normalizeIngressRejectIP(SecurityClientIP(c))
	var userID, apiKeyID int64
	if apiKey, ok := GetAPIKeyFromContext(c); ok && apiKey != nil {
		apiKeyID = apiKey.ID
		if apiKey.User != nil {
			userID = apiKey.User.ID
	REDACTED
REDACTED else if apiKey, ok := GetOpsFallbackAPIKey(c); ok && apiKey != nil {
		apiKeyID = apiKey.ID
		if apiKey.User != nil {
			userID = apiKey.User.ID
	REDACTED
REDACTED
	holder.recorder.RecordIngressReject(string(reason), routeFamily, protocol, clientIP, userID, apiKeyID)
REDACTED

func normalizeIngressRejectIP(raw string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return "0.0.0.0"
REDACTED
	addr = addr.Unmap()
	if addr.Is6() {
		return netip.PrefixFrom(addr, 64).Masked().Addr().String()
REDACTED
	return addr.String()
REDACTED

func ingressRejectRoute(path string) (string, string) {
	path = strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasPrefix(path, "/antigravity/v1beta"):
		return "antigravity", "google"
	case strings.HasPrefix(path, "/v1beta"):
		return "gemini", "google"
	case strings.HasPrefix(path, "/backend-api/codex"):
		return "codex", "openai"
	case strings.HasPrefix(path, "/antigravity"):
		return "antigravity", "anthropic"
	case strings.Contains(path, "/messages"):
		return "messages", "anthropic"
	case strings.Contains(path, "/responses"):
		return "responses", "openai"
	case strings.Contains(path, "/chat/completions"):
		return "chat_completions", "openai"
	case strings.Contains(path, "/images"):
		return "images", "openai"
	case strings.Contains(path, "/videos"):
		return "videos", "openai"
	case strings.Contains(path, "/embeddings"):
		return "embeddings", "openai"
	case strings.Contains(path, "/models"):
		return "models", "openai"
	default:
		return "other", "gateway"
REDACTED
REDACTED
