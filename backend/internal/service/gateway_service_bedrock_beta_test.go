package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type betaPolicySettingRepoStub struct {
	values map[string]string
REDACTED

func (s *betaPolicySettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
REDACTED

func (s *betaPolicySettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
REDACTED
	return "", ErrSettingNotFound
REDACTED

func (s *betaPolicySettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
REDACTED

func (s *betaPolicySettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
REDACTED

func (s *betaPolicySettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
REDACTED

func (s *betaPolicySettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
REDACTED

func (s *betaPolicySettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
REDACTED

func TestResolveBedrockBetaTokensForRequest_BlocksOnOriginalAnthropicToken(t *testing.T) {
	settings := &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken:    "advanced-tool-use-2025-11-20",
				Action:       BetaPolicyActionBlock,
				Scope:        BetaPolicyScopeAll,
				ErrorMessage: "advanced tool use is blocked",
		REDACTED,
	REDACTED,
REDACTED
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
REDACTED

	svc := &GatewayService{
		settingService: NewSettingService(
			&betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
	REDACTED
			&config.Config{REDACTED,
		),
REDACTED
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrockREDACTED

	_, err = svc.resolveBedrockBetaTokensForRequest(
		context.Background(),
		account,
		"advanced-tool-use-2025-11-20",
		[]byte(`{"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
		"us.anthropic.claude-opus-4-6-v1",
	)
	if err == nil {
		t.Fatal("expected raw advanced-tool-use token to be blocked before Bedrock transform")
REDACTED
	if err.Error() != "advanced tool use is blocked" {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestResolveBedrockBetaTokensForRequest_FiltersAfterBedrockTransform(t *testing.T) {
	settings := &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken: "tool-search-tool-2025-10-19",
				Action:    BetaPolicyActionFilter,
				Scope:     BetaPolicyScopeAll,
		REDACTED,
	REDACTED,
REDACTED
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
REDACTED

	svc := &GatewayService{
		settingService: NewSettingService(
			&betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
	REDACTED
			&config.Config{REDACTED,
		),
REDACTED
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrockREDACTED

	betaTokens, err := svc.resolveBedrockBetaTokensForRequest(
		context.Background(),
		account,
		"advanced-tool-use-2025-11-20",
		[]byte(`{"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
		"us.anthropic.claude-opus-4-6-v1",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
REDACTED
	for _, token := range betaTokens {
		if token == "tool-search-tool-2025-10-19" {
			t.Fatalf("expected transformed Bedrock token to be filtered")
	REDACTED
REDACTED
REDACTED

// TestResolveBedrockBetaTokensForRequest_BlocksBodyAutoInjectedComputerUse 验证：
// 管理员 block 了 computer-use，客户端不在 header 中带该 token，
// 但请求体包含 computer_use 工具 → 自动注入后应被 block。
func TestResolveBedrockBetaTokensForRequest_BlocksBodyAutoInjectedComputerUse(t *testing.T) {
	settings := &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken:    "computer-use-2025-11-24",
				Action:       BetaPolicyActionBlock,
				Scope:        BetaPolicyScopeAll,
				ErrorMessage: "computer use is blocked",
		REDACTED,
	REDACTED,
REDACTED
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
REDACTED

	svc := &GatewayService{
		settingService: NewSettingService(
			&betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
	REDACTED
			&config.Config{REDACTED,
		),
REDACTED
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrockREDACTED

	// header 中不带 beta token，但 body 中有 computer_use 工具
	_, err = svc.resolveBedrockBetaTokensForRequest(
		context.Background(),
		account,
		"", // 空 header
		[]byte(`{"tools":[{"type":"computer_20250124","name":"computer"REDACTED],"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
		"us.anthropic.claude-opus-4-6-v1",
	)
	if err == nil {
		t.Fatal("expected body-injected computer-use to be blocked")
REDACTED
	if err.Error() != "computer use is blocked" {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

// TestResolveBedrockBetaTokensForRequest_BlocksBodyAutoInjectedToolSearch 验证：
// 管理员 block 了 tool-search-tool，客户端不在 header 中带 beta token，
// 但请求体包含 tool search 工具 → 自动注入后应被 block。
func TestResolveBedrockBetaTokensForRequest_BlocksBodyAutoInjectedToolSearch(t *testing.T) {
	settings := &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken:    "tool-search-tool-2025-10-19",
				Action:       BetaPolicyActionBlock,
				Scope:        BetaPolicyScopeAll,
				ErrorMessage: "tool search is blocked",
		REDACTED,
	REDACTED,
REDACTED
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
REDACTED

	svc := &GatewayService{
		settingService: NewSettingService(
			&betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
	REDACTED
			&config.Config{REDACTED,
		),
REDACTED
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrockREDACTED

	// header 中不带 beta token，但 body 中有 tool_search_tool 工具
	_, err = svc.resolveBedrockBetaTokensForRequest(
		context.Background(),
		account,
		"",
		[]byte(`{"tools":[{"type":"tool_search_tool_regex_20251119","name":"search"REDACTED],"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
		"us.anthropic.claude-sonnet-4-6",
	)
	if err == nil {
		t.Fatal("expected body-injected tool-search-tool to be blocked")
REDACTED
	if err.Error() != "tool search is blocked" {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

// TestResolveBedrockBetaTokensForRequest_PassesWhenNoBlockRuleMatches 验证：
// body 自动注入的 token 如果没有对应的 block 规则，应正常通过。
func TestResolveBedrockBetaTokensForRequest_PassesWhenNoBlockRuleMatches(t *testing.T) {
	settings := &BetaPolicySettings{
		Rules: []BetaPolicyRule{
			{
				BetaToken:    "context-1m-2025-08-07",
				Action:       BetaPolicyActionBlock,
				Scope:        BetaPolicyScopeAll,
				ErrorMessage: "context is blocked",
		REDACTED,
	REDACTED,
REDACTED
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
REDACTED

	svc := &GatewayService{
		settingService: NewSettingService(
			&betaPolicySettingRepoStub{values: map[string]string{
				SettingKeyBetaPolicySettings: string(raw),
	REDACTED
			&config.Config{REDACTED,
		),
REDACTED
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrockREDACTED

	// body 中有 computer_use 工具（会注入 computer-use token），但 block 规则只针对 context-1m
	tokens, err := svc.resolveBedrockBetaTokensForRequest(
		context.Background(),
		account,
		"",
		[]byte(`{"tools":[{"type":"computer_20250124","name":"computer"REDACTED],"messages":[{"role":"user","content":"hi"REDACTED]REDACTED`),
		"us.anthropic.claude-opus-4-6-v1",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
REDACTED
	found := false
	for _, token := range tokens {
		if token == "computer-use-2025-11-24" {
			found = true
	REDACTED
REDACTED
	if !found {
		t.Fatal("expected computer-use token to be present")
REDACTED
REDACTED
