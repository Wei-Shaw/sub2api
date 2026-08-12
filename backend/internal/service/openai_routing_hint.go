package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"golang.org/x/net/http/httpguts"
)

const openAICodexRoutingHintHeader = "x-codex-routing-hint"

// setOpenAICodexRoutingHint mirrors the Codex backend routing-hint contract for
// OpenAI OAuth requests. The request model must already be the final upstream
// slug and serviceTier must already reflect any local policy rewrite/filter.
func setOpenAICodexRoutingHint(headers http.Header, account *Account, model string, serviceTier string) {
	if headers == nil {
		return
	}

	// The routing hint is gateway-owned. Strip every spelling before deciding
	// whether to synthesize it so API-key/provider credential paths cannot pass
	// through a caller- or account-override-supplied hint. http.Header.Del only
	// removes the canonical map key; inbound maps can contain raw lowercase keys.
	deleteOpenAIHeaderEqualFold(headers, openAICodexRoutingHintHeader)
	if account == nil || !account.IsOpenAIOAuth() {
		return
	}

	model = strings.TrimSpace(model)
	if model == "" || strings.ContainsAny(model, ";=") {
		return
	}

	// Codex treats "default" as an explicit standard-routing sentinel, not as a
	// service tier sent to the backend. Fast follows the gateway's existing
	// canonicalization and therefore becomes "priority"; flex stays "flex".
	canonicalTier := normalizedOpenAIServiceTierValue(serviceTier)
	// This backport has no Codex model-catalog snapshot with which to validate
	// arbitrary tier ids. Keep the hint to the two effective tiers Codex itself
	// selects; default, missing, and other gateway-compatible API values remain
	// model-only rather than expanding the ChatGPT routing protocol here.
	switch canonicalTier {
	case OpenAIFastTierPriority, OpenAIFastTierFlex:
	default:
		canonicalTier = ""
	}

	hint := "model=" + model
	if canonicalTier != "" {
		hint += ";tier=" + canonicalTier
	}
	if !httpguts.ValidHeaderFieldValue(hint) {
		return
	}
	headers.Set(openAICodexRoutingHintHeader, hint)
}

func deleteOpenAIHeaderEqualFold(headers http.Header, name string) {
	if headers == nil {
		return
	}
	name = strings.TrimSpace(name)
	for key := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			delete(headers, key)
		}
	}
}

func setOpenAICodexRoutingHintFromBody(headers http.Header, account *Account, body []byte) {
	fields := gjson.GetManyBytes(body, "model", "service_tier")
	setOpenAICodexRoutingHint(headers, account, fields[0].String(), fields[1].String())
}

// logOpenAIRoutingDiagnostics records only gateway-derived routing state. In
// particular, it deliberately does not include any header values, tokens, or
// credentials because these diagnostics run on authentication-bearing paths.
func logOpenAIRoutingDiagnostics(
	ctx context.Context,
	account *Account,
	transport string,
	model string,
	serviceTier string,
	hintGenerated bool,
	wsAffinityDecision string,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}

	logger.FromContext(ctx).Debug("openai routing decision",
		zap.String("component", "service.openai_routing"),
		zap.String("transport", strings.TrimSpace(transport)),
		zap.Int64("account_id", accountID),
		zap.String("final_model", strings.TrimSpace(model)),
		zap.String("final_service_tier", normalizedOpenAIServiceTierValue(serviceTier)),
		zap.Bool("routing_hint_generated", hintGenerated),
		zap.String("ws_affinity_decision", strings.TrimSpace(wsAffinityDecision)),
	)
}

func logOpenAIRoutingDiagnosticsFromBody(
	ctx context.Context,
	account *Account,
	transport string,
	headers http.Header,
	body []byte,
	wsAffinityDecision string,
) {
	fields := gjson.GetManyBytes(body, "model", "service_tier")
	logOpenAIRoutingDiagnostics(
		ctx,
		account,
		transport,
		fields[0].String(),
		fields[1].String(),
		strings.TrimSpace(headers.Get(openAICodexRoutingHintHeader)) != "",
		wsAffinityDecision,
	)
}
