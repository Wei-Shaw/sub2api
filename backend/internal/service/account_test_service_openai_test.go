//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// --- shared test helpers ---

type queuedHTTPUpstream struct {
	responses []*http.Response
	requests  []*http.Request
	tlsFlags  []bool
}

func (u *queuedHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected Do call")
}

func (u *queuedHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	u.tlsFlags = append(u.tlsFlags, profile != nil)
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("no mocked response")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func newJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// --- test functions ---

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	return c, rec
}

type openAIAccountTestRepo struct {
	mockAccountRepoForGemini
	updatedExtra       map[string]any
	bulkUpdatedIDs     []int64
	bulkUpdatedPayload AccountBulkUpdate
	rateLimitedID      int64
	rateLimitedAt      *time.Time
	clearedErrorID     int64
	setErrorID         int64
	setErrorMsg        string
}

func (r *openAIAccountTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updatedExtra = updates
	return nil
}

func (r *openAIAccountTestRepo) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.bulkUpdatedIDs = append([]int64(nil), ids...)
	r.bulkUpdatedPayload = updates
	return int64(len(ids)), nil
}

func (r *openAIAccountTestRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedID = id
	r.rateLimitedAt = &resetAt
	return nil
}

func (r *openAIAccountTestRepo) ClearError(_ context.Context, id int64) error {
	r.clearedErrorID = id
	return nil
}

func (r *openAIAccountTestRepo) SetError(_ context.Context, id int64, errorMsg string) error {
	r.setErrorID = id
	r.setErrorMsg = errorMsg
	return nil
}

func TestAccountTestService_OpenAISuccessPersistsSnapshotFromHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))
	resp.Header.Set("x-codex-primary-used-percent", "88")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "42")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          89,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.requests[0].Context()))
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 42.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Equal(t, 88.0, repo.updatedExtra["codex_7d_used_percent"])
	require.Contains(t, recorder.Body.String(), "test_complete")
}

func TestAccountTestService_OpenAIOAuthTestNormalizesGPT56Alias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))

	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:          90,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.6", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)

	body, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(body, "model").String())
	require.Equal(t, "hi", gjson.GetBytes(body, "input.0.content.0.text").String())
	require.False(t, gjson.GetBytes(body, "reasoning").Exists())
}

func TestAccountTestService_OpenAIPelicanModeReturnsSVGImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"```svg\\n<svg xmlns=\\\"http://www.w3.org/2000/svg\\\">\"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"<text>pelican on a bicycle</text></svg>\\n```\"}\n\n" +
			"data: {\"type\":\"response.completed\"}\n\n",
	))

	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:          94,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.6-sol", "ignored", AccountTestModePelican)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)

	body, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Equal(t, defaultOpenAIPelicanPrompt, gjson.GetBytes(body, "input.0.content.0.text").String())
	require.Equal(t, "medium", gjson.GetBytes(body, "reasoning.effort").String())
	require.Contains(t, recorder.Body.String(), `"type":"image"`)
	require.Contains(t, recorder.Body.String(), `"mime_type":"image/svg+xml"`)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.NotContains(t, recorder.Body.String(), "```svg")
}

func TestExtractAccountTestSVGRejectsMissingSVG(t *testing.T) {
	_, err := extractAccountTestSVG("I could not generate that image.")
	require.EqualError(t, err, "model response did not contain an SVG")
}

func pelicanTestSSE(t *testing.T, events ...any) string {
	t.Helper()
	var stream strings.Builder
	for _, event := range events {
		data, err := json.Marshal(event)
		require.NoError(t, err)
		stream.WriteString("data: " + string(data) + "\n\n")
	}
	return stream.String()
}

func pelicanTestTerminal(eventType, status string, texts ...string) apicompat.ResponsesStreamEvent {
	response := &apicompat.ResponsesResponse{Status: status}
	if texts != nil {
		content := make([]apicompat.ResponsesContentPart, 0, len(texts))
		for _, text := range texts {
			content = append(content, apicompat.ResponsesContentPart{Type: "output_text", Text: text})
		}
		response.Output = []apicompat.ResponsesOutput{{Type: "message", Role: "assistant", Content: content}}
	}
	return apicompat.ResponsesStreamEvent{Type: eventType, Response: response}
}

