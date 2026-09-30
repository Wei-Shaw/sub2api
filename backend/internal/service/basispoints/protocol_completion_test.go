package basispoints

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFunctionSchemaRejectsInvalidBatchWithoutReplay(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{
		"type": "object", "required": []any{"count"}, "additionalProperties": false,
		"properties": object{"count": object{"type": "integer", "minimum": json.Number("9007199254740993"), "maximum": json.Number("9007199254740995")}},
	}}}
	for _, args := range []string{`{}`, `{"count":1}`, `{"count":9007199254740993,"extra":1}`, `{"count":9007199254740993.5}`} {
		cache := new(ReplayCache)
		_, b := mustPrepare(t, source, "scope", cache)
		valid := nativeCall(object{"name": "shell", "arguments": object{"count": json.Number("9007199254740993")}})
		invalid := nativeCall(object{"name": "shell", "arguments": args})
		invalid["id"], invalid["call_id"] = "fc_bad", "call_bad"
		response := object{"output": []any{valid, invalid}}
		before := quoted(response)
		if err := b.translateResponse(response); err == nil {
			t.Fatalf("accepted invalid args %s", args)
		}
		if quoted(response) != before || cache.get("scope", "call_native") != nil {
			t.Fatal("invalid batch changed output or replay cache")
		}
	}
	_, b := mustPrepare(t, source, "scope", nil)
	if err := b.translateResponse(object{"output": []any{nativeCall(object{"name": "shell", "arguments": object{"count": json.Number("9007199254740993.0")}})}}); err != nil {
		t.Fatal(err)
	}
}

func TestFunctionSchemaConstraintsAndExternalReferences(t *testing.T) {
	for _, tc := range []struct {
		schema, args string
		valid        bool
	}{
		{`{"type":"object","properties":{"x":{"enum":["a","b"]}}}`, `{"x":"c"}`, false},
		{`{"type":"object","properties":{"x":{"const":9007199254740993}}}`, `{"x":9007199254740992}`, false},
		{`{"type":"object","properties":{"x":{"type":"number","multipleOf":0.1}}}`, `{"x":0.3}`, true},
		{`{"type":"object","properties":{"x":{"type":"number","exclusiveMinimum":1}}}`, `{"x":1}`, false},
		{`{"type":"object","oneOf":[{"required":["a"]},{"required":["b"]}]}`, `{"a":1,"b":2}`, false},
		{`{"type":"object","$defs":{"n":{"type":"integer"}},"properties":{"x":{"$ref":"#/$defs/n"}}}`, `{"x":1e0}`, true},
	} {
		var schema object
		_ = decode([]byte(tc.schema), &schema)
		source := testSource()
		source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": schema}}
		_, b := mustPrepare(t, source, "scope", nil)
		err := b.translateResponse(object{"output": []any{nativeCall(object{"name": "shell", "arguments": tc.args})}})
		if (err == nil) != tc.valid {
			t.Fatalf("%s / %s: %v", tc.schema, tc.args, err)
		}
	}
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"$ref": "https://example.invalid/private"}}}
	raw, _ := json.Marshal(source)
	if _, _, err := Prepare(raw, "", nil); err == nil {
		t.Fatal("external schema accepted")
	}
}

func TestReplayIdleTTLAndSnapshotIsolation(t *testing.T) {
	now := time.Unix(100, 0)
	cache := &ReplayCache{now: func() time.Time { return now }}
	cache.put("a", "id", object{"n": json.Number("9007199254740993")})
	now = now.Add(time.Hour)
	got := cache.get("a", "id")
	got["n"] = "changed"
	if cache.get("a", "id")["n"] != json.Number("9007199254740993") {
		t.Fatal("snapshot mutation or precision loss")
	}
	now = now.Add(replayCacheIdleTTL)
	if cache.get("a", "id") != nil || cache.bytes != 0 || cache.order.Len() != 0 {
		t.Fatal("idle entry not reclaimed")
	}
}

