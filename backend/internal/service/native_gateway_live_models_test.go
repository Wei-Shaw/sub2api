//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// No test here may reach a provider: every transport redirects to a local peer
// or fails the test.
type liveModelsFixture struct {
	t       *testing.T
	gateway *OpenAIGatewayService
	reader  *GatewayAccountModelsReader
	input   GatewayAccountModelsInput
	account *Account
	route   GatewayNativeRoute
	scope   GatewayNativeCredentialScope
	calls   atomic.Int32
	auth    atomic.Value
	path    atomic.Value
	now     atomic.Int64
}

func newLiveModelsFixture(t *testing.T, profile, base, anchor string, handler http.HandlerFunc) *liveModelsFixture {
	t.Helper()
	f := &liveModelsFixture{t: t}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.auth.Store(r.Header.Get("Authorization"))
		f.path.Store(r.URL.Path)
		handler(w, r)
	}))
	t.Cleanup(peer.Close)
	target, err := url.Parse(peer.URL)
	require.NoError(t, err)
	f.account = nativeReviewStreamAccount(t, base, profile)
	f.account.CreatedAt = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	f.account.Extra[GatewayModelExtraKey] = anchor
	f.route, err = GatewayNativeDescriptor(f.account)
	require.NoError(t, err)
	f.scope, err = GatewayNativeCredentialScopeForAccount(f.account)
	require.NoError(t, err)
	f.gateway = &OpenAIGatewayService{accountRepo: &gatewayIdentityRepoFixture{row: f.account}}
	f.gateway.gatewayLiveModels = NewGatewayLiveModels(kimiTierPeerTransport{peer: target})
	f.now.Store(time.Now().UnixNano())
	f.gateway.gatewayLiveModels.now = func() time.Time { return time.Unix(0, f.now.Load()) }
	f.input = GatewayAccountModelsInput{GatewayAccountLimitsInput: GatewayAccountLimitsInput{f.scope.Owner, f.scope.Account, profile, 7}}
	f.reader, err = NewGatewayAccountModelsReader(nil, f.gateway, func(context.Context, GatewayAccountLimitsInput) (GatewayNativeRoute, error) {
		return f.route, nil
	})
	require.NoError(t, err)
	return f
}

func (f *liveModelsFixture) advance(d time.Duration) { f.now.Add(int64(d)) }

func (f *liveModelsFixture) read(force bool) []string {
	f.t.Helper()
	input := f.input
	input.ForceRefresh = force
	result := f.reader.Read(nativeFixtureContext(f.t, context.Background()), input)
	wire, err := json.Marshal(result)
	require.NoError(f.t, err)
	require.NotContains(f.t, string(wire), "sandbox-fixture")
	require.NotContains(f.t, string(wire), "gcn1.")
	require.Equal(f.t, "snapshot", result.Status, "the catalogue is always a snapshot")
	require.Equal(f.t, gatewayModelCatalogSource(f.route.Profile), result.Source)
	ids := make([]string, 0, len(*result.Models))
	for _, model := range *result.Models {
		require.Equal(f.t, GatewayAccountModelChoices{Allowed: []string{"function"}, Default: "function"}, model.Tools)
		require.Nil(f.t, model.Reasoning)
		require.Nil(f.t, model.Fast)
		ids = append(ids, model.ModelID)
	}
	return ids
}

func (f *liveModelsFixture) principal() string {
	return gatewayLiveModelsKey(f.route.Profile, f.scope, f.account.GetCredential("api_key"))
}

func modelsJSON(ids ...string) string {
	rows := make([]string, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, fmt.Sprintf(`{"id":%q,"object":"model"}`, id))
	}
	return `{"object":"list","data":[` + strings.Join(rows, ",") + `]}`
}

