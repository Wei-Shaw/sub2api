//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokImagesPreservesUpstreamErrorOnSyncAndAsyncRoutes(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		accountType := "api-key"
		if oauth {
			accountType = "oauth"
		}
		for _, async := range []bool{false, true} {
			route := "sync"
			if async {
				route = "async"
			}
			t.Run(accountType+"/"+route, func(t *testing.T) {
				h, slots, _, upstream := newGrokMediaSlotHandler(t, oauth, false)
				h.grokMediaEligibilityProber = grokMediaSlotProber(func(context.Context, int64) (bool, string, error) {
					return true, "test account is eligible", nil
				})
				upstream.call = func(req *http.Request, _ int64) (*http.Response, error) {
					assert.Equal(t, "/v1/images/edits", req.URL.Path)
					return &http.Response{
						StatusCode: http.StatusBadRequest,
						Header:     http.Header{"Content-Type": {"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"code":"invalid_image","error":"Image could not be decoded: https://example.test/image?access_token=secret&size=1","debug":"private upstream diagnostic"}`)),
					}, nil
				}
				c, recorder := grokMediaSlotContext(context.Background(), true)
				path := "/v1/images/edits"
				if async {
					path += "/async"
				}
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"grok-imagine-image","prompt":"test","image":{"url":"https://example.test/input.png"}}`))
				c.Request.Header.Set("Content-Type", "application/json")

				var responseBody []byte
				if async {
					store := &asyncImageMemoryStore{tasks: make(map[string]*service.ImageTaskRecord)}
					tasks := service.NewImageTaskServiceWithUploader(store, nil, time.Hour, time.Minute)
					asyncHandler := NewAsyncImageHandler(tasks, h)
					asyncHandler.Submit(c)
					require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
					taskID := gjson.GetBytes(recorder.Body.Bytes(), "task_id").String()
					owner := service.ImageTaskOwner{UserID: 10, APIKeyID: 20}
					require.Eventually(t, func() bool {
						task, err := tasks.Get(context.Background(), owner, taskID)
						return err == nil && task.Status != service.ImageTaskStatusProcessing
					}, time.Second, 10*time.Millisecond)
					poll, pollRecorder := grokMediaSlotContext(context.Background(), false)
					poll.Params[0].Key, poll.Params[0].Value = "task_id", taskID
					asyncHandler.Get(poll)
					require.Equal(t, http.StatusOK, pollRecorder.Code)
					responseBody = pollRecorder.Body.Bytes()
					require.Equal(t, service.ImageTaskStatusFailed, gjson.GetBytes(responseBody, "status").String())
					require.Equal(t, int64(http.StatusBadRequest), gjson.GetBytes(responseBody, "http_status").Int())
				} else {
					h.GrokImages(c)
					require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
					responseBody = recorder.Body.Bytes()
				}
				require.Equal(t, "Image could not be decoded: https://example.test/image?access_token=***&size=1", gjson.GetBytes(responseBody, "error.message").String())
				require.Equal(t, "invalid_image", gjson.GetBytes(responseBody, "error.code").String())
				require.Equal(t, "invalid_request_error", gjson.GetBytes(responseBody, "error.type").String())
				require.NotContains(t, string(responseBody), "secret")
				require.NotContains(t, string(responseBody), "private upstream diagnostic")
				require.Equal(t, 1, upstream.calls, "deterministic image errors must not retry another account")
				slots.assertReleased(t)
			})
		}
	}
}