func requirePelicanTestSVG(t *testing.T, recorder *httptest.ResponseRecorder, svg string) {
	t.Helper()
	images, completions := 0, 0
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event TestEvent
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		require.NotEqual(t, "error", event.Type)
		switch event.Type {
		case "image":
			images++
			require.Equal(t, "image/svg+xml", event.MimeType)
			require.True(t, strings.HasPrefix(event.ImageURL, "data:image/svg+xml;base64,"))
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(event.ImageURL, "data:image/svg+xml;base64,"))
			require.NoError(t, err)
			require.Equal(t, svg, string(decoded))
		case "test_complete":
			completions++
			require.True(t, event.Success)
			require.Equal(t, 1, images, "image must be validated before success")
		}
	}
	require.Equal(t, 1, images)
	require.Equal(t, 1, completions)
}

func TestAccountTestService_OpenAIPelicanStreamTextRecovery(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg"><svg><text>pelican</text></svg><path d="M0 0"/></svg>`
	delta := func(text string) apicompat.ResponsesStreamEvent {
		return apicompat.ResponsesStreamEvent{Type: "response.output_text.delta", Delta: text}
	}
	done := func(text string) apicompat.ResponsesStreamEvent {
		return apicompat.ResponsesStreamEvent{Type: "response.output_text.done", Text: text}
	}
	completed := apicompat.ResponsesStreamEvent{Type: "response.completed"}
	for _, tt := range []struct {
		name   string
		events []any
	}{
		{"terminal only", []any{pelicanTestTerminal("response.completed", "completed", svg)}},
		{"done only then completion", []any{done(svg), completed}},
		{"partial delta then done", []any{delta("<svg"), done(svg), completed}},
		{"partial delta then terminal", []any{delta("<svg"), pelicanTestTerminal("response.completed", "completed", svg)}},
		{"deltas only", []any{delta(svg[:20]), delta(svg[20:]), completed}},
		{"done without text retains deltas", []any{delta(svg), map[string]any{"type": "response.output_text.done"}, completed}},
		{"deltas done and terminal", []any{delta(svg[:20]), delta(svg[20:]), done(svg), pelicanTestTerminal("response.completed", "completed", svg)}},
		{"done and terminal", []any{done(svg), pelicanTestTerminal("response.completed", "completed", svg)}},
		{"terminal corrects prior valid text", []any{delta("<svg><text>stale</text></svg>"), pelicanTestTerminal("response.completed", "completed", svg)}},
		{"terminal differently indexed", []any{
			apicompat.ResponsesStreamEvent{Type: "response.output_text.delta", OutputIndex: 5, Delta: "<svg"},
			pelicanTestTerminal("response.completed", "completed", svg),
		}},
		{"terminal multiple parts", []any{pelicanTestTerminal("response.completed", "completed", svg[:20], svg[20:])}},
		{"done alias", []any{pelicanTestTerminal("response.done", "completed", svg)}},
		{"done alias without status", []any{delta(svg), apicompat.ResponsesStreamEvent{Type: "response.done"}}},
		{"per-part recovery and ordering", []any{
			apicompat.ResponsesStreamEvent{Type: "response.output_text.delta", ContentIndex: 1, Delta: svg[20:]},
			delta(svg[:10]),
			done(svg[:20]),
			apicompat.ResponsesStreamEvent{Type: "response.output_text.done", ContentIndex: 1, Text: svg[20:]},
			completed,
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, trailingNewline := range []bool{true, false} {
				t.Run(fmt.Sprintf("trailing_newline=%t", trailingNewline), func(t *testing.T) {
					stream := pelicanTestSSE(t, tt.events...)
					if !trailingNewline {
						stream = strings.TrimRight(stream, "\n")
					}
					c, recorder := newTestContext()
					require.NoError(t, (&AccountTestService{}).processOpenAIPelicanStream(c, strings.NewReader(stream)))
					requirePelicanTestSVG(t, recorder, svg)
				})
			}
		})
	}
}

