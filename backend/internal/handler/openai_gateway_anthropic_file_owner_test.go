package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type handlerFileOwnerEntry struct {
	ownerUserID int64
	accountID   int64
	platform    string
}

// handlerFileOwnerCache 只实现归属读写；其余 GatewayCache 方法由嵌入的 nil
// 接口占位——拒绝路径不会调用它们，若误入选号会以 panic 暴露。
type handlerFileOwnerCache struct {
	service.GatewayCache
	entries map[string]handlerFileOwnerEntry
	getErr  error
}

func (c *handlerFileOwnerCache) SetAnthropicFileOwner(_ context.Context, _ string, _ int64, _ int64, _ string, _ time.Duration) error {
	return nil
}

func (c *handlerFileOwnerCache) GetAnthropicFileOwner(_ context.Context, fileID string) (int64, int64, string, bool, error) {
	if c.getErr != nil {
		return 0, 0, "", false, c.getErr
	}
	entry, ok := c.entries[fileID]
	if !ok {
		return 0, 0, "", false, nil
	}
	return entry.ownerUserID, entry.accountID, entry.platform, true, nil
}

// recordingFileOwnerAccountRepo 记录是否有任何选号查询发生。
type recordingFileOwnerAccountRepo struct {
	service.AccountRepository
	calls int
}

func (r *recordingFileOwnerAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	r.calls++
	return nil, nil
}
func (r *recordingFileOwnerAccountRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	r.calls++
	return nil, nil
}
func (r *recordingFileOwnerAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	r.calls++
	return nil, nil
}
func (r *recordingFileOwnerAccountRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]service.Account, error) {
	r.calls++
	return nil, nil
}

func newFileOwnerMessagesHandler(t *testing.T, cache service.GatewayCache, repo service.AccountRepository) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gatewayService := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, cache, cfg,
		nil, nil, nil, nil, billing, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return &OpenAIGatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(nil)},
		cfg:                 cfg,
	}
}

func runFileOwnerMessagesRequest(t *testing.T, h *OpenAIGatewayHandler, userID int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	groupID := int64(163)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 5101, UserID: userID, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Platform: service.PlatformDeepseek},
		User:  &service.User{ID: userID},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID, Concurrency: 1})
	h.Messages(c)
	return rec
}

const fileOwnerOwnAndUnknownBody = `{"model":"deepseek-v3.2","max_tokens":16,"messages":[{"role":"user","content":[` +
	`{"type":"image","source":{"type":"file","file_id":"file_own"}},` +
	`{"type":"image","source":{"type":"file","file_id":"file_unknown"}}]}]}`

const fileOwnerForeignBody = `{"model":"deepseek-v3.2","max_tokens":16,"messages":[{"role":"user","content":[` +
	`{"type":"image","source":{"type":"file","file_id":"file_owned_by_other"}}]}]}`

// TestOpenAIGatewayMessagesRejectsUnauthorizedFileOwnerBeforeScheduling 验证归属
// 拒绝发生在任何选号之前：混合自有+未知 file 必须是 400，不得 pinned 后放行；
// 他人的 file 是 403；缓存故障是 503。三种情况都不得触发账号选号查询。
func TestOpenAIGatewayMessagesRejectsUnauthorizedFileOwnerBeforeScheduling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const requester = int64(6101)

	tests := []struct {
		name    string
		cache   *handlerFileOwnerCache
		body    string
		status  int
		errType string
		message string
	}{
		{
			name: "own plus unknown file is rejected as unknown",
			cache: &handlerFileOwnerCache{entries: map[string]handlerFileOwnerEntry{
				"file_own": {ownerUserID: requester, accountID: 163, platform: service.PlatformDeepseek},
			}},
			body:    fileOwnerOwnAndUnknownBody,
			status:  http.StatusBadRequest,
			errType: "invalid_request_error",
			message: "file_id is not registered to this API key; re-upload the file",
		},
		{
			name: "file owned by another user is forbidden",
			cache: &handlerFileOwnerCache{entries: map[string]handlerFileOwnerEntry{
				"file_owned_by_other": {ownerUserID: requester + 1, accountID: 163, platform: service.PlatformDeepseek},
			}},
			body:    fileOwnerForeignBody,
			status:  http.StatusForbidden,
			errType: "permission_error",
			message: "file_id belongs to another user",
		},
		{
			name:    "owner lookup failure is retryable",
			cache:   &handlerFileOwnerCache{getErr: errors.New("redis down")},
			body:    fileOwnerForeignBody,
			status:  http.StatusServiceUnavailable,
			errType: "api_error",
			message: "file owner lookup is temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &recordingFileOwnerAccountRepo{}
			h := newFileOwnerMessagesHandler(t, tt.cache, repo)
			rec := runFileOwnerMessagesRequest(t, h, requester, tt.body)

			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.errType, gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
			require.Contains(t, rec.Body.String(), tt.message)
			require.Zero(t, repo.calls, "rejected file owner request must not reach account scheduling")
		})
	}
}

// TestRejectAnthropicMessagesFileOwner 验证判定到 HTTP 语义的映射：只有 pinned
// 与 none 放行，其余判定都必须写出拒绝响应并阻断后续选号。
func TestRejectAnthropicMessagesFileOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		decision service.AnthropicFileOwnerDecision
		rejected bool
		status   int
		errType  string
		message  string
	}{
		{decision: service.AnthropicFileOwnerNone, rejected: false},
		{decision: service.AnthropicFileOwnerPinned, rejected: false},
		{decision: service.AnthropicFileOwnerForeign, rejected: true, status: http.StatusForbidden, errType: "permission_error", message: "file_id belongs to another user"},
		{decision: service.AnthropicFileOwnerUnknown, rejected: true, status: http.StatusBadRequest, errType: "invalid_request_error", message: "file_id is not registered to this API key; re-upload the file"},
		{decision: service.AnthropicFileOwnerConflict, rejected: true, status: http.StatusBadRequest, errType: "invalid_request_error", message: "file_ids resolve to different accounts; re-upload the files"},
		{decision: service.AnthropicFileOwnerStoreError, rejected: true, status: http.StatusServiceUnavailable, errType: "api_error", message: "file owner lookup is temporarily unavailable"},
	}

	for _, tt := range tests {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		h := &OpenAIGatewayHandler{}
		rejected := h.rejectAnthropicMessagesFileOwner(c, nil, 1, tt.decision, errors.New("redis down"), false)
		require.Equal(t, tt.rejected, rejected, "decision=%d", tt.decision)
		if !tt.rejected {
			require.Equal(t, http.StatusOK, rec.Code)
			require.Empty(t, rec.Body.String())
			continue
		}
		require.Equal(t, tt.status, rec.Code)
		require.Equal(t, tt.errType, gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
		require.Contains(t, rec.Body.String(), tt.message)
	}
}
