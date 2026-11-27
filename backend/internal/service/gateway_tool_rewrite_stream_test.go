package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// toolNameSplitToolsBody 让 buildDynamicToolMap 把 read 映射成 analyze_rea01：
// 6 个工具触发动态映射，工具名与顺序固定 buildDynamicToolMap 的种子。
const toolNameSplitToolsBody = `{"tools":[{"name":"alpha53","input_schema":{}},{"name":"read","input_schema":{}},{"name":"write_file","input_schema":{}},{"name":"bash","input_schema":{}},{"name":"edit","input_schema":{}},{"name":"grep","input_schema":{}}]}`

const toolNameSplitFake = "analyze_rea01"

func newToolNameSplitGatewayService() *GatewayService {
	return &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		rateLimitService: &RateLimitService{},
	}
}

// newToolNameSplitContext 返回带请求侧工具名映射（read → analyze_rea01）的测试上下文。
func newToolNameSplitContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder, *ToolNameRewrite) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	rw := buildToolNameRewriteFromBody([]byte(toolNameSplitToolsBody))
	require.NotNil(t, rw)
	require.Equal(t, toolNameSplitFake, rw.Forward["read"])
	c.Set(toolNameRewriteKey, rw)
	return c, rec, rw
}

// tnSSEEvent 把一个 data JSON 包成 SSE 事件块，事件名取 type 字段。
func tnSSEEvent(dataJSON string) string {
	return "event: " + gjson.Get(dataJSON, "type").String() + "\ndata: " + dataJSON + "\n\n"
}

// tnSSEDelta 构造 content_block_delta 的 data JSON。
func tnSSEDelta(t *testing.T, idx int, deltaType, field, payload string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": idx,
		"delta": map[string]any{"type": deltaType, field: payload},
	})
	require.NoError(t, err)
	return string(b)
}

// tnClientJoined 解析客户端收到的 SSE 体，把 path 命中的字符串按出现顺序拼接。
func tnClientJoined(body, path string) string {
	var b strings.Builder
	for _, block := range strings.Split(body, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			rest, ok := strings.CutPrefix(strings.TrimSpace(line), "data:")
			if !ok {
				continue
			}
			data := strings.TrimSpace(rest)
			if !gjson.Valid(data) {
				continue
			}
			if v := gjson.Get(data, path); v.Type == gjson.String {
				_, _ = b.WriteString(v.String())
			}
		}
	}
	return b.String()
}

func TestHandleStreamingResponse_RestoresToolNameSplitAcrossTextDeltas(t *testing.T) {
	svc := newToolNameSplitGatewayService()
	c, rec, _ := newToolNameSplitContext(t)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`))
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`))
		_, _ = io.WriteString(pw, tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use analyze_r")))
		_, _ = io.WriteString(pw, tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "ea01 now")))
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"content_block_stop","index":0}`))
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"message_stop"}`))
	}()

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)

	require.Equal(t, "use read now", tnClientJoined(rec.Body.String(), "delta.text"))
	require.NotContains(t, rec.Body.String(), "analyze_")
}

func TestHandleStreamingResponse_RestoresToolNameSplitAcrossInputJSONDeltas(t *testing.T) {
	svc := newToolNameSplitGatewayService()
	c, rec, _ := newToolNameSplitContext(t)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`))
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_1","name":"analyze_rea01","input":{}}}`))
		_, _ = io.WriteString(pw, tnSSEEvent(tnSSEDelta(t, 1, "input_json_delta", "partial_json", `{"file_path":"analyze_r`)))
		_, _ = io.WriteString(pw, tnSSEEvent(tnSSEDelta(t, 1, "input_json_delta", "partial_json", `ea01"}`)))
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"content_block_stop","index":1}`))
		_, _ = io.WriteString(pw, tnSSEEvent(`{"type":"message_stop"}`))
	}()

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)

	require.Equal(t, `{"file_path":"read"}`, tnClientJoined(rec.Body.String(), "delta.partial_json"))
	require.NotContains(t, rec.Body.String(), "analyze_")
}

func TestHandleCCStreamingFromAnthropic_RestoresToolNameSplitAcrossTextDeltas(t *testing.T) {
	c, rec, _ := newToolNameSplitContext(t)
	stream := strings.Join([]string{
		tnSSEEvent(`{"type":"message_start","message":{"id":"msg_3","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`),
		tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use analyze_r")),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "ea01 now")),
		tnSSEEvent(`{"type":"content_block_stop","index":0}`),
		tnSSEEvent(`{"type":"message_stop"}`),
	}, "")
	resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}

	svc := &GatewayService{}
	_, err := svc.handleCCStreamingFromAnthropic(resp, c, "model", "model", nil, time.Now())
	require.NoError(t, err)

	require.Equal(t, "use read now", tnClientJoined(rec.Body.String(), "choices.0.delta.content"))
	require.NotContains(t, rec.Body.String(), "analyze_")
}

