//go:build unit

package inbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// stubPublisher 是 Publisher 的测试桩，返回预置结果并记录最后一次入参。
type stubPublisher struct {
	created       bool
	seq           int64
	err           error
	lastBroadcast PublishBroadcastInput
}

func (p *stubPublisher) PublishToUser(_ context.Context, _ PublishDirectInput) (bool, int64, error) {
	return p.created, p.seq, p.err
}
func (p *stubPublisher) PublishBroadcast(_ context.Context, in PublishBroadcastInput) (bool, int64, error) {
	p.lastBroadcast = in
	return p.created, p.seq, p.err
}

// auditRepo 在 stubRepo 基础上补齐审计分页方法。
type auditRepo struct {
	stubRepo
	broadcasts []Broadcast
	directs    []DirectMessage
}

func (r *auditRepo) ListBroadcastsPaged(_ context.Context, namespace string, limit, offset int) ([]Broadcast, int64, error) {
	var filtered []Broadcast
	for _, b := range r.broadcasts {
		if namespace == "" || b.Namespace == namespace {
			filtered = append(filtered, b)
		}
	}
	return page(filtered, limit, offset), int64(len(filtered)), nil
}
func (r *auditRepo) ListDirectMessagesPaged(_ context.Context, namespace string, userID int64, limit, offset int) ([]DirectMessage, int64, error) {
	var filtered []DirectMessage
	for _, m := range r.directs {
		if (namespace == "" || m.Namespace == namespace) && (userID == 0 || m.UserID == userID) {
			filtered = append(filtered, m)
		}
	}
	return page(filtered, limit, offset), int64(len(filtered)), nil
}

func page[T any](in []T, limit, offset int) []T {
	if offset >= len(in) {
		return nil
	}
	end := offset + limit
	if end > len(in) {
		end = len(in)
	}
	return in[offset:end]
}

// envelope 解析响应统一信封。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func doReq(h gin.HandlerFunc, method, target, body string) (*httptest.ResponseRecorder, envelope) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	c.Request = r
	h(c)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

func newTestHandler(repo Repository, pub Publisher, uid int64, authed bool) *Handler {
	svc := NewService(repo, &fakeSeq{seqs: []int64{999}}, NewNoopAttributeProvider(), nil, Config{})
	getUserID := func(*gin.Context) (int64, bool) { return uid, authed }
	return NewHandler(svc, pub, getUserID)
}

func TestHandler_Catchup_Unauthorized(t *testing.T) {
	h := newTestHandler(&stubRepo{}, &stubPublisher{}, 0, false)
	rec, _ := doReq(h.Catchup, http.MethodGet, "/inbox/catchup", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未鉴权应 401, got %d", rec.Code)
	}
}

func TestHandler_Catchup_InvalidSince(t *testing.T) {
	h := newTestHandler(&stubRepo{found: true}, &stubPublisher{}, 1, true)
	rec, _ := doReq(h.Catchup, http.MethodGet, "/inbox/catchup?since=abc", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 since 应 400, got %d", rec.Code)
	}
}

func TestHandler_Catchup_OK(t *testing.T) {
	repo := &stubRepo{found: true, acked: 5, direct: []Message{{Seq: 6, Scope: ScopeDirect, Namespace: "n"}}}
	h := newTestHandler(repo, &stubPublisher{}, 1, true)
	rec, env := doReq(h.Catchup, http.MethodGet, "/inbox/catchup?since=0", "")
	if rec.Code != http.StatusOK || env.Code != 0 {
		t.Fatalf("应 200/code0, got http=%d code=%d", rec.Code, env.Code)
	}
	var res CatchupResult
	if err := json.Unmarshal(env.Data, &res); err != nil {
		t.Fatalf("解析 data 失败: %v", err)
	}
	if res.AckedSeq != 5 || len(res.Messages) != 1 || res.Messages[0].Seq != 6 {
		t.Fatalf("catchup 结果异常: %+v", res)
	}
}

func TestHandler_Ack(t *testing.T) {
	h := newTestHandler(&stubRepo{found: true}, &stubPublisher{}, 1, true)

	rec, env := doReq(h.Ack, http.MethodPost, "/inbox/ack", `{"seq":42}`)
	if rec.Code != http.StatusOK || env.Code != 0 {
		t.Fatalf("ack 应 200, got %d", rec.Code)
	}

	// 非法 body → 400
	rec2, _ := doReq(h.Ack, http.MethodPost, "/inbox/ack", `not-json`)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400, got %d", rec2.Code)
	}

	// seq<=0 → service 返回 400 INBOX_INVALID_ACK
	rec3, _ := doReq(h.Ack, http.MethodPost, "/inbox/ack", `{"seq":0}`)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("seq<=0 应 400, got %d", rec3.Code)
	}
}