func serveModels(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

// The provider's own list is read with the existing builders and parser: path
// from the profile, Bearer key from the lease, anchor first, static models next,
// then the live list; invalid and duplicate IDs are dropped.
func TestGatewayCodingPlanLiveModelsFromProvider(t *testing.T) {
	for _, tuple := range []struct {
		profile, base, anchor, path string
		static                      []string
	}{
		{GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, "/api/v1/models", []string{GatewayGLMCodingResponsesModelFull}},
		{GatewayMiniMaxMPlanResponsesProfile, GatewayMiniMaxMPlanResponsesBaseURL, GatewayMiniMaxMPlanResponsesModel, "/v1/models", nil},
		{GatewayKimiCodeResponsesProfile, GatewayKimiCodeResponsesBaseURL, GatewayKimiCodeResponsesModel, "/coding/v1/models", nil},
	} {
		t.Run(tuple.profile, func(t *testing.T) {
			f := newLiveModelsFixture(t, tuple.profile, tuple.base, tuple.anchor, serveModels(modelsJSON("vendor/live-b", tuple.anchor, "vendor/live-a", "bad model", "vendor/live-a")))
			want := append(append([]string{tuple.anchor}, tuple.static...), "vendor/live-a", "vendor/live-b")
			require.Equal(t, want, f.read(false))
			require.EqualValues(t, 1, f.calls.Load())
			require.Equal(t, tuple.path, f.path.Load())
			require.Equal(t, "Bearer sandbox-fixture", f.auth.Load(), "the leased key is sent as a Bearer token")
			// A fresh entry serves reads without the provider; a forced refresh asks
			// again, but not within a minute of the last good read.
			require.Equal(t, want, f.read(false))
			require.Equal(t, want, f.read(true))
			require.EqualValues(t, 1, f.calls.Load())
			f.advance(gatewayLiveModelsForceAfter)
			require.Equal(t, want, f.read(true))
			require.EqualValues(t, 2, f.calls.Load())
		})
	}
}

// The catalogue is bounded by the Gateway contract (512 models), not by the old
// 8-model limit; the cut is deterministic and admission follows what is published.
func TestGatewayLiveModelsAreBoundedToTheContract(t *testing.T) {
	require.Equal(t, 512, gatewayLiveModelsMax, "the Gateway contract bounds a catalogue snapshot to 512 models")
	ids := make([]string, 0, 600)
	for i := 0; i < 600; i++ {
		ids = append(ids, fmt.Sprintf("vendor/model-%03d", i))
	}
	for _, tuple := range []struct{ profile, base, anchor string }{
		{GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel},
		{GatewayNVIDIABuildChatProfile, GatewayNVIDIABuildBaseURL, GatewayNVIDIABuildAnchorModel},
	} {
		t.Run(tuple.profile, func(t *testing.T) {
			f := newLiveModelsFixture(t, tuple.profile, tuple.base, tuple.anchor, serveModels(modelsJSON(ids...)))
			got := f.read(false)
			require.Len(t, got, gatewayLiveModelsMax)
			static := gatewayCatalogStaticModelIDs(tuple.profile)
			require.Equal(t, static, got[:len(static)], "the anchor and static models come first")
			require.Equal(t, "vendor/model-000", got[len(static)])
			last := got[len(got)-1]
			// Native admits exactly what it publishes, not the rest of the provider's list.
			envelope := f.account.GetCredential("api_key")
			require.True(t, f.gateway.gatewayModelCatalogueAdmitted(f.scope, envelope, tuple.profile, last))
			require.False(t, f.gateway.gatewayModelCatalogueAdmitted(f.scope, envelope, tuple.profile, "vendor/model-599"))
		})
	}
}

// NVIDIA's list is public: it is read without any key or lease, at the fixed
// models path, and only models that can review code reach the catalogue.
func TestGatewayNVIDIALiveModelsFromThePublicList(t *testing.T) {
	f := newLiveModelsFixture(t, GatewayNVIDIABuildChatProfile, GatewayNVIDIABuildBaseURL, GatewayNVIDIABuildAnchorModel, serveModels(modelsJSON(
		"openai/gpt-oss-20b", "nvidia/nemotron-3-embed-1b", GatewayNVIDIABuildAnchorModel, "nvidia/nemotron-3.5-content-safety",
		"z-ai/glm-5.3-flash", "nvidia/llama-3.1-nemoguard-8b-topic-control", "nvidia/nvclip", "openai/gpt-oss-20b", "bad model",
	)))
	want := []string{GatewayNVIDIABuildAnchorModel, "openai/gpt-oss-20b", "z-ai/glm-5.3-flash"}
	require.Equal(t, want, f.read(false))
	require.Equal(t, "/v1/models", f.path.Load())
	require.Empty(t, f.auth.Load(), "the public list is read without a key")
	// Repeated forced refreshes of the shared entry do not reach the provider
	// within a minute of the last good read.
	for i := 0; i < 3; i++ {
		require.Equal(t, want, f.read(true))
	}
	require.EqualValues(t, 1, f.calls.Load())
	// Without credential custody the public list is still read: no lease is involved.
	f.advance(gatewayLiveModelsForceAfter)
	ctx, err := WithGatewayNativeConsumer(context.Background(), custodyTestScope().Consumer)
	require.NoError(t, err)
	input := f.input
	input.ForceRefresh = true
	result := f.reader.Read(ctx, input)
	require.Equal(t, "snapshot", result.Status)
	require.Len(t, *result.Models, len(want))
	require.EqualValues(t, 2, f.calls.Load())
	// One entry serves every NVIDIA account: another account is admitted from it
	// without a read of its own.
	other := f.scope
	other.Account, other.Generation = "another-account", "22345678-1234-4234-8234-123456789abc"
	require.Equal(t, f.principal(), gatewayLiveModelsKey(GatewayNVIDIABuildChatProfile, other, "another-envelope"))
	require.True(t, f.gateway.gatewayModelCatalogueAdmitted(other, "another-envelope", GatewayNVIDIABuildChatProfile, "z-ai/glm-5.3-flash"))
	require.False(t, f.gateway.gatewayModelCatalogueAdmitted(other, "another-envelope", GatewayNVIDIABuildChatProfile, "nvidia/nvclip"), "a filtered model is never admitted")
	// A keyed list still has one entry per encrypted account.
	require.NotEqual(t, gatewayLiveModelsKey(GatewayGLMCodingResponsesProfile, f.scope, "envelope-a"), gatewayLiveModelsKey(GatewayGLMCodingResponsesProfile, other, "envelope-a"))
	require.EqualValues(t, 2, f.calls.Load())
}

// A full store makes room by dropping an account entry, never the shared public
// entry that admits every NVIDIA account.
func TestGatewayLiveModelsKeepThePublicEntryOnEviction(t *testing.T) {
	store := NewGatewayLiveModels(nil)
	public := gatewayLiveModelsKey(GatewayNVIDIABuildChatProfile, GatewayNativeCredentialScope{}, "")
	store.store(public, []string{"vendor/public"})
	for i := 1; i < gatewayLiveModelsEntriesMax; i++ {
		store.store(fmt.Sprintf("account-%03d", i), []string{"vendor/account"})
	}
	for i := 0; i < 3*gatewayLiveModelsEntriesMax; i++ {
		store.store(fmt.Sprintf("newcomer-%03d", i), []string{"vendor/new"})
		require.Equal(t, []string{"vendor/public"}, store.peek(public), "newcomer %d", i)
		require.LessOrEqual(t, len(store.entries), gatewayLiveModelsEntriesMax)
	}
}

// A key is never sent to a public list, and a keyed list never goes out without one.
func TestGatewayLiveModelsCredentialMatchesTheSource(t *testing.T) {
	var calls atomic.Int32
	collector := NewGatewayProviderQuotaCollector(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, io.ErrUnexpectedEOF
	}))
	_, err := collector.FetchModels(context.Background(), GatewayNVIDIABuildChatProfile, []byte("sandbox-fixture"))
	require.ErrorIs(t, err, ErrGatewayProviderQuota)
	_, err = collector.FetchModels(context.Background(), GatewayGLMCodingResponsesProfile, nil)
	require.ErrorIs(t, err, ErrGatewayProviderQuota)
	_, err = collector.FetchModels(context.Background(), GatewayOpenRouterResponsesProfile, nil)
	require.ErrorIs(t, err, ErrGatewayProviderQuota)
	require.Zero(t, calls.Load())
}