func TestHandleResponsesStreamingResponse_RestoresToolNameSplitAcrossTextDeltas(t *testing.T) {
	c, rec, _ := newToolNameSplitContext(t)
	stream := strings.Join([]string{
		tnSSEEvent(`{"type":"message_start","message":{"id":"msg_4","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`),
		tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use analyze_r")),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "ea01 now")),
		tnSSEEvent(`{"type":"content_block_stop","index":0}`),
		tnSSEEvent(`{"type":"message_stop"}`),
	}, "")
	resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}

	svc := &GatewayService{}
	_, err := svc.handleResponsesStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
	require.NoError(t, err)

	require.Equal(t, "use read now", tnClientJoined(rec.Body.String(), "delta"))
	require.NotContains(t, rec.Body.String(), "analyze_")
}

// --- 还原器单测 ---

const toolNameStaticToolsBody = `{"tools":[{"name":"sessions_x","input_schema":{}},{"name":"sessions_xy","input_schema":{}},{"name":"session_x","input_schema":{}},{"name":"session_xy","input_schema":{}}]}`

func tnDynamicRewrite(t *testing.T) *ToolNameRewrite {
	t.Helper()
	rw := buildToolNameRewriteFromBody([]byte(toolNameSplitToolsBody))
	require.NotNil(t, rw)
	require.Equal(t, toolNameSplitFake, rw.Forward["read"])
	return rw
}

func tnStaticRewrite(t *testing.T) *ToolNameRewrite {
	t.Helper()
	rw := buildToolNameRewriteFromBody([]byte(toolNameStaticToolsBody))
	require.NotNil(t, rw)
	require.Equal(t, "cc_sess_xy", rw.Forward["sessions_xy"])
	require.Equal(t, "cc_sess_x", rw.Forward["sessions_x"])
	return rw
}

// tnPayloadOf 取出 delta 的字符串载荷；text 与 partial_json 都认。
func tnPayloadOf(data []byte) string {
	for _, field := range []string{"text", "partial_json"} {
		if v := gjson.GetBytes(data, "delta."+field); v.Type == gjson.String {
			return v.String()
		}
	}
	return ""
}

// tnRestore 按生产调用点的语义调用还原器：nil 还原器与 nil data 都在这里挡住。
func tnRestore(r *toolNameStreamRestorer, data []byte) [][]byte {
	if r == nil {
		if data == nil {
			return nil
		}
		return [][]byte{data}
	}
	if data == nil {
		return r.flush()
	}
	var out [2][]byte
	return out[:r.restoreInto(data, &out)]
}

// tnRun 把上游事件喂给还原器，按顺序返回发出的全部事件 JSON。
func tnRun(r *toolNameStreamRestorer, upstream []string) []string {
	var out []string
	for _, data := range upstream {
		for _, ev := range tnRestore(r, []byte(data)) {
			out = append(out, string(ev))
		}
	}
	return out
}

// tnCheckInvariant 断言客户端按 index 拼接的载荷，在每个 stop 到达时与全部原始载荷
// 一次性还原的结果一致；返回拼接结果供调用方继续断言。
func tnCheckInvariant(t *testing.T, r *toolNameStreamRestorer, rw *ToolNameRewrite, upstream []string, want map[int]string) map[int]string {
	t.Helper()
	got := map[int]string{}
	for _, data := range upstream {
		for _, ev := range tnRestore(r, []byte(data)) {
			idx := int(gjson.GetBytes(ev, "index").Int())
			switch gjson.GetBytes(ev, "type").String() {
			case "content_block_delta":
				payload := tnPayloadOf(ev)
				if payload == "" && tnIsTextDelta(ev) {
					require.NotEmpty(t, payload, "empty delta must not be emitted: %s", ev)
				}
				if !tnIsTextDelta(ev) {
					// 非文本 delta 原样放行，不参与拼接。
					break
				}
				require.True(t, utf8.ValidString(payload), "payload must stay valid UTF-8: %q", payload)
				got[idx] += payload
			case "content_block_stop":
				require.Equal(t, want[idx], got[idx], "payload for index %d when its stop arrives", idx)
			}
		}
	}
	for _, ev := range tnRestore(r, nil) {
		got[int(gjson.GetBytes(ev, "index").Int())] += tnPayloadOf(ev)
	}
	require.Empty(t, r.carry)
	for idx, w := range want {
		require.Equal(t, w, got[idx], "payload for index %d after flush", idx)
	}
	return got
}

// tnIsTextDelta 指示事件是否为承载文本载荷的 delta（text_delta / input_json_delta）。
func tnIsTextDelta(ev []byte) bool {
	switch gjson.GetBytes(ev, "delta.type").String() {
	case "text_delta", "input_json_delta":
		return true
	}
	return false
}

// tnWant 返回按 index 拼接原始载荷后一次性还原的结果，作为不变式的判据。
func tnWant(rw *ToolNameRewrite, raw map[int]string) map[int]string {
	want := map[int]string{}
	for idx, s := range raw {
		want[idx] = string(restoreToolNamesInBytes([]byte(s), rw))
	}
	return want
}

// tnDeltas 把 full 按 cuts 切开，包成同 index、同类型的 content_block_delta。
func tnDeltas(t *testing.T, idx int, deltaType, field, full string, cuts []int) []string {
	t.Helper()
	var out []string
	prev := 0
	for _, cut := range append(cuts, len(full)) {
		out = append(out, string(tnDeltaJSON(t, idx, deltaType, field, full[prev:cut])))
		prev = cut
	}
	return out
}

