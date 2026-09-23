package apicompat

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// nativeCanaryObservation collects only the minimal protocol metadata this
// harness reports: request counts, which collaboration children carried the
// encrypted marker, where the nonce landed, and the raw shape of history
// function_call items. Raw request bodies and CLI transcripts are never kept.
type nativeCanaryObservation struct {
	mu sync.Mutex

	nonce string

	modelsRequests    int
	responsesRequests int
	otherRequests     []string

	setupErrors []string
	notes       []string

	// First POST /responses (pre-adapt, verified against the raw request).
	req1SawCollaborationNamespace bool
	req1SendMessageMarked         bool
	req1MarkedChildren            []string
	req1UnmarkedChildren          []string
	req1NamespaceCarrier          string // which JSON key carried the children ("tools"/"children")
	// First request after AdaptResponsesCollaborationPlaintext.
	req1AdaptChanged        bool
	req1AliasDeclared       bool
	req1AliasMarkerStripped bool
	req1Aliases             []string
	req1MarkedAliases       []string
	req1MappedAliases       []string
	req1NamespaceRemaining  []string

	// Follow-up POST /responses carrying the tool result (pre-adapt, raw).
	req2Seen                bool
	req2NonceInInputText    bool
	req2NonceInEncrypted    bool
	req2EncryptedBlocks     int
	req2AgentMessageCount   int
	req2AgentMessageAuthor  string
	req2AgentMessageRcpt    string
	req2HistoryCallShapes   []string // "name=<n>,namespace=<ns|absent>" per matching call
	req2HistoryPostAdapt    []string // same shape after adaptation
	req2FunctionCallOutputs int

	// Function_call item as restored onto the SSE stream toward the CLI.
	restoredCallSeen           bool
	restoredCallName           string
	restoredCallNamespace      string
	restoredEncryptedArgsValid bool // present and an empty JSON array
}

func (o *nativeCanaryObservation) fail(format string, args ...any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.failLocked(format, args...)
}

func (o *nativeCanaryObservation) failLocked(format string, args ...any) {
	o.setupErrors = append(o.setupErrors, fmt.Sprintf(format, args...))
}

func (o *nativeCanaryObservation) note(format string, args ...any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.noteLocked(format, args...)
}

func (o *nativeCanaryObservation) noteLocked(format string, args ...any) {
	o.notes = append(o.notes, fmt.Sprintf(format, args...))
}

func (o *nativeCanaryObservation) nextResponsesRequest() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.responsesRequests++
	return o.responsesRequests
}

func nativeCanaryMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func nativeCanaryList(v any) []any {
	l, _ := v.([]any)
	return l
}

// nativeCanaryMarkedMessage reports whether a function child declares
// parameters.properties.message with encrypted:true.
func nativeCanaryMarkedMessage(child map[string]any) (present bool, marked bool) {
	params := nativeCanaryMap(child["parameters"])
	props := nativeCanaryMap(params["properties"])
	msg := nativeCanaryMap(props["message"])
	if msg == nil {
		return false, false
	}
	enc, isBool := msg["encrypted"].(bool)
	return true, isBool && enc
}

// inspectFirstRequest verifies the real CLI declared MultiAgentV2
// collaboration.send_message with the encrypted:true marker before any
// adaptation runs. A missing namespace or marker is a setup error, never
// rewritten to look like a pass.
func (o *nativeCanaryObservation) inspectFirstRequest(req map[string]any) {
	found := false
	for _, raw := range nativeCanaryList(req["tools"]) {
		tool := nativeCanaryMap(raw)
		if tool == nil || stringValue(tool["type"]) != "namespace" || stringValue(tool["name"]) != "collaboration" {
			continue
		}
		found = true
		children := namespaceChildren(tool)
		key := "tools"
		if _, ok := tool["tools"].([]any); !ok {
			key = "children"
		}
		o.req1NamespaceCarrier = key
		for _, rawChild := range children {
			child := nativeCanaryMap(rawChild)
			if child == nil || stringValue(child["type"]) != "function" {
				continue
			}
			name := stringValue(child["name"])
			present, marked := nativeCanaryMarkedMessage(child)
			switch {
			case present && marked:
				o.req1MarkedChildren = append(o.req1MarkedChildren, name)
			default:
				o.req1UnmarkedChildren = append(o.req1UnmarkedChildren, name)
			}
			if name == "send_message" {
				o.req1SendMessageMarked = present && marked
			}
		}
	}
	o.req1SawCollaborationNamespace = found
	if !found {
		o.failLocked("setup: first request tools[] has no type=namespace name=collaboration entry; the CLI did not emit the MultiAgentV2 namespace declaration")
		return
	}
	if !o.req1SendMessageMarked {
		o.failLocked("setup: collaboration.send_message was not declared with parameters.properties.message.encrypted=true (marked=%v unmarked=%v)", o.req1MarkedChildren, o.req1UnmarkedChildren)
	}
}

