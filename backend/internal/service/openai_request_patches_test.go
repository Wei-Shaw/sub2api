package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
)

// Keep the previous sequential implementation as a wire-compatibility oracle.
// JSONEq would miss changes to duplicate keys, number spelling and unknown fields.
func applyOpenAIRequestPatchesSequential(body []byte, patches []openAIRequestPatch) ([]byte, error) {
	for _, patch := range patches {
		var err error
		if patch.delete {
			body, err = sjson.DeleteBytes(body, patch.path)
		} else {
			body, err = sjson.SetBytes(body, patch.path, patch.value)
		}
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func requestViewWithPatches(body []byte, patches []openAIRequestPatch) openAIRequestView {
	view := newOpenAIRequestView(body)
	for _, patch := range patches {
		if patch.delete {
			view.MarkPatchDelete(patch.path)
		} else {
			view.MarkPatchSet(patch.path, patch.value)
		}
	}
	return view
}

func denseOpenAIRequestPatchFixture(fields int, nested bool) ([]byte, []openAIRequestPatch) {
	var body strings.Builder
	body.Grow(fields*16 + 64)
	prefix := ""
	if nested {
		_, _ = body.WriteString(`{"model":"old","reasoning":`)
		prefix = "reasoning."
	}
	_ = body.WriteByte('{')
	for i := 0; i < fields; i++ {
		if i > 0 {
			_ = body.WriteByte(',')
		}
		fmt.Fprintf(&body, `"f%d":0`, i)
	}
	_ = body.WriteByte('}')
	if nested {
		_ = body.WriteByte('}')
	}
	return []byte(body.String()), []openAIRequestPatch{
		{path: fmt.Sprintf("%sf%d", prefix, fields-2), value: 1},
		{path: fmt.Sprintf("%sf%d", prefix, fields-1), value: 2},
	}
}

func TestOpenAIRequestPatches_BoundIndexedObjectMembers(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, fields := range []int{64, 65, 4096} {
			t.Run(fmt.Sprintf("nested_%t_%d_fields", nested, fields), func(t *testing.T) {
				body, patches := denseOpenAIRequestPatchFixture(fields, nested)
				original := bytes.Clone(body)
				view := requestViewWithPatches(body, patches)
				require.Equal(t, fields <= 64, view.canBatchPatches(), "dense edited objects must not amplify Doc scratch memory")
				want, err := applyOpenAIRequestPatchesSequential(body, patches)
				require.NoError(t, err)
				got, err := view.ApplyPatches(context.Background())
				require.NoError(t, err)
				require.Equal(t, want, got)
				require.Equal(t, original, body)
			})
		}
	}
}

func TestOpenAIRequestPatches_LargeUntouchedInputStillBatches(t *testing.T) {
	dense, _ := denseOpenAIRequestPatchFixture(4096, false)
	body := []byte(`{"model":"old","input":[{"content":"` + strings.Repeat("a", 1<<20) + `","opaque":` + string(dense) + `}],"reasoning":{"effort":"low"}}`)
	patches := []openAIRequestPatch{{path: "model", value: "mapped"}, {path: "reasoning.effort", value: "none"}}
	view := requestViewWithPatches(body, patches)
	require.True(t, view.canBatchPatches(), "unvisited input objects must remain opaque regardless of their size or member count")
	want, err := applyOpenAIRequestPatchesSequential(body, patches)
	require.NoError(t, err)
	got, err := view.ApplyPatches(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestOpenAIRequestPatches_DeletingParentCannotExposeUncheckedDuplicate(t *testing.T) {
	dense, _ := denseOpenAIRequestPatchFixture(4096, false)
	body := []byte(`{"model":"old","reasoning":{"effort":"low"},"reasoning":` + string(dense) + `}`)
	patches := []openAIRequestPatch{{path: "reasoning", delete: true}, {path: "reasoning.effort", value: "none"}}
	view := requestViewWithPatches(body, patches)
	require.False(t, view.canBatchPatches(), "deleting the first parent exposes a dense duplicate that the original lookup did not inspect")
	want, err := applyOpenAIRequestPatchesSequential(body, patches)
	require.NoError(t, err)
	got, err := view.ApplyPatches(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func repeatedNestedOpenAIRequestPatchFixture() ([]byte, []openAIRequestPatch) {
	body := []byte(`{"model":"old","reasoning":{"effort":"low","opaque":"` + strings.Repeat("a", 1<<20) + `"},"input":[]}`)
	return body, []openAIRequestPatch{
		{path: "reasoning.effort", value: "medium"},
		{path: "reasoning.effort", value: "high"},
		{path: "reasoning.effort", value: "none"},
	}
}

func TestOpenAIRequestPatches_RepeatedNestedLeafKeepsOpaqueSiblingBorrowed(t *testing.T) {
	body, patches := repeatedNestedOpenAIRequestPatchFixture()
	view := requestViewWithPatches(body, patches)
	require.True(t, view.canBatchPatches(), "repeated leaf edits do not replace their parent and must remain eligible")
	want, err := applyOpenAIRequestPatchesSequential(body, patches)
	require.NoError(t, err)
	got, err := view.ApplyPatches(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestOpenAIRequestPatches_PreserveSequentialWireBehavior(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		patches []openAIRequestPatch
	}{
		{
			name: "single edit preserves opaque wire fields",
			body: " \n{\"model\":\"old\", \"opaque\": {\"integer\":9007199254740993,\"decimal\":1.2300e+04,\"zero\":-0,\"text\":\"\\u4f60\\u597d\"}, \"input\":[]}\t ",
			patches: []openAIRequestPatch{
				{path: "model", value: "new"},
			},
		},
		{
			name: "multiple edits preserve opaque wire fields",
			body: " \n{\"model\":\"old\", \"opaque\": {\"integer\":9007199254740993,\"decimal\":1.2300e+04,\"zero\":-0,\"text\":\"\\u4f60\\u597d\"}, \"input\":[],\"reasoning\":{\"effort\":\"low\",\"future\":true}}\t ",
			patches: []openAIRequestPatch{
				{path: "model", value: "new"},
				{path: "reasoning.effort", value: "none"},
				{path: "instructions", value: "quoted \"text\"\n你好 <tag>"},
			},
		},
		{
			name: "duplicate top-level and nested keys",
			body: `{"model":"first","model":"second","reasoning":{"effort":"low","effort":"high"},"reasoning":{"effort":"max"},"opaque":1,"opaque":2}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: "reasoning.effort", value: "none"},
				{path: "model", delete: true},
				{path: "model", value: "final"},
			},
		},
		{
			name: "forward token normalization followed by clamp",
			body: `{"model":"old","max_tokens":65536,"max_completion_tokens":1000,"prompt_cache_options":{"future":true},"input":[],"previous_response_id":"resp_1","service_tier":"flex"}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: "max_output_tokens", value: float64(65536)},
				{path: "max_tokens", delete: true},
				{path: "max_completion_tokens", delete: true},
				{path: "prompt_cache_options", delete: true},
				{path: "max_output_tokens", value: 8192},
				{path: "previous_response_id", delete: true},
				{path: "service_tier", value: "priority"},
			},
		},
		{
			name: "replace parent after child then edit new child",
			body: `{"model":"gpt-5","reasoning":{"effort":"low","opaque":123},"input":[]}`,
			patches: []openAIRequestPatch{
				{path: "reasoning.effort", value: "high"},
				{path: "reasoning", value: map[string]any{"effort": "medium", "future": true}},
				{path: "reasoning.effort", value: "none"},
			},
		},
		{
			name: "delete parent and recreate child",
			body: `{"model":"gpt-5","reasoning":{"effort":"low","opaque":123},"input":[]}`,
			patches: []openAIRequestPatch{
				{path: "reasoning.effort", value: "high"},
				{path: "reasoning", delete: true},
				{path: "reasoning.effort", value: "none"},
			},
		},
		{
			name: "create replace delete and recreate same field",
			body: `{"model":"gpt-5","input":[]}`,
			patches: []openAIRequestPatch{
				{path: "instructions", value: "first"},
				{path: "instructions", value: "second"},
				{path: "instructions", delete: true},
				{path: "instructions", value: "third"},
			},
		},
		{
			name: "missing deletes around effective edits",
			body: `{"model":"old","reasoning":{"effort":"low"},"input":[]}`,
			patches: []openAIRequestPatch{
				{path: "previous_response_id", delete: true},
				{path: "model", value: "mapped"},
				{path: "text.verbosity", delete: true},
				{path: "reasoning.effort", value: "none"},
				{path: "previous_response_id", delete: true},
			},
		},
		{
			name: "deleting first middle and last members",
			body: `{"previous_response_id":"resp_1", "model":"old", "text":{"verbosity":"high","format":{"type":"text"}}, "max_tokens":100}`,
			patches: []openAIRequestPatch{
				{path: "previous_response_id", delete: true},
				{path: "text.verbosity", delete: true},
				{path: "max_tokens", delete: true},
			},
		},
		{
			name: "only missing deletes",
			body: ` {"model":"old", "input":[]} `,
			patches: []openAIRequestPatch{
				{path: "previous_response_id", delete: true},
				{path: "text.verbosity", delete: true},
			},
		},
		{
			name: "replace scalar parent with object",
			body: `{"model":"gpt-5","reasoning":null,"input":[]}`,
			patches: []openAIRequestPatch{
				{path: "reasoning.effort", value: "none"},
				{path: "instructions", value: ""},
			},
		},
		{
			name: "scalar types and exact json number",
			body: `{"model":"gpt-5","input":[]}`,
			patches: []openAIRequestPatch{
				{path: "store", value: false},
				{path: "stream", value: true},
				{path: "max_output_tokens", value: json.Number("9007199254740993")},
				{path: "instructions", value: nil},
			},
		},
		{
			name: "existing string parent containing JSON text",
			body: `{"model":"old","reasoning":"{\"effort\":\"low\"}","input":[]}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: "reasoning.effort", value: "none"},
			},
		},
		{
			name: "parent becomes string before child edit",
			body: `{"model":"old","reasoning":{"effort":"low"},"input":[]}`,
			patches: []openAIRequestPatch{
				{path: "reasoning", value: `{"effort":"high"}`},
				{path: "reasoning.effort", value: "none"},
			},
		},
		{
			name: "array parent before child edit",
			body: `{"model":"old","reasoning":[{"effort":"low"}],"input":[]}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: "reasoning.effort", value: "none"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			want, wantErr := applyOpenAIRequestPatchesSequential(body, tt.patches)
			view := requestViewWithPatches(body, tt.patches)
			require.True(t, view.HasPatches())

			got, err := view.ApplyPatches(context.Background())
			if wantErr != nil {
				require.EqualError(t, err, wantErr.Error())
				require.Equal(t, want, got)
				require.Equal(t, tt.body, string(body))
				return
			}
			require.NoError(t, err)
			require.Equal(t, string(want), string(got))
			require.True(t, json.Valid(got))
			require.Equal(t, tt.body, string(body), "patching must not mutate the source body")
		})
	}
}

func TestOpenAIRequestPatches_PreserveCallerCancellationPolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, edits := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d_edits", edits), func(t *testing.T) {
			body := []byte(`{"model":"old","input":[]}`)
			view := newOpenAIRequestView(body)
			for i := 0; i < edits; i++ {
				view.MarkPatchSet("model", "mapped")
			}
			got, err := view.ApplyPatches(ctx)
			// Forward owns cancellation/drain policy; editing must not change
			// the existing OAuth path's detached upstream-request behavior.
			require.NoError(t, err)
			require.Equal(t, `{"model":"mapped","input":[]}`, string(got))
			require.Equal(t, `{"model":"old","input":[]}`, string(body))
		})
	}
}

