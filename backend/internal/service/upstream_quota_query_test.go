package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type upstreamQuotaHTTPStub struct {
	mu       sync.Mutex
	requests []*http.Request
	proxies  []string
	profiles []*tlsfingerprint.Profile
	handler  func(*http.Request) (*http.Response, error)
}

func (u *upstreamQuotaHTTPStub) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (u *upstreamQuotaHTTPStub) DoWithTLS(req *http.Request, proxyURL string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.mu.Lock()
	u.requests = append(u.requests, req.Clone(req.Context()))
	u.proxies = append(u.proxies, proxyURL)
	u.profiles = append(u.profiles, profile)
	u.mu.Unlock()
	if u.handler == nil {
		return nil, errors.New("unexpected upstream request")
	}
	return u.handler(req)
}

func (u *upstreamQuotaHTTPStub) snapshot() ([]*http.Request, []string, []*tlsfingerprint.Profile) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]*http.Request(nil), u.requests...), append([]string(nil), u.proxies...), append([]*tlsfingerprint.Profile(nil), u.profiles...)
}

func quotaHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type upstreamQuotaTrackedBody struct {
	io.Reader
	closed atomic.Bool
}

type pendingUpstreamIdentityAccountRepo struct {
	*upstreamBillingProbeAccountRepo
	pending         []Account
	detectorVersion int
	limit           int
}

type pendingUpstreamIdentityLogoRepo struct {
	*pendingUpstreamIdentityAccountRepo
	logoMu sync.Mutex
	logos  map[string]*UpstreamSiteLogo
}

func (r *pendingUpstreamIdentityLogoRepo) GetUpstreamSiteLogoCache(_ context.Context, key string) (*UpstreamSiteLogo, bool, error) {
	r.logoMu.Lock()
	defer r.logoMu.Unlock()
	logo, found := r.logos[key]
	return logo, found, nil
}

func (r *pendingUpstreamIdentityLogoRepo) PutUpstreamSiteLogoCache(_ context.Context, key string, logo *UpstreamSiteLogo) error {
	r.logoMu.Lock()
	defer r.logoMu.Unlock()
	if _, found := r.logos[key]; !found {
		r.logos[key] = logo
	}
	return nil
}

func (r *pendingUpstreamIdentityAccountRepo) ListPendingUpstreamIdentityAccounts(_ context.Context, detectorVersion, limit int) ([]Account, error) {
	r.detectorVersion = detectorVersion
	r.limit = limit
	return append([]Account(nil), r.pending...), nil
}

func (r *pendingUpstreamIdentityAccountRepo) UpdateUpstreamIdentitySnapshot(_ context.Context, expected *Account, snapshot *UpstreamIdentitySnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[expected.ID]
	if !sameUpstreamProtocolIdentity(expected, account) {
		return ErrUpstreamQuotaIdentityChanged
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[UpstreamIdentityExtraKey] = snapshot
	return nil
}

func (b *upstreamQuotaTrackedBody) Close() error {
	b.closed.Store(true)
	return nil
}

func newUpstreamQuotaAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-sensitive", "base_url": "https://upstream.example/v1"},
		Extra:       map[string]any{"quota_limit": 77.0, "quota_used": 12.0},
	}
}

func validSub2APIQuotaBody() string {
	return `{"mode":"quota_limited","isValid":true,"status":"active","quota":{"limit":100,"used":20,"remaining":80,"unit":"USD"},"remaining":80,"unit":"USD"}`
}

func validNewAPISubscriptionBody() string {
	return `{"object":"billing_subscription","has_payment_method":true,"soft_limit_usd":90,"hard_limit_usd":100,"system_hard_limit_usd":100,"access_until":1800000000}`
}

func validNewAPIUsageBody() string {
	return `{"object":"list","total_usage":1234}`
}