// inspectAdaptedTools records what the adapter produced for the request that
// carried the collaboration namespace.
func (o *nativeCanaryObservation) inspectAdaptedTools(req map[string]any, mapping ResponsesCollaborationPlaintextMapping, changed bool) {
	o.req1AdaptChanged = changed
	for _, raw := range nativeCanaryList(req["tools"]) {
		tool := nativeCanaryMap(raw)
		if tool == nil {
			continue
		}
		if stringValue(tool["type"]) == "namespace" && stringValue(tool["name"]) == "collaboration" {
			for _, rawChild := range namespaceChildren(tool) {
				child := nativeCanaryMap(rawChild)
				if child != nil {
					o.req1NamespaceRemaining = append(o.req1NamespaceRemaining, stringValue(child["name"]))
				}
			}
			continue
		}
		if stringValue(tool["type"]) != "function" || !strings.HasPrefix(stringValue(tool["name"]), "collaboration__") {
			continue
		}
		name := stringValue(tool["name"])
		o.req1Aliases = append(o.req1Aliases, name)
		_, marked := nativeCanaryMarkedMessage(tool)
		if marked {
			o.req1MarkedAliases = append(o.req1MarkedAliases, name)
		}
		if name == "collaboration__send_message" {
			o.req1AliasDeclared = true
			o.req1AliasMarkerStripped = !marked
		}
	}
	for name := range mapping.Calls {
		o.req1MappedAliases = append(o.req1MappedAliases, name)
	}
	if !changed || len(mapping.Calls) == 0 {
		o.failLocked("adapter returned changed=%v with %d call mappings for a request that carried the marked collaboration namespace", changed, len(mapping.Calls))
		return
	}
	if _, ok := mapping.Calls["collaboration__send_message"]; !ok {
		o.failLocked("adapter did not mint the collaboration__send_message alias (calls=%v)", mapping.Calls)
	}
	if !o.req1AliasDeclared {
		o.failLocked("adapted tools[] has no flat function named collaboration__send_message")
	}
	if !o.req1AliasMarkerStripped {
		o.failLocked("adapted collaboration__send_message still carries encrypted marker on message")
	}
}

// inspectFollowupRequest verifies the raw (pre-adapt) follow-up request carries
// the queued self-message as input[].type=agent_message content at the exact
// positions, plus the shape of the replayed function_call history.
func (o *nativeCanaryObservation) inspectFollowupRequest(req map[string]any, callID string) {
	o.req2Seen = true
	for _, raw := range nativeCanaryList(req["input"]) {
		item := nativeCanaryMap(raw)
		if item == nil {
			continue
		}
		switch stringValue(item["type"]) {
		case "agent_message":
			o.req2AgentMessageCount++
			o.req2AgentMessageAuthor = stringValue(item["author"])
			o.req2AgentMessageRcpt = stringValue(item["recipient"])
			for _, rawContent := range nativeCanaryList(item["content"]) {
				content := nativeCanaryMap(rawContent)
				if content == nil {
					continue
				}
				switch stringValue(content["type"]) {
				case "input_text":
					if strings.Contains(stringValue(content["text"]), o.nonce) {
						o.req2NonceInInputText = true
					}
				case "encrypted_content":
					o.req2EncryptedBlocks++
					if strings.Contains(stringValue(content["encrypted_content"]), o.nonce) {
						o.req2NonceInEncrypted = true
					}
				}
			}
		case "function_call":
			if stringValue(item["call_id"]) != callID {
				continue
			}
			ns := stringValue(item["namespace"])
			if ns == "" {
				ns = "<absent>"
			}
			o.req2HistoryCallShapes = append(o.req2HistoryCallShapes,
				fmt.Sprintf("name=%q namespace=%s encrypted_function_args=%v", stringValue(item["name"]), ns, item["encrypted_function_args"] != nil))
		case "function_call_output":
			if stringValue(item["call_id"]) == callID {
				o.req2FunctionCallOutputs++
			}
		}
	}
	if o.req2AgentMessageCount == 0 {
		o.failLocked("follow-up request has no input[].type=agent_message item; the queued self-message never reached the model input")
	}
	if !o.req2NonceInInputText {
		o.failLocked("no agent_message content[].type=input_text contains the synthetic nonce")
	}
	if o.req2EncryptedBlocks != 0 {
		o.failLocked("agent_message contains %d content[].type=encrypted_content blocks; plaintext-only path was not taken", o.req2EncryptedBlocks)
	}
	if len(o.req2HistoryCallShapes) == 0 {
		o.failLocked("follow-up request replayed no function_call with the issued call_id")
	}
}