func TestOpenAIRequestPatches_CompatibilityFallback(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		patches []openAIRequestPatch
	}{
		{
			name: "malformed object after a valid prefix",
			body: `{"model":"a","input":[`,
			patches: []openAIRequestPatch{
				{path: "model", value: "b"},
				{path: "reasoning.effort", value: "none"},
			},
		},
		{
			name: "query path",
			body: `{"model":"old","input":[{"role":"user","content":"hello"}]}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: `input.#(role=="user").content`, value: "new"},
			},
		},
		{
			name: "special path character",
			body: `{"model":"old","metadata":{"user*":"old"}}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: "metadata.user*", value: "new"},
			},
		},
		{
			name: "key under array parent",
			body: `{"model":"old","reasoning":[1]}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: "reasoning.effort", value: "none"},
			},
		},
		{
			name: "invalid UTF-8 path",
			body: `{"model":"old","metadata":{}}`,
			patches: []openAIRequestPatch{
				{path: "model", value: "mapped"},
				{path: "metadata.\xff", value: "new"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			want, wantErr := applyOpenAIRequestPatchesSequential(body, tt.patches)
			view := requestViewWithPatches(body, tt.patches)
			require.True(t, view.HasPatches())
			got, gotErr := view.ApplyPatches(context.Background())
			if wantErr != nil {
				require.EqualError(t, gotErr, wantErr.Error())
			} else {
				require.NoError(t, gotErr)
			}
			require.Equal(t, want, got)
			require.Equal(t, tt.body, string(body))
		})
	}
}

