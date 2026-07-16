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
}

func buildOpenAICompatAnthropicDigestChain(req *apicompat.AnthropicRequest) string {
	if req == nil {
		return ""
	}

	parts := make([]string, 0, len(req.Messages)+1)
	if len(req.System) > 0 && strings.TrimSpace(string(req.System)) != "" && strings.TrimSpace(string(req.System)) != "null" {
		parts = append(parts, "s:"+shortHash(req.System))
	}
	for _, msg := range req.Messages {
		content := msg.Content
		if len(content) == 0 || strings.TrimSpace(string(content)) == "" {
			continue
		}
		prefix := "u"
		if strings.TrimSpace(msg.Role) == "assistant" {
			prefix = "a"
		}
		parts = append(parts, prefix+":"+shortHash(content))
	}
	return strings.Join(parts, "-")
}

func openAICompatAnthropicDigestNamespace(account *Account, cAPIKeyID int64) string {
	if account == nil || account.ID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d|%d|", account.ID, cAPIKeyID)
}

func (s *OpenAIGatewayService) findOpenAICompatAnthropicDigestPromptCacheKey(account *Account, cAPIKeyID int64, digestChain string) (promptCacheKey string, matchedChain string) {
	if s == nil || digestChain == "" {
		return "", ""
	}
	ns := openAICompatAnthropicDigestNamespace(account, cAPIKeyID)
	if ns == "" {
		return "", ""
	}
	chain := digestChain
	for {
		if raw, ok := s.openaiCompatAnthropicDigestSessions.Load(ns + chain); ok {
			if binding, ok := raw.(openAICompatAnthropicDigestBinding); ok {
				if binding.ExpiresAt.IsZero() || time.Now().Before(binding.ExpiresAt) {
					if key := strings.TrimSpace(binding.PromptCacheKey); key != "" {
						return key, chain
					}
				}
			}
			s.openaiCompatAnthropicDigestSessions.Delete(ns + chain)
		}
		i := strings.LastIndex(chain, "-")
		if i < 0 {
			return "", ""
		}
		chain = chain[:i]
	}
}

func (s *OpenAIGatewayService) bindOpenAICompatAnthropicDigestPromptCacheKey(account *Account, cAPIKeyID int64, digestChain, promptCacheKey, oldDigestChain string) {
	if s == nil || digestChain == "" || strings.TrimSpace(promptCacheKey) == "" {
		return
	}
	ns := openAICompatAnthropicDigestNamespace(account, cAPIKeyID)
	if ns == "" {
		return
	}
	binding := openAICompatAnthropicDigestBinding{
		PromptCacheKey: strings.TrimSpace(promptCacheKey),
		ExpiresAt:      time.Now().Add(s.openAIWSResponseStickyTTL()),
	}
	s.openaiCompatAnthropicDigestSessions.Store(ns+digestChain, binding)
	if oldDigestChain != "" && oldDigestChain != digestChain {
		s.openaiCompatAnthropicDigestSessions.Delete(ns + oldDigestChain)
	}
}

func promptCacheKeyFromAnthropicDigest(digestChain string) string {
	if strings.TrimSpace(digestChain) == "" {
		return ""
	}
	return "anthropic-digest-" + hashSensitiveValueForLog(digestChain)
}

func promptCacheKeyFromAnthropicMetadataSession(req *apicompat.AnthropicRequest) string {
	if req == nil || len(req.Metadata) == 0 {
		return ""
	}
	var metadata struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(req.Metadata, &metadata); err != nil {
		return ""
	}
	parsed := ParseMetadataUserID(metadata.UserID)
	if parsed == nil || strings.TrimSpace(parsed.SessionID) == "" {
		return ""
	}
	seed := strings.Join([]string{
		"anthropic-metadata",
		strings.TrimSpace(parsed.DeviceID),
		strings.TrimSpace(parsed.AccountUUID),
		strings.TrimSpace(parsed.SessionID),
	}, "|")
	return "anthropic-metadata-" + hashSensitiveValueForLog(seed)
}

func cloneAnthropicRequestForDigest(req *apicompat.AnthropicRequest) *apicompat.AnthropicRequest {
	if req == nil {
		return nil
	}
	cp := *req
	if len(req.System) > 0 {
		cp.System = append(json.RawMessage(nil), req.System...)
	}
	if len(req.Messages) > 0 {
		cp.Messages = append([]apicompat.AnthropicMessage(nil), req.Messages...)
	}
	return &cp
}
