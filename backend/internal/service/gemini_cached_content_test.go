//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type geminiCachedContentRepoStub struct {
	records map[string]*GeminiCachedContent
	nextID  int64
	deleted []int64
}

func newGeminiCachedContentRepoStub() *geminiCachedContentRepoStub {
	return &geminiCachedContentRepoStub{records: map[string]*GeminiCachedContent{}}
}

func (r *geminiCachedContentRepoStub) Create(_ context.Context, record *GeminiCachedContent) error {
	r.nextID++
	record.ID = r.nextID
	r.records[record.PublicID] = record
	return nil
}

func (r *geminiCachedContentRepoStub) GetForOwner(_ context.Context, apiKeyID int64, publicID string) (*GeminiCachedContent, error) {
	record, ok := r.records[publicID]
	if !ok || record.APIKeyID != apiKeyID {
		return nil, ErrGeminiCachedContentNotFound
	}
	return record, nil
}

func (r *geminiCachedContentRepoStub) ListForOwner(_ context.Context, apiKeyID, groupID int64, activeAt time.Time, beforeID int64, limit int) ([]*GeminiCachedContent, error) {
	var out []*GeminiCachedContent
	for _, record := range r.records {
		if record.APIKeyID == apiKeyID && record.GroupID == groupID && record.ExpireTime.After(activeAt) && (beforeID <= 0 || record.ID < beforeID) {
			out = append(out, record)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *geminiCachedContentRepoStub) UpdateExpireTime(_ context.Context, id int64, expireTime time.Time) (time.Time, error) {
	for _, record := range r.records {
		if record.ID == id {
			previous := record.ExpireTime
			record.ExpireTime = expireTime
			return previous, nil
		}
	}
	return time.Time{}, ErrGeminiCachedContentNotFound
}

func (r *geminiCachedContentRepoStub) SoftDelete(_ context.Context, id int64) error {
	r.deleted = append(r.deleted, id)
	for key, record := range r.records {
		if record.ID == id {
			delete(r.records, key)
		}
	}
	return nil
}

func TestParseGeminiCachedContentCreateRequest(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	maxTTL := 24 * time.Hour

	t.Run("defaults to one hour and strips output-only fields", func(t *testing.T) {
		req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"models/gemini-3.8-flash","displayName":"docs",
			"contents":[{"role":"user","parts":[{"text":"x"}]}],"name":"cachedContents/forged","usageMetadata":{"totalTokenCount":1}}`), now, maxTTL)
		require.NoError(t, err)
		require.Equal(t, "gemini-3.8-flash", req.Model)
		require.Equal(t, time.Hour, req.TTL)
		require.Equal(t, int64(3600), req.TTLSeconds())
		require.Equal(t, "docs", req.DisplayName)
		require.NotContains(t, req.payload, "name")
		require.NotContains(t, req.payload, "usageMetadata")
		require.Contains(t, req.payload, "contents")
	})

	t.Run("default is capped by a shorter max ttl", func(t *testing.T) {
		req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash"}`), now, 30*time.Minute)
		require.NoError(t, err)
		require.Equal(t, 30*time.Minute, req.TTL)
	})

	t.Run("fractional ttl rounds up to whole seconds", func(t *testing.T) {
		req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","ttl":"3.5s"}`), now, maxTTL)
		require.NoError(t, err)
		require.Equal(t, int64(4), req.TTLSeconds())
	})

	t.Run("expireTime converts to ttl", func(t *testing.T) {
		req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","expireTime":"2026-10-09T10:00:00Z"}`), now, maxTTL)
		require.NoError(t, err)
		require.Equal(t, 2*time.Hour, req.TTL)
	})

	for name, body := range map[string]string{
		"missing model":        `{"ttl":"60s"}`,
		"vertex style model":   `{"model":"projects/p/locations/l/publishers/google/models/x"}`,
		"ttl and expireTime":   `{"model":"m","ttl":"60s","expireTime":"2026-10-09T10:00:00Z"}`,
		"ttl over max":         `{"model":"m","ttl":"86401s"}`,
		"non positive ttl":     `{"model":"m","ttl":"0s"}`,
		"ttl without unit":     `{"model":"m","ttl":"60"}`,
		"expireTime in past":   `{"model":"m","expireTime":"2026-10-09T07:00:00Z"}`,
		"displayName too long": `{"model":"m","displayName":"` + strings.Repeat("字", 129) + `"}`,
		"not an object":        `[]`,
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := ParseGeminiCachedContentCreateRequest([]byte(body), now, maxTTL)
			require.Error(t, err)
		})
	}
}