func TestOpenAIRequestPatches_ReserveOutputUntilRequestRelease(t *testing.T) {
	body := []byte(`{"model":"old","reasoning":{"effort":"low"},"input":[]}`)
	patches := []openAIRequestPatch{{path: "model", value: "mapped"}, {path: "reasoning.effort", value: "none"}}
	want, err := applyOpenAIRequestPatchesSequential(body, patches)
	require.NoError(t, err)
	limit := int64(len(body) + len(want))
	budget := httputil.NewBodyMemoryBudget(limit)
	ctx, release := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
	t.Cleanup(release)
	require.NoError(t, httputil.ReserveRequestBodyMemory(ctx, int64(len(body))))
	view := requestViewWithPatches(body, patches)

	got, err := view.ApplyPatches(ctx)
	require.NoError(t, err, "the exact input plus output budget must suffice")
	require.Equal(t, want, got)
	probe, releaseProbe := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
	t.Cleanup(releaseProbe)
	require.ErrorIs(t, httputil.ReserveRequestBodyMemory(probe, 1), httputil.ErrRequestBodyMemoryBudget)
	release()
	require.NoError(t, httputil.ReserveRequestBodyMemory(probe, limit))
	// A second release must not refund another request's reservation.
	release()
	require.ErrorIs(t, httputil.ReserveRequestBodyMemory(probe, 1), httputil.ErrRequestBodyMemoryBudget)
}

