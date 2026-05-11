package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type gatewayTTLSettingRepo struct {
	data map[string]string
REDACTED

func (r *gatewayTTLSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
REDACTED

func (r *gatewayTTLSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if r == nil {
		return "", ErrSettingNotFound
REDACTED
	v, ok := r.data[key]
	if !ok {
		return "", ErrSettingNotFound
REDACTED
	return v, nil
REDACTED

func (r *gatewayTTLSettingRepo) Set(_ context.Context, key, value string) error {
	if r == nil {
		return errors.New("setting repo is nil")
REDACTED
	if r.data == nil {
		r.data = map[string]string{REDACTED
REDACTED
	r.data[key] = value
	return nil
REDACTED

func (r *gatewayTTLSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string)
	if r == nil {
		return result, nil
REDACTED
	for _, key := range keys {
		if v, ok := r.data[key]; ok {
			result[key] = v
	REDACTED
REDACTED
	return result, nil
REDACTED

func (r *gatewayTTLSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	if r == nil {
		return errors.New("setting repo is nil")
REDACTED
	if r.data == nil {
		r.data = map[string]string{REDACTED
REDACTED
	for key, value := range settings {
		r.data[key] = value
REDACTED
	return nil
REDACTED

func (r *gatewayTTLSettingRepo) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string)
	if r == nil {
		return result, nil
REDACTED
	for key, value := range r.data {
		result[key] = value
REDACTED
	return result, nil
REDACTED

func (r *gatewayTTLSettingRepo) Delete(_ context.Context, key string) error {
	if r != nil {
		delete(r.data, key)
REDACTED
	return nil
REDACTED

func assertJSONTokenOrder(t *testing.T, body string, tokens ...string) {
REDACTED

	last := -1
	for _, token := range tokens {
		pos := strings.Index(body, token)
		require.NotEqualf(t, -1, pos, "missing token %s in body %s", token, body)
		require.Greaterf(t, pos, last, "token %s should appear after previous tokens in body %s", token, body)
		last = pos
REDACTED
REDACTED

func TestReplaceModelInBody_PreservesTopLevelFieldOrder(t *testing.T) {
	svc := &GatewayService{REDACTED
	body := []byte(`{"alpha":1,"model":"claude-3-5-sonnet-latest","messages":[],"omega":2REDACTED`)

	result := svc.replaceModelInBody(body, "claude-3-5-sonnet-20241022")
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"model"`, `"messages"`, `"omega"`)
	require.Contains(t, resultStr, `"model":"claude-3-5-sonnet-20241022"`)
REDACTED

func TestNormalizeClaudeOAuthRequestBody_PreservesTopLevelFieldOrder(t *testing.T) {
	body := []byte(`{"alpha":1,"model":"claude-3-5-sonnet-latest","temperature":0.2,"system":"You are OpenCode, the best coding agent on the planet.","messages":[],"tool_choice":{"type":"auto"REDACTED,"omega":2REDACTED`)

	result, modelID := normalizeClaudeOAuthRequestBody(body, "claude-3-5-sonnet-latest", claudeOAuthNormalizeOptions{
		injectMetadata: true,
		metadataUserID: "user-1",
REDACTED)
	resultStr := string(result)

	require.Equal(t, claude.NormalizeModelID("claude-3-5-sonnet-latest"), modelID)
	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"model"`, `"temperature"`, `"system"`, `"messages"`, `"omega"`, `"tools"`, `"metadata"`, `"max_tokens"`)
	require.Contains(t, resultStr, `"temperature":0.2`)
	require.NotContains(t, resultStr, `"tool_choice"`)
	require.Contains(t, resultStr, `"system":"`+claudeCodeSystemPrompt+`"`)
	require.Contains(t, resultStr, `"tools":[]`)
	require.Contains(t, resultStr, `"metadata":{"user_id":"user-1"REDACTED`)
	require.Contains(t, resultStr, `"max_tokens":128000`)
REDACTED

func TestInjectClaudeCodePrompt_PreservesFieldOrder(t *testing.T) {
	body := []byte(`{"alpha":1,"system":[{"id":"block-1","type":"text","text":"Custom"REDACTED],"messages":[],"omega":2REDACTED`)

	result := injectClaudeCodePrompt(body, []any{
		map[string]any{"id": "block-1", "type": "text", "text": "Custom"REDACTED,
REDACTED)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"system"`, `"messages"`, `"omega"`)
	require.Contains(t, resultStr, `{"id":"block-1","type":"text","text":"`+claudeCodeSystemPrompt+`\n\nCustom"REDACTED`)
REDACTED

func TestEnforceCacheControlLimit_PreservesTopLevelFieldOrder(t *testing.T) {
	body := []byte(`{"alpha":1,"system":[{"type":"text","text":"s1","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"s2","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"role":"user","content":[{"type":"text","text":"m1","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"m2","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"m3","cache_control":{"type":"ephemeral"REDACTEDREDACTED]REDACTED],"omega":2REDACTED`)

	result := enforceCacheControlLimit(body)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"system"`, `"messages"`, `"omega"`)
	require.Equal(t, 4, strings.Count(resultStr, `"cache_control"`))
REDACTED

func TestEnforceCacheControlLimit_CountsToolsAndPreservesMessageAnchorsFirst(t *testing.T) {
	body := []byte(`{"alpha":1,"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"role":"user","content":[{"type":"text","text":"m1","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"m2","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"m3","cache_control":{"type":"ephemeral"REDACTEDREDACTED]REDACTED],"tools":[{"name":"a","input_schema":{REDACTED,"cache_control":{"type":"ephemeral"REDACTEDREDACTED],"omega":2REDACTED`)

	result := enforceCacheControlLimit(body)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"system"`, `"messages"`, `"tools"`, `"omega"`)
	require.Equal(t, 4, strings.Count(resultStr, `"cache_control"`))
	require.True(t, gjson.GetBytes(result, "system.0.cache_control").Exists())
	require.True(t, gjson.GetBytes(result, "messages.0.content.0.cache_control").Exists())
	require.True(t, gjson.GetBytes(result, "messages.0.content.1.cache_control").Exists())
	require.True(t, gjson.GetBytes(result, "messages.0.content.2.cache_control").Exists())
	require.False(t, gjson.GetBytes(result, "tools.0.cache_control").Exists())
REDACTED

func TestInjectAnthropicCacheControlTTL1h_OnlyUpdatesExistingEphemeralCacheControl(t *testing.T) {
	body := []byte(`{"alpha":1,"cache_control":{"type":"ephemeral"REDACTED,"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral","ttl":"5m"REDACTEDREDACTED,{"type":"text","text":"plain"REDACTED],"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"REDACTEDREDACTED,{"type":"text","text":"non","cache_control":{"type":"persistent","ttl":"5m"REDACTEDREDACTED]REDACTED],"tools":[{"name":"a","input_schema":{REDACTED,"cache_control":{"type":"ephemeral"REDACTEDREDACTED],"omega":2REDACTED`)

	result := injectAnthropicCacheControlTTL1h(body)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"cache_control"`, `"system"`, `"messages"`, `"tools"`, `"omega"`)
	require.Equal(t, "1h", gjson.GetBytes(result, "cache_control.ttl").String())
	require.Equal(t, "1h", gjson.GetBytes(result, "system.0.cache_control.ttl").String())
	require.False(t, gjson.GetBytes(result, "system.1.cache_control").Exists())
	require.Equal(t, "1h", gjson.GetBytes(result, "messages.0.content.0.cache_control.ttl").String())
	require.Equal(t, "5m", gjson.GetBytes(result, "messages.0.content.1.cache_control.ttl").String())
	require.Equal(t, "1h", gjson.GetBytes(result, "tools.0.cache_control.ttl").String())
REDACTED

func TestGatewayCacheTTLGlobalSetting_TargetResolution(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyEnableAnthropicCacheTTL1hInjection: "true",
REDACTEDREDACTED
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{REDACTED)
	svc := &GatewayService{
		settingService: NewSettingService(repo, &config.Config{REDACTED),
REDACTED
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED

	target, ok := svc.resolveCacheTTLUsageOverrideTarget(context.Background(), account)
	require.True(t, ok)
	require.Equal(t, cacheTTLTarget5m, target)

	account.Extra = map[string]any{
		"cache_ttl_override_enabled": true,
		"cache_ttl_override_target":  "1h",
REDACTED
	target, ok = svc.resolveCacheTTLUsageOverrideTarget(context.Background(), account)
	require.True(t, ok)
	require.Equal(t, cacheTTLTarget1h, target)
REDACTED

func TestGatewayCacheTTLGlobalSetting_RequestInjectionScope(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyEnableAnthropicCacheTTL1hInjection: "true",
REDACTEDREDACTED
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{REDACTED)
	svc := &GatewayService{
		settingService: NewSettingService(repo, &config.Config{REDACTED),
REDACTED

	require.True(t, svc.shouldInjectAnthropicCacheTTL1h(context.Background(), &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED))
	require.True(t, svc.shouldInjectAnthropicCacheTTL1h(context.Background(), &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupTokenREDACTED))
	require.False(t, svc.shouldInjectAnthropicCacheTTL1h(context.Background(), &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKeyREDACTED))
	require.False(t, svc.shouldInjectAnthropicCacheTTL1h(context.Background(), &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED))

	repo.data[SettingKeyEnableAnthropicCacheTTL1hInjection] = "false"
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{REDACTED)
	require.False(t, svc.shouldInjectAnthropicCacheTTL1h(context.Background(), &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED))
REDACTED