// The filter keeps chat models and drops the families that cannot review code.
// The IDs are from NVIDIA's public list of 2026-11-17.
func TestGatewayReviewModelFilter(t *testing.T) {
	for _, id := range []string{
		GatewayNVIDIABuildAnchorModel, "nvidia/nemotron-3-ultra-550b-a55b", "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning",
		"nvidia/cosmos-reason2-8b", "openai/gpt-oss-20b", "z-ai/glm-5.3-flash", "minimaxai/minimax-m3.1-flash",
		"google/gemma-4-31b-it", "mistralai/mistral-large-4-1t-a52b", "poolside/laguna-xs-2.1", "glm-5.3", "kimi-k3",
	} {
		require.True(t, gatewayReviewModelID(id), id)
	}
	for _, id := range []string{
		"nvidia/nemotron-3-embed-1b", "nvidia/llama-3.2-nv-embedqa-1b-v1", "snowflake/arctic-embed-l",
		"nvidia/llama-3.2-nemoretriever-1b-vlm-embed-v1", "nvidia/llama-3.2-nv-rerankqa-1b-v2",
		"nvidia/llama-3.1-nemotron-safety-guard-8b-v3", "nvidia/nemotron-3.5-content-safety", "meta/llama-guard-4-12b",
		"google/shieldgemma-9b", "nvidia/llama-3.1-nemotron-70b-reward", "nvidia/nvclip",
		"nvidia/ai-synthetic-video-detector", "nvidia/riva-translate-4b-instruct-v2", "", "bad model",
	} {
		require.False(t, gatewayReviewModelID(id), id)
	}
}