func TestOpenAIRequestPatches_RejectUnavailableOutputBudget(t *testing.T) {
	body := []byte(`{"model":"old","reasoning":{"effort":"low"},"input":[]}`)
	for _, tt := range []struct {
		name    string
		patches []openAIRequestPatch
	}{
		{name: "batched", patches: []openAIRequestPatch{{path: "model", value: "mapped"}, {path: "reasoning.effort", value: "none"}}},
		{name: "single", patches: []openAIRequestPatch{{path: "model", value: "mapped"}}},
		{name: "sequential", patches: []openAIRequestPatch{{path: "model", value: "mapped"}, {path: "metadata", value: map[string]any{"k": "v"}}}},
	} {
		want, err := applyOpenAIRequestPatchesSequential(body, tt.patches)
		require.NoError(t, err)
		for _, scenario := range []string{"exhausted", "below input", "closed", "canceled and closed"} {
			t.Run(tt.name+"/"+scenario, func(t *testing.T) {
				limit := int64(len(want))
				switch scenario {
				case "exhausted":
					limit--
				case "below input":
					// Rejected by the up-front input-size reservation, before editing.
					limit = int64(len(body)) - 1
				}
				budget := httputil.NewBodyMemoryBudget(limit)
				parent, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				ctx, release := httputil.WithRequestBodyMemoryBudget(parent, budget)
				t.Cleanup(release)
				switch scenario {
				case "closed":
					release()
				case "canceled and closed":
					cancel()
					release()
				}
				view := requestViewWithPatches(body, tt.patches)
				got, err := view.ApplyPatches(ctx)
				require.ErrorIs(t, err, httputil.ErrRequestBodyMemoryBudget)
				require.Nil(t, got)
				probe, releaseProbe := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
				t.Cleanup(releaseProbe)
				require.NoError(t, httputil.ReserveRequestBodyMemory(probe, limit), "failed materialization must not consume the shared budget")
			})
		}
		t.Run(tt.name+"/exact", func(t *testing.T) {
			budget := httputil.NewBodyMemoryBudget(int64(len(want)))
			ctx, release := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
			t.Cleanup(release)
			got, err := requestViewWithPatches(body, tt.patches).ApplyPatches(ctx)
			require.NoError(t, err)
			require.Equal(t, want, got)
			probe, releaseProbe := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
			t.Cleanup(releaseProbe)
			require.ErrorIs(t, httputil.ReserveRequestBodyMemory(probe, 1), httputil.ErrRequestBodyMemoryBudget, "the reservation must settle to the exact output")
		})
	}
}