// inspectAdaptedFollowup checks the post-adapt shape of the replayed history
// call against the alias the adapter declared for this request.
func (o *nativeCanaryObservation) inspectAdaptedFollowup(req map[string]any, mapping ResponsesCollaborationPlaintextMapping, callID string) {
	for _, raw := range nativeCanaryList(req["input"]) {
		item := nativeCanaryMap(raw)
		if item == nil || stringValue(item["type"]) != "function_call" || stringValue(item["call_id"]) != callID {
			continue
		}
		ns := stringValue(item["namespace"])
		if ns == "" {
			ns = "<absent>"
		}
		o.req2HistoryPostAdapt = append(o.req2HistoryPostAdapt,
			fmt.Sprintf("name=%q namespace=%s", stringValue(item["name"]), ns))
		name := stringValue(item["name"])
		if _, ok := mapping.Calls[name]; ok && ns == "<absent>" {
			continue
		}
		o.failLocked("replayed history call %q stayed as name=%q namespace=%s after adaptation; it does not match a minted alias", callID, name, ns)
	}
}

// nativeCanaryFixture is the scripted fake upstream plus the adapt/restore
// sandwich the real service would apply around it.
type nativeCanaryFixture struct {
	obs    *nativeCanaryObservation
	models []byte
	callID string
	args   string
}

func (f *nativeCanaryFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/models"):
		f.obs.mu.Lock()
		f.obs.modelsRequests++
		f.obs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.models)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/responses"):
		f.handleResponses(w, r)
	default:
		f.obs.mu.Lock()
		f.obs.otherRequests = append(f.obs.otherRequests, r.Method+" "+path)
		f.obs.mu.Unlock()
		http.Error(w, "unhandled canary path", http.StatusNotFound)
	}
}

func (f *nativeCanaryFixture) handleResponses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.obs.fail("reading request body: %v", err)
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var req map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		f.obs.fail("decoding request body: %v", err)
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	n := f.obs.nextResponsesRequest()

	f.obs.mu.Lock()
	if n == 1 {
		f.obs.inspectFirstRequest(req)
	} else if n == 2 {
		f.obs.inspectFollowupRequest(req, f.callID)
	} else {
		f.obs.failLocked("unexpected extra POST /responses #%d", n)
	}
	f.obs.mu.Unlock()

	mapping, changed, err := AdaptResponsesCollaborationPlaintext(req)
	if err != nil {
		f.obs.fail("AdaptResponsesCollaborationPlaintext rejected request #%d: %v", n, err)
		f.writeFailedResponse(w, "canary adaptation error")
		return
	}

	f.obs.mu.Lock()
	if n == 1 {
		f.obs.inspectAdaptedTools(req, mapping, changed)
	} else if n == 2 {
		f.obs.inspectAdaptedFollowup(req, mapping, f.callID)
	}
	f.obs.mu.Unlock()

	switch n {
	case 1:
		f.writeToolCallTurn(w, mapping)
	default:
		// Later turns only get the terminal assistant message once the
		// follow-up conditions above were evaluated; on a recorded failure the
		// CLI still finishes so the test can report what it saw.
		f.writeFinalTurn(w, mapping)
	}
}

func (f *nativeCanaryFixture) writeFailedResponse(w http.ResponseWriter, message string) {
	f.writeEvents(w, ResponsesCollaborationPlaintextMapping{}, []map[string]any{{
		"type": "response.failed",
		"response": map[string]any{
			"id":     "resp_canary_err",
			"status": "failed",
			"error":  map[string]any{"code": "invalid_prompt", "message": message},
		},
	}})
}