func prepareCatalogTest(t *testing.T, c *CatalogCache, source object, scope string) *Bridge {
	t.Helper()
	raw, _ := json.Marshal(source)
	_, b, err := PrepareWithCatalog(raw, scope, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestCatalogInheritanceReplacementAndIsolation(t *testing.T) {
	c := new(CatalogCache)
	source := testSource()
	source["tools"] = []any{object{"type": "namespace", "name": "fs", "tools": []any{object{"type": "custom", "name": "patch", "format": object{"type": "text"}}}}}
	prepareCatalogTest(t, c, source, "account/key/thread")
	delete(source, "tools")
	b := prepareCatalogTest(t, c, source, "account/key/thread")
	if len(b.tools) != 1 || b.tools["fs.patch"].Kind != "custom" {
		t.Fatal("catalog not inherited")
	}
	if len(prepareCatalogTest(t, c, source, "other/key/thread").tools) != 0 {
		t.Fatal("catalog leaked across scopes")
	}
	source["input"] = []any{object{"type": "additional_tools", "tools": []any{object{"type": "function", "name": "shell"}}}, message("user", "continue")}
	if len(prepareCatalogTest(t, c, source, "account/key/thread").tools) != 2 {
		t.Fatal("increment not merged")
	}
	source["input"] = "continue"
	source["tool_choice"] = "none"
	if len(prepareCatalogTest(t, c, source, "account/key/thread").tools) != 0 {
		t.Fatal("none enabled tools")
	}
	delete(source, "tool_choice")
	if len(prepareCatalogTest(t, c, source, "account/key/thread").tools) != 2 {
		t.Fatal("none erased catalog")
	}
	source["tools"] = []any{}
	prepareCatalogTest(t, c, source, "account/key/thread")
	delete(source, "tools")
	if len(prepareCatalogTest(t, c, source, "account/key/thread").tools) != 0 {
		t.Fatal("explicit empty did not clear")
	}
	source["tools"] = []any{object{"type": "function", "name": "private"}}
	prepareCatalogTest(t, c, source, "")
	delete(source, "tools")
	if len(prepareCatalogTest(t, c, source, "").tools) != 0 {
		t.Fatal("anonymous catalog cached")
	}
}
func TestCatalogConcurrentIncrementAndStaleCommit(t *testing.T) {
	c := new(CatalogCache)
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			source := testSource()
			source["input"] = []any{object{"type": "additional_tools", "tools": []any{object{"type": "function", "name": fmt.Sprint("tool", i)}}}, message("user", "go")}
			raw, _ := json.Marshal(source)
			_, _, err := PrepareWithCatalog(raw, "scope", nil, c)
			failures <- err
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(prepareCatalogTest(t, c, testSource(), "scope").tools) != 4 {
		t.Fatal("concurrent additions lost")
	}
	_, version := c.snapshot("scope")
	if !c.commit("scope", version, []byte("[]")) || c.commit("scope", version, []byte(`[{"type":"function","name":"stale"}]`)) {
		t.Fatal("stale version accepted")
	}
}

func pngAttachment(t *testing.T) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes(), "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
func TestNativeAttachmentsMultipartDedupAndFileIDs(t *testing.T) {
	original, url := pngAttachment(t)
	source := testSource()
	source["input"] = []any{object{"role": "user", "content": []any{object{"type": "input_image", "image_url": url, "detail": "high"}}}, object{"type": "custom_tool_call_output", "call_id": "x", "output": []any{object{"type": "input_image", "image_url": url}}}}
	raw, _ := json.Marshal(source)
	calls := 0
	plan, err := PrepareNativeImages(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := plan.Upload(context.Background(), new(AttachmentCache), "", func(ctx context.Context, img InlineAttachment) (string, error) {
		calls++
		readerBody, contentType, length, err := img.Multipart()
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, AttachmentsURL, readerBody)
		if err == nil {
			req.Header.Set("Authorization", "Bearer test")
			req.Header.Set("Content-Type", contentType)
			req.ContentLength = length
		}
		if err != nil {
			return "", err
		}
		if req.URL.String() != AttachmentsURL || req.Header.Get("Authorization") != "Bearer test" {
			t.Fatal("wrong attachment route/auth")
		}
		wire, _ := io.ReadAll(req.Body)
		if int64(len(wire)) != req.ContentLength {
			t.Fatal("wrong multipart length")
		}
		req.Body = io.NopCloser(bytes.NewReader(wire))
		reader, err := req.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(part)
		if part.FormName() != "file" || part.Header.Get("Content-Type") != "image/png" || !bytes.Equal(got, original) {
			t.Fatal("image bytes or multipart fields changed")
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Fatal("unexpected extra multipart part")
		}
		return "file-native", nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("upload: %d %v", calls, err)
	}
	var got object
	_ = decode(out, &got)
	items := mustTestValue[[]any](t, got["input"])
	first := mustTestValue[object](t, items[0])
	content := mustTestValue[[]any](t, first["content"])
	part := mustTestValue[object](t, content[0])
	if part["file_id"] != "file-native" || part["detail"] != "high" || part["image_url"] != nil {
		t.Fatal("image reference/detail changed")
	}
	if !bytes.Contains(raw, []byte("data:image")) {
		t.Fatal("input mutated")
	}
	fileSource := testSource()
	fileSource["input"] = []any{object{"role": "user", "content": []any{part}}}
	mustPrepare(t, fileSource, "", nil)
}
func TestNativeAttachmentsValidateWholeBatchBeforeUpload(t *testing.T) {
	_, url := pngAttachment(t)
	for _, bad := range []object{
		{"type": "input_image", "image_url": "data:image/png;base64,AAAA"},
		{"type": "input_image", "image_url": url, "detail": "invalid"},
		{"type": "input_image", "file_id": "file-invalid", "image_url": url},
	} {
		source := testSource()
		source["input"] = []any{object{"role": "user", "content": []any{object{"type": "input_image", "image_url": url}, bad}}}
		raw, _ := json.Marshal(source)
		calls := 0
		plan, err := PrepareNativeImages(raw)
		if err == nil {
			_, err = plan.Upload(context.Background(), new(AttachmentCache), "", func(context.Context, InlineAttachment) (string, error) { calls++; return "file-id", nil })
		}
		if err == nil || calls != 0 {
			t.Fatalf("invalid batch uploaded %d: %v", calls, err)
		}
	}

}

func TestToolRepairPreservesStreamAndAggregatesUsage(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"type": "object"}}}
	_, b := mustPrepare(t, source, "scope", new(ReplayCache))
	bad := nativeCall(object{"name": "unknown", "arguments": object{}})
	good := nativeCall(object{"name": "shell", "arguments": object{"cmd": "pwd"}})
	good["id"], good["call_id"] = "fc_fixed", "call_fixed"
	original := object{"id": "resp_original", "status": "completed", "usage": object{"input_tokens": 10, "output_tokens": 2, "input_tokens_details": object{"cached_tokens": 3}}, "output": []any{bad}}
	fixed := object{"id": "resp_fixed", "status": "completed", "usage": object{"input_tokens": 7, "output_tokens": 1, "input_tokens_details": object{"cached_tokens": 2}}, "output": []any{good}}
	calls := 0
	body := b.StreamWithRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": original}))), func(context.Context) (io.ReadCloser, error) {
		calls++
		return io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": fixed}))), nil
	})
	out, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || calls != 1 || bytes.Contains(out, []byte("response.failed")) {
		t.Fatalf("%d %v %s", calls, err, out)
	}
	var final object
	_ = readEvents(bytes.NewReader(out), func(_ string, data []byte) error {
		var e object
		_ = decode(data, &e)
		if e["type"] == "response.completed" {
			final = mustTestValue[object](t, e["response"])
		}
		return nil
	})
	if final["id"] != "resp_original" || quoted(final["usage"]) != `{"input_tokens":17,"input_tokens_details":{"cached_tokens":5},"output_tokens":3}` {
		t.Fatalf("identity/usage lost: %v", final)
	}
	if b.replay.get("scope", "call_native") != nil || b.replay.get("scope", "call_fixed") == nil {
		t.Fatal("incorrect tool cache committed")
	}
}
func TestToolRepairNeverRetriesContinuationOrSchemaFailure(t *testing.T) {
	for _, history := range []bool{false, true} {
		source := testSource()
		source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"type": "object", "required": []any{"cmd"}}}}
		if history {
			source["input"] = []any{message("user", "hi"), object{"type": "function_call", "name": "shell", "call_id": "old", "arguments": `{"cmd":"pwd"}`}, object{"type": "function_call_output", "call_id": "old", "output": "ok"}}
		}
		_, b := mustPrepare(t, source, "scope", nil)
		name := "shell"
		if history {
			name = "unknown"
		}
		response := object{"status": "completed", "output": []any{nativeCall(object{"name": name, "arguments": object{}})}}
		calls := 0
		body := b.StreamWithRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": response}))), func(context.Context) (io.ReadCloser, error) { calls++; return nil, fmt.Errorf("must not retry") })
		out, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || calls != 0 || !bytes.Contains(out, []byte("response.failed")) || bytes.Contains(out, []byte("response.function_call_arguments")) {
			t.Fatalf("unsafe retry: %d %v %s", calls, err, out)
		}
	}
}

