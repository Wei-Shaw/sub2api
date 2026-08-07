package service

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"golang.org/x/net/http/httpguts"
)

const openAICodexRoutingHintHeader = "x-codex-routing-hint"

// setOpenAICodexRoutingHint mirrors the Codex backend routing-hint contract for
// OpenAI OAuth requests. The request model must already be the final upstream
// slug and serviceTier must already reflect any local policy rewrite/filter.
func setOpenAICodexRoutingHint(headers http.Header, account *Account, model string, serviceTier string) {
	if headers == nil || account == nil || !account.IsOpenAIOAuth() {
		return
REDACTED

	// OAuth headers are built afresh for every request/dial, but delete first so
	// callers updating a cached header set cannot retain a stale routing hint.
	headers.Del(openAICodexRoutingHintHeader)
	model = strings.TrimSpace(model)
	if model == "" {
		return
REDACTED

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
REDACTED

	hint := "model=" + model
	if canonicalTier != "" {
		hint += ";tier=" + canonicalTier
REDACTED
	if !httpguts.ValidHeaderFieldValue(hint) {
		return
REDACTED
	headers.Set(openAICodexRoutingHintHeader, hint)
REDACTED

func setOpenAICodexRoutingHintFromBody(headers http.Header, account *Account, body []byte) {
	fields := gjson.GetManyBytes(body, "model", "service_tier")
	setOpenAICodexRoutingHint(headers, account, fields[0].String(), fields[1].String())
REDACTED