func TestParseGeminiCachedContentPatchRequest(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	ttl, err := ParseGeminiCachedContentPatchRequest([]byte(`{"ttl":"7200s"}`), "ttl", now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, ttl)

	ttl, err = ParseGeminiCachedContentPatchRequest([]byte(`{"expireTime":"2026-10-09T09:00:00Z"}`), "", now, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, time.Hour, ttl)

	_, err = ParseGeminiCachedContentPatchRequest([]byte(`{"displayName":"x"}`), "displayName", now, 24*time.Hour)
	require.ErrorContains(t, err, "immutable")
	_, err = ParseGeminiCachedContentPatchRequest([]byte(`{"ttl":"60s"}`), "ttl,displayName", now, 24*time.Hour)
	require.ErrorContains(t, err, "immutable")
	_, err = ParseGeminiCachedContentPatchRequest([]byte(`{}`), "", now, 24*time.Hour)
	require.Error(t, err)
	_, err = ParseGeminiCachedContentPatchRequest([]byte(`{"ttl":"90000s"}`), "", now, 24*time.Hour)
	require.ErrorContains(t, err, "maximum")
}

func TestGeminiCachedContentReferenceRewrite(t *testing.T) {
	ref, err := FindGeminiCachedContentReference([]byte(`{"contents":[]}`))
	require.NoError(t, err)
	require.Nil(t, ref)

	body := []byte(`{"cachedContent":"cachedContents/abc","contents":[]}`)
	ref, err = FindGeminiCachedContentReference(body)
	require.NoError(t, err)
	require.Equal(t, "cachedContents/abc", ref.Name)
	out, err := RewriteGeminiCachedContentReference(body, ref, "cachedContents/upstream1")
	require.NoError(t, err)
	require.JSONEq(t, `{"cachedContent":"cachedContents/upstream1","contents":[]}`, string(out))

	countBody := []byte(`{"generateContentRequest":{"model":"models/m","cachedContent":"cachedContents/abc","contents":[]}}`)
	ref, err = FindGeminiCachedContentReference(countBody)
	require.NoError(t, err)
	out, err = RewriteGeminiCachedContentReference(countBody, ref, "projects/1/locations/us-central1/cachedContents/9")
	require.NoError(t, err)
	require.JSONEq(t, `{"generateContentRequest":{"model":"models/m","cachedContent":"projects/1/locations/us-central1/cachedContents/9","contents":[]}}`, string(out))

	_, err = FindGeminiCachedContentReference([]byte(`{"cachedContent":"cachedContents/a","generateContentRequest":{"cachedContent":"cachedContents/b"}}`))
	require.Error(t, err)
	_, err = FindGeminiCachedContentReference([]byte(`{"cachedContent":1}`))
	require.Error(t, err)
}

func TestIsGeminiExplicitCacheAccount(t *testing.T) {
	cases := []struct {
		name    string
		account *Account
		want    bool
	}{
		{"official api key", &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}}, true},
		{"explicit official base url", &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k", "base_url": "https://generativelanguage.googleapis.com/"}}, true},
		{"reseller base url", &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k", "base_url": "https://relay.example.com"}}, false},
		{"upstream gateway with explicit cache switch", &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k", "base_url": "https://relay.example.com", "explicit_cache_upstream": true}}, true},
		{"official host with path prefix", &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k", "base_url": "https://generativelanguage.googleapis.com/antigravity"}}, false},
		{"missing api key", &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey}, false},
		{"vertex service account", &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount}, true},
		{"oauth", &Account{Platform: PlatformGemini, Type: AccountTypeOAuth}, false},
		{"antigravity api key", &Account{Platform: PlatformAntigravity, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, IsGeminiExplicitCacheAccount(tc.account))
		})
	}
}

