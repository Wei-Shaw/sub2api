package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMapUserErrorCategory(t *testing.T) {
	cases := []struct {
		phase, etype, want string
REDACTED{
		{"auth", "authentication_error", "auth"REDACTED,
		{"request", "rate_limit_error", "rate_limit"REDACTED,
		{"request", "billing_error", "quota"REDACTED,
		{"request", "subscription_error", "quota"REDACTED,
		{"request", "invalid_request_error", "invalid_request"REDACTED,
		{"routing", "api_error", "service_unavailable"REDACTED,
		{"upstream", "upstream_error", "upstream"REDACTED,
		{"network", "api_error", "upstream"REDACTED,
		{"internal", "api_error", "internal"REDACTED,
		{"weird", "weird", "other"REDACTED,
REDACTED
	for _, c := range cases {
		if got := MapUserErrorCategory(c.phase, c.etype); got != c.want {
			t.Errorf("MapUserErrorCategory(%q,%q)=%q want %q", c.phase, c.etype, got, c.want)
	REDACTED
REDACTED
REDACTED

func TestCategoryToFilter(t *testing.T) {
	phases, types := CategoryToFilter("rate_limit")
	if len(types) != 1 || types[0] != "rate_limit_error" || len(phases) != 0 {
		t.Fatalf("rate_limit => phases=%v types=%v", phases, types)
REDACTED
	phases, types = CategoryToFilter("auth")
	if len(phases) != 1 || phases[0] != "auth" || len(types) != 0 {
		t.Fatalf("auth => phases=%v types=%v", phases, types)
REDACTED
	phases, types = CategoryToFilter("service_unavailable")
	if len(phases) != 1 || phases[0] != "routing" || len(types) != 0 {
		t.Fatalf("service_unavailable => phases=%v types=%v", phases, types)
REDACTED
	phases, types = CategoryToFilter("upstream")
	if len(phases) != 2 || phases[0] != "upstream" || phases[1] != "network" || len(types) != 0 {
		t.Fatalf("upstream => phases=%v types=%v", phases, types)
REDACTED
	phases, types = CategoryToFilter("internal")
	if len(phases) != 1 || phases[0] != "internal" || len(types) != 0 {
		t.Fatalf("internal => phases=%v types=%v", phases, types)
REDACTED
	phases, types = CategoryToFilter("quota")
	if len(types) != 2 || types[0] != "billing_error" || types[1] != "subscription_error" || len(phases) != 0 {
		t.Fatalf("quota => phases=%v types=%v", phases, types)
REDACTED
	phases, types = CategoryToFilter("invalid_request")
	if len(types) != 1 || types[0] != "invalid_request_error" || len(phases) != 0 {
		t.Fatalf("invalid_request => phases=%v types=%v", phases, types)
REDACTED
	phases, types = CategoryToFilter("other")
	if len(phases) != 0 || len(types) != 0 {
		t.Fatalf("other => phases=%v types=%v", phases, types)
REDACTED
REDACTED

func TestToUserErrorRequest_RedactsSensitiveFields(t *testing.T) {
	src := &OpsErrorLog{
		ID:              123,
		CreatedAt:       time.Unix(0, 0).UTC(),
		Model:           "m",
		RequestedModel:  "rm",
		InboundEndpoint: "/v1/chat/completions",
		StatusCode:      429,
		Platform:        "openai",
		Phase:           "request",
		Type:            "rate_limit_error",
		Message:         "rate limit exceeded",
		APIKeyName:      "my-key",
		APIKeyDeleted:   true,
REDACTED
	out := ToUserErrorRequest(src)
	if out.ID != 123 {
		t.Errorf("want ID=123, got %d", out.ID)
REDACTED
	if out.Model != "rm" {
		t.Errorf("want requested_model preferred, got %q", out.Model)
REDACTED
	if out.Category != "rate_limit" {
		t.Errorf("category=%q", out.Category)
REDACTED
	if out.StatusCode != 429 || out.InboundEndpoint != "/v1/chat/completions" || out.Platform != "openai" {
		t.Errorf("basic fields wrong: %+v", out)
REDACTED
	if out.Message != "rate limit exceeded" {
		t.Errorf("want message=%q, got %q", "rate limit exceeded", out.Message)
REDACTED
	if out.KeyName != "my-key" {
		t.Errorf("want key_name=my-key, got %q", out.KeyName)
REDACTED
	if !out.KeyDeleted {
		t.Error("want key_deleted=true")
REDACTED
REDACTED

func TestToUserErrorRequestDetail_WhitelistAndRedacts(t *testing.T) {
	uid := int64(42)
	upstreamStatus := 503
	src := &OpsErrorLogDetail{
		OpsErrorLog: OpsErrorLog{
			ID:               999,
			CreatedAt:        time.Unix(1000, 0).UTC(),
			Model:            "gpt-4",
			RequestedModel:   "gpt-4-turbo",
			InboundEndpoint:  "/v1/chat/completions",
			StatusCode:       502,
			Platform:         "openai",
			Phase:            "upstream",
			Type:             "api_error",
			Message:          "upstream error",
			UserID:           &uid,
			UserEmail:        "secret@example.com",
			ClientIP:         func() *string { s := "1.2.3.4"; return &s REDACTED(),
			UpstreamEndpoint: "https://api.openai.com/v1/chat/completions",
	REDACTED,
		ErrorBody:          `{"error":{"message":"upstream failed","type":"server_error"REDACTEDREDACTED`,
		UserAgent:          "Mozilla/5.0 secret-agent",
		UpstreamStatusCode: &upstreamStatus,
REDACTED

	out := ToUserErrorRequestDetail(src)
	if out == nil {
		t.Fatal("expected non-nil detail")
REDACTED

	// 基础字段正确映射
	if out.ID != 999 {
		t.Errorf("want ID=999, got %d", out.ID)
REDACTED
	if out.Message != "upstream error" {
		t.Errorf("want message=%q, got %q", "upstream error", out.Message)
REDACTED
	if out.ErrorBody != src.ErrorBody {
		t.Errorf("ErrorBody mismatch")
REDACTED
	if out.UpstreamStatusCode == nil || *out.UpstreamStatusCode != 503 {
		t.Errorf("UpstreamStatusCode mismatch")
REDACTED

	// 序列化后不含敏感字段
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
REDACTED
	raw := string(b)
	for _, forbidden := range []string{"user_email", "client_ip", "upstream_endpoint", "user_agent"REDACTED {
		if strings.Contains(raw, forbidden) {
			t.Errorf("sensitive field %q leaked in JSON output: %s", forbidden, raw)
	REDACTED
REDACTED
REDACTED

func TestToUserErrorRequestDetail_Nil(t *testing.T) {
	if out := ToUserErrorRequestDetail(nil); out != nil {
		t.Errorf("expected nil for nil input, got %+v", out)
REDACTED
REDACTED