func TestAccountTestService_OpenAIPelicanStreamRejectsUnsuccessfulOutput(t *testing.T) {
	const svg = "<svg><text>pelican</text></svg>"
	delta := apicompat.ResponsesStreamEvent{Type: "response.output_text.delta", Delta: svg}
	incompleteMessage := pelicanTestTerminal("response.completed", "completed", svg)
	incompleteMessage.Response.Output[0].Status = "incomplete"
	for _, tt := range []struct {
		name   string
		stream string
		error  string
	}{
		{"EOF after deltas", pelicanTestSSE(t, delta), "before response.completed"},
		{"DONE after deltas", pelicanTestSSE(t, delta) + "data: [DONE]", "before response.completed"},
		{"EOF after text done", pelicanTestSSE(t, apicompat.ResponsesStreamEvent{Type: "response.output_text.done", Text: svg}), "before response.completed"},
		{"incomplete", pelicanTestSSE(t, delta, pelicanTestTerminal("response.incomplete", "incomplete", svg)), "incomplete"},
		{"failed", pelicanTestSSE(t, delta, pelicanTestTerminal("response.failed", "failed", svg)), "failed"},
		{"done with incomplete status", pelicanTestSSE(t, delta, pelicanTestTerminal("response.done", "incomplete", svg)), "did not complete"},
		{"completed with failed status", pelicanTestSSE(t, delta, pelicanTestTerminal("response.completed", "failed", svg)), "did not complete"},
		{"completed with in-progress status", pelicanTestSSE(t, delta, pelicanTestTerminal("response.completed", "in_progress", svg)), "did not complete"},
		{"incomplete message", pelicanTestSSE(t, delta, incompleteMessage), "message did not complete"},
		{"incomplete details", pelicanTestSSE(t, delta) + `data: {"type":"response.completed","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`, "did not complete"},
		{"terminal error", pelicanTestSSE(t, delta) + `data: {"type":"response.done","response":{"error":{"message":"upstream failed"}}}`, "upstream failed"},
		{"nested error", pelicanTestSSE(t, delta) + `data: {"type":"error","error":{"message":"upstream failed"}}`, "upstream failed"},
		{"top-level error", pelicanTestSSE(t, delta) + `data: {"type":"error","message":"upstream failed"}`, "upstream failed"},
		{"malformed JSON", pelicanTestSSE(t, delta) + "data: not-json\n\n", "expected JSON data"},
		{"truncated terminal", pelicanTestSSE(t, delta) + `data: {"type":"response.completed","response":`, "expected JSON data"},
		{"malformed SVG", pelicanTestSSE(t, pelicanTestTerminal("response.completed", "completed", "<svg><g></svg>")), "invalid SVG"},
		{"incomplete SVG", pelicanTestSSE(t, pelicanTestTerminal("response.completed", "completed", "<svg><svg></svg>")), "incomplete SVG"},
		{"final text overrides old SVG", pelicanTestSSE(t, delta, pelicanTestTerminal("response.completed", "completed", "Cannot draw")), "did not contain an SVG"},
		{"empty done text overrides old SVG", pelicanTestSSE(t, delta, apicompat.ResponsesStreamEvent{Type: "response.output_text.done"}, apicompat.ResponsesStreamEvent{Type: "response.completed"}), "did not contain an SVG"},
		{"empty final output overrides old SVG", pelicanTestSSE(t, delta) + `data: {"type":"response.completed","response":{"status":"completed","output":[]}}`, "did not contain an SVG"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newTestContext()
			err := (&AccountTestService{}).processOpenAIPelicanStream(c, strings.NewReader(tt.stream))
			require.ErrorContains(t, err, tt.error)
			require.Contains(t, recorder.Body.String(), `"type":"error"`)
			require.NotContains(t, recorder.Body.String(), `"success":true`)
			require.NotContains(t, recorder.Body.String(), `"type":"image"`)
		})
	}
}