func TestToolRepairFailsOnceWithoutDispatchAndKeepsUsage(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	cache := new(ReplayCache)
	_, b := mustPrepare(t, source, "scope", cache)
	bad := nativeCall(object{"name": "missing", "arguments": object{}})
	terminal := func(tokens int) string {
		return sse(object{"type": "response.completed", "response": object{"status": "completed", "usage": object{"input_tokens": tokens}, "output": []any{bad}}})
	}
	attempts := 0
	body := b.StreamWithRepair(context.Background(), io.NopCloser(strings.NewReader(terminal(3))), func(context.Context) (io.ReadCloser, error) {
		attempts++
		return io.NopCloser(strings.NewReader(terminal(5))), nil
	})
	out, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || attempts != 1 || !bytes.Contains(out, []byte(`"input_tokens":8`)) || !bytes.Contains(out, []byte("response.failed")) || bytes.Contains(out, []byte("response.function_call_arguments")) || cache.get("scope", "call_native") != nil {
		t.Fatalf("unsafe failure: %d %v %s", attempts, err, out)
	}
}
func TestToolRepairCancellationStopsPendingCorrection(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	_, b := mustPrepare(t, source, "scope", nil)
	bad := nativeCall(object{"name": "missing", "arguments": object{}})
	started, stopped := make(chan struct{}), make(chan struct{})
	body := b.StreamWithRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": object{"status": "completed", "output": []any{bad}}}))), func(ctx context.Context) (io.ReadCloser, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("correction did not start")
	}
	_ = body.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("correction context not canceled")
	}
}
func TestToolBatchIdentityAndParallelConstraints(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		source := testSource()
		source["tools"] = []any{object{"type": "function", "name": "shell"}}
		if !duplicate {
			source["parallel_tool_calls"] = false
		}
		_, b := mustPrepare(t, source, "scope", new(ReplayCache))
		first := nativeCall(object{"name": "shell", "arguments": object{}})
		second := nativeCall(object{"name": "shell", "arguments": object{}})
		if !duplicate {
			second["id"], second["call_id"] = "second", "second"
		}
		if err := b.translateResponse(object{"output": []any{first, second}}); err == nil {
			t.Fatal("invalid batch accepted")
		}
		if b.replay.get("scope", "call_native") != nil {
			t.Fatal("partial batch cached")
		}
	}
}
func TestCatalogExpiryAndInvalidIncrementAreAtomic(t *testing.T) {
	now := time.Unix(100, 0)
	cache := &CatalogCache{now: func() time.Time { return now }}
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	prepareCatalogTest(t, cache, source, "scope")
	delete(source, "tools")
	source["input"] = []any{object{"type": "additional_tools", "tools": []any{object{"type": "custom", "name": "shell"}}}, message("user", "continue")}
	raw, _ := json.Marshal(source)
	if _, _, err := PrepareWithCatalog(raw, "scope", nil, cache); err == nil {
		t.Fatal("conflicting increment accepted")
	}
	b := prepareCatalogTest(t, cache, testSource(), "scope")
	if len(b.tools) != 1 || b.tools["shell"].Kind != "function" {
		t.Fatal("invalid delta mutated catalog")
	}
	now = now.Add(replayCacheIdleTTL)
	if len(prepareCatalogTest(t, cache, testSource(), "scope").tools) != 0 {
		t.Fatal("idle catalog inherited")
	}
}
func TestRepairRequestKeepsAttachmentAndCompactionLast(t *testing.T) {
	original := []byte(`{"model":"gpt-6-astra","metadata":{"turn_id":"stable"},"input":[{"role":"user","content":[{"type":"input_image","file_id":"file-existing"}]},{"type":"compaction_trigger"}]}`)
	fixed, err := RepairRequest(original)
	if err != nil {
		t.Fatal(err)
	}
	var req object
	_ = decode(fixed, &req)
	input := mustTestValue[[]any](t, req["input"])
	if req["model"] != "gpt-6-astra" || mustTestValue[object](t, input[len(input)-1])["type"] != "compaction_trigger" || !bytes.Contains(fixed, []byte("file-existing")) {
		t.Fatal("repair changed prepared request")
	}
}

