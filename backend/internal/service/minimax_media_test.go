package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func mediaFloat(v float64) *float64 { return &v }
func mediaTestPrice(res string) MiniMaxMediaPricing {
	return MiniMaxMediaPricing{Resolution: res, CostCurrency: "CNY", CNYPerUSD: 7, Multiplier: mediaFloat(1.2), Retail: MiniMaxMediaRates{OutputSecond: mediaFloat(.8), InputSecond: mediaFloat(.8), ExtraImage: mediaFloat(.2)}, Cost: MiniMaxMediaRates{OutputSecond: mediaFloat(.8), InputSecond: mediaFloat(.8), ExtraImage: mediaFloat(.2)}}
}
func TestMiniMaxMediaPricingReceipts(t *testing.T) {
	p := mediaTestPrice("2K")
	for _, tc := range []struct {
		u    minimax.Usage
		want float64
	}{{minimax.Usage{OutputSeconds: 6}, 4.8}, {minimax.Usage{OutputSeconds: 6, InputImages: 5}, 4.8}, {minimax.Usage{OutputSeconds: 6, InputImages: 6}, 5}, {minimax.Usage{OutputSeconds: 6, InputSeconds: 6, InputImages: 9}, 10.4}} {
		b, err := p.Bill(minimax.H3, tc.u)
		require.NoError(t, err)
		require.InDelta(t, tc.want, b.UpstreamCost, 1e-8)
		require.InDelta(t, tc.want*1.2, b.ChargedUSD, 1e-8)
		require.InDelta(t, tc.want/7, b.UpstreamUSD, 1e-8)
	}
	p.Retail.OutputSecond = nil
	_, err := p.Bill(minimax.H3, minimax.Usage{OutputSeconds: 6})
	require.Error(t, err, "missing price must fail closed")
	p = mediaTestPrice("2K")
	p.Multiplier = mediaFloat(0)
	b, err := p.Bill(minimax.H3, minimax.Usage{OutputSeconds: 6})
	require.NoError(t, err)
	require.Zero(t, b.ChargedUSD)
	p.Multiplier = nil
	require.Error(t, p.Validate(minimax.H3))
	p = mediaTestPrice("2K")
	p.CNYPerUSD = 0
	require.Error(t, p.Validate(minimax.H3))
	for _, tc := range []struct {
		model       string
		price, want float64
	}{{minimax.SpeechHD, 350, .7}, {minimax.SpeechTurbo, 200, .4}} {
		p := MiniMaxMediaPricing{CostCurrency: "CNY", CNYPerUSD: 7, Multiplier: mediaFloat(1), Cost: MiniMaxMediaRates{CharactersPerMillion: mediaFloat(tc.price)}, Retail: MiniMaxMediaRates{CharactersPerMillion: mediaFloat(tc.price)}}
		b, err := p.Bill(tc.model, minimax.Usage{Characters: 2000})
		require.NoError(t, err)
		require.Equal(t, tc.want, b.ChargedUSD)
	}
}
func TestMiniMaxMediaExplicitAccountAndOrigin(t *testing.T) {
	a := &Account{Platform: PlatformMiniMax, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": "payg", "api_key": "fake"}, Extra: map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.cn"}}
	origin, err := MiniMaxMediaOrigin(a)
	require.NoError(t, err)
	require.Equal(t, "https://api.minimax.cn", origin)
	for _, value := range []string{"https://api.minimax.cn/v1", "https://api.minimax.cn/anthropic", "https://api.minimaxi.com/v1", "http://api.minimax.cn", "https://evil.example"} {
		a.Extra["minimax_media_base_url"] = value
		_, err := MiniMaxMediaOrigin(a)
		require.Error(t, err)
	}
	a.Extra["minimax_media_base_url"] = "https://api.minimax.cn"
	a.Credentials["account_mode"] = "coding"
	origin, err = MiniMaxMediaOrigin(a)
	require.NoError(t, err)
	require.Equal(t, "https://api.minimax.cn", origin)
}