func tnDeltaJSON(t *testing.T, idx int, deltaType, field, payload string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type": "content_block_delta", "index": idx,
		"delta": map[string]any{"type": deltaType, field: payload},
	})
	require.NoError(t, err)
	return b
}

func tnStopJSON(idx int) string {
	return fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, idx)
}

func tnCutPositions(n int) []int {
	var out []int
	for cut := 1; cut < n; cut++ {
		out = append(out, cut)
	}
	return out
}

type tnDeltaKind struct{ deltaType, field string }

var tnKinds = []tnDeltaKind{{"text_delta", "text"}, {"input_json_delta", "partial_json"}}

// 切分点落在假名每个字节位置：动态假名与静态假名、text 与 partial_json。
func TestToolNameStreamRestorer_SplitAtEveryFakeByte(t *testing.T) {
	for _, tc := range []struct {
		name string
		rw   *ToolNameRewrite
		fake string
		real string
	}{
		{"dynamic", tnDynamicRewrite(t), toolNameSplitFake, "read"},
		{"static-prefix-of-fake", tnStaticRewrite(t), "cc_sess_xy", "sessions_xy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full := "use " + tc.fake + " now"
			at := strings.Index(full, tc.fake)
			for _, cut := range tnCutPositions(len(full)) {
				if cut <= at || cut >= at+len(tc.fake) {
					continue
				}
				for _, kind := range tnKinds {
					up := tnDeltas(t, 0, kind.deltaType, kind.field, full, []int{cut})
					up = append(up, tnStopJSON(0))
					r := newToolNameStreamRestorer(tc.rw)
					got := tnCheckInvariant(t, r, tc.rw, up, tnWant(tc.rw, map[int]string{0: full}))
					require.Equal(t, "use "+tc.real+" now", got[0])
					require.NotContains(t, got[0], "cc_ss")
				}
			}
		})
	}
}

func tnEvenCuts(n, parts, offset int) []int {
	step := max(1, (n-2)/(parts-1))
	var cuts []int
	for c := 1 + offset; c < n && len(cuts) < parts-1; c += step {
		cuts = append(cuts, c)
	}
	for len(cuts) < parts-1 {
		cuts = append(cuts, cuts[len(cuts)-1]+1)
	}
	return cuts
}

// 一个假名跨 2 到 5 个 delta。
func TestToolNameStreamRestorer_SplitAcrossManyDeltas(t *testing.T) {
	rw := tnDynamicRewrite(t)
	full := "a " + toolNameSplitFake + " b"
	want := tnWant(rw, map[int]string{0: full})
	for _, kind := range tnKinds {
		for parts := 2; parts <= 5; parts++ {
			for offset := 0; offset < 4; offset++ {
				cuts := tnEvenCuts(len(full), parts, offset)
				up := append(tnDeltas(t, 0, kind.deltaType, kind.field, full, cuts), tnStopJSON(0))
				r := newToolNameStreamRestorer(rw)
				got := tnCheckInvariant(t, r, rw, up, want)
				require.Equal(t, want[0], got[0])
				require.NotContains(t, got[0], "analyze_")
			}
		}
	}
}

// 一个 delta 里多个假名。
func TestToolNameStreamRestorer_MultipleFakesInOneDelta(t *testing.T) {
	rw := tnDynamicRewrite(t)
	full := rw.Forward["read"] + " + " + rw.Forward["write_file"] + " x"
	want := tnWant(rw, map[int]string{0: full})
	for _, cut := range tnCutPositions(len(full)) {
		up := append(tnDeltas(t, 0, "text_delta", "text", full, []int{cut}), tnStopJSON(0))
		r := newToolNameStreamRestorer(rw)
		got := tnCheckInvariant(t, r, rw, up, want)
		require.Equal(t, want[0], got[0])
	}
}

// 两个 index 交错，各自留各自的 carry。
func TestToolNameStreamRestorer_InterleavedIndexes(t *testing.T) {
	rw := tnDynamicRewrite(t)
	text := "zero " + rw.Forward["read"] + " end"
	args := `{"path":"` + rw.Forward["write_file"] + `"}`
	up := []string{
		string(tnDeltaJSON(t, 0, "text_delta", "text", text[:9])),
		string(tnDeltaJSON(t, 1, "input_json_delta", "partial_json", args[:12])),
		string(tnDeltaJSON(t, 0, "text_delta", "text", text[9:])),
		tnStopJSON(0),
		string(tnDeltaJSON(t, 1, "input_json_delta", "partial_json", args[12:])),
		tnStopJSON(1),
	}
	r := newToolNameStreamRestorer(rw)
	got := tnCheckInvariant(t, r, rw, up, tnWant(rw, map[int]string{0: text, 1: args}))
	require.Equal(t, "zero read end", got[0])
	require.Equal(t, `{"path":"write_file"}`, got[1])
	for _, ev := range tnRun(r, nil) {
		_ = ev
	}
}