func TestToolSchemaRejectsUnboundedExponentsBeforeExpansion(t *testing.T) {
	for _, number := range []string{"1e1000000000", "1e-1000000000", "1e999999999999999999999999"} {
		source := testSource()
		source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"type": "object", "properties": object{"n": object{"type": "number"}}}}}
		_, b := mustPrepare(t, source, "scope", nil)
		response := object{"output": []any{nativeCall(object{"name": "shell", "arguments": object{"n": json.Number(number)}})}}
		if err := b.translateResponse(response); err == nil {
			t.Fatal("unbounded argument exponent accepted")
		}
		source["tools"] = []any{object{"type": "function", "name": "shell", "parameters": object{"type": "object", "properties": object{"n": object{"type": "number", "minimum": json.Number(number)}}}}}
		raw, _ := json.Marshal(source)
		if _, _, err := Prepare(raw, "", nil); err == nil {
			t.Fatal("unbounded schema exponent accepted")
		}
	}
}

func TestCorrectionDisconnectRetainsReportedUsageWithoutDoubleCounting(t *testing.T) {
	wire := sse(object{"type": "response.in_progress", "response": object{"usage": object{"input_tokens": 7, "output_tokens": 1}}}) +
		sse(object{"type": "response.in_progress", "response": object{"usage": object{"input_tokens": 7, "output_tokens": 2}}})
	response, err := readRepairResponse(strings.NewReader(wire))
	if err == nil || quoted(response["usage"]) != `{"input_tokens":7,"output_tokens":2}` {
		t.Fatalf("progress usage lost or summed twice: %v %v", response, err)
	}
}

