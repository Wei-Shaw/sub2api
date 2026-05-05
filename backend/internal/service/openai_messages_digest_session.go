package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type openAICompatAnthropicDigestBinding struct {
	PromptCacheKey string
	ExpiresAt      time.Time
REDACTED

func buildOpenAICompatAnthropicDigestChain(req *apicompat.AnthropicRequest) string {
	if req == nil {
		return ""
REDACTED

	parts := make([]string, 0, len(req.Messages)+1)
	if len(req.System) > 0 && strings.TrimSpace(string(req.System)) != "" && strings.TrimSpace(string(req.System)) != "null" {
		parts = append(parts, "s:"+shortHash(req.System))
REDACTED
	for _, msg := range req.Messages {
		content := msg.Content
		if len(content) == 0 || strings.TrimSpace(string(content)) == "" {
			continue
	REDACTED
		prefix := "u"
		if strings.TrimSpace(msg.Role) == "assistant" {
			prefix = "a"
	REDACTED
		parts = append(parts, prefix+":"+shortHash(content))
REDACTED
	return strings.Join(parts, "-")
REDACTED

func openAICompatAnthropicDigestNamespace(account *Account, cAPIKeyID int64) string {
	if account == nil || account.ID <= 0 {
		return ""
REDACTED
	return fmt.Sprintf("%d|%d|", account.ID, cAPIKeyID)
REDACTED

func (s *OpenAIGatewayService) findOpenAICompatAnthropicDigestPromptCacheKey(account *Account, cAPIKeyID int64, digestChain string) (promptCacheKey string, matchedChain string) {
	if s == nil || digestChain == "" {
		return "", ""
REDACTED
	ns := openAICompatAnthropicDigestNamespace(account, cAPIKeyID)
	if ns == "" {
		return "", ""
REDACTED
	chain := digestChain
	for {
		if raw, ok := s.openaiCompatAnthropicDigestSessions.Load(ns + chain); ok {
			if binding, ok := raw.(openAICompatAnthropicDigestBinding); ok {
				if binding.ExpiresAt.IsZero() || time.Now().Before(binding.ExpiresAt) {
					if key := strings.TrimSpace(binding.PromptCacheKey); key != "" {
						return key, chain
				REDACTED
			REDACTED
		REDACTED
			s.openaiCompatAnthropicDigestSessions.Delete(ns + chain)
	REDACTED
		i := strings.LastIndex(chain, "-")
		if i < 0 {
			return "", ""
	REDACTED
		chain = chain[:i]
REDACTED
REDACTED

func (s *OpenAIGatewayService) bindOpenAICompatAnthropicDigestPromptCacheKey(account *Account, cAPIKeyID int64, digestChain, promptCacheKey, oldDigestChain string) {
	if s == nil || digestChain == "" || strings.TrimSpace(promptCacheKey) == "" {
		return
REDACTED
	ns := openAICompatAnthropicDigestNamespace(account, cAPIKeyID)
	if ns == "" {
		return
REDACTED
	binding := openAICompatAnthropicDigestBinding{
		PromptCacheKey: strings.TrimSpace(promptCacheKey),
		ExpiresAt:      time.Now().Add(s.openAIWSResponseStickyTTL()),
REDACTED
	s.openaiCompatAnthropicDigestSessions.Store(ns+digestChain, binding)
	if oldDigestChain != "" && oldDigestChain != digestChain {
		s.openaiCompatAnthropicDigestSessions.Delete(ns + oldDigestChain)
REDACTED
REDACTED

func promptCacheKeyFromAnthropicDigest(digestChain string) string {
	if strings.TrimSpace(digestChain) == "" {
		return ""
REDACTED
	return "anthropic-digest-" + hashSensitiveValueForLog(digestChain)
REDACTED

func promptCacheKeyFromAnthropicMetadataSession(req *apicompat.AnthropicRequest) string {
	if req == nil || len(req.Metadata) == 0 {
		return ""
REDACTED
	var metadata struct {
		UserID string `json:"user_id"`
REDACTED
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return ""
REDACTED
	parsed := ParseMetadataUserID(metadata.UserID)
	if parsed == nil || strings.TrimSpace(parsed.SessionID) == "" {
		return ""
REDACTED
	seed := strings.Join([]string{
		"anthropic-metadata",
		strings.TrimSpace(parsed.DeviceID),
		strings.TrimSpace(parsed.AccountUUID),
		strings.TrimSpace(parsed.SessionID),
REDACTED, "|")
	return "anthropic-metadata-" + hashSensitiveValueForLog(seed)
REDACTED

func cloneAnthropicRequestForDigest(req *apicompat.AnthropicRequest) *apicompat.AnthropicRequest {
	if req == nil {
		return nil
REDACTED
	cp := *req
	if len(req.System) > 0 {
		cp.System = append(json.RawMessage(nil), req.System...)
REDACTED
	if len(req.Messages) > 0 {
		cp.Messages = append([]apicompat.AnthropicMessage(nil), req.Messages...)
REDACTED
	return &cp
REDACTED
