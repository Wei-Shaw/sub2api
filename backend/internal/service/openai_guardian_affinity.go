package service

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	codexAutoReviewModel      = "codex-auto-review"
	openAISubagentHeader      = "x-openai-subagent"
	codexParentThreadIDHeader = "x-codex-parent-thread-id"
	codexTurnMetadataHeader   = "x-codex-turn-metadata"
)

type openAIGuardianParentAffinityContextKey struct{REDACTED

type openAIGuardianParentAffinity struct {
	currentSessionHash string
	legacySessionHash  string
REDACTED

// WithOpenAIGuardianParentAffinity records a Codex review request's parent
// thread as a routing hint. The hint is resolved against the current group's
// sticky-session namespace later; client headers never carry an account ID.
func WithOpenAIGuardianParentAffinity(ctx context.Context, c *gin.Context, body []byte, requestedModel string) context.Context {
	if ctx == nil || c == nil || !strings.EqualFold(strings.TrimSpace(requestedModel), codexAutoReviewModel) {
		return ctx
REDACTED

	headerMetadata := c.GetHeader(codexTurnMetadataHeader)
	bodyMetadata := openAIRequestPayloadView(body).Get("client_metadata.x-codex-turn-metadata").String()
	if !hasUnambiguousOpenAICodexReviewSubagent(
		c.GetHeader(openAISubagentHeader),
		codexSubagentKindFromMetadata(headerMetadata),
		codexSubagentKindFromMetadata(bodyMetadata),
	) {
		return ctx
REDACTED

	parentID := ""
	for _, candidate := range []string{
		strings.TrimSpace(c.GetHeader(codexParentThreadIDHeader)),
		codexParentThreadIDFromMetadata(headerMetadata),
		codexParentThreadIDFromMetadata(bodyMetadata),
REDACTED {
		if candidate == "" {
			continue
	REDACTED
		if parentID != "" && parentID != candidate {
			return ctx
	REDACTED
		parentID = candidate
REDACTED
	if parentID == "" {
		return ctx
REDACTED

	currentHash, legacyHash := deriveOpenAISessionHashes(parentID)
	if currentHash == "" {
		return ctx
REDACTED
	return context.WithValue(ctx, openAIGuardianParentAffinityContextKey{REDACTED, openAIGuardianParentAffinity{
		currentSessionHash: currentHash,
		legacySessionHash:  legacyHash,
REDACTED)
REDACTED

func codexParentThreadIDFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return ""
REDACTED
	return strings.TrimSpace(gjson.Get(raw, "parent_thread_id").String())
REDACTED

func codexSubagentKindFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return ""
REDACTED
	return strings.TrimSpace(gjson.Get(raw, "subagent_kind").String())
REDACTED

func hasUnambiguousOpenAICodexReviewSubagent(candidates ...string) bool {
	subagent := ""
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
	REDACTED
		if subagent != "" && subagent != candidate {
			return false
	REDACTED
		subagent = candidate
REDACTED
	return subagent == "guardian" || subagent == "review"
REDACTED

func openAIGuardianParentAffinityFromContext(ctx context.Context) (openAIGuardianParentAffinity, bool) {
	if ctx == nil {
		return openAIGuardianParentAffinity{REDACTED, false
REDACTED
	affinity, ok := ctx.Value(openAIGuardianParentAffinityContextKey{REDACTED).(openAIGuardianParentAffinity)
	return affinity, ok && affinity.currentSessionHash != ""
REDACTED

func preserveOpenAIGuardianParentBinding(ctx context.Context, sessionHash string) bool {
	affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
	if !ok {
		return false
REDACTED
	sessionHash = strings.TrimSpace(sessionHash)
	return sessionHash != "" && (sessionHash == affinity.currentSessionHash || sessionHash == affinity.legacySessionHash)
REDACTED

func (s *OpenAIGatewayService) resolveOpenAIGuardianParentAccountID(ctx context.Context, groupID *int64) int64 {
	if s == nil || s.cache == nil {
		return 0
REDACTED
	affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
	if !ok {
		return 0
REDACTED
	lookupCtx := withOpenAILegacySessionHash(ctx, affinity.legacySessionHash)
	accountID, err := s.getStickySessionAccountID(lookupCtx, groupID, affinity.currentSessionHash)
	if err != nil || accountID <= 0 {
		return 0
REDACTED
	return accountID
REDACTED