func TestAccountTestPelicanTextReplacesDoneWithoutDuplication(t *testing.T) {
	var text accountTestPelicanText
	require.NoError(t, text.update([2]int{}, "<svg>", false))
	require.NoError(t, text.update([2]int{}, "<text>pel", false))
	require.NoError(t, text.update([2]int{}, "<svg><text>pelican</text>", true))
	require.NoError(t, text.update([2]int{0, 1}, "</svg>", false))
	require.NoError(t, text.update([2]int{0, 1}, "</svg>", true))
	require.Equal(t, "<svg><text>pelican</text></svg>", text.String())
	require.Equal(t, len(text.String()), text.size)
	require.NoError(t, text.update([2]int{}, "", true))
	require.Equal(t, "</svg>", text.String())
}

func TestExtractAccountTestSVGValidation(t *testing.T) {
	const simple = `<svg xmlns="http://www.w3.org/2000/svg"><text>pelican &amp; bicycle</text></svg>`
	const nested = `<svg><svg><svg/></svg><text>after nested SVG</text></svg>`
	const quoted = `<svg><text title="not a > tag"><![CDATA[<svg></svg> & prose]]></text><!-- </svg> --></svg>`
	for _, tt := range []struct {
		name string
		text string
		want string
	}{
		{"plain", simple, simple},
		{"fenced prose is not XML", "Here is SVG, 1 < 2 & ready:\n```svg\n" + simple + "\n```\nDone & enjoy!", simple},
		{"nested", nested, nested},
		{"comments CDATA and attributes", quoted, quoted},
		{"ignore surrounding comments and CDATA", "<!-- <svg>fake</svg> -->\n<![CDATA[<svg>fake</svg>]]>\n" + simple, simple},
		{"skip false prefix", "<svgfoo>not SVG</svgfoo>\n" + simple, simple},
		{"XML declaration outside root", "<?xml version=\"1.0\"?>\n" + simple, simple},
		{"self-closing root", "<svg /> trailing prose", "<svg />"},
		{"multiline root", "<svg\n><text>pelican</text></svg \n>", "<svg\n><text>pelican</text></svg \n>"},
		{"only first root", simple + "\n<svg>second</svg>", simple},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractAccountTestSVG(tt.text)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
	for _, text := range []string{
		"<svgfoo></svg>", "<svg:foo></svg:foo>", "<svg-foo></svg-foo>",
		"<svg", "<svg><svg></svg>", "<svg><g></svg>", "<svg></g></svg>",
		"<svg><text>a & b</text></svg>", `<svg width="1" width="2"></svg>`,
		"<svg><text title=unquoted/></svg>", `<svg xmlns="urn:not-svg"></svg>`,
		"<svg><!-- incomplete</svg>", "<svg><![CDATA[unterminated</svg>",
		"<svg><!DOCTYPE svg></svg>", `<!-- <svg/> -->`, "<![CDATA[<svg/>]]>",
		`<svg><?xml version="1.0"?></svg>`,
	} {
		t.Run(text, func(t *testing.T) {
			got, err := extractAccountTestSVG(text)
			require.Error(t, err)
			require.Empty(t, got)
		})
	}
}

func TestExtractAccountTestSVGSizeLimit(t *testing.T) {
	svg := "<svg>" + strings.Repeat(" ", maxAccountTestSVGBytes-len("<svg></svg>")) + "</svg>"
	got, err := extractAccountTestSVG("```svg\n" + svg + "\n``` extra text")
	require.NoError(t, err)
	require.Equal(t, svg, got)
	for _, oversized := range []string{
		strings.Replace(svg, "</svg>", " </svg>", 1),
		"<svg><!--" + strings.Repeat("a", maxAccountTestSVGBytes) + "--></svg>",
		`<svg data-long="` + strings.Repeat("a", maxAccountTestSVGBytes) + `"/>`,
	} {
		_, err := extractAccountTestSVG(oversized)
		require.ErrorContains(t, err, "2 MiB limit")
	}
}

func TestAccountTestService_OpenAIPelicanStreamBounds(t *testing.T) {
	for _, chat := range []bool{false, true} {
		t.Run(fmt.Sprintf("chat=%t", chat), func(t *testing.T) {
			process := (&AccountTestService{}).processOpenAIPelicanStream
			delta := func(text string) string {
				return pelicanTestSSE(t, apicompat.ResponsesStreamEvent{Type: "response.output_text.delta", Delta: text})
			}
			completed := pelicanTestSSE(t, apicompat.ResponsesStreamEvent{Type: "response.completed"})
			if chat {
				process = (&AccountTestService{}).processOpenAIPelicanChatCompletionsStream
				delta = func(text string) string {
					return pelicanTestSSE(t, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": text}}}})
				}
				completed = "data: [DONE]"
			}
			t.Run("accumulated text", func(t *testing.T) {
				c, recorder := newTestContext()
				chunk := delta(strings.Repeat(" ", maxAccountTestPelicanTextBytes/2))
				err := process(c, strings.NewReader(chunk+chunk+delta("overflow")+completed))
				require.ErrorContains(t, err, "4 MiB limit")
				require.NotContains(t, recorder.Body.String(), `"success":true`)
			})
			t.Run("oversized line", func(t *testing.T) {
				c, recorder := newTestContext()
				err := process(c, strings.NewReader("data: "+strings.Repeat(" ", maxAccountTestPelicanLineBytes)))
				require.ErrorContains(t, err, "token too long")
				require.NotContains(t, recorder.Body.String(), `"success":true`)
			})
			t.Run("SVG exceeds limit", func(t *testing.T) {
				c, recorder := newTestContext()
				err := process(c, strings.NewReader(delta("<svg>"+strings.Repeat(" ", maxAccountTestSVGBytes)+"</svg>")+completed))
				require.ErrorContains(t, err, "2 MiB limit")
				require.NotContains(t, recorder.Body.String(), `"success":true`)
				require.NotContains(t, recorder.Body.String(), `"type":"image"`)
			})
			t.Run("large valid SVG", func(t *testing.T) {
				c, recorder := newTestContext()
				svg := "<svg>" + strings.Repeat(" ", maxAccountTestSVGBytes-len("<svg></svg>")) + "</svg>"
				require.NoError(t, process(c, strings.NewReader(delta("```svg\n"+svg+"\n```")+completed)))
				requirePelicanTestSVG(t, recorder, svg)
			})
		})
	}
	t.Run("text parts", func(t *testing.T) {
		var output accountTestPelicanText
		for i := 0; i < maxAccountTestPelicanTextParts; i++ {
			require.NoError(t, output.update([2]int{i, 0}, "x", false))
		}
		require.ErrorContains(t, output.update([2]int{maxAccountTestPelicanTextParts, 0}, "x", false), "too many text parts")
	})
	for _, event := range []apicompat.ResponsesStreamEvent{
		{Type: "response.output_text.done", Text: strings.Repeat(" ", maxAccountTestPelicanTextBytes+1)},
		pelicanTestTerminal("response.completed", "completed", strings.Repeat(" ", maxAccountTestPelicanTextBytes+1)),
	} {
		t.Run(event.Type+" text limit", func(t *testing.T) {
			c, recorder := newTestContext()
			err := (&AccountTestService{}).processOpenAIPelicanStream(c, strings.NewReader(pelicanTestSSE(t, event)))
			require.ErrorContains(t, err, "4 MiB limit")
			require.NotContains(t, recorder.Body.String(), `"success":true`)
		})
	}
}

type pelicanTestReadError struct{}

func (pelicanTestReadError) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestAccountTestService_OpenAIPelicanStreamReadErrors(t *testing.T) {
	for _, chat := range []bool{false, true} {
		t.Run(fmt.Sprintf("chat=%t", chat), func(t *testing.T) {
			c, recorder := newTestContext()
			process := (&AccountTestService{}).processOpenAIPelicanStream
			if chat {
				process = (&AccountTestService{}).processOpenAIPelicanChatCompletionsStream
			}
			err := process(c, pelicanTestReadError{})
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "stream read error")
			require.NotContains(t, recorder.Body.String(), `"success":true`)
		})
	}
}

func TestAccountTestService_OpenAIPelicanChatCompletionsTerminal(t *testing.T) {
	const svg = "<svg><svg/><text>pelican</text></svg>"
	delta := map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": svg}}}}
	for _, tt := range []struct {
		name   string
		ending string
		error  string
	}{
		{"DONE without newline", "data: [DONE]", ""},
		{"stop without newline", `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`, ""},
		{"EOF", "", "before [DONE]"},
		{"length", `data: {"choices":[{"finish_reason":"length"}]}` + "\n\ndata: [DONE]", "did not complete"},
		{"filtered", `data: {"choices":[{"finish_reason":"content_filter"}]}`, "did not complete"},
		{"error", `data: {"error":{"message":"upstream failed"}}`, "upstream failed"},
		{"malformed JSON", "data: not-json", "expected JSON data"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newTestContext()
			err := (&AccountTestService{}).processOpenAIPelicanChatCompletionsStream(c, strings.NewReader(pelicanTestSSE(t, delta)+tt.ending))
			if tt.error == "" {
				require.NoError(t, err)
				requirePelicanTestSVG(t, recorder, svg)
			} else {
				require.ErrorContains(t, err, tt.error)
				require.NotContains(t, recorder.Body.String(), `"success":true`)
				require.NotContains(t, recorder.Body.String(), `"type":"image"`)
			}
		})
	}
}

