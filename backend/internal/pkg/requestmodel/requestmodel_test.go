package requestmodel

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
)

func TestFromBodyJSONModel(t *testing.T) {
	if got := FromBody("application/json", []byte(`{"model":"gpt-5.4"}`)); got != "gpt-5.4" {
		t.Fatalf("got %q", got)
	}
}

func TestFromBodyJSONSessionModel(t *testing.T) {
	if got := FromBody("application/json", []byte(`{"session":{"model":"gpt-live"},"sdp":"v=0"}`)); got != "gpt-live" {
		t.Fatalf("got %q", got)
	}
}

func TestFromBodyJSONModelWinsOverSession(t *testing.T) {
	body := []byte(`{"model":"top-level","session":{"model":"nested"}}`)
	if got := FromBody("application/json", body); got != "top-level" {
		t.Fatalf("got %q", got)
	}
}

func TestFromBodyJSONBlankModelFallsThrough(t *testing.T) {
	if got := FromBody("application/json", []byte(`{"model":"  "}`)); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestFromBodyMultipartModelField(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("prompt", "draw")
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.Close()

	if got := FromBody(writer.FormDataContentType(), body.Bytes()); got != "gpt-image-1" {
		t.Fatalf("got %q", got)
	}
}

func TestFromBodyMultipartSessionField(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("sdp", "v=0")
	_ = writer.WriteField("session", `{"model":"live-alias"}`)
	_ = writer.WriteField("media", "file-part") // 字段名不匹配，应被跳过
	_ = writer.Close()

	if got := FromBody(writer.FormDataContentType(), body.Bytes()); got != "live-alias" {
		t.Fatalf("got %q", got)
	}
}

func TestFromBodySkipsFilePartsBeforeModelField(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "input.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write(bytes.Repeat([]byte{0}, 4096))
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.Close()

	if got := FromBody(writer.FormDataContentType(), body.Bytes()); got != "gpt-image-1" {
		t.Fatalf("got %q", got)
	}
}

func TestFromBodyNoModelReturnsEmpty(t *testing.T) {
	if got := FromBody("application/json", []byte(`{"input":"hi"}`)); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := FromBody("text/plain", []byte("plain")); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResetRequestBodyAllowsRereadWithoutCopy(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	ResetRequestBody(req, body)

	got, err := httputil.ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if &got[0] != &body[0] {
		t.Fatalf("expected zero-copy reread of the same slice")
	}
	if req.ContentLength != int64(len(body)) {
		t.Fatalf("content length mismatch: %d", req.ContentLength)
	}
	if req.Header.Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Fatalf("content-length header mismatch: %q", req.Header.Get("Content-Length"))
	}
}

func TestResetRequestBodyNilRequestIsNoop(t *testing.T) {
	ResetRequestBody(nil, []byte("x"))
}

func TestFromBodyForRouteLivePrefersSessionModel(t *testing.T) {
	body := []byte(`{"model":"top-level","session":{"model":"session-model"},"sdp":"v=0"}`)

	if got := FromBodyForRoute("/v1/live", "application/json", body); got != "session-model" {
		t.Fatalf("live route should prefer session.model, got %q", got)
	}
	if got := FromBodyForRoute("/backend-api/codex/realtime/calls", "application/json", body); got != "session-model" {
		t.Fatalf("codex realtime route should prefer session.model, got %q", got)
	}
	if got := FromBodyForRoute("/live", "application/json", body); got != "session-model" {
		t.Fatalf("root live alias should prefer session.model, got %q", got)
	}
}

func TestFromBodyForRouteNonLivePrefersTopLevelModel(t *testing.T) {
	body := []byte(`{"model":"top-level","session":{"model":"session-model"}}`)

	if got := FromBodyForRoute("/v1/messages", "application/json", body); got != "top-level" {
		t.Fatalf("non-live route should prefer top-level model, got %q", got)
	}
	if got := FromBodyForRoute("/v1/responses", "application/json", body); got != "top-level" {
		t.Fatalf("responses route should prefer top-level model, got %q", got)
	}
}

func TestFromBodyForRouteFallsBackWhenPreferredFieldMissing(t *testing.T) {
	if got := FromBodyForRoute("/v1/live", "application/json", []byte(`{"model":"top-level"}`)); got != "top-level" {
		t.Fatalf("live without session.model should fall back to model, got %q", got)
	}
	if got := FromBodyForRoute("/v1/messages", "application/json", []byte(`{"session":{"model":"session-model"}}`)); got != "session-model" {
		t.Fatalf("non-live without model should fall back to session.model, got %q", got)
	}
}

func TestFromBodyForRouteMultipartSessionFirstForLive(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "top-level")
	_ = writer.WriteField("session", `{"model":"session-model"}`)
	_ = writer.Close()
	contentType := writer.FormDataContentType()

	if got := FromBodyForRoute("/v1/live", contentType, body.Bytes()); got != "session-model" {
		t.Fatalf("live multipart should prefer session field, got %q", got)
	}
	if got := FromBodyForRoute("/v1/messages", contentType, body.Bytes()); got != "top-level" {
		t.Fatalf("non-live multipart should prefer model field, got %q", got)
	}
}