func TestRunPendingUpstreamIdentityDetectionPersistsOneBoundedBatch(t *testing.T) {
	account := newUpstreamQuotaAccount(72)
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	repo := &pendingUpstreamIdentityAccountRepo{
		upstreamBillingProbeAccountRepo: baseRepo,
		pending:                         []Account{*account},
	}
	upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/usage", req.URL.Path)
		return quotaHTTPResponse(http.StatusOK, validSub2APIQuotaBody()), nil
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	fixedNow := time.Date(2026, time.July, 20, 9, 30, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	require.NoError(t, svc.RunPendingUpstreamIdentityDetection(context.Background()))
	require.Equal(t, UpstreamIdentityDetectorVersion, repo.detectorVersion)
	require.Equal(t, upstreamIdentityBatchSize, repo.limit)

	baseRepo.mu.Lock()
	stored := decodeUpstreamIdentitySnapshot(baseRepo.accounts[account.ID].Extra)
	baseRepo.mu.Unlock()
	require.NotNil(t, stored)
	require.Equal(t, UpstreamIdentityStatusIdentified, stored.Status)
	require.Equal(t, UpstreamIdentityProviderSub2API, stored.Provider)
	require.Equal(t, fixedNow, stored.DetectedAt)
}

func TestRunPendingUpstreamIdentityDetectionReusesStrictStoredBillingSnapshot(t *testing.T) {
	data, err := parseUpstreamBillingProbeResponse([]byte(`{
		"object":"sub2api.key_billing",
		"schema_version":1,
		"billing_scope":"token",
		"group_rate_multiplier":0.02,
		"resolved_rate_multiplier":0.02,
		"peak_rate_enabled":false,
		"effective_rate_multiplier":0.02,
		"observed_at":"2026-07-20T09:00:00Z"
	}`))
	require.NoError(t, err)
	account := newUpstreamQuotaAccount(71)
	account.Extra[UpstreamBillingProbeExtraKey] = map[string]any{
		"status":          UpstreamBillingProbeStatusOK,
		"data":            data,
		"last_attempt_at": "2026-07-20T09:00:00Z",
		"next_probe_at":   "2026-07-20T09:30:00Z",
	}
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	repo := &pendingUpstreamIdentityAccountRepo{
		upstreamBillingProbeAccountRepo: baseRepo,
		pending:                         []Account{*account},
	}
	upstream := &upstreamQuotaHTTPStub{handler: func(*http.Request) (*http.Response, error) {
		t.Fatal("stored billing snapshot should avoid an upstream request")
		return nil, nil
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	require.NoError(t, svc.RunPendingUpstreamIdentityDetection(context.Background()))
	require.Empty(t, upstream.requests)

	baseRepo.mu.Lock()
	stored := decodeUpstreamIdentitySnapshot(baseRepo.accounts[account.ID].Extra)
	baseRepo.mu.Unlock()
	require.NotNil(t, stored)
	require.Equal(t, UpstreamIdentityProviderSub2API, stored.Provider)
}

func TestUpstreamIdentityCachesOneCustomSiteLogoPerDeployment(t *testing.T) {
	first := newUpstreamQuotaAccount(75)
	second := newUpstreamQuotaAccount(76)
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{first.ID: first, second.ID: second}}
	repo := &pendingUpstreamIdentityLogoRepo{
		pendingUpstreamIdentityAccountRepo: &pendingUpstreamIdentityAccountRepo{upstreamBillingProbeAccountRepo: baseRepo},
		logos:                              make(map[string]*UpstreamSiteLogo),
	}
	var settingsCalls atomic.Int64
	var logoCalls atomic.Int64
	customPNG := "\x89PNG\r\n\x1a\ncustom-site-logo"
	upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/usage":
			return quotaHTTPResponse(http.StatusOK, validSub2APIQuotaBody()), nil
		case "/api/v1/settings/public":
			settingsCalls.Add(1)
			return quotaHTTPResponse(http.StatusOK, `{"code":0,"data":{"site_logo":"/brand.png"}}`), nil
		case "/brand.png":
			logoCalls.Add(1)
			return quotaHTTPResponse(http.StatusOK, customPNG), nil
		default:
			return nil, errors.New("unexpected path: " + req.URL.Path)
		}
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	require.NoError(t, svc.detectAndPersistUpstreamIdentity(context.Background(), first.ID))
	require.NoError(t, svc.detectAndPersistUpstreamIdentity(context.Background(), second.ID))
	require.Equal(t, int64(1), settingsCalls.Load())
	require.Equal(t, int64(1), logoCalls.Load())

	firstIdentity := decodeUpstreamIdentitySnapshot(baseRepo.accounts[first.ID].Extra)
	secondIdentity := decodeUpstreamIdentitySnapshot(baseRepo.accounts[second.ID].Extra)
	require.NotNil(t, firstIdentity)
	require.NotNil(t, secondIdentity)
	require.NotEmpty(t, firstIdentity.SiteLogoKey)
	require.Equal(t, firstIdentity.SiteLogoKey, secondIdentity.SiteLogoKey)
	logo, found, err := repo.GetUpstreamSiteLogoCache(context.Background(), firstIdentity.SiteLogoKey)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "image/png", logo.ContentType)
	require.Equal(t, []byte(customPNG), logo.Data)
}

func TestUpstreamIdentityDetectsStrictProviderAndNewAPIGeneration(t *testing.T) {
	tests := []struct {
		name       string
		usageBody  string
		statusBody string
		provider   string
		variant    string
	}{
		{name: "Sub2API", usageBody: validSub2APIQuotaBody(), provider: UpstreamIdentityProviderSub2API},
		{
			name:       "legacy New API",
			statusBody: `{"success":true,"data":{"version":"v0.2.8.7","display_in_currency":true}}`,
			provider:   UpstreamIdentityProviderNewAPI,
			variant:    UpstreamIdentityVariantLegacy,
		},
		{
			name:       "modern New API",
			statusBody: `{"success":true,"data":{"version":"v1.0.0","logo":"/custom-logo.png","quota_display_type":"USD","display_in_currency":true}}`,
			provider:   UpstreamIdentityProviderNewAPI,
			variant:    UpstreamIdentityVariantModern,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := newUpstreamQuotaAccount(73)
			upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/v1/usage":
					if tt.usageBody != "" {
						return quotaHTTPResponse(http.StatusOK, tt.usageBody), nil
					}
					return quotaHTTPResponse(http.StatusNotFound, `{}`), nil
				case "/v1/dashboard/billing/subscription":
					return quotaHTTPResponse(http.StatusOK, validNewAPISubscriptionBody()), nil
				case "/api/status":
					require.Empty(t, req.Header.Get("Authorization"))
					return quotaHTTPResponse(http.StatusOK, tt.statusBody), nil
				default:
					return nil, errors.New("unexpected path: " + req.URL.Path)
				}
			}}
			client := &upstreamQuotaQueryClient{
				upstream: upstream,
				account:  account,
				baseURL:  "https://upstream.example/v1",
				apiKey:   "sk-sensitive",
			}
			result, err := client.detectIdentity(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.provider, result.provider)
			require.Equal(t, tt.variant, result.variant)
			if tt.variant == UpstreamIdentityVariantModern {
				require.Equal(t, "/custom-logo.png", result.logoCandidate)
			}
		})
	}
}