func TestOpenAIRequestPatches_MovePatchesToDecodedBody(t *testing.T) {
	body := []byte(`{"model":"old","reasoning":[1],"previous_response_id":"resp_1"}`)
	view := requestViewWithPatches(body, []openAIRequestPatch{
		{path: "model", value: "mapped"},
		{path: "reasoning.effort", value: "none"},
		{path: "previous_response_id", delete: true},
	})
	_, err := view.ApplyPatches(context.Background())
	require.Error(t, err, "raw edits cannot set a key under an array")
	decoded, err := view.Decode(nil)
	require.NoError(t, err)
	view.MovePatchesTo(decoded)
	require.False(t, view.HasPatches())
	out, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"mapped","reasoning":{"effort":"none"}}`, string(out))
	require.Equal(t, `{"model":"old","reasoning":[1],"previous_response_id":"resp_1"}`, string(body))
}

func TestOpenAIRequestPatches_ForwardReleasesPatchedBodyWhenAttemptEnds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5","stream":false,"reasoning":{"effort":"minimal"},"max_tokens":1024,"input":[{"type":"message","content":"hi"}]}`)
	const limit = 1 << 20
	budget := httputil.NewBodyMemoryBudget(limit)
	ctx, release := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
	t.Cleanup(release)
	for attempt := range 3 {
		upstream := &httpUpstreamRecorder{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2}}`)),
			},
		}
		cfg := &config.Config{}
		cfg.Security.URLAllowlist.Enabled = false
		svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
		account := &Account{
			ID:          int64(attempt + 1),
			Name:        "openai-apikey",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com"},
			Extra:       map[string]any{"use_responses_api": true},
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil).WithContext(ctx)
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

		_, err := svc.Forward(ctx, c, account, body)
		require.NoError(t, err)
		require.NotNil(t, upstream.lastReq)
		// Failover attempts must not accumulate rewritten bodies until the handler returns.
		probe, releaseProbe := httputil.WithRequestBodyMemoryBudget(context.Background(), budget)
		require.NoError(t, httputil.ReserveRequestBodyMemory(probe, limit), "attempt %d retained its patched body", attempt)
		releaseProbe()
	}
}

func TestOpenAIRequestPatches_ForwardStopsBeforeUpstreamOnOverload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name string
		body string
	}{
		{
			name: "final raw patch flush",
			body: `{"model":"gpt-5","stream":false,"reasoning":{"effort":"minimal"},"max_tokens":1024,"input":[{"type":"message","content":"hi"}]}`,
		},
		{
			name: "flush before schema decode",
			body: `{"model":"client-alias","stream":false,"reasoning":{"effort":"minimal"},"text":{"format":{"type":"json_schema","name":"test","schema":{"type":"object","properties":{}}}},"input":[{"type":"message","content":"hi"}]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2}}`)),
				},
			}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := &Account{
				ID:          1,
				Name:        "openai-apikey",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{
					"api_key":       "sk-test",
					"base_url":      "https://example.com",
					"model_mapping": map[string]any{"client-alias": "gpt-5"},
				},
				Extra: map[string]any{"use_responses_api": true},
			}
			ctx, release := httputil.WithRequestBodyMemoryBudget(context.Background(), httputil.NewBodyMemoryBudget(1))
			t.Cleanup(release)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil).WithContext(ctx)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

			result, err := svc.Forward(ctx, c, account, []byte(tt.body))
			require.ErrorIs(t, err, httputil.ErrRequestBodyMemoryBudget)
			require.Nil(t, result)
			require.Nil(t, upstream.lastReq, "overload must not fall back to unbudgeted serialization or forwarding")
		})
	}
}

func TestOpenAIRequestPatches_EffectiveEditsOwnOutput(t *testing.T) {
	for _, patches := range [][]openAIRequestPatch{
		{{path: "model", value: "mapped"}},
		{{path: "previous_response_id", delete: true}},
		{{path: "model", value: "mapped"}, {path: "reasoning.effort", value: "none"}, {path: "previous_response_id", delete: true}},
	} {
		t.Run(fmt.Sprintf("%d_edits_%s", len(patches), patches[0].path), func(t *testing.T) {
			body := []byte(`{"model":"old","reasoning":{"effort":"low"},"previous_response_id":"resp_1","opaque":"` + strings.Repeat("payload", 4096) + `"}`)
			view := requestViewWithPatches(body, patches)
			got, err := view.ApplyPatches(context.Background())
			require.NoError(t, err)
			snapshot := bytes.Clone(got)

			// The borrowed source may be released or reused after ApplyPatches returns.
			for i := range body {
				body[i] = 'x'
			}
			require.Equal(t, snapshot, got)
			got[0] = '!'
			require.Equal(t, bytes.Repeat([]byte{'x'}, len(body)), body)
		})
	}
}

type failingOpenAIRequestPatchValue struct {
	err error
}

func (v failingOpenAIRequestPatchValue) MarshalJSON() ([]byte, error) {
	return nil, v.err
}

func TestOpenAIRequestPatches_PropagateValueErrorWithoutPartialOutput(t *testing.T) {
	valueErr := errors.New("request patch encoding failed")
	for _, priorEdits := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("after_%d_edits", priorEdits), func(t *testing.T) {
			body := []byte(`{"model":"old","instructions":"original","input":[]}`)
			original := bytes.Clone(body)
			view := newOpenAIRequestView(body)
			for i := 0; i < priorEdits; i++ {
				view.MarkPatchSet("model", fmt.Sprintf("mapped-%d", i))
			}
			view.MarkPatchSet("instructions", failingOpenAIRequestPatchValue{err: valueErr})

			got, err := view.ApplyPatches(context.Background())
			require.ErrorIs(t, err, valueErr)
			require.Nil(t, got)
			require.Equal(t, original, body)
		})
	}
}

func TestOpenAIRequestPatches_ConcurrentImmutableInput(t *testing.T) {
	body := []byte(`{"model":"old","reasoning":{"effort":"low"},"previous_response_id":"resp_1","opaque":"` + strings.Repeat("payload", 4096) + `"}`)
	original := bytes.Clone(body)
	t.Cleanup(func() {
		require.Equal(t, original, body)
	})
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprintf("worker_%d", worker), func(t *testing.T) {
			t.Parallel()
			patches := []openAIRequestPatch{
				{path: "model", value: fmt.Sprintf("model-%d", worker)},
				{path: "reasoning.effort", value: "none"},
				{path: "previous_response_id", delete: true},
			}
			want, err := applyOpenAIRequestPatchesSequential(body, patches)
			require.NoError(t, err)
			for i := 0; i < 16; i++ {
				view := requestViewWithPatches(body, patches)
				got, err := view.ApplyPatches(context.Background())
				require.NoError(t, err)
				require.Equal(t, want, got)
				got[0] = '!'
			}
		})
	}
}