func TestHandler_UnreadCount(t *testing.T) {
	repo := &stubRepo{found: true, acked: 5, unackedDirect: []int64{6, 7}}
	h := newTestHandler(repo, &stubPublisher{}, 1, true)
	rec, env := doReq(h.UnreadCount, http.MethodGet, "/inbox/unread-count", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("应 200, got %d", rec.Code)
	}
	var data struct {
		Count     int  `json:"count"`
		Truncated bool `json:"truncated"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if data.Count != 2 {
		t.Fatalf("未读应为 2, got %d", data.Count)
	}
}

func TestHandler_Broadcast(t *testing.T) {
	pub := &stubPublisher{created: true, seq: 123}
	h := newTestHandler(&stubRepo{}, pub, 7, true)

	body := `{"namespace":"support_ticket","dedup_key":"x:1","targeting":{"op":"all_users"},"payload":{"a":1}}`
	rec, env := doReq(h.Broadcast, http.MethodPost, "/admin/inbox/broadcast", body)
	if rec.Code != http.StatusOK || env.Code != 0 {
		t.Fatalf("broadcast 应 200, got %d", rec.Code)
	}
	var data struct {
		Seq     int64 `json:"seq"`
		Created bool  `json:"created"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if data.Seq != 123 || !data.Created {
		t.Fatalf("broadcast 返回异常: %+v", data)
	}
	if pub.lastBroadcast.Namespace != "support_ticket" || pub.lastBroadcast.DedupKey != "x:1" {
		t.Fatalf("发布入参未透传: %+v", pub.lastBroadcast)
	}

	// 非法 body → 400
	rec2, _ := doReq(h.Broadcast, http.MethodPost, "/admin/inbox/broadcast", `oops`)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("非法 body 应 400, got %d", rec2.Code)
	}
}

func TestHandler_AdminListBroadcasts(t *testing.T) {
	repo := &auditRepo{broadcasts: []Broadcast{
		{Seq: 3, Namespace: "support_ticket"},
		{Seq: 2, Namespace: "announcement"},
		{Seq: 1, Namespace: "support_ticket"},
	}}
	h := newTestHandler(repo, &stubPublisher{}, 7, true)

	// 过滤 namespace=support_ticket → 2 条
	rec, env := doReq(h.AdminListBroadcasts, http.MethodGet, "/admin/inbox/broadcasts?namespace=support_ticket", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("应 200, got %d", rec.Code)
	}
	var res AuditPage[Broadcast]
	_ = json.Unmarshal(env.Data, &res)
	if res.Total != 2 || len(res.Items) != 2 {
		t.Fatalf("应命中 2 条 support_ticket 广播, got total=%d items=%d", res.Total, len(res.Items))
	}
	if res.Page != 1 || res.PageSize != defaultAuditPageSize {
		t.Fatalf("分页默认值异常: page=%d size=%d", res.Page, res.PageSize)
	}
}

func TestHandler_AdminListDirectMessages(t *testing.T) {
	repo := &auditRepo{directs: []DirectMessage{
		{Seq: 3, UserID: 1, Namespace: "support_ticket"},
		{Seq: 2, UserID: 2, Namespace: "support_ticket"},
		{Seq: 1, UserID: 1, Namespace: "announcement"},
	}}
	h := newTestHandler(repo, &stubPublisher{}, 7, true)

	// 过滤 user_id=1 → 2 条
	rec, env := doReq(h.AdminListDirectMessages, http.MethodGet, "/admin/inbox/direct-messages?user_id=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("应 200, got %d", rec.Code)
	}
	var res AuditPage[DirectMessage]
	_ = json.Unmarshal(env.Data, &res)
	if res.Total != 2 || len(res.Items) != 2 {
		t.Fatalf("user_id=1 应命中 2 条, got total=%d items=%d", res.Total, len(res.Items))
	}

	// 非法 user_id → 400
	rec2, _ := doReq(h.AdminListDirectMessages, http.MethodGet, "/admin/inbox/direct-messages?user_id=abc", "")
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("非法 user_id 应 400, got %d", rec2.Code)
	}
}