func TestUpstreamSiteLogoHelpersUseDeploymentRootAndHTMLIcon(t *testing.T) {
	keyA, err := upstreamSiteLogoCacheKey("https://UPSTREAM.example/subpath/v1")
	require.NoError(t, err)
	keyB, err := upstreamSiteLogoCacheKey("https://upstream.example/subpath")
	require.NoError(t, err)
	require.Equal(t, keyA, keyB)
	require.Equal(t, "/assets/icon.webp", parseHTMLIconHref([]byte(`<html><head><link rel="shortcut icon" href="/assets/icon.webp"></head></html>`)))
	require.Nil(t, validatedUpstreamSiteLogo([]byte(`<svg></svg>`)))
}

func TestNormalizeUpstreamSiteLogoForDisplayTrimsPadding(t *testing.T) {
	var encoded bytes.Buffer
	for _, background := range []color.Color{color.Transparent, color.White} {
		source := image.NewNRGBA(image.Rect(0, 0, 32, 32))
		draw.Draw(source, source.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
		draw.Draw(source, image.Rect(8, 8, 24, 24), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
		encoded.Reset()
		require.NoError(t, png.Encode(&encoded, source))

		got := normalizeUpstreamSiteLogoForDisplay(&UpstreamSiteLogo{ContentType: "image/png", Data: encoded.Bytes()})
		decoded, format, err := image.Decode(bytes.NewReader(got.Data))
		require.NoError(t, err)
		require.Equal(t, "png", format)
		require.Equal(t, image.Rect(0, 0, 18, 18), decoded.Bounds())
	}

	ico := make([]byte, 22+encoded.Len())
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], 1)
	ico[6], ico[7] = 32, 32
	binary.LittleEndian.PutUint32(ico[14:18], uint32(encoded.Len()))
	binary.LittleEndian.PutUint32(ico[18:22], 22)
	copy(ico[22:], encoded.Bytes())
	got := normalizeUpstreamSiteLogoForDisplay(&UpstreamSiteLogo{ContentType: "image/x-icon", Data: ico})
	require.Equal(t, "image/png", got.ContentType)
	decoded, _, err := image.Decode(bytes.NewReader(got.Data))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 18, 18), decoded.Bounds())

	palette := color.Palette{color.White, color.Black}
	paletted := image.NewPaletted(image.Rect(0, 0, 1254, 1254), palette)
	draw.Draw(paletted, image.Rect(179, 150, 1055, 1017), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	encoded.Reset()
	require.NoError(t, png.Encode(&encoded, paletted))
	got = normalizeUpstreamSiteLogoForDisplay(&UpstreamSiteLogo{ContentType: "image/png", Data: encoded.Bytes()})
	require.LessOrEqual(t, int64(len(got.Data)), upstreamSiteLogoMaxBytes)
	decoded, _, err = image.Decode(bytes.NewReader(got.Data))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 884, 875), decoded.Bounds())
}

func TestUpstreamIdentityRequiresResolvedNewAPIGeneration(t *testing.T) {
	account := newUpstreamQuotaAccount(74)
	upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/usage":
			return quotaHTTPResponse(http.StatusNotFound, `{}`), nil
		case "/v1/dashboard/billing/subscription":
			return quotaHTTPResponse(http.StatusOK, validNewAPISubscriptionBody()), nil
		case "/api/status":
			return quotaHTTPResponse(http.StatusOK, `{"success":true,"data":{"version":"custom"}}`), nil
		default:
			return nil, errors.New("unexpected request")
		}
	}}
	client := &upstreamQuotaQueryClient{upstream: upstream, account: account, baseURL: "https://upstream.example/v1", apiKey: "sk-sensitive"}
	_, err := client.detectIdentity(context.Background())
	require.ErrorIs(t, err, ErrUpstreamQuotaInvalidResponse)
}