type mediaHTTPStub struct {
	calls int
	fn    func(*http.Request) (*http.Response, error)
}

func (s *mediaHTTPStub) Do(r *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.calls++
	return s.fn(r)
}
func (s *mediaHTTPStub) DoWithTLS(r *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(r, p, id, n)
}
func TestMiniMaxMediaTransportDoesNotReplayUnknownCreate(t *testing.T) {
	upstream := &mediaHTTPStub{fn: func(r *http.Request) (*http.Response, error) {
		require.True(t, HTTPUpstreamRedirectsDisabled(r.Context()))
		require.Nil(t, r.GetBody)
		require.Empty(t, r.Header.Get("Idempotency-Key"))
		return nil, errors.New("unknown outcome")
	}}
	s := NewMiniMaxMediaService(nil, &OpenAIGatewayService{httpUpstream: upstream}, nil)
	defer s.cancel()
	_, err := s.Do(context.Background(), &Account{Credentials: map[string]any{"api_key": "test"}}, "https://api.minimax.cn", "POST", "/v2/video_generation", []byte(`{}`))
	require.Error(t, err)
	require.Equal(t, 1, upstream.calls)
}
func TestMiniMaxMediaSystemVoiceIsolation(t *testing.T) {
	upstream := &mediaHTTPStub{fn: func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		require.JSONEq(t, `{"voice_type":"system"}`, string(body))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"base_resp":{"status_code":0},"system_voice":[{"voice_id":"sys"}],"voice_cloning":[{"voice_id":"private"}]}`))}, nil
	}}
	s := NewMiniMaxMediaService(nil, &OpenAIGatewayService{httpUpstream: upstream}, nil)
	defer s.cancel()
	a := &Account{ID: 1, Platform: PlatformMiniMax, Type: AccountTypeAPIKey, Credentials: map[string]any{"account_mode": "payg", "api_key": "fake"}, Extra: map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.cn"}}
	body, ids, err := s.SystemVoices(context.Background(), a)
	require.NoError(t, err)
	require.True(t, ids["sys"])
	require.False(t, ids["private"])
	require.NotContains(t, string(body), "private")
	_, _, err = s.SystemVoices(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, 1, upstream.calls)
}
func TestMiniMaxMediaGroupPricingValidation(t *testing.T) {
	p := mediaTestPrice("768P")
	p2 := mediaTestPrice("2K")
	entries := []ChannelModelPricing{{Models: []string{minimax.H3}, BillingMode: BillingModeVideo, MiniMaxMedia: &p}, {Models: []string{minimax.H3}, BillingMode: BillingModeVideo, MiniMaxMedia: &p2}}
	_, err := normalizeGroupModelPricing(PlatformMiniMax, entries)
	require.NoError(t, err)
	entries[1].MiniMaxMedia = &p
	_, err = normalizeGroupModelPricing(PlatformMiniMax, entries)
	require.Error(t, err)
	_, err = normalizeGroupModelPricing(PlatformOpenAI, entries[:1])
	require.Error(t, err)
}

type mediaRefreshRepo struct {
	MiniMaxMediaRepository
	task      *MiniMaxMediaTask
	refreshes int
}

func (r *mediaRefreshRepo) RefreshResult(_ context.Context, _ string, result json.RawMessage) error {
	r.refreshes++
	r.task.Result = result
	return nil
}
func (r *mediaRefreshRepo) Get(context.Context, string, int64, int64) (*MiniMaxMediaTask, error) {
	return r.task, nil
}

type mediaRefreshAccounts struct{ AccountRepository }

func (*mediaRefreshAccounts) GetByID(context.Context, int64) (*Account, error) {
	return &Account{Platform: PlatformMiniMax, Credentials: map[string]any{"api_key": "fake"}}, nil
}
func TestMiniMaxMediaCompletedVideoRefreshDoesNotRebill(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same receipt", true: "changed receipt rejected"}[changed], func(t *testing.T) {
			task := &MiniMaxMediaTask{ID: "mm_refresh", UpstreamID: "provider", Kind: "video", Model: minimax.H3, Resolution: "2K", Duration: 6, Origin: "https://api.minimax.cn", Status: "succeeded", BillingState: "settled", CreatedAt: time.Now(), Bill: &MiniMaxMediaBill{Usage: minimax.Usage{OutputSeconds: 6}}}
			repo := &mediaRefreshRepo{task: task}
			output := 6
			if changed {
				output = 7
			}
			upstream := &mediaHTTPStub{fn: func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "GET", req.Method)
				require.Equal(t, "/v2/query/video_generation/provider", req.URL.Path)
				body := fmt.Sprintf(`{"task":{"id":"provider","model":"MiniMax-H3","status":"succeeded","resolution":"2K","duration":6,"content":{"url":"https://example.com/refreshed.mp4"},"usage":{"output_seconds":%d,"input_seconds":0,"input_image_count":0}}}`, output)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			}}
			s := NewMiniMaxMediaService(repo, &OpenAIGatewayService{accountRepo: &mediaRefreshAccounts{}, httpUpstream: upstream}, nil)
			defer s.cancel()
			s.RefreshCompleted(context.Background(), task)
			s.RefreshCompleted(context.Background(), task)
			require.Equal(t, 1, upstream.calls)
			if changed {
				require.Zero(t, repo.refreshes)
			} else {
				require.Equal(t, 1, repo.refreshes)
				require.Contains(t, string(task.Result), "refreshed.mp4")
			}
			// An embedded, unimplemented ledger panics if any rebilling is attempted.
		})
	}
}
func TestMiniMaxMediaRefreshCacheIsBounded(t *testing.T) {
	s := NewMiniMaxMediaService(nil, nil, nil)
	defer s.cancel()
	now := time.Now()
	for i := 0; i < 1024; i++ {
		require.True(t, s.claimResultRefresh(fmt.Sprint(i), now))
	}
	require.False(t, s.claimResultRefresh("overflow", now))
	require.False(t, s.claimResultRefresh("0", now))
	require.True(t, s.claimResultRefresh("new", now.Add(time.Minute)))
	require.Len(t, s.resultRefresh, 1)
}

type mediaCatalogAccounts struct {
	AccountRepository
	accounts []Account
}

func (r *mediaCatalogAccounts) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return r.accounts, nil
}

type mediaCatalogGroup struct {
	GroupRepository
	group *Group
}

func (r *mediaCatalogGroup) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }

func TestMiniMaxMediaCatalogRequiresCapabilityAndPrice(t *testing.T) {
	p := mediaTestPrice("2K")
	group := &Group{ID: 1, Platform: PlatformMiniMax, ModelPricing: []ChannelModelPricing{{Models: []string{minimax.H3}, MiniMaxMedia: &p}}}
	account := Account{Platform: PlatformMiniMax, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fake", "account_mode": "coding", "model_mapping": map[string]any{"MiniMax-M2": "MiniMax-M2"}}, Extra: map[string]any{"minimax_media_enabled": true, "minimax_media_base_url": "https://api.minimax.cn"}}
	accounts := &mediaCatalogAccounts{accounts: []Account{account}}
	s := &GatewayService{accountRepo: accounts, groupRepo: &mediaCatalogGroup{group: group}}
	require.Contains(t, s.GetAvailableModels(context.Background(), &group.ID, PlatformMiniMax), minimax.H3)
	require.Contains(t, s.GetAvailableModels(context.Background(), &group.ID, PlatformMiniMax), minimax.Image)
	account.Extra["minimax_media_enabled"] = false
	require.Equal(t, []string{"MiniMax-M2"}, s.GetAvailableModels(context.Background(), &group.ID, PlatformMiniMax))
	account.Extra["minimax_media_enabled"] = true
	group.ModelPricing = nil
	require.Contains(t, s.GetAvailableModels(context.Background(), &group.ID, PlatformMiniMax), minimax.SpeechHD)
}