func TestGeminiCachedContentVertexURLs(t *testing.T) {
	account := &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{
		"project_id":             "proj-1",
		"location":               "us-central1",
		"vertex_model_locations": map[string]any{"gemini-3.8-flash": "global"},
	}}
	model, err := GeminiCachedContentUpstreamModelResource(account, "gemini-3.8-flash")
	require.NoError(t, err)
	require.Equal(t, "projects/proj-1/locations/global/publishers/google/models/gemini-3.8-flash", model)

	collection, err := geminiCachedContentCollectionURL(account, "", "gemini-3.8-flash")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/proj-1/locations/global/cachedContents", collection)

	collection, err = geminiCachedContentCollectionURL(account, "", "gemini-3.1-pro-preview")
	require.NoError(t, err)
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1/projects/proj-1/locations/us-central1/cachedContents", collection)

	resource, err := geminiCachedContentResourceURL(account, "", "projects/123/locations/europe-west4/cachedContents/456")
	require.NoError(t, err)
	require.Equal(t, "https://europe-west4-aiplatform.googleapis.com/v1/projects/123/locations/europe-west4/cachedContents/456", resource)

	_, err = geminiCachedContentResourceURL(account, "", "projects/123/locations/europe-west4/cachedContents/../x")
	require.Error(t, err)
	_, err = geminiCachedContentResourceURL(&Account{Type: AccountTypeAPIKey}, "https://generativelanguage.googleapis.com", "cachedContents/a/../b")
	require.Error(t, err)
}

func newGeminiCachedContentServiceForTest(t *testing.T, status int, respBody string) (*GeminiCachedContentService, *geminiCompatHTTPUpstreamStub) {
	t.Helper()
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(respBody)),
	}}
	compat := &GeminiMessagesCompatService{httpUpstream: httpStub, cfg: &config.Config{}}
	return NewGeminiCachedContentService(newGeminiCachedContentRepoStub(), compat, &config.Config{}), httpStub
}

func TestGeminiCachedContentCreateUpstreamAIStudio(t *testing.T) {
	svc, httpStub := newGeminiCachedContentServiceForTest(t, http.StatusOK,
		`{"name":"cachedContents/up123","model":"models/gemini-3.8-flash","usageMetadata":{"totalTokenCount":6273},"expireTime":"2026-10-09T09:50:26.591575799Z"}`)
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"models/gemini-3.8-flash","displayName":"user label",
		"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"doc"}]}]}`), time.Now(), 24*time.Hour)
	require.NoError(t, err)

	upstream, err := svc.CreateUpstream(context.Background(), account, req, "gemini-3.8-flash", "pub1", req.TTL)
	require.NoError(t, err)
	require.Equal(t, "cachedContents/up123", upstream.Name)
	require.Equal(t, int64(6273), upstream.TotalTokenCount)
	require.Equal(t, 2026, upstream.ExpireTime.Year())

	sent := httpStub.lastReq
	require.Equal(t, http.MethodPost, sent.Method)
	require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/cachedContents", sent.URL.String())
	require.Equal(t, "secret", sent.Header.Get("x-goog-api-key"))
	raw, err := io.ReadAll(sent.Body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, "models/gemini-3.8-flash", payload["model"])
	require.Equal(t, "3600s", payload["ttl"])
	require.Equal(t, "s2a-pub1", payload["displayName"], "上游 displayName 写网关标记，用户标签只存本地")
	require.Contains(t, payload, "systemInstruction")
	require.Contains(t, payload, "contents")
}