func TestUpstreamQuotaParsesAllSub2APIModes(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		check func(*testing.T, *UpstreamQuotaInfo)
	}{
		{
			name: "quota with rate windows",
			body: `{"mode":"quota_limited","isValid":true,"status":"active","quota":{"limit":100,"used":20,"remaining":80,"unit":"USD"},"remaining":80,"unit":"USD","rate_limits":[{"window":"5h","limit":50,"used":10,"remaining":40,"window_start":null}]}`,
			check: func(t *testing.T, got *UpstreamQuotaInfo) {
				require.Equal(t, "quota", got.Mode)
				require.Equal(t, "USD", got.Unit)
				require.Equal(t, 80.0, *got.Remaining)
				require.Len(t, got.Windows, 1)
			},
		},
		{
			name: "key quota with shared subscription",
			body: `{"mode":"quota_limited","isValid":true,"status":"active","planName":"Pro Monthly","quota":{"limit":100,"used":20,"remaining":80,"unit":"USD"},"remaining":80,"unit":"USD","subscription":{"daily_usage_usd":2,"weekly_usage_usd":5,"monthly_usage_usd":20,"daily_limit_usd":10,"weekly_limit_usd":20,"monthly_limit_usd":100,"daily_reset_at":"2026-07-20T00:00:00Z","weekly_reset_at":"2026-07-26T00:00:00Z","monthly_reset_at":"2026-08-18T00:00:00Z","unlimited":false,"expires_at":"2026-09-01T00:00:00Z"}}`,
			check: func(t *testing.T, got *UpstreamQuotaInfo) {
				require.Equal(t, "quota", got.Mode)
				require.Equal(t, 80.0, *got.Remaining)
				require.NotNil(t, got.Subscription)
				require.Equal(t, "Pro Monthly", got.Subscription.PlanName)
				require.Equal(t, 8.0, *got.Subscription.Remaining)
				require.Len(t, got.Subscription.Windows, 3)
				require.NotNil(t, got.Subscription.Windows[0].ResetAt)
				require.Nil(t, got.ExpiresAt)
			},
		},
		{
			name: "rate limits without invented unit",
			body: `{"mode":"quota_limited","isValid":true,"status":"quota_exhausted","rate_limits":[{"window":"1d","limit":20,"used":7,"remaining":13,"window_start":"2026-07-17T00:00:00+08:00","reset_at":"2026-07-18T00:00:00+08:00"}]}`,
			check: func(t *testing.T, got *UpstreamQuotaInfo) {
				require.Equal(t, "rate_limits", got.Mode)
				require.Empty(t, got.Unit)
				require.Nil(t, got.Remaining)
				require.Equal(t, "1d", got.Windows[0].Name)
				require.NotNil(t, got.Windows[0].ResetAt)
			},
		},
		{
			name: "limited subscription",
			body: `{"mode":"unrestricted","isValid":true,"planName":"Weekly","unit":"USD","remaining":8,"subscription":{"daily_usage_usd":2,"weekly_usage_usd":5,"monthly_usage_usd":0,"daily_limit_usd":10,"weekly_limit_usd":20,"monthly_limit_usd":null,"weekly_window_start":"2026-07-13T00:00:00+08:00","expires_at":"2026-08-01T00:00:00+08:00"}}`,
			check: func(t *testing.T, got *UpstreamQuotaInfo) {
				require.Equal(t, "subscription", got.Mode)
				require.NotNil(t, got.Subscription)
				require.False(t, got.Subscription.Unlimited)
				require.Equal(t, 8.0, *got.Subscription.Remaining)
				require.Len(t, got.Subscription.Windows, 2)
				require.Equal(t, "weekly", got.Subscription.Windows[1].Name)
				require.NotNil(t, got.Subscription.Windows[1].ResetAt)
				require.Nil(t, got.Remaining)
			},
		},
		{
			name: "unlimited subscription",
			body: `{"mode":"unrestricted","isValid":true,"planName":"Unlimited","unit":"USD","remaining":-1,"subscription":{"daily_usage_usd":0,"weekly_usage_usd":0,"monthly_usage_usd":0,"daily_limit_usd":null,"weekly_limit_usd":null,"monthly_limit_usd":null,"weekly_window_start":null,"expires_at":"2026-08-01T00:00:00Z"}}`,
			check: func(t *testing.T, got *UpstreamQuotaInfo) {
				require.Equal(t, "subscription", got.Mode)
				require.NotNil(t, got.Subscription)
				require.True(t, got.Subscription.Unlimited)
				require.Nil(t, got.Subscription.Remaining)
				require.Nil(t, got.Remaining)
			},
		},
		{
			name: "wallet balance",
			body: `{"mode":"unrestricted","isValid":true,"planName":"wallet","unit":"USD","remaining":-2.5,"balance":-2.5}`,
			check: func(t *testing.T, got *UpstreamQuotaInfo) {
				require.Equal(t, "balance", got.Mode)
				require.Equal(t, -2.5, *got.Remaining)
				require.Nil(t, got.Subscription)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSub2APIUsage([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, "sub2api", got.Provider)
			tt.check(t, got)
		})
	}
}