func TestAccountTestService_OpenAIPelicanChatCompletionsOpenCodeSession(t *testing.T) {
	const svg = "<svg><text>pelican</text></svg>"
	for _, tt := range []struct {
		name     string
		override string
		inbound  string
		want     string
	}{
		{"generated", "", "", ""},
		{"account override", "configured-session", "", "configured-session"},
		{"inbound", "", "caller-session", "caller-session"},
		{"inbound beats override", "configured-session", "caller-session", "caller-session"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newTestContext()
			c.Request.Header.Set(openCodeSessionHeader, tt.inbound)
			account := &Account{
				ID: 96, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{
					"api_key":  "sk-opencode-test",
					"base_url": "https://opencode.ai/zen/go/v1",
				},
				Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
			}
			if tt.override != "" {
				account.Credentials[credKeyHeaderOverrideEnabled] = true
				account.Credentials[credKeyHeaderOverrides] = map[string]any{"x-opencode-session": tt.override}
			}
			stream := pelicanTestSSE(t, map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": svg}, "finish_reason": "stop"}},
			}) + "data: [DONE]"
			upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(http.StatusOK, stream)}}
			svc := &AccountTestService{
				httpUpstream: upstream,
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
			}
			require.NoError(t, svc.testOpenAIAccountConnection(c, account, "gpt-5.4", "", AccountTestModePelican))
			require.Len(t, upstream.requests, 1)
			req := upstream.requests[0]
			require.Equal(t, "https://opencode.ai/zen/go/v1/chat/completions", req.URL.String())
			want := tt.want
			if want == "" {
				want = req.Header.Get(openCodeSessionHeader)
				_, err := uuid.Parse(want)
				require.NoError(t, err, "official OpenCode Go requires a generated session when none is provided")
			}
			requireSingleOpenCodeSessionHeader(t, req.Header, want)
			payload, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, defaultOpenAIPelicanPrompt, gjson.GetBytes(payload, "messages.0.content").String())
			require.Equal(t, accountTestReasoningEffort, gjson.GetBytes(payload, "reasoning_effort").String())
			requirePelicanTestSVG(t, recorder, svg)
		})
	}
}

