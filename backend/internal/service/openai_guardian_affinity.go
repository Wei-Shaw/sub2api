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

type openAIGuardianParentAffinityContextKey struct{}

type openAIGuardianParentAffinity struct {
	currentSessionHash string
	legacySessionHash  string
}

// WithOpenAIGuardianParentAffinity records a Codex review request's parent
// thread as a routing hint. The hint is resolved against the current group's
// sticky-session namespace later; client headers never carry an account ID.
func WithOpenAIGuardianParentAffinity(ctx context.Context, c *gin.Context, body []byte, requestedModel string) context.Context {
	if ctx == nil || c == nil || !strings.EqualFold(strings.TrimSpace(requestedModel), codexAutoReviewModel) {
		return ctx
	}

	headerMetadata := c.GetHeader(codexTurnMetadataHeader)
	bodyMetadata := openAIRequestPayloadView(body).Get("client_metadata.x-codex-turn-metadata").String()
	if !hasUnambiguousOpenAICodexReviewSubagent(
		c.GetHeader(openAISubagentHeader),
		codexSubagentKindFromMetadata(headerMetadata),
		codexSubagentKindFromMetadata(bodyMetadata),
	) {
		return ctx
	}

	parentID := ""
	for _, candidate := range []string{
		strings.TrimSpace(c.GetHeader(codexParentThreadIDHeader)),
		codexParentThreadIDFromMetadata(headerMetadata),
		codexParentThreadIDFromMetadata(bodyMetadata),
	} {
		if candidate == "" {
			continue
		}
		if parentID != "" && parentID != candidate {
			return ctx
		}
		parentID = candidate
	}
	if parentID == "" {
		return ctx
	}

	currentHash, legacyHash := deriveOpenAISessionHashes(parentID)
	if currentHash == "" {
		return ctx
	}
	return context.WithValue(ctx, openAIGuardianParentAffinityContextKey{}, openAIGuardianParentAffinity{
		currentSessionHash: currentHash,
		legacySessionHash:  legacyHash,
	})
}

func codexParentThreadIDFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return ""
	}
	return strings.TrimSpace(gjson.Get(raw, "parent_thread_id").String())
}

func codexSubagentKindFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !gjson.Valid(raw) {
		return ""
	}
	return strings.TrimSpace(gjson.Get(raw, "subagent_kind").String())
}

func hasUnambiguousOpenAICodexReviewSubagent(candidates ...string) bool {
	subagent := ""
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if subagent != "" && subagent != candidate {
			return false
		}
		subagent = candidate
	}
	return subagent == "guardian" || subagent == "review"
}

func openAIGuardianParentAffinityFromContext(ctx context.Context) (openAIGuardianParentAffinity, bool) {
	if ctx == nil {
		return openAIGuardianParentAffinity{}, false
	}
	affinity, ok := ctx.Value(openAIGuardianParentAffinityContextKey{}).(openAIGuardianParentAffinity)
	return affinity, ok && affinity.currentSessionHash != ""
}

func preserveOpenAIGuardianParentBinding(ctx context.Context, sessionHash string) bool {
	affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
	if !ok {
		return false
	}
	sessionHash = strings.TrimSpace(sessionHash)
	return sessionHash != "" && (sessionHash == affinity.currentSessionHash || sessionHash == affinity.legacySessionHash)
}

func (s *OpenAIGatewayService) resolveOpenAIGuardianParentAccountID(ctx context.Context, groupID *int64) int64 {
	if s == nil || s.cache == nil {
		return 0
	}
	affinity, ok := openAIGuardianParentAffinityFromContext(ctx)
	if !ok {
		return 0
	}
	lookupCtx := withOpenAILegacySessionHash(ctx, affinity.legacySessionHash)
	accountID, err := s.getStickySessionAccountID(lookupCtx, groupID, affinity.currentSessionHash)
	if err != nil || accountID <= 0 {
		return 0
	}
	return accountID
}