func TestUpstreamQuotaRejectsMalformedSub2APIContracts(t *testing.T) {
	bodies := []string{
		`{"mode":"quota_limited","isValid":false,"status":"active"}`,
		`{"mode":"quota_limited","isValid":true,"status":"active"}`,
		`{"mode":"quota_limited","isValid":true,"status":"active","quota":{"limit":100,"used":20,"remaining":80,"unit":"CNY"},"remaining":80,"unit":"CNY"}`,
		`{"mode":"quota_limited","isValid":true,"status":"active","quota":{"limit":100,"used":20,"remaining":81,"unit":"USD"},"remaining":81,"unit":"USD"}`,
		`{"mode":"quota_limited","isValid":true,"status":"active","rate_limits":[{"window":"5h","limit":10,"used":2,"remaining":8}]}`,
		`{"mode":"quota_limited","isValid":true,"status":"active","rate_limits":[{"window":"5h","limit":10,"used":2,"remaining":7,"window_start":null}]}`,
		`{"mode":"quota_limited","isValid":true,"status":"active","rate_limits":[{"window":"5h","limit":10,"used":2,"remaining":8,"window_start":null},{"window":"5h","limit":10,"used":2,"remaining":8,"window_start":null}]}`,
		`{"mode":"unrestricted","isValid":true,"planName":"missing data","unit":"USD"}`,
		`{"mode":"unrestricted","isValid":true,"planName":"wallet","unit":"TOKENS","remaining":10,"balance":10}`,
		`{"mode":"unrestricted","isValid":true,"planName":"limited","unit":"CNY","remaining":8,"subscription":{"daily_usage_usd":2,"weekly_usage_usd":5,"monthly_usage_usd":0,"daily_limit_usd":10,"weekly_limit_usd":20,"monthly_limit_usd":null,"weekly_window_start":null,"expires_at":"2026-08-01T00:00:00Z"}}`,
		`{"mode":"unrestricted","isValid":true,"planName":"limited","unit":"USD","remaining":9,"subscription":{"daily_usage_usd":2,"weekly_usage_usd":0,"monthly_usage_usd":0,"daily_limit_usd":10,"weekly_limit_usd":null,"monthly_limit_usd":null,"weekly_window_start":null,"expires_at":"2026-08-01T00:00:00Z"}}`,
		`{"mode":"quota_limited","isValid":true,"status":"active","quota":{"limit":100,"used":20,"remaining":80,"unit":"USD"},"remaining":80,"unit":"USD","subscription":{"daily_usage_usd":2,"weekly_usage_usd":0,"monthly_usage_usd":0,"daily_limit_usd":10,"weekly_limit_usd":null,"monthly_limit_usd":null,"unlimited":false,"expires_at":"2026-08-01T00:00:00Z"}}`,
		`{"mode":"quota_limited","isValid":true,"status":"active","planName":"broken","quota":{"limit":100,"used":20,"remaining":80,"unit":"USD"},"remaining":80,"unit":"USD","subscription":{"daily_usage_usd":2,"weekly_usage_usd":0,"monthly_usage_usd":0,"daily_limit_usd":10,"weekly_limit_usd":null,"monthly_limit_usd":null,"unlimited":true,"expires_at":"2026-08-01T00:00:00Z"}}`,
	}
	for _, body := range bodies {
		_, err := parseSub2APIUsage([]byte(body))
		require.Error(t, err, body)
	}
}

func TestUpstreamQuotaNewAPISuccessAndUnitLookup(t *testing.T) {
	proxyID := int64(9)
	account := newUpstreamQuotaAccount(41)
	account.Credentials[credKeyHeaderOverrideEnabled] = true
	account.Credentials[credKeyHeaderOverrides] = map[string]any{
		"x-tenant":      "tenant-a",
		"authorization": "Bearer attacker-controlled",
	}
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "proxy.example", Port: 8080, Username: "proxy-user", Password: "proxy-pass", Status: StatusActive}
	extraBefore := map[string]any{"quota_limit": 77.0, "quota_used": 12.0}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	var bodies []*upstreamQuotaTrackedBody
	var closeOrderInvalid bool
	var operationDeadline, statusDeadline time.Time
	upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
		if len(bodies) > 0 && !bodies[len(bodies)-1].closed.Load() {
			closeOrderInvalid = true
		}
		deadline, _ := req.Context().Deadline()
		if operationDeadline.IsZero() {
			operationDeadline = deadline
		}
		if req.URL.Path == "/api/status" {
			statusDeadline = deadline
		}
		status, body := http.StatusNotFound, `{}`
		switch req.URL.Path {
		case "/v1/usage":
			body = `{"error":"not found"}`
		case "/v1/dashboard/billing/subscription":
			status, body = http.StatusOK, validNewAPISubscriptionBody()
		case "/v1/dashboard/billing/usage":
			status, body = http.StatusOK, validNewAPIUsageBody()
		case "/api/status":
			status, body = http.StatusOK, `{"success":true,"data":{"quota_display_type":"USD"}}`
		}
		tracked := &upstreamQuotaTrackedBody{Reader: strings.NewReader(body)}
		bodies = append(bodies, tracked)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: tracked}, nil
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	fixedNow := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	result, err := svc.QueryAccountQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, fixedNow, result.ObservedAt)
	require.Equal(t, "new_api", result.Quota.Provider)
	require.Equal(t, "USD", result.Quota.Unit)
	require.Equal(t, 12.34, *result.Quota.Used)
	require.Equal(t, 100.0, *result.Quota.Total)
	require.InDelta(t, 87.66, *result.Quota.Remaining, 1e-9)
	require.NotNil(t, result.Quota.ExpiresAt)
	require.False(t, closeOrderInvalid)
	for _, body := range bodies {
		require.True(t, body.closed.Load())
	}
	require.False(t, operationDeadline.IsZero())
	require.False(t, statusDeadline.IsZero())
	require.False(t, statusDeadline.After(operationDeadline))
	require.True(t, statusDeadline.After(time.Now()))
	require.LessOrEqual(t, time.Until(statusDeadline), upstreamQuotaStatusTimeout)

	requests, proxies, profiles := upstream.snapshot()
	require.Len(t, requests, 4)
	require.Equal(t, []string{
		"/v1/usage", "/v1/dashboard/billing/subscription", "/v1/dashboard/billing/usage", "/api/status",
	}, []string{requests[0].URL.Path, requests[1].URL.Path, requests[2].URL.Path, requests[3].URL.Path})
	for i, request := range requests {
		require.True(t, HTTPUpstreamRedirectsDisabled(request.Context()))
		require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(request.Context()))
		require.Equal(t, "tenant-a", getHeaderRaw(request.Header, "x-tenant"))
		require.Equal(t, account.Proxy.URL(), proxies[i])
		require.Nil(t, profiles[i])
		if request.URL.Path == "/api/status" {
			require.Empty(t, request.Header.Get("Authorization"))
		} else {
			require.Equal(t, "Bearer sk-sensitive", request.Header.Get("Authorization"))
		}
	}
	require.True(t, reflect.DeepEqual(extraBefore, account.Extra))
	require.Empty(t, repo.updates)
	require.NotContains(t, account.Extra, UpstreamBillingProbeExtraKey)
}