func TestReprepareKeepsValidatedCatalogDuringAttachmentUpload(t *testing.T) {
	cache := new(CatalogCache)
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "original"}}
	prepareCatalogTest(t, cache, source, "scope")
	delete(source, "tools")
	b := prepareCatalogTest(t, cache, source, "scope")
	newer := testSource()
	newer["tools"] = []any{object{"type": "function", "name": "newer"}}
	prepareCatalogTest(t, cache, newer, "scope")
	source["input"] = []any{object{"role": "user", "content": []any{object{"type": "input_image", "file_id": "file-uploaded"}}}}
	raw, _ := json.Marshal(source)
	body, final, err := b.Reprepare(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := final.tools["original"]; !ok || len(final.tools) != 1 || !bytes.Contains(body, []byte("file-uploaded")) {
		t.Fatal("upload changed validated request catalog")
	}
	current := prepareCatalogTest(t, cache, testSource(), "scope")
	if _, ok := current.tools["newer"]; !ok || len(current.tools) != 1 {
		t.Fatal("upload overwrote newer session catalog")
	}
}

func TestRawCommandCompatibilityDoesNotBypassOrdinarySchema(t *testing.T) {
	source := testSource()
	spec := functionCmdTestTool("exec_command")
	params := mustTestValue[object](t, spec["parameters"])
	params["additionalProperties"] = false
	source["tools"] = []any{spec}
	_, b := mustPrepare(t, source, "scope", nil)
	if _, err := b.translateCall(nativeCall(object{"name": "exec_command", "arguments": object{"cmd": "pwd", "undeclared": true}})); err == nil {
		t.Fatal("ordinary command envelope bypassed schema")
	}
	if _, err := b.translateCall(functionCmdTestNative(t, "exec_command", "pwd", "{\"undeclared\":true}")); err != nil {
		t.Fatal(err)
	}
}