// 假名前缀后面接别的文字，不成名。
func TestToolNameStreamRestorer_PrefixFollowedByOtherText(t *testing.T) {
	rw := tnDynamicRewrite(t)
	for k := 1; k < len(toolNameSplitFake); k++ {
		full := "see " + toolNameSplitFake[:k] + "zzz more"
		up := append(tnDeltas(t, 0, "text_delta", "text", full, []int{4 + k}), tnStopJSON(0))
		r := newToolNameStreamRestorer(rw)
		got := tnCheckInvariant(t, r, rw, up, tnWant(rw, map[int]string{0: full}))
		require.Equal(t, full, got[0])
	}
}

// 假名前后是多字节 UTF-8，切点遍历整段。
func TestToolNameStreamRestorer_MultibyteAroundFake(t *testing.T) {
	rw := tnDynamicRewrite(t)
	full := "中文" + toolNameSplitFake + "中文"
	want := tnWant(rw, map[int]string{0: full})
	for _, cut := range tnRuneCuts(full) {
		up := append(tnDeltas(t, 0, "text_delta", "text", full, []int{cut}), tnStopJSON(0))
		r := newToolNameStreamRestorer(rw)
		got := tnCheckInvariant(t, r, rw, up, want)
		require.Equal(t, want[0], got[0])
	}
}

// tnRuneCuts 返回 s 的每个 rune 起始位置（不含 0）。
func tnRuneCuts(s string) []int {
	var out []int
	for i := range s {
		if i > 0 {
			out = append(out, i)
		}
	}
	return out
}

// 块尾的假名真前缀整段扣住，只在 content_block_stop 之前以同 index、同类型补发；
// 补发的是扣下的原字节（handler 会再走一次还原，这里断言还原器本身的行为）。
func TestToolNameStreamRestorer_CarryFlushedBeforeStop(t *testing.T) {
	rw := tnStaticRewrite(t)
	for k := 1; k < len("cc_sess_x"); k++ {
		held := "cc_sess_x"[:k]
		prefix := "tail " + held
		up := []string{string(tnDeltaJSON(t, 3, "input_json_delta", "partial_json", prefix)), tnStopJSON(3)}
		got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, map[int]string{3: prefix})
		require.Equal(t, prefix, got[3])

		emitted := tnRun(newToolNameStreamRestorer(rw), up)
		require.Len(t, emitted, 3, "扣住的尾巴不能跟着第一段走")
		require.Equal(t, "tail ", tnPayloadOf([]byte(emitted[0])))
		flushed := emitted[1]
		require.Equal(t, held, tnPayloadOf([]byte(flushed)))
		require.Equal(t, tnStopJSON(3), emitted[2], "carry must be flushed before the stop")
		require.Equal(t, "content_block_delta", gjson.Get(flushed, "type").String())
		require.Equal(t, "input_json_delta", gjson.Get(flushed, "delta.type").String())
		require.Equal(t, int64(3), gjson.Get(flushed, "index").Int())
	}
}

// flush 按 index 升序补发所有残留 carry。
func TestToolNameStreamRestorer_FlushAscending(t *testing.T) {
	rw := tnDynamicRewrite(t)
	prefix := toolNameSplitFake[:4]
	r := newToolNameStreamRestorer(rw)
	for _, idx := range []int{2, 0, 1} {
		tnRestore(r, tnDeltaJSON(t, idx, "text_delta", "text", "x "+prefix))
	}
	out := tnRestore(r, nil)
	require.Len(t, out, 3)
	for i, ev := range out {
		require.Equal(t, int64(i), gjson.GetBytes(ev, "index").Int())
	}
	require.Empty(t, tnRestore(r, nil))
}

// 无映射：构造为 nil，tnRestore 逐字节原样返回。
func TestToolNameStreamRestorer_NoMapping(t *testing.T) {
	require.Nil(t, newToolNameStreamRestorer(nil))
	require.Nil(t, newToolNameStreamRestorer(toolNameRewriteFromContext(toolRewriteCtxStub{})))
	single := &ToolNameRewrite{ReverseOrdered: [][2]string{{"a", "b"}}}
	require.NotNil(t, newToolNameStreamRestorer(single), "静态假前缀不经映射也会被还原，还原器照样要建")

	var nilRestorer *toolNameStreamRestorer
	events := []string{
		string(tnDeltaJSON(t, 0, "text_delta", "text", "use analyze_r")),
		string(tnDeltaJSON(t, 0, "text_delta", "text", "ea01 now")),
		tnStopJSON(0),
	}
	for _, ev := range events {
		out := tnRestore(nilRestorer, []byte(ev))
		require.Len(t, out, 1)
		require.Equal(t, ev, string(out[0]))
	}
	require.Empty(t, tnRestore(nilRestorer, nil))
}

// 已经还原过的文本再走一遍还原器不变（第二次还原幂等）。
func TestToolNameStreamRestorer_SecondPassIsIdempotent(t *testing.T) {
	rw := tnDynamicRewrite(t)
	text := "use " + rw.Forward["read"] + " and " + rw.Forward["write_file"] + " done"
	once := string(restoreToolNamesInBytes([]byte(text), rw))
	twice := string(restoreToolNamesInBytes([]byte(once), rw))
	require.Equal(t, once, twice)
	require.NotContains(t, once, "analyze_")

	want := map[int]string{0: once}
	for _, cut := range tnCutPositions(len(once)) {
		up := append(tnDeltas(t, 0, "text_delta", "text", once, []int{cut}), tnStopJSON(0))
		got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, want)
		require.Equal(t, once, got[0])
	}
}