func TestIsLiveRequestRoute(t *testing.T) {
	for _, path := range []string{"/v1/live", "/live", "/backend-api/codex/realtime/calls"} {
		if !IsLiveRequestRoute(path) {
			t.Fatalf("%s should be a live route", path)
		}
	}
	for _, path := range []string{"/v1/messages", "/v1/responses", "/realtime", ""} {
		if IsLiveRequestRoute(path) {
			t.Fatalf("%s should not be a live route", path)
		}
	}
}

// gjson 的部分扫描会在 multipart 字节流里误匹配 session JSON 内的 model；
// 声明为 multipart 的请求体必须走 multipart 解析。
func TestFromBodyMultipartContentTypeSkipsJSONScan(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "top-level")
	_ = writer.WriteField("session", `{"model":"session-model"}`)
	_ = writer.Close()

	if got := FromBody(writer.FormDataContentType(), body.Bytes()); got != "top-level" {
		t.Fatalf("multipart body should use the model field, got %q", got)
	}
	if got := FromBodyForRoute("/v1/live", writer.FormDataContentType(), body.Bytes()); got != "session-model" {
		t.Fatalf("live multipart should use the session field, got %q", got)
	}
}

func TestFromBodyCandidatesIncludesCaseVariantsAndDuplicates(t *testing.T) {
	// encoding/json 绑定对键名大小写不敏感、重复键取末值；gjson 取首个。
	// 候选集必须包含全部出现。
	got := FromBodyCandidates("", "application/json", []byte(`{"Model":"caps-model"}`))
	if len(got) != 1 || got[0] != "caps-model" {
		t.Fatalf("case-variant key must be a candidate, got %#v", got)
	}

	got = FromBodyCandidates("", "application/json", []byte(`{"model":"first","model":"last"}`))
	if len(got) != 2 || got[0] != "first" || got[1] != "last" {
		t.Fatalf("duplicate keys must both be candidates, got %#v", got)
	}
}

func TestFromBodyCandidatesSessionVariants(t *testing.T) {
	body := []byte(`{"Session":{"Model":"s-caps"},"session":{"model":"s-lower"},"model":"top"}`)

	// 非 Live 入口：顶层候选。
	got := FromBodyCandidates("", "application/json", body)
	if len(got) != 1 || got[0] != "top" {
		t.Fatalf("non-live should use top-level candidates, got %#v", got)
	}

	// Live 入口：session 候选（含大小写变体与重复）。
	got = FromBodyCandidates("/v1/live", "application/json", body)
	if len(got) != 2 || got[0] != "s-caps" || got[1] != "s-lower" {
		t.Fatalf("live should collect all session candidates, got %#v", got)
	}
}

func TestFromBodyCandidatesMultipartDuplicateFields(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-1")
	_ = writer.WriteField("model", "gpt-image-1.5")
	_ = writer.WriteField("session", `{"model":"live-a"}`)
	_ = writer.WriteField("session", `{"model":"live-b"}`)
	_ = writer.Close()

	models, sessions := multipartModelCandidates(writer.FormDataContentType(), body.Bytes())
	if len(models) != 2 || models[0] != "gpt-image-1" || models[1] != "gpt-image-1.5" {
		t.Fatalf("duplicate model fields must all be candidates, got %#v", models)
	}
	if len(sessions) != 2 {
		t.Fatalf("duplicate session fields must all be captured, got %#v", sessions)
	}

	got := FromBodyCandidates("", writer.FormDataContentType(), body.Bytes())
	if len(got) != 2 || got[0] != "gpt-image-1" || got[1] != "gpt-image-1.5" {
		t.Fatalf("non-live multipart candidates mismatch, got %#v", got)
	}

	got = FromBodyCandidates("/v1/live", writer.FormDataContentType(), body.Bytes())
	if len(got) != 2 || got[0] != "live-a" || got[1] != "live-b" {
		t.Fatalf("live multipart session candidates mismatch, got %#v", got)
	}
}

func TestFromBodyForRouteReturnsFirstCandidate(t *testing.T) {
	got := FromBodyForRoute("", "application/json", []byte(`{"model":"first","model":"last"}`))
	if got != "first" {
		t.Fatalf("dispatch helper keeps gjson-first semantics, got %q", got)
	}
}