func TestCreateOpenAICompactProbePayloadHasNoReasoning(t *testing.T) {
	payload := createOpenAICompactProbePayload("gpt-5.6-sol", true)
	_, exists := payload["reasoning"]
	require.False(t, exists)
}

func TestAccountTestService_OpenAIShadowUsesParentCredentialsAndShadowModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))

	parentID := int64(100)
	parent := &Account{
		ID:       parentID,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":       "parent-token",
			"chatgpt_account_id": "org-parent",
		},
	}
	shadow := &Account{
		ID:              200,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Concurrency:     2,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5.3-codex-spark": "gpt-5.3-codex-spark",
			},
		},
	}

	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{
				parentID: parent,
				200:      shadow,
			},
		},
	}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}

	err := svc.TestAccountConnection(ctx, shadow.ID, "gpt-5.3-codex-spark", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "Bearer parent-token", req.Header.Get("Authorization"))
	require.Equal(t, "org-parent", req.Header.Get("chatgpt-account-id"))
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.3-codex-spark", gjson.GetBytes(body, "model").String())
	require.Contains(t, recorder.Body.String(), `"success":true`)
}

func TestAccountTestService_OpenAIStreamEOFBeforeCompletedFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.output_text.delta","delta":"hi"}

`))

	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:          90,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), "response.completed")
	require.NotContains(t, recorder.Body.String(), `"success":true`)
}

func TestAccountTestService_DeepSeekCustomBaseURLUsesV1ResponsesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          91,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"base_url":     "https://relay.example.com/v1",
			"api_protocol": APIProtocolResponses,
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: true,
		},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://relay.example.com/v1/responses", upstream.requests[0].URL.String())
}

func TestAccountTestService_DeepSeekResponsesRoutesToOpenAIProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          93,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"base_url":     "https://relay.example.com/v1",
			"api_protocol": APIProtocolResponses,
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: true,
		},
	}
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{93: account},
		},
	}
	svc.accountRepo = repo

	err := svc.TestAccountConnection(ctx, account.ID, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://relay.example.com/v1/responses", upstream.requests[0].URL.String())
}

func TestAccountTestService_DeepSeekDefaultBaseURLUsesNativeResponsesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          92,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-test",
			"api_protocol": APIProtocolResponses,
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesSupported: true,
		},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.deepseek.com/responses", upstream.requests[0].URL.String())
}

func TestAccountTestService_OpenAI429PersistsSnapshotAndRateLimitState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached","resets_at":1777283883}}`)
	resp.Header.Set("x-codex-primary-used-percent", "100")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "100")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          88,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusError,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 100.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Equal(t, account.ID, repo.rateLimitedID)
	require.NotNil(t, repo.rateLimitedAt)
	require.Equal(t, account.ID, repo.clearedErrorID)
	require.Equal(t, StatusActive, account.Status)
	require.Empty(t, account.ErrorMessage)
	require.NotNil(t, account.RateLimitResetAt)
}