// thinking_delta 与 signature_delta 等事件原样返回，不经 carry。
func TestToolNameStreamRestorer_NonDeltaEventsUntouched(t *testing.T) {
	rw := tnDynamicRewrite(t)
	r := newToolNameStreamRestorer(rw)
	events := []string{
		`{"type":"message_start","message":{"id":"m1"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		string(tnDeltaJSON(t, 0, "thinking_delta", "thinking", "ends with "+toolNameSplitFake[:6])),
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig=="}}`,
		`{"type":"ping"}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		`{"type":"message_stop"}`,
	}
	for _, ev := range events {
		out := tnRestore(r, []byte(ev))
		require.Len(t, out, 1, ev)
		require.Equal(t, ev, string(out[0]))
	}
	require.Empty(t, r.carry)
	out := tnRestore(r, []byte(tnStopJSON(0)))
	require.Len(t, out, 1, "no synthetic delta without carry")
	require.Equal(t, tnStopJSON(0), string(out[0]))
}

// 改载荷只动 delta 的载荷字段，事件其余字节不变。
func TestToolNameStreamRestorer_OnlyPayloadFieldChanges(t *testing.T) {
	rw := tnDynamicRewrite(t)
	original := "use " + rw.Forward["read"]
	b, err := json.Marshal(map[string]any{
		"type": "content_block_delta", "index": 7, "extra": map[string]any{"a": 1},
		"delta": map[string]any{"type": "text_delta", "text": original},
	})
	require.NoError(t, err)
	out := tnRestore(newToolNameStreamRestorer(rw), b)
	require.Len(t, out, 1)
	require.Equal(t, int64(7), gjson.GetBytes(out[0], "index").Int())
	require.Equal(t, int64(1), gjson.GetBytes(out[0], "extra.a").Int())
	require.Equal(t, "use read", gjson.GetBytes(out[0], "delta.text").String())
	back, err := sjson.SetBytes(out[0], "delta.text", original)
	require.NoError(t, err)
	require.Equal(t, string(b), string(back))
}

// 改写后的载荷重新序列化时只转义 JSON 要求的字节：< > & 与非 ASCII 原样保留，
// 解出的值仍与期望文本相等，事件也仍是合法 JSON。
func TestToolNameStreamRestorer_KeepsHTMLAndUTF8Unescaped(t *testing.T) {
	rw := tnDynamicRewrite(t)
	full := `a<b>c&d"e\f` + "\n" + "g é " + toolNameSplitFake + " end"
	want := `a<b>c&d"e\f` + "\n" + "g é read end"
	at := strings.Index(full, toolNameSplitFake)
	up := append(tnDeltas(t, 0, "text_delta", "text", full, []int{at + 5}), tnStopJSON(0))

	emitted := tnEmitted(rw, up)
	require.Len(t, emitted, 3) // rewritten delta, second delta, stop
	first := emitted[0]
	require.True(t, json.Valid([]byte(first)), "rewritten event must stay valid JSON: %s", first)
	require.Contains(t, first, `<b>c&d`)
	require.Contains(t, first, "é")
	require.Contains(t, first, `\"e\\f`)
	require.NotContains(t, first, `\u003c`)
	require.NotContains(t, first, `\u0026`)
	require.NotContains(t, first, `\u00e9`)
	require.Equal(t, `a<b>c&d"e\f`+"\n"+"g é ", gjson.Get(first, "delta.text").String())
	require.Equal(t, want, tnPayloadOf([]byte(first))+tnPayloadOf([]byte(emitted[1])))
}

func tnStaticContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(toolNameRewriteKey, tnStaticRewrite(t))
	return c, rec
}

type tnErrReader struct {
	data []byte
	err  error
}

func (r *tnErrReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

func (r *tnErrReader) Close() error { return nil }

// tnHeldStream 造一段以扣住的假名开头结尾的上游流：块尾 cc_sess 是假名 cc_sess_x、cc_sess_xy
// 的真前缀，这几个字节只能靠结束路径补发（真名 sessions_x 不是任何假名的前缀，扣不住）。
func tnHeldStream(t *testing.T) string {
	t.Helper()
	return tnSSEEvent(`{"type":"message_start","message":{"id":"msg_h","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`) +
		tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use cc_sess"))
}

const tnErrorEvent = "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"boom\"}}\n\n"

// 结束路径：上游 EOF 无 stop、error 事件、读错误，都要先把 carry 补发给客户端。
func TestHandleStreamingResponse_FlushesCarryOnEndPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame string // 结束帧的标记；EOF 没有帧
		body  func(t *testing.T) io.ReadCloser
	}{
		{name: "eof-without-stop", body: func(t *testing.T) io.ReadCloser { return io.NopCloser(strings.NewReader(tnHeldStream(t))) }},
		{name: "error-event", body: func(t *testing.T) io.ReadCloser { // 上游 error 事件不回帧，只返回错误
			return io.NopCloser(strings.NewReader(tnHeldStream(t) + tnErrorEvent))
		}},
		{name: "read-error", frame: "event: error", body: func(t *testing.T) io.ReadCloser {
			return &tnErrReader{data: []byte(tnHeldStream(t)), err: errors.New("connection reset by peer")}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newToolNameSplitGatewayService()
			c, rec := tnStaticContext(t)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: tc.body(t)}
			_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
			require.Error(t, err)
			body := rec.Body.String()
			require.Equal(t, "use cc_sess", tnClientJoined(body, "delta.text"))
			if tc.frame != "" {
				require.Contains(t, body, tc.frame)
				require.Less(t, strings.Index(body, "cc_sess"), strings.Index(body, tc.frame), "carry 必须先于结束帧补发")
			}
		})
	}
}