func TestUpstreamQuotaNewAPIUnitLookupIsBestEffort(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		statusBody string
		wantUnit   string
	}{
		{name: "USD", statusCode: http.StatusOK, statusBody: `{"success":true,"data":{"quota_display_type":"USD"}}`, wantUnit: "USD"},
		{name: "CNY", statusCode: http.StatusOK, statusBody: `{"success":true,"data":{"quota_display_type":"CNY"}}`, wantUnit: "CNY"},
		{name: "TOKENS", statusCode: http.StatusOK, statusBody: `{"success":true,"data":{"quota_display_type":"TOKENS"}}`, wantUnit: "TOKENS"},
		{name: "CUSTOM", statusCode: http.StatusOK, statusBody: `{"success":true,"data":{"quota_display_type":"CUSTOM","custom_currency_symbol":"X"}}`},
		{name: "malformed", statusCode: http.StatusOK, statusBody: `{"success":true}`},
		{name: "failed", statusCode: http.StatusBadGateway, statusBody: `secret upstream body`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := newUpstreamQuotaAccount(42)
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/v1/usage":
					return quotaHTTPResponse(http.StatusNotFound, `{}`), nil
				case "/v1/dashboard/billing/subscription":
					return quotaHTTPResponse(http.StatusOK, validNewAPISubscriptionBody()), nil
				case "/v1/dashboard/billing/usage":
					return quotaHTTPResponse(http.StatusOK, validNewAPIUsageBody()), nil
				default:
					return quotaHTTPResponse(tt.statusCode, tt.statusBody), nil
				}
			}}
			svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
			result, err := svc.QueryAccountQuota(context.Background(), account.ID)
			require.NoError(t, err)
			require.Equal(t, tt.wantUnit, result.Quota.Unit)
		})
	}
}

func TestUpstreamQuotaStatusURLUsesDeploymentRoot(t *testing.T) {
	tests := map[string]string{
		"https://user:pass@upstream.example/v1?token=secret#fragment": "https://upstream.example/api/status",
		"https://upstream.example/subpath/v1":                         "https://upstream.example/subpath/api/status",
		"https://upstream.example/subpath":                            "https://upstream.example/subpath/api/status",
	}
	for base, want := range tests {
		got, err := upstreamQuotaStatusURL(base)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestUpstreamQuotaFallbackAndSafeErrorMatrix(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantReason string
		wantCalls  int
	}{
		{name: "auth", statusCode: http.StatusUnauthorized, body: `{"secret":"sk-body"}`, wantReason: "UPSTREAM_QUOTA_AUTH_FAILED", wantCalls: 1},
		{name: "forbidden", statusCode: http.StatusForbidden, body: `{}`, wantReason: "UPSTREAM_QUOTA_AUTH_FAILED", wantCalls: 1},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, body: `{}`, wantReason: "UPSTREAM_QUOTA_RATE_LIMITED", wantCalls: 1},
		{name: "server error", statusCode: http.StatusInternalServerError, body: `{}`, wantReason: "UPSTREAM_QUOTA_INVALID_RESPONSE", wantCalls: 1},
		{name: "malformed success", statusCode: http.StatusOK, body: `{"secret":"raw-upstream-body"}`, wantReason: "UPSTREAM_QUOTA_INVALID_RESPONSE", wantCalls: 1},
		{name: "not found falls back", statusCode: http.StatusNotFound, body: `{}`, wantReason: "UPSTREAM_QUOTA_UNSUPPORTED", wantCalls: 2},
		{name: "method not allowed falls back", statusCode: http.StatusMethodNotAllowed, body: `{}`, wantReason: "UPSTREAM_QUOTA_UNSUPPORTED", wantCalls: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := newUpstreamQuotaAccount(43)
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/v1/usage" {
					return quotaHTTPResponse(tt.statusCode, tt.body), nil
				}
				return quotaHTTPResponse(http.StatusNotFound, `{"secret":"new-api-body"}`), nil
			}}
			svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
			result, err := svc.QueryAccountQuota(context.Background(), account.ID)
			require.Nil(t, result)
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
			requests, _, _ := upstream.snapshot()
			require.Len(t, requests, tt.wantCalls)
			require.NotContains(t, err.Error(), "sk-body")
			require.NotContains(t, err.Error(), "raw-upstream-body")
			require.NotContains(t, err.Error(), "new-api-body")
		})
	}
}

