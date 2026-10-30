package service

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/stretchr/testify/require"
)

type miniMaxOwnedAssetRepo struct {
	MiniMaxMediaRepository
	asset *MiniMaxMediaAsset
}

func (r *miniMaxOwnedAssetRepo) FindAsset(_ context.Context, _, _ string, userID, keyID int64) (*MiniMaxMediaAsset, error) {
	if r.asset == nil || r.asset.UserID != userID || r.asset.APIKeyID != keyID {
		return nil, ErrMiniMaxMediaNotFound
	}
	return r.asset, nil
}

func (*miniMaxOwnedAssetRepo) SaveAsset(context.Context, *MiniMaxMediaAsset) error { return nil }

func TestMiniMaxPrivateVoicePricingRequiresOwnerAndOriginalAccount(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		userID, keyID, accountID int64
		activated, allowed       bool
	}{
		{"owned voice", 1, 2, 3, false, true},
		{"activated voice", 1, 2, 3, true, true},
		{"other user", 4, 2, 3, false, false},
		{"other key", 1, 4, 3, false, false},
		{"other provider account", 1, 2, 4, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &miniMaxOwnedAssetRepo{asset: &MiniMaxMediaAsset{UserID: tc.userID, APIKeyID: tc.keyID, AccountID: tc.accountID, ID: "PrivateVoice", Category: "voice_clone", Activated: tc.activated}}
			upstream := &mediaHTTPStub{fn: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"base_resp":{"status_code":0},"system_voice":[{"voice_id":"system"}]}`))}, nil
			}}
			s := NewMiniMaxMediaService(repo, &OpenAIGatewayService{httpUpstream: upstream}, nil)
			defer s.cancel()
			key := &APIKey{ID: 2, UserID: 1, Group: &Group{RateMultiplier: 1}}
			a := &Account{ID: 3, Platform: PlatformMiniMax, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Extra: map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.io"}}
			req, err := minimax.ParseRequest([]byte(`{"model":"speech-2.8-turbo","text":"hi","voice_setting":{"voice_id":"PrivateVoice"}}`), "speech")
			require.NoError(t, err)
			price, err := s.Price(key.Group, req.Model, "")
			require.NoError(t, err)
			err = s.prepareVoicePricing(context.Background(), key, a, req, &price)
			require.Equal(t, tc.allowed, err == nil)
			if tc.allowed && !tc.activated {
				require.Len(t, price.CustomVoices, 1)
				require.Equal(t, 1.5, price.CustomVoices[0].Retail)
			} else {
				require.Empty(t, price.CustomVoices)
			}
		})
	}
}

type miniMaxMeterRepo struct {
	MiniMaxMediaRepository
	bill *MiniMaxMediaBill
}

func (r *miniMaxMeterRepo) Record(_ context.Context, _, _ string, b *MiniMaxMediaBill, _ json.RawMessage) error {
	r.bill = b
	return nil
}
func (*miniMaxMeterRepo) Settle(context.Context, string) (bool, error) { return false, nil }

func TestMiniMaxVoiceDesignReceiptRetainsMeterSource(t *testing.T) {
	for _, source := range []string{"upstream_usage", "request_characters"} {
		t.Run(source, func(t *testing.T) {
			repo := &miniMaxMeterRepo{}
			s := NewMiniMaxMediaService(repo, nil, nil)
			defer s.cancel()
			price, err := s.Price(&Group{RateMultiplier: 1}, "voice_design", "")
			require.NoError(t, err)
			task := &MiniMaxMediaTask{ID: "mm_design", Model: "voice_design", Pricing: price}
			require.NoError(t, s.Complete(context.Background(), task, "succeeded", minimax.Usage{Characters: 3}, nil, source))
			require.Equal(t, source, repo.bill.MeterSource)
		})
	}
}

func TestMiniMaxUnifiedDefaultsOverridesAndLegacySnapshots(t *testing.T) {
	s := &MiniMaxMediaService{}
	g := &Group{RateMultiplier: 2}
	p, err := s.Price(g, minimax.SpeechHD, "")
	require.NoError(t, err)
	b, err := p.Bill(minimax.SpeechHD, minimax.Usage{Characters: 1000})
	require.NoError(t, err)
	require.Equal(t, .2, b.ChargedUSD)
	// New zero overrides remain free, including when a legacy card exists.
	legacy := p
	legacy.Retail.CharactersPerMillion = mediaFloat(350)
	legacy.Multiplier = mediaFloat(19)
	g.ModelPricing = []ChannelModelPricing{{Models: []string{minimax.SpeechHD}, MiniMaxMedia: &legacy}}
	p, err = s.Price(g, minimax.SpeechHD, "")
	require.NoError(t, err)
	require.Equal(t, 350.0, *p.Retail.CharactersPerMillion)
	require.Equal(t, 2.0, *p.Multiplier)
	g.MiniMaxMediaPrices = map[string]map[string]float64{minimax.SpeechHD: {"characters_per_million": 0}}
	p, err = s.Price(g, minimax.SpeechHD, "")
	require.NoError(t, err)
	b, err = p.Bill(minimax.SpeechHD, minimax.Usage{Characters: 1000})
	require.NoError(t, err)
	require.Zero(t, b.ChargedUSD)
	require.Equal(t, .1, b.UpstreamUSD)
	g.MiniMaxRateIndependent = true
	g.MiniMaxRateMultiplier = .25
	p, err = s.Price(g, minimax.SpeechTurbo, "")
	require.NoError(t, err)
	require.Equal(t, .25, *p.Multiplier)
	// A frozen v1 receipt retains its original multiplier and five free images.
	old := mediaTestPrice("2K")
	b, err = old.Bill(minimax.H3, minimax.Usage{OutputSeconds: 6, InputImages: 6})
	require.NoError(t, err)
	require.Equal(t, 6.0, b.ChargedUSD)
}
func TestMiniMaxUnifiedModelUnitsAndFreeImages(t *testing.T) {
	s := &MiniMaxMediaService{}
	g := &Group{RateMultiplier: 1}
	for _, tc := range []struct {
		model, res string
		u          minimax.Usage
		want       float64
	}{
		{minimax.H3, "768P", minimax.Usage{OutputSeconds: 4, InputSeconds: 2, InputImages: 6}, .52},
		{minimax.H3Highspeed, "768P", minimax.Usage{OutputSeconds: 4, InputImages: 6}, .27},
		{minimax.H3Lite, "480P", minimax.Usage{OutputSeconds: 4}, .1},
		{minimax.H3Max, "480P", minimax.Usage{OutputSeconds: 5, InputSeconds: 2, InputImages: 3}, .4346},
		{minimax.Image, "", minimax.Usage{OutputImages: 3}, .0105},
		{"voice_design", "", minimax.Usage{Characters: 100}, .006},
		{"voice_clone", "", minimax.Usage{}, 0},
	} {
		p, e := s.Price(g, tc.model, tc.res)
		require.NoError(t, e)
		b, e := p.Bill(tc.model, tc.u)
		require.NoError(t, e)
		require.InDelta(t, tc.want, b.ChargedUSD, 1e-8)
	}
	_, e := s.Price(g, minimax.H3Highspeed, "2K")
	require.Error(t, e)
	require.Error(t, ValidateMiniMaxMediaPrices(map[string]map[string]float64{minimax.Image: {"image": math.NaN()}}))
	require.Error(t, ValidateMiniMaxMediaPrices(map[string]map[string]float64{minimax.Image: {"output_second": 1}}))
	require.NoError(t, ValidateMiniMaxMediaPrices(map[string]map[string]float64{minimax.Image: {"image": 0}}))
}
func TestMiniMaxUnifiedVoiceFeeReceipt(t *testing.T) {
	p, e := (&MiniMaxMediaService{}).Price(&Group{RateMultiplier: 2}, minimax.SpeechTurbo, "")
	require.NoError(t, e)
	p.CustomVoices = []MiniMaxVoiceCharge{{ID: "private-voice", Kind: "voice_clone", Retail: 1.5, Cost: 1.5}}
	b, e := p.Bill(minimax.SpeechTurbo, minimax.Usage{Characters: 1000, VoiceCount: 1})
	require.NoError(t, e)
	require.Equal(t, 3.12, b.ChargedUSD)
	b, e = p.Bill(minimax.SpeechTurbo, minimax.Usage{Characters: 1000})
	require.NoError(t, e)
	require.Equal(t, .12, b.ChargedUSD)
	_, e = p.Bill(minimax.SpeechTurbo, minimax.Usage{Characters: 1000, VoiceCount: 2})
	require.Error(t, e)
	p.CustomVoices = nil
	_, e = p.Bill(minimax.SpeechTurbo, minimax.Usage{Characters: 1000, VoiceCount: 1})
	require.Error(t, e)
}

type minimaxReserveCapture struct {
	MiniMaxMediaRepository
	task *MiniMaxMediaTask
}

func (r *minimaxReserveCapture) Reserve(_ context.Context, t *MiniMaxMediaTask) (*MiniMaxMediaTask, bool, error) {
	r.task = t
	return t, true, nil
}
func TestMiniMaxUnifiedReservationUsesUserGroupMultiplier(t *testing.T) {
	override := .35
	resolver := newUserGroupRateResolver(&userGroupRateResolverRepoStub{rate: &override}, nil, 0, nil, "")
	repo := &minimaxReserveCapture{}
	s := NewMiniMaxMediaService(repo, &OpenAIGatewayService{userGroupRateResolver: resolver}, nil)
	defer s.cancel()
	id := int64(1)
	key := &APIKey{ID: 2, UserID: 3, GroupID: &id, Group: &Group{ID: id, RateMultiplier: 2, PeakRateEnabled: true, PeakRateMultiplier: 10}}
	a := &Account{ID: 4, Platform: PlatformMiniMax, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Extra: map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.io"}}
	_, _, e := s.Reserve(context.Background(), key, nil, a, &minimax.Request{Model: minimax.Image, N: 2}, "image", "id", []byte(`{}`))
	require.NoError(t, e)
	require.Equal(t, .35, *repo.task.Pricing.Multiplier)
	require.Equal(t, .00245, repo.task.ReservedUSD)
	key.Group.MiniMaxRateIndependent = true
	key.Group.MiniMaxRateMultiplier = 0
	_, _, e = s.Reserve(context.Background(), key, nil, a, &minimax.Request{Model: minimax.Image, N: 2}, "image", "id2", []byte(`{}`))
	require.NoError(t, e)
	require.Zero(t, repo.task.ReservedUSD)
}

func TestMiniMaxSimpleModeFreezesBillingPolicy(t *testing.T) {
	for _, windows := range []bool{false, true} {
		repo := &minimaxReserveCapture{}
		cfg := &config.Config{RunMode: config.RunModeSimple, SimpleModeKeyRateLimitEnabled: windows}
		s := NewMiniMaxMediaService(repo, &OpenAIGatewayService{cfg: cfg}, nil)
		defer s.cancel()
		id := int64(1)
		key := &APIKey{ID: 2, UserID: 3, GroupID: &id, RateLimit5h: 1, Group: &Group{ID: id, RateMultiplier: 1, SubscriptionType: "subscription"}}
		a := &Account{ID: 4, Platform: PlatformMiniMax, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Extra: map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.io"}}
		_, _, err := s.Reserve(context.Background(), key, nil, a, &minimax.Request{Model: minimax.Image, N: 1}, "image", "id", []byte(`{}`))
		require.NoError(t, err, "simple mode needs no active subscription")
		require.Nil(t, repo.task.SubscriptionID)
		if windows {
			require.Equal(t, MiniMaxBillingKeyWindows, repo.task.BillingPolicy)
			require.Equal(t, .0035, repo.task.ReservedUSD)
		} else {
			require.Equal(t, MiniMaxBillingSimple, repo.task.BillingPolicy)
			require.Zero(t, repo.task.ReservedUSD)
		}
		cfg.RunMode = config.RunModeStandard
		require.False(t, repo.task.UsesStandardBilling(), "a runtime mode change cannot change the saved task policy")
	}
}

func TestMiniMaxCompositePublicModelAllowlist(t *testing.T) {
	s := &MiniMaxMediaService{}
	g := &Group{RateMultiplier: 1, ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"public-image"}}}
	ctx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{Matched: true, PublicModel: "public-image", UpstreamModel: minimax.Image, TargetPlatform: PlatformMiniMax})
	require.NoError(t, s.ValidateRequest(ctx, g, minimax.Image))
	require.Error(t, s.ValidateRequest(context.Background(), g, minimax.Image))
	_, err := s.Price(g, minimax.Image, "")
	require.NoError(t, err, "native pricing is independent of public model access")
}

type miniMaxReplayRepo struct {
	MiniMaxMediaRepository
	task *MiniMaxMediaTask
}

func (r *miniMaxReplayRepo) FindByIdempotencyKey(_ context.Context, key int64, idem string) (*MiniMaxMediaTask, error) {
	if r.task == nil || r.task.APIKeyID != key || r.task.IdempotencyKey != idem {
		return nil, ErrMiniMaxMediaNotFound
	}
	return r.task, nil
}
func TestMiniMaxReplayUsesOwnerAndOriginalRequest(t *testing.T) {
	original := []byte(`{"model":"public-image","prompt":"hi"}`)
	rewritten := []byte(`{"model":"image-01","prompt":"hi"}`)
	repo := &miniMaxReplayRepo{task: &MiniMaxMediaTask{ID: "mm_original", UserID: 1, APIKeyID: 2, Kind: "image", IdempotencyKey: "same", RequestHash: HashUsageRequestPayload(original)}}
	s := &MiniMaxMediaService{repo: repo}
	key := &APIKey{ID: 2, UserID: 1}
	ctx := WithMiniMaxRequestHash(context.Background(), original)
	got, err := s.Replay(ctx, key, "image", "same", rewritten)
	require.NoError(t, err)
	require.Equal(t, repo.task, got)
	_, err = s.Replay(context.Background(), key, "image", "same", rewritten)
	require.ErrorIs(t, err, ErrMiniMaxMediaConflict)
	_, err = s.Replay(ctx, key, "speech", "same", rewritten)
	require.ErrorIs(t, err, ErrMiniMaxMediaConflict)
	key.UserID = 3
	_, err = s.Replay(ctx, key, "image", "same", rewritten)
	require.ErrorIs(t, err, ErrMiniMaxMediaConflict)
	got, err = s.Replay(ctx, key, "image", "missing", rewritten)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestMiniMaxCompletePreservesUnattributedUsage(t *testing.T) {
	repo := &miniMaxMeterRepo{}
	s := NewMiniMaxMediaService(repo, nil, nil)
	defer s.cancel()
	p, err := s.Price(&Group{RateMultiplier: 1}, minimax.SpeechTurbo, "")
	require.NoError(t, err)
	p.CustomVoices = []MiniMaxVoiceCharge{{ID: "A", Retail: 1.5, Cost: 1.5}, {ID: "B", Retail: 3, Cost: 3}}
	u := minimax.Usage{Characters: 100, VoiceCount: 1}
	require.NoError(t, s.Complete(context.Background(), &MiniMaxMediaTask{ID: "mm_partial", Model: minimax.SpeechTurbo, Pricing: p}, "succeeded", u, nil, "upstream_usage"))
	require.Equal(t, u, repo.bill.Usage)
	require.True(t, repo.bill.ReconciliationRequired)
	require.Equal(t, "upstream_usage", repo.bill.MeterSource)
	require.False(t, repo.bill.CompletedAt.IsZero())
}