func TestHandleCCStreamingFromAnthropic_FlushesCarryOnEndPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame string
		body  func(t *testing.T) io.ReadCloser
	}{
		{name: "eof-without-stop", body: func(t *testing.T) io.ReadCloser { return io.NopCloser(strings.NewReader(tnHeldStream(t))) }},
		{name: "error-event", frame: `{"error":{`, body: func(t *testing.T) io.ReadCloser {
			return io.NopCloser(strings.NewReader(tnHeldStream(t) + tnErrorEvent))
		}},
		{name: "read-error", frame: `{"error":{`, body: func(t *testing.T) io.ReadCloser {
			return &tnErrReader{data: []byte(tnHeldStream(t)), err: errors.New("connection reset by peer")}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := tnStaticContext(t)
			resp := &http.Response{Header: http.Header{}, Body: tc.body(t)}
			_, err := (&GatewayService{}).handleCCStreamingFromAnthropic(resp, c, "model", "model", nil, time.Now())
			require.Error(t, err)
			body := rec.Body.String()
			require.Equal(t, "use cc_sess", tnClientJoined(body, "choices.0.delta.content"))
			if tc.frame != "" {
				require.Contains(t, body, tc.frame)
				require.Less(t, strings.Index(body, "cc_sess"), strings.Index(body, tc.frame), "carry 必须先于结束帧补发")
			}
		})
	}
}

func TestHandleResponsesStreamingResponse_FlushesCarryOnEndPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame string
		body  func(t *testing.T) io.ReadCloser
	}{
		{name: "eof-without-stop", body: func(t *testing.T) io.ReadCloser { return io.NopCloser(strings.NewReader(tnHeldStream(t))) }},
		{name: "error-event", frame: "response.failed", body: func(t *testing.T) io.ReadCloser {
			return io.NopCloser(strings.NewReader(tnHeldStream(t) + tnErrorEvent))
		}},
		{name: "read-error", frame: "response.failed", body: func(t *testing.T) io.ReadCloser {
			return &tnErrReader{data: []byte(tnHeldStream(t)), err: errors.New("connection reset by peer")}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := tnStaticContext(t)
			resp := &http.Response{Header: http.Header{}, Body: tc.body(t)}
			_, err := (&GatewayService{}).handleResponsesStreamingResponse(resp, c, "claude-sonnet-4.5", "claude-sonnet-4.5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			require.Error(t, err)
			body := rec.Body.String()
			require.Equal(t, "use cc_sess", tnClientJoined(body, "delta"))
			if tc.frame != "" {
				require.Contains(t, body, tc.frame)
				require.Less(t, strings.Index(body, "cc_sess"), strings.Index(body, tc.frame), "carry 必须先于结束帧补发")
			}
		})
	}
}

// 无映射时逐字节不改：假名原样送到客户端，静态前缀还原照旧。
func TestHandleStreamingResponse_NoMappingKeepsBytes(t *testing.T) {
	svc := newToolNameSplitGatewayService()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	stream := tnSSEEvent(`{"type":"message_start","message":{"id":"msg_n","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`) +
		tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use analyze_r")) +
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "ea01 now")) +
		tnSSEEvent(`{"type":"content_block_stop","index":0}`) +
		tnSSEEvent(`{"type":"message_stop"}`)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}
	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.NoError(t, err)
	require.Equal(t, "use analyze_rea01 now", tnClientJoined(rec.Body.String(), "delta.text"))
}

// 块尾停在不构成任何完整假名的裸假前缀上也要整段扣住，stop 前原样补发，不丢不移。
func TestToolNameStreamRestorer_BareStaticPrefixSplit(t *testing.T) {
	rw := tnStaticRewrite(t)
	for _, held := range []string{"cc_sess_", "cc_ses_"} {
		prefix := "a " + held
		up := []string{string(tnDeltaJSON(t, 0, "text_delta", "text", prefix)), tnStopJSON(0)}
		got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, map[int]string{0: prefix})
		require.Equal(t, prefix, got[0], "裸前缀不成名，补发的就是原字节")

		emitted := tnRun(newToolNameStreamRestorer(rw), up)
		require.Len(t, emitted, 3)
		require.Equal(t, "a ", tnPayloadOf([]byte(emitted[0])))
		require.Equal(t, held, tnPayloadOf([]byte(emitted[1])))
		require.Equal(t, tnStopJSON(0), emitted[2])
	}
}