func TestUpstreamQuotaRejectsNewAPIHTTP200ErrorEnvelopes(t *testing.T) {
	tests := []struct {
		name      string
		errorPath string
		wantCalls int
	}{
		{name: "subscription", errorPath: "/v1/dashboard/billing/subscription", wantCalls: 2},
		{name: "usage", errorPath: "/v1/dashboard/billing/usage", wantCalls: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := newUpstreamQuotaAccount(44)
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/v1/usage" {
					return quotaHTTPResponse(http.StatusNotFound, `{}`), nil
				}
				if req.URL.Path == tt.errorPath {
					return quotaHTTPResponse(http.StatusOK, `{"error":{"message":"secret detail","type":"new_api_error","param":"","code":null}}`), nil
				}
				if req.URL.Path == "/v1/dashboard/billing/subscription" {
					return quotaHTTPResponse(http.StatusOK, validNewAPISubscriptionBody()), nil
				}
				return quotaHTTPResponse(http.StatusOK, validNewAPIUsageBody()), nil
			}}
			svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
			_, err := svc.QueryAccountQuota(context.Background(), account.ID)
			require.Equal(t, "UPSTREAM_QUOTA_INVALID_RESPONSE", infraerrors.Reason(err))
			requests, _, _ := upstream.snapshot()
			require.Len(t, requests, tt.wantCalls)
			require.NotContains(t, err.Error(), "secret detail")
		})
	}
}

func TestUpstreamQuotaPreservesRepositoryErrors(t *testing.T) {
	repo := &upstreamQuotaErrorRepo{err: ErrAccountNotFound}
	upstream := &upstreamQuotaHTTPStub{}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	result, err := svc.QueryAccountQuota(context.Background(), 404)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAccountNotFound)
	requests, _, _ := upstream.snapshot()
	require.Empty(t, requests)
}

func TestUpstreamQuotaRejectsIneligibleOrIncompleteAccountsWithoutNetworkCalls(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
	}{
		{name: "oauth", account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}},
		{name: "other platform", account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}},
		{name: "missing api key", account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://upstream.example"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: tt.account}}
			upstream := &upstreamQuotaHTTPStub{}
			svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
			_, err := svc.QueryAccountQuota(context.Background(), 1)
			require.ErrorIs(t, err, ErrUpstreamQuotaAccountInvalid)
			requests, _, _ := upstream.snapshot()
			require.Empty(t, requests)
		})
	}
}

func TestUpstreamQuotaConfiguredProxyNeverFallsBackToDirect(t *testing.T) {
	proxyID := int64(17)
	account := newUpstreamQuotaAccount(50)
	account.ProxyID = &proxyID
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &upstreamQuotaHTTPStub{}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	_, err := svc.QueryAccountQuota(context.Background(), account.ID)
	require.ErrorIs(t, err, ErrUpstreamQuotaRequestFailed)
	requests, _, _ := upstream.snapshot()
	require.Empty(t, requests)
}

func TestUpstreamQuotaRejectsInvalidNewAPINumbers(t *testing.T) {
	invalidSubscriptions := []string{
		`{"object":"billing_subscription","has_payment_method":true,"soft_limit_usd":0,"hard_limit_usd":-1,"system_hard_limit_usd":0,"access_until":0}`,
		`{"object":"billing_subscription","has_payment_method":true,"soft_limit_usd":0,"hard_limit_usd":1,"system_hard_limit_usd":0,"access_until":253402300800}`,
	}
	for _, body := range invalidSubscriptions {
		_, err := parseNewAPISubscription([]byte(body))
		require.Error(t, err)
	}
	_, err := parseNewAPIUsage([]byte(`{"object":"list","total_usage":-1}`))
	require.Error(t, err)
}

type upstreamQuotaErrorRepo struct {
	AccountRepository
	err error
}

func (r *upstreamQuotaErrorRepo) GetByID(context.Context, int64) (*Account, error) {
	return nil, r.err
}

func TestUpstreamQuotaRejectsOversizedBodiesAndMapsTimeouts(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(*http.Request) (*http.Response, error)
		wantReason string
	}{
		{
			name: "oversized",
			handler: func(*http.Request) (*http.Response, error) {
				return quotaHTTPResponse(http.StatusNotFound, strings.Repeat("x", upstreamBillingProbeMaxBodyBytes+1)), nil
			},
			wantReason: "UPSTREAM_QUOTA_INVALID_RESPONSE",
		},
		{
			name: "timeout",
			handler: func(*http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			},
			wantReason: "UPSTREAM_QUOTA_TIMEOUT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := newUpstreamQuotaAccount(45)
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			svc := newUpstreamBillingProbeTestService(repo, &upstreamQuotaHTTPStub{handler: tt.handler}, &upstreamBillingProbeSettingRepo{})
			_, err := svc.QueryAccountQuota(context.Background(), account.ID)
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
		})
	}
}

