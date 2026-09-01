package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSmartPlanner struct {
	plan []service.SmartRoutingCandidate
	err  error
}

func (f *fakeSmartPlanner) BuildSmartRoutingPlan(ctx context.Context, smartGroup *service.Group, requestedModel string) ([]service.SmartRoutingCandidate, error) {
	return f.plan, f.err
}

func smartGroupFixture() *service.Group {
	return &service.Group{
		ID:       100,
		Platform: service.PlatformSmartRouting,
		Status:   service.StatusActive,
		SmartRoutingMembers: []domain.SmartRoutingMember{
			{GroupID: 1, Priority: 1, Weight: 1},
			{GroupID: 2, Priority: 2, Weight: 1},
		},
	}
}

func newSmartRoutingTestContext(bodyStr string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(bodyStr))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func setSmartAPIKey(c *gin.Context) {
	groupID := int64(100)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		Group:   smartGroupFixture(),
		GroupID: &groupID,
	})
}

func TestSmartRoutingShouldRetryStatus(t *testing.T) {
	retryable := []int{404, 408, 425, 429, 502, 503, 504, 500, 529}
	for _, s := range retryable {
		assert.True(t, smartRoutingShouldRetryStatus(s), "status %d should retry", s)
	}
	notRetryable := []int{200, 201, 400, 401, 402, 403, 405, 422, 0}
	for _, s := range notRetryable {
		assert.False(t, smartRoutingShouldRetryStatus(s), "status %d should not retry", s)
	}
}

func TestSmartRoutingDispatcher_PassthroughForNonSmartKey(t *testing.T) {
	called := false
	d := newSmartRoutingDispatcher(&fakeSmartPlanner{}, nil)
	handler := d.wrap(func(c *gin.Context) { called = true })

	c, _ := newSmartRoutingTestContext("{}")
	handler(c)
	assert.True(t, called)
}

func TestSmartRoutingDispatcher_FailoverToNextMember(t *testing.T) {
	member1 := &service.Group{ID: 1, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	member2 := &service.Group{ID: 2, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	planner := &fakeSmartPlanner{plan: []service.SmartRoutingCandidate{
		{Group: member1, Priority: 1, Weight: 1},
		{Group: member2, Priority: 2, Weight: 1},
	}}
	d := newSmartRoutingDispatcher(planner, nil)

	var attempts []int64
	terminal := func(c *gin.Context) {
		apiKey, _ := middleware.GetAPIKeyFromContext(c)
		require.NotNil(t, apiKey.Group)
		attempts = append(attempts, apiKey.Group.ID)
		if apiKey.Group.ID == 1 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"member": 1})
			return
		}
		c.JSON(http.StatusOK, gin.H{"member": 2})
	}

	c, w := newSmartRoutingTestContext("{\"model\":\"claude-3\"}")
	setSmartAPIKey(c)
	d.wrap(terminal)(c)

	assert.Equal(t, []int64{1, 2}, attempts, "should retry member 2 after member 1 fails")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "member")
}

func TestSmartRoutingDispatcher_AllMembersFailReturnsLastError(t *testing.T) {
	member1 := &service.Group{ID: 1, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	member2 := &service.Group{ID: 2, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	planner := &fakeSmartPlanner{plan: []service.SmartRoutingCandidate{
		{Group: member1, Priority: 1, Weight: 1},
		{Group: member2, Priority: 2, Weight: 1},
	}}
	d := newSmartRoutingDispatcher(planner, nil)

	terminal := func(c *gin.Context) {
		apiKey, _ := middleware.GetAPIKeyFromContext(c)
		c.JSON(http.StatusServiceUnavailable, gin.H{"member": apiKey.Group.ID})
	}

	c, w := newSmartRoutingTestContext("{}")
	setSmartAPIKey(c)
	d.wrap(terminal)(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "member")
}

func TestSmartRoutingDispatcher_NoRetryAfterCommit(t *testing.T) {
	member1 := &service.Group{ID: 1, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	planner := &fakeSmartPlanner{plan: []service.SmartRoutingCandidate{
		{Group: member1, Priority: 1, Weight: 1},
	}}
	d := newSmartRoutingDispatcher(planner, nil)

	attempts := 0
	terminal := func(c *gin.Context) {
		attempts++
		c.Writer.WriteHeader(http.StatusOK)
		_, _ = c.Writer.Write([]byte("data: partial"))
		c.Writer.Flush()
	}

	c, w := newSmartRoutingTestContext("{}")
	setSmartAPIKey(c)
	d.wrap(terminal)(c)

	assert.Equal(t, 1, attempts, "committed stream must not be retried")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "data: partial")
}

func TestSmartRoutingDispatcher_EmptyPlanReturns503(t *testing.T) {
	d := newSmartRoutingDispatcher(&fakeSmartPlanner{plan: nil}, nil)
	terminal := func(c *gin.Context) { t.Fatal("terminal should not run") }

	c, w := newSmartRoutingTestContext("{}")
	setSmartAPIKey(c)
	d.wrap(terminal)(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func newGinResponseWriterForTest() (gin.ResponseWriter, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c.Writer, w
}

func TestSmartRouteRecorder_BuffersUntilCommit(t *testing.T) {
	targetWriter, target := newGinResponseWriterForTest()
	rec := newSmartRouteRecorder(targetWriter)

	rec.WriteHeader(http.StatusServiceUnavailable)
	_, _ = rec.Write([]byte("buffered"))
	assert.False(t, rec.committed)
	assert.Equal(t, 0, target.Body.Len(), "nothing should reach target before commit")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Status())
	assert.True(t, rec.Written())

	rec.flushToTarget()
	assert.True(t, rec.committed)
	assert.Equal(t, http.StatusServiceUnavailable, target.Code)
	assert.Equal(t, "buffered", target.Body.String())
}

func TestSmartRouteRecorder_FlushCommitsAndPassesThrough(t *testing.T) {
	targetWriter, target := newGinResponseWriterForTest()
	rec := newSmartRouteRecorder(targetWriter)
	_, _ = rec.Write([]byte("stream"))
	rec.Flush()
	assert.True(t, rec.committed)
	assert.Equal(t, "stream", target.Body.String())
	_, _ = rec.Write([]byte("-more"))
	assert.Equal(t, "stream-more", target.Body.String())
}

func TestSmartRouteRecorder_FailedAttemptDiscardsBuffer(t *testing.T) {
	targetWriter, target := newGinResponseWriterForTest()
	rec := newSmartRouteRecorder(targetWriter)
	rec.WriteHeader(http.StatusBadGateway)
	_, _ = rec.Write([]byte("upstream failed"))
	// A retryable failure: caller simply drops the recorder without flushing.
	assert.False(t, rec.committed)
	assert.Equal(t, 0, target.Body.Len(), "discarded buffer must not reach target")
}