// 只有动态映射时，静态假前缀照样被 restoreToolNamesInBytes 整体还原，被切开时也要扣住。
func TestToolNameStreamRestorer_StaticFakeSplitWithDynamicMapping(t *testing.T) {
	rw := tnDynamicRewrite(t)
	for real, fake := range staticToolNameRewrites {
		full := "a " + fake + "x b"
		at := strings.Index(full, fake)
		for cut := at + 1; cut < at+len(fake); cut++ {
			up := append(tnDeltas(t, 0, "text_delta", "text", full, []int{cut}), tnStopJSON(0))
			got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, tnWant(rw, map[int]string{0: full}))
			require.Equal(t, "a "+real+"x b", got[0])
		}
	}
}

// 同块出现非文本 delta（citations_delta 与不认识的类型）时，非文本事件原样放行，
// 留下的尾巴保留到同块下一个文本 delta 或 stop。
const (
	tnCitationsEvent    = `{"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":{"type":"web_search_result_location","cited_text":"x","url":"https://example.com","title":"t","encrypted_index":"e"}}}`
	tnUnknownDeltaEvent = `{"type":"content_block_delta","index":0,"delta":{"type":"foo_delta","foo":"bar"}}`
)

// tnEmitted 用新的还原器跑一遍，返回按顺序发出的全部事件 JSON。
func tnEmitted(rw *ToolNameRewrite, upstream []string) []string {
	return tnRun(newToolNameStreamRestorer(rw), upstream)
}

// tnNonTextEvents 返回发出事件里的非文本 delta 原字节，保持相对顺序。
func tnNonTextEvents(emitted []string) []string {
	var out []string
	for _, ev := range emitted {
		if gjson.Get(ev, "type").String() == "content_block_delta" && !tnIsTextDelta([]byte(ev)) {
			out = append(out, ev)
		}
	}
	return out
}

// 非文本 delta 不打断扣留：事件原样放行，carry 留到下一个同 index 文本 delta 或 stop，
// 被切名字的后半段因此仍能还原。
func TestToolNameStreamRestorer_NonTextDeltaKeepsCarry(t *testing.T) {
	rw := tnDynamicRewrite(t)
	full := "see the a" // 块尾 "a" 是动态假名 analyze_ 的前缀，必须留在 carry 里
	for _, tc := range []struct {
		name  string
		event string
	}{
		{name: "citations", event: tnCitationsEvent},
		{name: "unknown-type", event: tnUnknownDeltaEvent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := []string{
				string(tnDeltaJSON(t, 0, "text_delta", "text", "s")),
				tc.event,
				string(tnDeltaJSON(t, 0, "text_delta", "text", "ee the a")),
				tnStopJSON(0),
			}
			got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, tnWant(rw, map[int]string{0: full}))
			require.Equal(t, full, got[0])
			require.NotContains(t, got[0], "analyze_")

			emitted := tnEmitted(rw, up)
			require.Equal(t, []string{tc.event}, tnNonTextEvents(emitted), "non-text delta must pass through unchanged")
			require.Equal(t, tnStopJSON(0), emitted[len(emitted)-1])
		})
	}
}

// 非文本 delta 排在留下 carry 的 text_delta 之前：原样到达，不打扰之后的 carry 与拼接。
func TestToolNameStreamRestorer_NonTextDeltaBeforeHeldDelta(t *testing.T) {
	rw := tnDynamicRewrite(t)
	full := "see the a"
	up := []string{
		tnCitationsEvent,
		string(tnDeltaJSON(t, 0, "text_delta", "text", "s")),
		string(tnDeltaJSON(t, 0, "text_delta", "text", "ee the a")),
		tnStopJSON(0),
	}
	got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, tnWant(rw, map[int]string{0: full}))
	require.Equal(t, full, got[0])

	emitted := tnEmitted(rw, up)
	require.Equal(t, []string{tnCitationsEvent}, tnNonTextEvents(emitted))
	require.Equal(t, tnStopJSON(0), emitted[len(emitted)-1])
}