func (f *nativeCanaryFixture) writeToolCallTurn(w http.ResponseWriter, mapping ResponsesCollaborationPlaintextMapping) {
	item := map[string]any{
		"type":      "function_call",
		"id":        "fc_canary_1",
		"call_id":   f.callID,
		"name":      "collaboration__send_message",
		"arguments": f.args,
	}
	f.writeEvents(w, mapping, []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_canary_1", "status": "in_progress"}},
		{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{
			"type": "function_call", "id": "fc_canary_1", "call_id": f.callID, "name": "collaboration__send_message", "arguments": "",
		}},
		{"type": "response.function_call_arguments.done", "item_id": "fc_canary_1", "output_index": 0, "name": "collaboration__send_message", "arguments": f.args},
		{"type": "response.output_item.done", "output_index": 0, "item": item},
		{"type": "response.completed", "response": map[string]any{
			"id": "resp_canary_1", "status": "completed", "output": []any{item},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		}},
	})
}

func (f *nativeCanaryFixture) writeFinalTurn(w http.ResponseWriter, mapping ResponsesCollaborationPlaintextMapping) {
	item := map[string]any{
		"type": "message",
		"id":   "msg_canary_final",
		"role": "assistant",
		"content": []any{map[string]any{
			"type": "output_text",
			"text": "canary final answer",
		}},
	}
	f.writeEvents(w, mapping, []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_canary_final", "status": "in_progress"}},
		{"type": "response.output_item.done", "output_index": 0, "item": item},
		{"type": "response.completed", "response": map[string]any{
			"id": "resp_canary_final", "status": "completed", "output": []any{item},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 5, "total_tokens": 25},
		}},
	})
}

// writeEvents restores every upstream payload through
// RestoreResponsesCollaborationPlaintextPayload before it reaches the client,
// exactly like the service's per-event restore hook.
func (f *nativeCanaryFixture) writeEvents(w http.ResponseWriter, mapping ResponsesCollaborationPlaintextMapping, events []map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			f.obs.fail("marshal scripted event: %v", err)
			return
		}
		restored, _, err := RestoreResponsesCollaborationPlaintextPayload(payload, mapping)
		if err != nil {
			f.obs.fail("RestoreResponsesCollaborationPlaintextPayload rejected an event: %v", err)
			return
		}
		f.checkRestoredCall(restored)
		var kind struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(restored, &kind)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind.Type, restored); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// checkRestoredCall inspects the post-restore event payload and verifies the
// scripted function_call reaches the client with the original namespace/name
// and an explicit empty encrypted_function_args array.
func (f *nativeCanaryFixture) checkRestoredCall(restored []byte) {
	var root map[string]any
	if err := json.Unmarshal(restored, &root); err != nil {
		return
	}
	var item map[string]any
	if candidate := nativeCanaryMap(root["item"]); candidate != nil && stringValue(candidate["type"]) == "function_call" {
		item = candidate
	}
	if item == nil {
		for _, raw := range nativeCanaryList(nativeCanaryMap(root["response"])["output"]) {
			candidate := nativeCanaryMap(raw)
			if candidate != nil && stringValue(candidate["type"]) == "function_call" {
				item = candidate
			}
		}
	}
	if item == nil || stringValue(item["call_id"]) != f.callID {
		return
	}
	f.obs.mu.Lock()
	defer f.obs.mu.Unlock()
	f.obs.restoredCallSeen = true
	f.obs.restoredCallName = stringValue(item["name"])
	f.obs.restoredCallNamespace = stringValue(item["namespace"])
	args, isList := item["encrypted_function_args"].([]any)
	f.obs.restoredEncryptedArgsValid = isList && len(args) == 0
	switch {
	case f.obs.restoredCallName != "send_message" || f.obs.restoredCallNamespace != "collaboration":
		f.obs.failLocked("restored function_call reached the client as name=%q namespace=%q, expected send_message in collaboration", f.obs.restoredCallName, f.obs.restoredCallNamespace)
	case !f.obs.restoredEncryptedArgsValid:
		f.obs.failLocked("restored function_call lacks an explicit empty encrypted_function_args array")
	}
}