// A broken endpoint is asked once, not once per request: concurrent reads share one
// attempt, and the failure is remembered for a minute even when a refresh is forced.
func TestGatewayCodingPlanLiveModelsFailureIsRememberedAndShared(t *testing.T) {
	f := newLiveModelsFixture(t, GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(gatewayLiveModelsTimeout + 300*time.Millisecond)
		w.WriteHeader(http.StatusGatewayTimeout)
	})
	static := []string{GatewayGLMCodingResponsesModel, GatewayGLMCodingResponsesModelFull}
	started := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.Equal(t, static, f.read(false))
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, f.calls.Load(), "concurrent reads share a single provider attempt")
	require.Less(t, time.Since(started), 2*gatewayLiveModelsTimeout+time.Second, "waiters are not queued behind one another")
	require.Equal(t, static, f.read(true))
	require.EqualValues(t, 1, f.calls.Load(), "within the backoff even a forced read does not ask again")
	f.advance(gatewayLiveModelsFailBackoff + time.Second)
	require.Equal(t, static, f.read(false))
	require.EqualValues(t, 2, f.calls.Load(), "after the backoff the provider is asked again")
}

// Any failure of the provider read leaves exactly the static lower bound; the
// catalogue never becomes an error status.
func TestGatewayCodingPlanLiveModelsFailuresFallBackToStatic(t *testing.T) {
	static := []string{GatewayGLMCodingResponsesModel, GatewayGLMCodingResponsesModelFull}
	huge := modelsJSON(strings.Repeat("v", 200))
	huge = huge + strings.Repeat(" ", 300*1024)
	for name, handler := range map[string]http.HandlerFunc{
		"server error": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"unauthorized": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			// Relative: if the client followed it, the peer would be called twice.
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		},
		"empty list":     serveModels(modelsJSON()),
		"malformed":      serveModels(`{"data":[`),
		"not json":       serveModels(`<html>`),
		"oversized body": serveModels(huge),
		"slow": func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(gatewayLiveModelsTimeout + 300*time.Millisecond)
			_, _ = io.WriteString(w, modelsJSON("vendor/too-late"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newLiveModelsFixture(t, GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, handler)
			require.Equal(t, static, f.read(false))
			require.NotZero(t, f.calls.Load())
			if name == "redirect" {
				require.EqualValues(t, 1, f.calls.Load(), "a redirect is never followed")
			}
		})
	}
}