func TestAccountTestService_OpenAI429BodyOnlyPersistsRateLimitAndClearsStaleError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached","resets_at":"1777283883"}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:           77,
		Platform:     PlatformOpenAI,
		Type:         AccountTypeOAuth,
		Status:       StatusError,
		ErrorMessage: "Access forbidden (403): account may be suspended or lack permissions",
		Concurrency:  1,
		Credentials:  map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Equal(t, account.ID, repo.rateLimitedID)
	require.NotNil(t, repo.rateLimitedAt)
	require.Equal(t, account.ID, repo.clearedErrorID)
	require.Equal(t, StatusActive, account.Status)
	require.Empty(t, account.ErrorMessage)
	require.NotNil(t, account.RateLimitResetAt)
	require.Empty(t, repo.updatedExtra)
}

func TestAccountTestService_OpenAI429SyncsObservedPlanType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached","plan_type":"free","resets_at":1777283883}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          81,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "plan_type": "plus"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Equal(t, []int64{account.ID}, repo.bulkUpdatedIDs)
	require.Equal(t, "free", repo.bulkUpdatedPayload.Credentials["plan_type"])
	require.Equal(t, "free", account.Credentials["plan_type"])
	require.Equal(t, account.ID, repo.rateLimitedID)
	require.NotNil(t, account.RateLimitResetAt)
}