// 假名前半段先到、非文本 delta 插在中间、后半段随后到：citations_delta 原样经过，
// 名字仍按同一 index 拼回并还原。若在非文本 delta 处就把 carry 补发掉，后半段再也拼不回来。
func TestToolNameStreamRestorer_TextDeltaSplitByCitationsStitches(t *testing.T) {
	rw := tnDynamicRewrite(t)
	for _, tc := range []struct {
		name             string
		deltaType, field string
		head, tail, want string
	}{
		{"text", "text_delta", "text", "use analyze_r", "ea01 now", "use read now"},
		{"input_json", "input_json_delta", "partial_json", `{"file_path":"analyze_r`, `ea01"}`, `{"file_path":"read"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := []string{
				string(tnDeltaJSON(t, 0, tc.deltaType, tc.field, tc.head)),
				tnCitationsEvent,
				string(tnDeltaJSON(t, 0, tc.deltaType, tc.field, tc.tail)),
				tnStopJSON(0),
			}
			got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, tnWant(rw, map[int]string{0: tc.head + tc.tail}))
			require.Equal(t, tc.want, got[0])
			require.NotContains(t, got[0], "analyze_")

			emitted := tnEmitted(rw, up)
			require.Equal(t, []string{tnCitationsEvent}, tnNonTextEvents(emitted), "citations_delta must pass through unchanged")
			require.Equal(t, tnStopJSON(0), emitted[len(emitted)-1])
		})
	}
}

// 不切分时块尾恰好停在假名开头，随后到达 citations_delta：尾巴留到 stop 前，不能丢。

func TestToolNameStreamRestorer_WholeDeltaThenCitationsKeepsTail(t *testing.T) {
	rw := tnDynamicRewrite(t)
	full := "see the a"
	up := []string{
		string(tnDeltaJSON(t, 0, "text_delta", "text", full)),
		tnCitationsEvent,
		tnStopJSON(0),
	}
	got := tnCheckInvariant(t, newToolNameStreamRestorer(rw), rw, up, tnWant(rw, map[int]string{0: full}))
	require.Equal(t, full, got[0])

	emitted := tnEmitted(rw, up)
	require.Equal(t, []string{tnCitationsEvent}, tnNonTextEvents(emitted))
	require.Equal(t, "a", tnPayloadOf([]byte(emitted[len(emitted)-2])), "held tail must arrive just before the stop")
	require.Equal(t, tnStopJSON(0), emitted[len(emitted)-1])
}

// tnCheckEventLinesMatchData 断言每个带 data 的 SSE 块，event 行与 data 的 type 字段一致。
func tnCheckEventLinesMatchData(t *testing.T, body string) {
	t.Helper()
	for _, block := range strings.Split(body, "\n\n") {
		event, data := "", ""
		for _, line := range strings.Split(block, "\n") {
			if rest, ok := strings.CutPrefix(line, "event:"); ok {
				event = strings.TrimSpace(rest)
			}
			if rest, ok := strings.CutPrefix(line, "data:"); ok {
				data = strings.TrimSpace(rest)
			}
		}
		if data == "" || !gjson.Valid(data) {
			continue
		}
		require.Equal(t, gjson.Get(data, "type").String(), event, "event line must match the data type in block %q", block)
	}
}

// 块尾扣下的字节在 content_block_stop 前补发：补发的事件必须是 content_block_delta 的
// event 行，不能沿用 stop 块的 event 行。
func TestHandleStreamingResponse_CarryFlushUsesDeltaEventLine(t *testing.T) {
	svc := newToolNameSplitGatewayService()
	c, rec := tnStaticContext(t)
	stream := strings.Join([]string{
		tnSSEEvent(`{"type":"message_start","message":{"id":"msg_e","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`),
		tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use cc_sess_x")),
		tnSSEEvent(`{"type":"content_block_stop","index":0}`),
		tnSSEEvent(`{"type":"message_stop"}`),
	}, "")
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.NoError(t, err)

	tnCheckEventLinesMatchData(t, rec.Body.String())
	require.Equal(t, "use sessions_x", tnClientJoined(rec.Body.String(), "delta.text"))
	require.Equal(t, 2, strings.Count(rec.Body.String(), "event: content_block_delta"))
}

// carry 与 stop 之间夹一个 citations_delta 时同样如此：citations 原样先到，carry 在 stop 前补发。
func TestHandleStreamingResponse_CarryFlushAfterCitationsUsesDeltaEventLine(t *testing.T) {
	svc := newToolNameSplitGatewayService()
	c, rec := tnStaticContext(t)
	stream := strings.Join([]string{
		tnSSEEvent(`{"type":"message_start","message":{"id":"msg_c","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`),
		tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use cc_sess_x")),
		tnSSEEvent(tnCitationsEvent),
		tnSSEEvent(`{"type":"content_block_stop","index":0}`),
		tnSSEEvent(`{"type":"message_stop"}`),
	}, "")
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.NoError(t, err)

	tnCheckEventLinesMatchData(t, rec.Body.String())
	require.Equal(t, "use sessions_x", tnClientJoined(rec.Body.String(), "delta.text"))
	require.Equal(t, 3, strings.Count(rec.Body.String(), "event: content_block_delta"))
	require.Equal(t, 1, strings.Count(rec.Body.String(), "citations_delta"))
	require.Contains(t, rec.Body.String(), tnCitationsEvent)
}

// 原生入口：text_delta 停在假名中间，citations_delta 插进来，再由后续 text_delta 补完。
// 客户端按 index 拼出的文本还原为真名，citations_delta 原样到达。
func TestHandleStreamingResponse_CitationsBetweenTextDeltasStitches(t *testing.T) {
	svc := newToolNameSplitGatewayService()
	c, rec, _ := newToolNameSplitContext(t)
	stream := strings.Join([]string{
		tnSSEEvent(`{"type":"message_start","message":{"id":"msg_s","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","usage":{"input_tokens":1}}}`),
		tnSSEEvent(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "use analyze_r")),
		tnSSEEvent(tnCitationsEvent),
		tnSSEEvent(tnSSEDelta(t, 0, "text_delta", "text", "ea01 now")),
		tnSSEEvent(`{"type":"content_block_stop","index":0}`),
		tnSSEEvent(`{"type":"message_stop"}`),
	}, "")
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	require.NoError(t, err)

	tnCheckEventLinesMatchData(t, rec.Body.String())
	require.Equal(t, "use read now", tnClientJoined(rec.Body.String(), "delta.text"))
	require.NotContains(t, rec.Body.String(), "analyze_")
	require.Equal(t, 1, strings.Count(rec.Body.String(), "citations_delta"))
	require.Contains(t, rec.Body.String(), tnCitationsEvent)
}