// A failed refresh never replaces the last good list, and the admission floor
// outlives the freshness window by a day.
func TestGatewayCodingPlanLiveModelsKeepTheLastGoodList(t *testing.T) {
	var fail atomic.Bool
	f := newLiveModelsFixture(t, GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		serveModels(modelsJSON("vendor/live-a"))(w, r)
	})
	good := []string{GatewayGLMCodingResponsesModel, GatewayGLMCodingResponsesModelFull, "vendor/live-a"}
	require.Equal(t, good, f.read(false))
	fail.Store(true)
	f.advance(gatewayLiveModelsForceAfter)
	require.Equal(t, good, f.read(true), "a failed forced refresh keeps the last good list")
	require.EqualValues(t, 2, f.calls.Load())
	f.advance(gatewayLiveModelsFreshFor + time.Second)
	require.Equal(t, good, f.read(false), "a stale entry with a failing provider keeps the last good list")
	require.True(t, f.gateway.gatewayModelCatalogueAdmitted(f.scope, f.account.GetCredential("api_key"), GatewayGLMCodingResponsesProfile, "vendor/live-a"))
	f.advance(gatewayLiveModelsAdmitFor)
	require.False(t, f.gateway.gatewayModelCatalogueAdmitted(f.scope, f.account.GetCredential("api_key"), GatewayGLMCodingResponsesProfile, "vendor/live-a"), "after a day the live list no longer admits")
	require.True(t, f.gateway.gatewayModelCatalogueAdmitted(f.scope, f.account.GetCredential("api_key"), GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesModelFull), "the static lower bound always admits")
}

// Without credential custody the lease refuses before any network use and the
// catalogue is the static lower bound.
func TestGatewayCodingPlanLiveModelsNeedTheLease(t *testing.T) {
	f := newLiveModelsFixture(t, GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, serveModels(modelsJSON("vendor/live-a")))
	ctx, err := WithGatewayNativeConsumer(context.Background(), custodyTestScope().Consumer)
	require.NoError(t, err)
	result := f.reader.Read(ctx, f.input)
	require.Equal(t, "snapshot", result.Status)
	require.Len(t, *result.Models, 2)
	require.Zero(t, f.calls.Load())
}

// At most two provider reads run at once; a busy slot returns what exists.
func TestGatewayCodingPlanLiveModelsBoundConcurrentFetches(t *testing.T) {
	store := NewGatewayLiveModels(nil)
	release := make(chan struct{})
	var running, peak atomic.Int32
	fetch := func(context.Context) ([]string, error) {
		now := running.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		<-release
		running.Add(-1)
		return []string{"vendor/a"}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.refresh(context.Background(), fmt.Sprintf("principal-%d", i), false, fetch)
		}(i)
	}
	deadline := time.After(2 * time.Second)
	for running.Load() < gatewayLiveModelsFetchSlots {
		select {
		case <-deadline:
			t.Fatal("two fetches did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	time.Sleep(50 * time.Millisecond)
	require.EqualValues(t, gatewayLiveModelsFetchSlots, running.Load(), "no third read runs while both slots are busy")
	close(release)
	wg.Wait()
	require.LessOrEqual(t, int(peak.Load()), gatewayLiveModelsFetchSlots)
}

// Dispatch admits a live-listed model only from the stored list, never reads
// the network and never takes the key lease: with no stored list the same request
// is refused before the provider and without a call to the models endpoint.
func TestGatewayNativeLiveModelAdmissionAtDispatch(t *testing.T) {
	const live = "vendor/live-only-model"
	for _, tuple := range []struct{ profile, base, anchor, path, stream string }{
		{GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, "/api/v1/responses",
			"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"},
		// NVIDIA keeps its fixed Chat backend; only the selected model changes.
		{GatewayNVIDIABuildChatProfile, GatewayNVIDIABuildBaseURL, GatewayNVIDIABuildAnchorModel, "/v1/chat/completions",
			nativeReviewCCDelta + nativeReviewCCFinish + nativeReviewDone},
	} {
		for _, seeded := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/seeded=%v", tuple.profile, seeded), func(t *testing.T) {
				f := newLiveModelsFixture(t, tuple.profile, tuple.base, tuple.anchor, func(http.ResponseWriter, *http.Request) {
					t.Error("dispatch must never read the models endpoint")
				})
				locks := &lockCountingRepo{gatewayIdentityRepoFixture: gatewayIdentityRepoFixture{row: f.account}}
				f.gateway.accountRepo = locks
				var providerCalls atomic.Int32
				var providerModel, providerPath atomic.Value
				provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					providerCalls.Add(1)
					providerPath.Store(r.URL.Path)
					wire, _ := io.ReadAll(r.Body)
					providerModel.Store(gjson.GetBytes(wire, "model").String())
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, tuple.stream)
				}))
				defer provider.Close()
				target, err := url.Parse(provider.URL)
				require.NoError(t, err)
				f.gateway.httpUpstream = &gatewayIdentityRealHTTP{client: &http.Client{Transport: kimiTierPeerTransport{peer: target}}}
				f.gateway.cfg = rawChatCompletionsTestConfig()
				if seeded {
					f.gateway.gatewayLiveModels.store(f.principal(), []string{live})
				}
				payload := []byte(fmt.Sprintf(`{"model":%q,"input":"text","stream":true,"store":false,"max_output_tokens":100}`, live))
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/private/native/v1/responses", strings.NewReader(string(payload)))
				_, entered, err := f.gateway.ForwardGatewayRoute(nativeFixtureContext(t, context.Background()), c, f.route, payload)
				require.Zero(t, f.calls.Load(), "no models read at dispatch")
				if !seeded {
					require.ErrorIs(t, err, ErrGatewayNativeIdentity)
					require.False(t, entered)
					require.Zero(t, providerCalls.Load())
					require.Zero(t, locks.locks.Load(), "an unlisted model is refused at admission, before the credential row lock")
					return
				}
				require.NoError(t, err)
				require.True(t, entered)
				require.EqualValues(t, 1, providerCalls.Load())
				require.Equal(t, tuple.path, providerPath.Load())
				require.Equal(t, live, providerModel.Load())
			})
		}
	}
}