func TestAccountTestService_OpenAI429ActiveAccountDoesNotClearError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached","resets_in_seconds":3600}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          78,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Equal(t, account.ID, repo.rateLimitedID)
	require.NotNil(t, repo.rateLimitedAt)
	require.Zero(t, repo.clearedErrorID)
	require.Equal(t, StatusActive, account.Status)
	require.NotNil(t, account.RateLimitResetAt)
}

func TestAccountTestService_OpenAI429WithoutResetSignalDoesNotMutateRuntimeState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached"}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:           79,
		Platform:     PlatformOpenAI,
		Type:         AccountTypeOAuth,
		Status:       StatusError,
		ErrorMessage: "stale 403",
		Concurrency:  1,
		Credentials:  map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Zero(t, repo.rateLimitedID)
	require.Nil(t, repo.rateLimitedAt)
	require.Zero(t, repo.clearedErrorID)
	require.Equal(t, StatusError, account.Status)
	require.Equal(t, "stale 403", account.ErrorMessage)
	require.Nil(t, account.RateLimitResetAt)
}

func TestAccountTestService_OpenAI401SetsPermanentErrorOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusUnauthorized, `{"error":"bad token"}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          80,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Equal(t, account.ID, repo.setErrorID)
	require.Contains(t, repo.setErrorMsg, "Authentication failed (401)")
	require.Zero(t, repo.rateLimitedID)
	require.Zero(t, repo.clearedErrorID)
	require.Nil(t, account.RateLimitResetAt)
}

func TestAccountTestService_OpenAIAPIKeyResponsesUsesCodexProbeHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n"))
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          95,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example/v1",
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: true},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.requests[0]
	require.Equal(t, "https://compat-upstream.example/v1/responses", req.URL.String())
	requireOpenAICodexProbeHeaders(t, req.Header)
}

func TestAccountTestService_OpenAIAPIKeyResponsesUnsupportedUsesChatCompletionsPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          91,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example/v1",
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "hello", "")
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, "https://compat-upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	body := recorder.Body.String()
	require.Contains(t, body, "pong")
	require.Contains(t, body, "已通过 /v1/chat/completions 验证")
	require.Contains(t, body, `"success":true`)
	require.NotContains(t, body, "当前测试接口仅支持 Responses API 路径")
}

func TestAccountTestService_OpenAIChatCompletionsPathReturns4xx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	upstream := &httpUpstreamRecorder{resp: newJSONResponse(http.StatusBadRequest, `{"error":{"message":"bad request"}}`)}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          92,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example",
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Equal(t, "https://compat-upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Contains(t, err.Error(), "Chat Completions API (/v1/chat/completions) returned 400")
	require.Contains(t, recorder.Body.String(), "/v1/chat/completions")
	require.NotContains(t, recorder.Body.String(), `"success":true`)
}

func TestAccountTestService_OpenAIChatCompletionsPathTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	upstream := &httpUpstreamRecorder{err: context.DeadlineExceeded}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          93,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example",
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Equal(t, "https://compat-upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Contains(t, err.Error(), "Chat Completions API (/v1/chat/completions) request failed")
	require.Contains(t, err.Error(), context.DeadlineExceeded.Error())
	require.Contains(t, recorder.Body.String(), "/v1/chat/completions")
	require.NotContains(t, recorder.Body.String(), `"success":true`)
}

func TestAccountTestService_OpenAIChatCompletionsPathRejectsNonJSONStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: not-json\n\n")),
	}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          94,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example",
		},
		Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Equal(t, "https://compat-upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Contains(t, err.Error(), "Invalid Chat Completions response from /v1/chat/completions")
	require.Contains(t, recorder.Body.String(), "/v1/chat/completions")
	require.NotContains(t, recorder.Body.String(), `"success":true`)
}