func TestGeminiCachedContentUpstreamErrorClassification(t *testing.T) {
	notFoundBody := `{"error":{"code":403,"message":"CachedContent not found (or permission denied)","status":"PERMISSION_DENIED"}}`
	svc, _ := newGeminiCachedContentServiceForTest(t, http.StatusForbidden, notFoundBody)
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	record := &GeminiCachedContent{UpstreamName: "cachedContents/up123"}

	err := svc.DeleteUpstream(context.Background(), account, record)
	var upErr *GeminiCachedContentUpstreamError
	require.ErrorAs(t, err, &upErr)
	require.True(t, upErr.NotFound())
	require.True(t, upErr.RetryableOnOtherAccount())

	quota := &GeminiCachedContentUpstreamError{StatusCode: http.StatusTooManyRequests}
	require.True(t, quota.RetryableOnOtherAccount())
	badRequest := &GeminiCachedContentUpstreamError{StatusCode: http.StatusBadRequest}
	require.False(t, badRequest.RetryableOnOtherAccount())
	require.False(t, badRequest.NotFound())
}

func TestGeminiCachedContentResolve(t *testing.T) {
	repo := newGeminiCachedContentRepoStub()
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	svc := NewGeminiCachedContentService(repo, nil, &config.Config{})
	svc.now = func() time.Time { return now }
	groupID := int64(5)
	require.NoError(t, repo.Create(context.Background(), &GeminiCachedContent{
		PublicID: "abc123", APIKeyID: 1, GroupID: groupID, ExpireTime: now.Add(time.Hour),
	}))
	require.NoError(t, repo.Create(context.Background(), &GeminiCachedContent{
		PublicID: "expired1", APIKeyID: 1, GroupID: groupID, ExpireTime: now.Add(-time.Second),
	}))

	record, err := svc.Resolve(context.Background(), 1, &groupID, "cachedContents/abc123")
	require.NoError(t, err)
	require.Equal(t, "abc123", record.PublicID)

	otherGroup := int64(6)
	for name, call := range map[string]func() error{
		"other api key": func() error {
			_, err := svc.Resolve(context.Background(), 2, &groupID, "cachedContents/abc123")
			return err
		},
		"other group": func() error {
			_, err := svc.Resolve(context.Background(), 1, &otherGroup, "cachedContents/abc123")
			return err
		},
		"expired": func() error {
			_, err := svc.Resolve(context.Background(), 1, &groupID, "cachedContents/expired1")
			return err
		},
		"malformed": func() error {
			_, err := svc.Resolve(context.Background(), 1, &groupID, "cachedContents/../x")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, call(), ErrGeminiCachedContentNotFound)
		})
	}
}

func TestGeminiCachedContentPublicID(t *testing.T) {
	id, err := NewGeminiCachedContentPublicID()
	require.NoError(t, err)
	require.Len(t, id, 40)
	parsed, ok := ParseGeminiCachedContentPublicID(GeminiCachedContentNamePrefix + id)
	require.True(t, ok)
	require.Equal(t, id, parsed)
}

func TestGeminiCachedContentStoragePricingAndSurcharge(t *testing.T) {
	pricing := &PricingService{}
	data, err := pricing.parsePricingData([]byte(`{"gemini-3.8-flash":{"litellm_provider":"gemini","input_cost_per_token":0.00000075,
		"output_cost_per_token":0.00000375,"cache_read_input_token_cost":0.000000075,"cache_storage_cost_per_token_per_hour":0.0000005}}`))
	require.NoError(t, err)
	pricing.pricingData = data
	billing := NewBillingService(&config.Config{}, pricing)

	price := billing.CacheStoragePricePerTokenHour("models/gemini-3.8-flash")
	require.InDelta(t, 0.0000005, price, 1e-15)
	require.Zero(t, billing.CacheStoragePricePerTokenHour("gemini-unknown-model"))

	tokenHours := GeminiCachedContentStorageTokenHours(10000, 2*time.Hour)
	require.InDelta(t, 20000, tokenHours, 1e-9)
	storage := tokenHours * price
	require.InDelta(t, 0.01, storage, 1e-12)

	gateway := &GatewayService{billingService: billing}
	groupID := int64(3)
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Hydrated: true}}
	cost := gateway.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model:                  "gemini-3.8-flash",
		Usage:                  ClaudeUsage{InputTokens: 10000},
		CacheStorageTokenHours: tokenHours,
	}, apiKey, "gemini-3.8-flash", 2, 2, time.Now())
	inputCost := 10000 * 0.00000075
	require.InDelta(t, inputCost+storage, cost.TotalCost, 1e-12)
	require.InDelta(t, (inputCost+storage)*2, cost.ActualCost, 1e-12)

	storageOnly := gateway.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model:                  "gemini-3.8-flash",
		CacheStorageTokenHours: tokenHours,
	}, apiKey, "gemini-3.8-flash", 1.5, 1.5, time.Now())
	require.InDelta(t, storage, storageOnly.TotalCost, 1e-12)
	require.InDelta(t, storage*1.5, storageOnly.ActualCost, 1e-12)
}