// The locked entry boundary rechecks the stored list: a model that was admitted
// earlier but is no longer listed is refused before the credential is decoded.
func TestGatewayNativeLiveModelFinalEntryBoundary(t *testing.T) {
	const live = "vendor/live-only-model"
	for _, tuple := range []struct {
		name, profile, base, anchor string
		stored                      []string
		ok                          bool
	}{
		{"listed", GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, []string{live}, true},
		{"withdrawn", GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, nil, false},
		{"another model listed", GatewayGLMCodingResponsesProfile, GatewayGLMCodingResponsesBaseURL, GatewayGLMCodingResponsesModel, []string{"vendor/other"}, false},
		{"NVIDIA listed", GatewayNVIDIABuildChatProfile, GatewayNVIDIABuildBaseURL, GatewayNVIDIABuildAnchorModel, []string{live}, true},
		{"NVIDIA withdrawn", GatewayNVIDIABuildChatProfile, GatewayNVIDIABuildBaseURL, GatewayNVIDIABuildAnchorModel, nil, false},
	} {
		t.Run(tuple.name, func(t *testing.T) {
			f := newLiveModelsFixture(t, tuple.profile, tuple.base, tuple.anchor, func(http.ResponseWriter, *http.Request) {
				t.Error("the entry boundary must never read the models endpoint")
			})
			if tuple.stored != nil {
				f.gateway.gatewayLiveModels.store(f.principal(), tuple.stored)
			}
			state := &gatewayNativeDispatch{route: f.route, account: f.account, scope: f.scope, custody: nativeFixtureCustody(t), requestedModel: live}
			ctx := context.WithValue(nativeFixtureContext(t, context.Background()), gatewayNativeContextKey{}, state)
			target, err := f.gateway.gatewayNativeTargetURL(f.account)
			require.NoError(t, err)
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(fmt.Sprintf(`{"model":%q,"input":"text"}`, live)))
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer "+f.account.GetCredential("api_key"))
			release, err := f.gateway.checkGatewayNativeDispatch(request, f.account)
			release()
			if tuple.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrGatewayNativeIdentity)
			}
			require.Zero(t, f.calls.Load())
		})
	}
}

// lockCountingRepo counts how often the credential row lock is taken.
type lockCountingRepo struct {
	gatewayIdentityRepoFixture
	locks atomic.Int32
}

func (r *lockCountingRepo) LockGatewayNativeAccount(ctx context.Context, id int64) (*Account, func(), error) {
	r.locks.Add(1)
	return r.gatewayIdentityRepoFixture.LockGatewayNativeAccount(ctx, id)
}
