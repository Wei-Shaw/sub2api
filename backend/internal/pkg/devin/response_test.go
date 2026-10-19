// Tests for the Oh My Pi (MIT) adaptation, source commit
// 4049c62fbb0a7f5068954d01f11a73112c942a11. See root THIRD_PARTY_NOTICES.md.
package devin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestInterleavedToolFragmentsAndSnapshots(t *testing.T) {
	state := newCompletion("requested")
	calls := [][]byte{
		ps(ps(ps(nil, 1, "a"), 2, "look"), 3, `{"x":`),
		ps(ps(nil, 2, "up"), 3, `1`),
		ps(ps(ps(nil, 1, "a"), 2, "lookup"), 3, `{"x":1`), // cumulative snapshot: no duplicate delta
		ps(ps(ps(nil, 1, "b"), 2, "other"), 3, `{}`),
		ps(ps(nil, 1, "a"), 3, `}`),
		ps(nil, 3, `{"x":1}`), // no ID attaches to active a; identical snapshot is empty
	}
	var names [2]string
	var args [2]string
	for _, tc := range calls {
		delta, err := state.apply(mustProto(t, pb(nil, 6, tc)))
		if err != nil {
			t.Fatal(err)
		}
		for _, out := range delta.ToolCalls {
			index := *out.Index
			names[index] += out.Function.Name
			args[index] += out.Function.Arguments
		}
	}
	if err := state.validate(); err != nil {
		t.Fatal(err)
	}
	if names != [2]string{"lookup", "other"} || args != [2]string{`{"x":1}`, `{}`} || len(state.tools) != 2 {
		t.Fatalf("names=%v args=%v tools=%v", names, args, state.tools)
	}
	if state.tools[0].Function.Name != "lookup" || state.tools[0].Function.Arguments != `{"x":1}` || state.tools[1].ID != "b" {
		t.Fatal("nonstream state differs from stream reconstruction")
	}
}

func TestUsageSnapshotsAndAuthoritativeObservations(t *testing.T) {
	state := newCompletion("requested")
	usage := ps(ps(pv(pv(nil, 2, 10), 3, 2), 7, "usage-id"), 9, "usage-model")
	if _, err := state.apply(mustProto(t, pb(nil, 7, usage))); err != nil {
		t.Fatal(err)
	}
	if state.id != "usage-id" || state.model != "usage-model" {
		t.Fatal("usage fallback missing")
	}
	if _, err := state.apply(mustProto(t, ps(ps(nil, 1, "actual-id"), 23, "actual-model"))); err != nil {
		t.Fatal(err)
	}
	// Usage is a snapshot, not a delta, and must not overwrite explicit upstream observations.
	usage = ps(ps(pv(pv(nil, 2, 20), 3, 4), 7, "stale-id"), 9, "stale-model")
	if _, err := state.apply(mustProto(t, pb(nil, 7, usage))); err != nil {
		t.Fatal(err)
	}
	if state.usage.PromptTokens != 20 || state.usage.CompletionTokens != 4 || state.usage.TotalTokens != 24 {
		t.Fatal("usage snapshots were incorrectly summed")
	}
	if state.id != "actual-id" || state.model != "actual-model" {
		t.Fatal("authoritative observations overwritten")
	}
	overflow := pv(pv(nil, 2, ^uint64(0)), 3, 1)
	_, err := state.apply(mustProto(t, pb(nil, 7, overflow)))
	assertStatus(t, err, 502)
}

func TestBadToolOutputNeverSucceeds(t *testing.T) {
	cases := map[string][]byte{
		"no active ID":          ps(nil, 3, `{}`),
		"invalid arguments":     ps(ps(ps(nil, 1, "a"), 2, "f"), 3, `{"unfinished":`),
		"missing name":          ps(ps(nil, 1, "a"), 3, `{}`),
		"custom tool":           pv(ps(ps(ps(nil, 1, "a"), 2, "f"), 3, `{}`), 6, 1),
		"upstream invalid JSON": ps(ps(ps(ps(nil, 1, "a"), 2, "f"), 3, `{}`), 5, "token-secret"),
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			// The leading valid text message makes all failures post-return reader errors.
			c := baseClient(t, func(r *http.Request) (*http.Response, error) {
				return responseBytes(200, successFrames(ps(nil, 3, "text"), pb(nil, 6, call))), nil
			})
			resp, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(streamBody))
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			assertStatus(t, err, 502)
			if strings.Contains(string(b), "[DONE]") || strings.Contains(err.Error(), "token-secret") {
				t.Fatalf("unsafe success/error: %s %v", b, err)
			}
		})
	}
}

func TestToolCapabilitiesAndChoices(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"x"}],"parallel_tool_calls":true,"tools":[{"type":"function","function":{"name":"f"}}],"tool_choice":"required"}`
	model := Model{ID: "parallel", SupportsParallelTools: true}
	mapped, err := mapRequest([]byte(body), "cascade", model)
	if err != nil {
		t.Fatal(err)
	}
	wire := mustProto(t, mapped.wire)
	if wire.num(11) != 0 || mustProto(t, wire.data(12)).text(1) != "required" {
		t.Fatal("parallel or required choice mapping incorrect")
	}
	schema := mustProto(t, wire.data(10)).text(3)
	if !json.Valid([]byte(schema)) || !strings.Contains(schema, `"type":"object"`) {
		t.Fatal("default tool schema missing")
	}
	noTools := false
	model.supportsTools = &noTools
	_, err = mapRequest([]byte(body), "cascade", model)
	assertStatus(t, err, 400)
}

func TestGenerationFailureAndTransportFailure(t *testing.T) {
	for _, message := range [][]byte{pv(nil, 5, 13), pv(nil, 5, 999), pv(nil, 8, 1), pv(nil, 11, 1)} {
		c := baseClient(t, func(r *http.Request) (*http.Response, error) { return responseBytes(200, successFrames(message)), nil })
		_, err := c.Chat(context.Background(), "token", "cascade", DefaultModels()[0], []byte(streamBody))
		assertStatus(t, err, 502)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Client{Do: func(*http.Request) (*http.Response, error) {
		t.Fatal("canceled request reached transport")
		return nil, nil
	}}
	if _, err := c.Models(ctx, "token"); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}