func TestUpstreamQuotaCoalescesWithIndependentCancellationAndNoCache(t *testing.T) {
	account := newUpstreamQuotaAccount(46)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	upstream := &upstreamQuotaHTTPStub{handler: func(*http.Request) (*http.Response, error) {
		once.Do(func() { close(started) })
		<-release
		return quotaHTTPResponse(http.StatusOK, validSub2APIQuotaBody()), nil
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.QueryAccountQuota(firstCtx, account.ID)
		firstDone <- err
	}()
	<-started
	secondDone := make(chan error, 1)
	secondEntered := make(chan struct{})
	go func() {
		close(secondEntered)
		_, err := svc.QueryAccountQuota(context.Background(), account.ID)
		secondDone <- err
	}()
	<-secondEntered
	time.Sleep(25 * time.Millisecond)
	cancelFirst()
	require.ErrorIs(t, <-firstDone, context.Canceled)
	close(release)
	require.NoError(t, <-secondDone)
	requests, _, _ := upstream.snapshot()
	require.Len(t, requests, 1)

	_, err := svc.QueryAccountQuota(context.Background(), account.ID)
	require.NoError(t, err)
	requests, _, _ = upstream.snapshot()
	require.Len(t, requests, 2, "completed quota results must not be cached")
}

func TestUpstreamQuotaIdentityRecheckRejectsChangedCredentials(t *testing.T) {
	account := newUpstreamQuotaAccount(47)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &upstreamQuotaHTTPStub{handler: func(*http.Request) (*http.Response, error) {
		repo.mu.Lock()
		repo.accounts[account.ID].Credentials = map[string]any{"api_key": "sk-replaced", "base_url": "https://other.example"}
		repo.mu.Unlock()
		return quotaHTTPResponse(http.StatusOK, validSub2APIQuotaBody()), nil
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	result, err := svc.QueryAccountQuota(context.Background(), account.ID)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrUpstreamQuotaIdentityChanged)
	require.Empty(t, repo.updates)
	require.NotContains(t, account.Extra, UpstreamBillingProbeExtraKey)
}

func TestUpstreamQuotaIdentityRecheckRejectsChangedConcurrency(t *testing.T) {
	account := newUpstreamQuotaAccount(49)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &upstreamQuotaHTTPStub{handler: func(*http.Request) (*http.Response, error) {
		repo.mu.Lock()
		repo.accounts[account.ID].Concurrency++
		repo.mu.Unlock()
		return quotaHTTPResponse(http.StatusOK, validSub2APIQuotaBody()), nil
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	result, err := svc.QueryAccountQuota(context.Background(), account.ID)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrUpstreamQuotaIdentityChanged)
}

func TestUpstreamQuotaDeadlineIncludesSharedSlotWait(t *testing.T) {
	account := newUpstreamQuotaAccount(48)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	deadlineRemaining := make(chan time.Duration, 1)
	upstream := &upstreamQuotaHTTPStub{handler: func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		require.True(t, ok)
		deadlineRemaining <- time.Until(deadline)
		return quotaHTTPResponse(http.StatusOK, validSub2APIQuotaBody()), nil
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	for range upstreamBillingProbeConcurrency {
		svc.probeSlots <- struct{}{}
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.QueryAccountQuota(context.Background(), account.ID)
		done <- err
	}()
	time.Sleep(120 * time.Millisecond)
	<-svc.probeSlots
	require.NoError(t, <-done)
	remaining := <-deadlineRemaining
	require.Less(t, remaining, upstreamBillingProbeRequestTimeout-75*time.Millisecond)
	require.Greater(t, remaining, 8*time.Second)
	for len(svc.probeSlots) > 0 {
		<-svc.probeSlots
	}
}

type sharedQuotaBlockingHTTP struct {
	active  atomic.Int64
	max     atomic.Int64
	calls   atomic.Int64
	started chan struct{}
	release <-chan struct{}
}

func (u *sharedQuotaBlockingHTTP) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return u.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (u *sharedQuotaBlockingHTTP) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls.Add(1)
	active := u.active.Add(1)
	defer u.active.Add(-1)
	for {
		peak := u.max.Load()
		if active <= peak || u.max.CompareAndSwap(peak, active) {
			break
		}
	}
	u.started <- struct{}{}
	<-u.release
	if req.URL.Path == "/v1/sub2api/billing" {
		return quotaHTTPResponse(http.StatusOK, `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":1,"resolved_rate_multiplier":1,"peak_rate_enabled":false,"effective_rate_multiplier":1,"observed_at":"2026-07-17T00:00:00Z"}`), nil
	}
	return quotaHTTPResponse(http.StatusOK, validSub2APIQuotaBody()), nil
}

func TestUpstreamQuotaSharesFourSlotsWithBillingRateProbes(t *testing.T) {
	accounts := make(map[int64]*Account, 8)
	for id := int64(1); id <= 8; id++ {
		accounts[id] = newUpstreamQuotaAccount(id)
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: accounts}
	release := make(chan struct{})
	upstream := &sharedQuotaBlockingHTTP{started: make(chan struct{}, 8), release: release}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for id := int64(1); id <= 4; id++ {
		wg.Add(1)
		go func(accountID int64) {
			defer wg.Done()
			_, err := svc.ProbeAccount(context.Background(), accountID)
			errs <- err
		}(id)
	}
	for id := int64(5); id <= 8; id++ {
		wg.Add(1)
		go func(accountID int64) {
			defer wg.Done()
			_, err := svc.QueryAccountQuota(context.Background(), accountID)
			errs <- err
		}(id)
	}
	for range upstreamBillingProbeConcurrency {
		select {
		case <-upstream.started:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for shared probe slots")
		}
	}
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(upstreamBillingProbeConcurrency), upstream.calls.Load())
	require.Equal(t, int64(upstreamBillingProbeConcurrency), upstream.max.Load())
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.LessOrEqual(t, upstream.max.Load(), int64(upstreamBillingProbeConcurrency))
}