func TestGeminiCachedContentStorageCostInAccountStats(t *testing.T) {
	const (
		model       = "gemini-3.8-flash"
		groupID     = int64(31)
		inputTokens = 10000
		storage     = 0.01
		inputPrice  = 0.00000075
		rulePrice   = 0.0000002
	)
	newPricing := func() *PricingService {
		pricing := &PricingService{}
		data, err := pricing.parsePricingData([]byte(`{"gemini-3.8-flash":{"litellm_provider":"gemini","input_cost_per_token":0.00000075,
			"output_cost_per_token":0.00000375,"cache_storage_cost_per_token_per_hour":0.0000005}}`))
		require.NoError(t, err)
		pricing.pricingData = data
		return pricing
	}
	officialInput := inputTokens * inputPrice

	for _, tc := range []struct {
		name      string
		channel   *Channel
		tokens    int
		wantStats float64
	}{
		{name: "no_channel", tokens: inputTokens, wantStats: officialInput + storage},
		{name: "model_file_pricing", channel: &Channel{ID: 1, Status: StatusActive}, tokens: inputTokens, wantStats: officialInput + storage},
		{name: "apply_pricing_to_account_stats", channel: &Channel{ID: 1, Status: StatusActive, ApplyPricingToAccountStats: true},
			tokens: inputTokens, wantStats: officialInput + storage},
		{name: "custom_rule", channel: &Channel{ID: 1, Status: StatusActive, AccountStatsPricingRules: []AccountStatsPricingRule{{
			GroupIDs: []int64{groupID},
			Pricing:  []ChannelModelPricing{{Models: []string{model}, InputPrice: testPtrFloat64(rulePrice)}},
		}}}, tokens: inputTokens, wantStats: inputTokens*rulePrice + storage},
		{name: "storage_only_with_custom_rule", channel: &Channel{ID: 1, Status: StatusActive, AccountStatsPricingRules: []AccountStatsPricingRule{{
			GroupIDs: []int64{groupID},
			Pricing:  []ChannelModelPricing{{Models: []string{model}, InputPrice: testPtrFloat64(rulePrice)}},
		}}}, wantStats: storage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
			svc.billingService = NewBillingService(svc.cfg, newPricing())
			if tc.channel != nil {
				svc.channelService = newTestChannelServiceForStats(t, tc.channel, groupID, PlatformGemini)
			}
			groupRate, accountRate := 1.5, 0.6
			gid := groupID
			err := svc.RecordUsage(context.Background(), &RecordUsageInput{
				Result: &ForwardResult{
					RequestID:              "gemini-cached-content:create:" + tc.name,
					Model:                  model,
					Usage:                  ClaudeUsage{InputTokens: tc.tokens},
					CacheStorageTokenHours: storage / 0.0000005,
				},
				APIKey:  &APIKey{ID: 811, GroupID: &gid, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: groupRate, Hydrated: true}},
				User:    &User{ID: 611},
				Account: &Account{ID: 711, Platform: PlatformGemini, RateMultiplier: &accountRate},
			})
			require.NoError(t, err)
			log := usageRepo.lastLog
			require.NotNil(t, log)

			wantTotal := float64(tc.tokens)*inputPrice + storage
			require.InDelta(t, wantTotal, log.TotalCost, 1e-12)
			require.InDelta(t, wantTotal*groupRate, log.ActualCost, 1e-12, "售价 = 官方价 × 分组倍率")

			statsCost := log.TotalCost
			if log.AccountStatsCost != nil {
				statsCost = *log.AccountStatsCost
			}
			require.InDelta(t, tc.wantStats, statsCost, 1e-12, "账号统计成本须含存储费且只计一次")
			require.NotNil(t, log.AccountRateMultiplier)
			require.InDelta(t, tc.wantStats*accountRate, statsCost**log.AccountRateMultiplier, 1e-12, "成本 = 统计成本 × 账号倍率")
		})
	}
}