func TestNativeCanaryUnexpectedThirdRequestReturns(t *testing.T) {
	obs := &nativeCanaryObservation{}
	fixture := &nativeCanaryFixture{obs: obs}
	obs.responsesRequests = 2
	done := make(chan struct{})
	go func() {
		defer close(done)
		request := httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{}`))
		fixture.handleResponses(httptest.NewRecorder(), request)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("third POST /responses did not return within one second")
	}
	obs.mu.Lock()
	defer obs.mu.Unlock()
	require.Equal(t, 3, obs.responsesRequests)
	require.Contains(t, obs.setupErrors, "unexpected extra POST /responses #3")
}

func nativeCanarySameNames(actual, expected []string) bool {
	got := slices.Clone(actual)
	want := slices.Clone(expected)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}

// nativeCanaryFailures is also exercised with negative fixtures so observations
// printed by the real CLI run cannot silently become a false positive.
func nativeCanaryFailures(o *nativeCanaryObservation, version string, eventTypes []string) []string {
	failures := append([]string(nil), o.setupErrors...)
	check := func(ok bool, format string, args ...any) {
		if !ok {
			failures = append(failures, fmt.Sprintf(format, args...))
		}
	}
	marked := []string{"followup_task", "send_message", "spawn_agent"}
	unmarked := []string{"interrupt_agent", "list_agents", "wait_agent"}
	aliases := []string{"collaboration__followup_task", "collaboration__send_message", "collaboration__spawn_agent"}
	check(version == "codex-cli 0.154.0", "unexpected codex version %q", version)
	check(o.responsesRequests == 2, "expected exactly 2 POST /responses calls, got %d", o.responsesRequests)
	check(len(o.otherRequests) == 0, "unexpected request paths: %v", o.otherRequests)
	check(o.req1SawCollaborationNamespace && o.req1NamespaceCarrier == "tools", "first request lacked collaboration namespace in tools carrier")
	check(o.req1SendMessageMarked && nativeCanarySameNames(o.req1MarkedChildren, marked), "first request marked children: %v", o.req1MarkedChildren)
	check(nativeCanarySameNames(o.req1UnmarkedChildren, unmarked), "first request unmarked children: %v", o.req1UnmarkedChildren)
	check(o.req1AdaptChanged && o.req1AliasDeclared && o.req1AliasMarkerStripped, "first request send_message alias/marker adaptation failed")
	check(nativeCanarySameNames(o.req1Aliases, aliases) && len(o.req1MarkedAliases) == 0, "adapted aliases or markers differ: aliases=%v marked=%v", o.req1Aliases, o.req1MarkedAliases)
	check(nativeCanarySameNames(o.req1MappedAliases, aliases), "adapter mapping aliases differ: %v", o.req1MappedAliases)
	check(nativeCanarySameNames(o.req1NamespaceRemaining, unmarked), "adapted namespace remaining children differ: %v", o.req1NamespaceRemaining)
	check(o.restoredCallSeen && o.restoredCallName == "send_message" && o.restoredCallNamespace == "collaboration" && o.restoredEncryptedArgsValid,
		"restored SSE call lacks expected namespace/name/empty encrypted_function_args")
	check(o.req2Seen && o.req2AgentMessageCount == 1 && o.req2AgentMessageAuthor == "/root" && o.req2AgentMessageRcpt == "/root",
		"self-message count/address differ: seen=%v count=%d author=%q recipient=%q", o.req2Seen, o.req2AgentMessageCount, o.req2AgentMessageAuthor, o.req2AgentMessageRcpt)
	check(o.req2NonceInInputText, "synthetic nonce absent from raw agent_message content[].input_text")
	check(o.req2EncryptedBlocks == 0 && !o.req2NonceInEncrypted, "raw agent_message contains encrypted content blocks (%d)", o.req2EncryptedBlocks)
	check(slices.Equal(o.req2HistoryCallShapes, []string{`name="send_message" namespace=collaboration encrypted_function_args=false`}),
		"raw history call namespace/name differ: %v", o.req2HistoryCallShapes)
	check(slices.Equal(o.req2HistoryPostAdapt, []string{`name="collaboration__send_message" namespace=<absent>`}),
		"post-adapt history alias differs: %v", o.req2HistoryPostAdapt)
	check(o.req2FunctionCallOutputs == 1, "matching function_call_output count: %d", o.req2FunctionCallOutputs)
	completed := 0
	for _, kind := range eventTypes {
		if kind == "turn.completed" {
			completed++
		}
		check(kind != "error" && !strings.Contains(kind, "failed") && !strings.HasSuffix(kind, ".error"), "CLI failure event: %s", kind)
	}
	check(completed == 1, "expected one CLI turn.completed event, got %d", completed)
	return failures
}

func nativeCanaryPassingObservation() *nativeCanaryObservation {
	return &nativeCanaryObservation{
		responsesRequests: 2, req1SawCollaborationNamespace: true, req1NamespaceCarrier: "tools",
		req1SendMessageMarked: true, req1MarkedChildren: []string{"followup_task", "send_message", "spawn_agent"},
		req1UnmarkedChildren: []string{"interrupt_agent", "list_agents", "wait_agent"},
		req1AdaptChanged:     true, req1AliasDeclared: true, req1AliasMarkerStripped: true,
		req1Aliases:            []string{"collaboration__followup_task", "collaboration__send_message", "collaboration__spawn_agent"},
		req1MappedAliases:      []string{"collaboration__followup_task", "collaboration__send_message", "collaboration__spawn_agent"},
		req1NamespaceRemaining: []string{"interrupt_agent", "list_agents", "wait_agent"},
		restoredCallSeen:       true, restoredCallName: "send_message", restoredCallNamespace: "collaboration", restoredEncryptedArgsValid: true,
		req2Seen: true, req2AgentMessageCount: 1, req2AgentMessageAuthor: "/root", req2AgentMessageRcpt: "/root", req2NonceInInputText: true,
		req2HistoryCallShapes: []string{`name="send_message" namespace=collaboration encrypted_function_args=false`},
		req2HistoryPostAdapt:  []string{`name="collaboration__send_message" namespace=<absent>`}, req2FunctionCallOutputs: 1,
	}
}

func TestNativeCanaryNegativeObservations(t *testing.T) {
	tests := []struct {
		name   string
		breaks func(*nativeCanaryObservation, *string, *[]string)
		want   string
	}{
		{"wrong author", func(o *nativeCanaryObservation, _ *string, _ *[]string) { o.req2AgentMessageAuthor = "/other" }, "self-message count/address"},
		{"wrong recipient", func(o *nativeCanaryObservation, _ *string, _ *[]string) { o.req2AgentMessageRcpt = "/other" }, "self-message count/address"},
		{"missing message", func(o *nativeCanaryObservation, _ *string, _ *[]string) { o.req2AgentMessageCount = 0 }, "self-message count/address"},
		{"encrypted block without nonce", func(o *nativeCanaryObservation, _ *string, _ *[]string) { o.req2EncryptedBlocks = 1 }, "contains encrypted content blocks"},
		{"missing raw history", func(o *nativeCanaryObservation, _ *string, _ *[]string) { o.req2HistoryCallShapes = nil }, "raw history call"},
		{"wrong raw namespace", func(o *nativeCanaryObservation, _ *string, _ *[]string) {
			o.req2HistoryCallShapes = []string{`name="send_message" namespace=<absent> encrypted_function_args=false`}
		}, "raw history call"},
		{"missing adapted alias", func(o *nativeCanaryObservation, _ *string, _ *[]string) { o.req2HistoryPostAdapt = nil }, "post-adapt history alias"},
		{"missing output", func(o *nativeCanaryObservation, _ *string, _ *[]string) { o.req2FunctionCallOutputs = 0 }, "matching function_call_output"},
		{"missing tool marker", func(o *nativeCanaryObservation, _ *string, _ *[]string) {
			o.req1MarkedChildren = []string{"send_message"}
		}, "first request marked children"},
		{"missing alias", func(o *nativeCanaryObservation, _ *string, _ *[]string) {
			o.req1Aliases = []string{"collaboration__send_message"}
		}, "adapted aliases"},
		{"unexpected version", func(_ *nativeCanaryObservation, v *string, _ *[]string) { *v = "codex-cli 0.155.0" }, "unexpected codex version"},
		{"missing completion", func(_ *nativeCanaryObservation, _ *string, events *[]string) { *events = nil }, "turn.completed"},
		{"failure event", func(_ *nativeCanaryObservation, _ *string, events *[]string) {
			*events = append(*events, "turn.failed")
		}, "CLI failure event"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obs := nativeCanaryPassingObservation()
			version := "codex-cli 0.154.0"
			events := []string{"turn.completed"}
			require.Empty(t, nativeCanaryFailures(obs, version, events), "normal observation must pass")
			tc.breaks(obs, &version, &events)
			require.Contains(t, strings.Join(nativeCanaryFailures(obs, version, events), "; "), tc.want)
		})
	}
}

// The opt-in negative run checks that an observation failure reaches go test's
// exit status, without making the normal suite intentionally fail.
func TestNativeCanaryFailurePropagation(t *testing.T) {
	if os.Getenv("SUB2API_TEST_NATIVE_CANARY_FORCE_FAILURE") != "1" {
		t.Skip("intentional negative run requires SUB2API_TEST_NATIVE_CANARY_FORCE_FAILURE=1")
	}
	obs := nativeCanaryPassingObservation()
	obs.req2AgentMessageAuthor = "/other"
	for _, failure := range nativeCanaryFailures(obs, "codex-cli 0.154.0", []string{"turn.completed"}) {
		t.Errorf("harness: %s", failure)
	}
}

func TestNativeCanaryRawMessageInspection(t *testing.T) {
	for _, tc := range []struct {
		name, author, recipient string
		encrypted               bool
		want                    string
	}{
		{"normal", "/root", "/root", false, ""},
		{"wrong author", "/other", "/root", false, "self-message count/address"},
		{"wrong recipient", "/root", "/other", false, "self-message count/address"},
		{"encrypted content without nonce", "/root", "/root", true, "encrypted content blocks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs := nativeCanaryPassingObservation()
			obs.nonce = "synthetic-nonce"
			obs.req2AgentMessageCount, obs.req2EncryptedBlocks = 0, 0
			obs.req2NonceInInputText = false
			obs.req2HistoryCallShapes = nil
			obs.req2FunctionCallOutputs = 0
			content := []any{map[string]any{"type": "input_text", "text": "synthetic-nonce"}}
			if tc.encrypted {
				content = append(content, map[string]any{"type": "encrypted_content", "encrypted_content": "opaque"})
			}
			obs.inspectFollowupRequest(map[string]any{"input": []any{
				map[string]any{"type": "agent_message", "author": tc.author, "recipient": tc.recipient, "content": content},
				map[string]any{"type": "function_call", "call_id": "call_1", "name": "send_message", "namespace": "collaboration"},
				map[string]any{"type": "function_call_output", "call_id": "call_1"},
			}}, "call_1")
			failures := strings.Join(nativeCanaryFailures(obs, "codex-cli 0.154.0", []string{"turn.completed"}), "; ")
			if tc.want == "" {
				require.Empty(t, failures)
			} else {
				require.Contains(t, failures, tc.want)
			}
		})
	}
}

// TestAdaptResponsesCollaborationPlaintext_NativeCodexCanary drives the real
// pinned Codex CLI against a scripted local fake upstream and verifies the
// plaintext collaboration path end to end: the CLI emits the MultiAgentV2
// collaboration namespace with encrypted:true message markers, the adapter
// lowers marked tools to aliases, the restored function_call executes as a
// plaintext self-message, and the follow-up request carries the delivered
// message as an agent_message input_text item.
//
// The test only runs with SUB2API_TEST_NATIVE_CANARY=1 and
// SUB2API_TEST_CODEX_BIN pointing at the pinned codex binary.
func TestAdaptResponsesCollaborationPlaintext_NativeCodexCanary(t *testing.T) {
	if os.Getenv("SUB2API_TEST_NATIVE_CANARY") != "1" {
		t.Skip("native canary requires SUB2API_TEST_NATIVE_CANARY=1")
	}
	codexBin := os.Getenv("SUB2API_TEST_CODEX_BIN")
	if codexBin == "" {
		t.Skip("native canary requires SUB2API_TEST_CODEX_BIN")
	}
	if info, err := os.Stat(codexBin); err != nil || info.IsDir() {
		t.Fatalf("setup: SUB2API_TEST_CODEX_BIN %q is not a file: %v", codexBin, err)
	}

	verCtx, verCancel := context.WithTimeout(context.Background(), 30*time.Second)
	verOut, verErr := exec.CommandContext(verCtx, codexBin, "--version").CombinedOutput()
	verCancel()
	require.NoError(t, verErr, "setup: codex --version failed")
	codexVersion := strings.TrimSpace(string(verOut))
	t.Logf("codex binary version: %s", codexVersion)

	modelsJSON, err := os.ReadFile("testdata/codex_plaintext_canary_models.json")
	require.NoError(t, err, "setup: read canary models metadata")

	nonceBytes := make([]byte, 12)
	_, err = rand.Read(nonceBytes)
	require.NoError(t, err)
	nonce := "canarynonce-" + hex.EncodeToString(nonceBytes)

	obs := &nativeCanaryObservation{nonce: nonce}
	fixture := &nativeCanaryFixture{
		obs:    obs,
		models: modelsJSON,
		callID: "call_canary_1",
		args:   fmt.Sprintf(`{"target":"/root","message":%q}`, nonce),
	}
	server := httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	defer server.Close()

	// The prompt deliberately excludes the nonce: any nonce found later came
	// from the scripted send_message call, not from prompt echo.
	workDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	args := []string{
		"exec",
		"--ignore-user-config",
		"--ignore-rules",
		"--skip-git-repo-check",
		"--ephemeral",
		"--json",
		"--sandbox", "read-only",
		"--enable", "multi_agent_v2",
		"-m", "canary-plaintext",
		"-c", `model_provider="canary"`,
		"-c", `model_providers.canary.name="canary"`,
		"-c", fmt.Sprintf(`model_providers.canary.base_url=%q`, server.URL),
		"-c", `model_providers.canary.env_key="SUB2API_NATIVE_CANARY_API_KEY"`,
		"-c", `model_providers.canary.wire_api="responses"`,
		"-C", workDir,
		"canary run: a scripted tool call will arrive; wait for it and then answer.",
	}
	cmd := exec.CommandContext(ctx, codexBin, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "SUB2API_NATIVE_CANARY_API_KEY=synthetic-canary-dummy")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	// Close waits for any in-flight handler so the observation reads below are
	// synchronized with the last write.
	server.Close()

	var eventTypes []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil {
			if typ, ok := event["type"].(string); ok {
				eventTypes = append(eventTypes, typ)
			}
		}
	}

	obs.mu.Lock()
	t.Logf("requests: models=%d responses=%d other=%v", obs.modelsRequests, obs.responsesRequests, obs.otherRequests)
	t.Logf("request1: namespace=%v carrier=%s marked=%v unmarked=%v -> adaptChanged=%v aliasDeclared=%v markerStripped=%v aliases=%v mapped=%v markedAliases=%v namespaceRemaining=%v",
		obs.req1SawCollaborationNamespace, obs.req1NamespaceCarrier, obs.req1MarkedChildren, obs.req1UnmarkedChildren,
		obs.req1AdaptChanged, obs.req1AliasDeclared, obs.req1AliasMarkerStripped, obs.req1Aliases, obs.req1MappedAliases, obs.req1MarkedAliases, obs.req1NamespaceRemaining)
	t.Logf("request2: seen=%v agentMessages=%d author=%q recipient=%q nonceInInputText=%v encryptedBlocks=%d historyRaw=%v historyAdapted=%v outputs=%d",
		obs.req2Seen, obs.req2AgentMessageCount, obs.req2AgentMessageAuthor, obs.req2AgentMessageRcpt,
		obs.req2NonceInInputText, obs.req2EncryptedBlocks, obs.req2HistoryCallShapes, obs.req2HistoryPostAdapt, obs.req2FunctionCallOutputs)
	t.Logf("restored call toward cli: seen=%v name=%q namespace=%q encryptedArgsEmpty=%v",
		obs.restoredCallSeen, obs.restoredCallName, obs.restoredCallNamespace, obs.restoredEncryptedArgsValid)
	t.Logf("cli event types: %v", eventTypes)
	for _, note := range obs.notes {
		t.Logf("observation: %s", note)
	}
	failures := nativeCanaryFailures(obs, codexVersion, eventTypes)
	obs.mu.Unlock()
	if runErr != nil {
		// The CLI transcript can contain request details; report only metadata.
		t.Errorf("codex exec failed: %v (ctx timeout reached: %v); stderr bytes: %d", runErr, ctx.Err() != nil, stderr.Len())
	}
	for _, failure := range failures {
		t.Errorf("harness: %s", failure)
	}
}