func TestGeminiExplicitCacheUpstreamTTL(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	upstream := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "k", "base_url": "https://relay.example.com", "explicit_cache_upstream": true,
		"explicit_cache_upstream_max_ttl_seconds": float64(1800),
	}}
	official := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "k", "explicit_cache_upstream_max_ttl_seconds": float64(1800),
	}}
	require.Equal(t, 30*time.Minute, upstream.GeminiExplicitCacheUpstreamMaxTTL())
	require.Zero(t, official.GeminiExplicitCacheUpstreamMaxTTL(), "未开启开关时上限字段不生效")

	svc := NewGeminiCachedContentService(newGeminiCachedContentRepoStub(), nil, &config.Config{})
	require.Equal(t, 30*time.Minute, svc.MaxTTLForAccount(upstream))
	require.Equal(t, 24*time.Hour, svc.MaxTTLForAccount(official))

	implicit, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash"}`), now, 24*time.Hour)
	require.NoError(t, err)
	ttl, ok := implicit.TTLForAccount(upstream)
	require.True(t, ok)
	require.Equal(t, 30*time.Minute, ttl, "未显式指定有效期时按上游上限创建")
	ttl, ok = implicit.TTLForAccount(official)
	require.True(t, ok)
	require.Equal(t, time.Hour, ttl)

	explicit, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","ttl":"3600s"}`), now, 24*time.Hour)
	require.NoError(t, err)
	_, ok = explicit.TTLForAccount(upstream)
	require.False(t, ok, "显式有效期超过上游上限时该账号不可用")
	ttl, ok = explicit.TTLForAccount(official)
	require.True(t, ok)
	require.Equal(t, time.Hour, ttl)
}

func TestGeminiCachedContentCreateUpstreamGateway(t *testing.T) {
	svc, httpStub := newGeminiCachedContentServiceForTest(t, http.StatusOK,
		`{"name":"cachedContents/gatewayid0001","model":"models/gemini-3.8-flash","usageMetadata":{"totalTokenCount":2730},"expireTime":"2026-10-09T09:00:00Z"}`)
	account := &Account{ID: 11, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "downstream-key", "base_url": "https://upstream.example.com/gw/", "explicit_cache_upstream": true,
	}}
	req, err := ParseGeminiCachedContentCreateRequest([]byte(`{"model":"gemini-3.8-flash","contents":[{"role":"user","parts":[{"text":"doc"}]}]}`), time.Now(), 24*time.Hour)
	require.NoError(t, err)

	upstream, err := svc.CreateUpstream(context.Background(), account, req, "gemini-3.8-flash", "pub2", 30*time.Minute)
	require.NoError(t, err)
	require.Equal(t, "cachedContents/gatewayid0001", upstream.Name)
	require.Equal(t, "https://upstream.example.com/gw/v1beta/cachedContents", httpStub.lastReq.URL.String())
	require.Equal(t, "downstream-key", httpStub.lastReq.Header.Get("x-goog-api-key"))
	raw, err := io.ReadAll(httpStub.lastReq.Body)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"ttl":"1800s"`)

	resource, err := geminiCachedContentResourceURL(account, geminiCachedContentAPIBase(account), upstream.Name)
	require.NoError(t, err)
	require.Equal(t, "https://upstream.example.com/gw/v1beta/cachedContents/gatewayid0001", resource)
}
