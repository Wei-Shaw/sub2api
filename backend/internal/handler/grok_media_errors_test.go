//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokImageEditLocalValidationSyncAndAsync(t *testing.T) {
	enableGrokMediaValidationTestScheduler(t)
	for _, oauth := range []bool{false, true} {
		for _, async := range []bool{false, true} {
			for _, format := range []string{"json", "multipart"} {
				for _, count := range []int{3, 4, 5, 6} {
					t.Run(fmt.Sprintf("oauth=%t/async=%t/%s/%d", oauth, async, format, count), func(t *testing.T) {
						h, slots, _, upstream := newGrokMediaSlotHandler(t, oauth, false)
						h.grokMediaEligibilityProber = grokMediaSlotProber(func(context.Context, int64) (bool, string, error) {
							return true, "eligible", nil
						})
						upstream.call = func(req *http.Request, _ int64) (*http.Response, error) {
							assert.Equal(t, "/v1/images/edits", req.URL.Path)
							body, err := io.ReadAll(req.Body)
							assert.NoError(t, err)
							assert.Len(t, gjson.GetBytes(body, "images").Array(), count)
							assert.NotEmpty(t, gjson.GetBytes(body, "mask.url").String())
							return &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"application/json"}},
								Body: io.NopCloser(strings.NewReader(`{"code":"invalid_image","error":"upstream image rejection"}`))}, nil
						}
						body, contentType := grokImageEditLimitBody(t, format, count)
						c, recorder := grokMediaSlotContext(context.Background(), true)
						path := "/v1/images/edits"
						if async {
							path += "/async"
						}
						c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
						c.Request.Header.Set("Content-Type", contentType)
						responseBody := []byte(nil)
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
						require.Equal(t, "invalid_request_error", gjson.GetBytes(responseBody, "error.type").String())
						metrics := h.gatewayService.SnapshotOpenAIAccountSchedulerMetrics()
						if count > 5 {
							require.Equal(t, "a maximum of 5 source images is supported for image edits", gjson.GetBytes(responseBody, "error.message").String())
							require.Zero(t, upstream.calls)
							require.Zero(t, metrics.RuntimeStatsAccountCount, "local rejection must report neither success nor failure")
						} else {
							require.Equal(t, "upstream image rejection", gjson.GetBytes(responseBody, "error.message").String())
							require.Equal(t, 1, upstream.calls)
							require.Equal(t, 1, metrics.RuntimeStatsAccountCount, "real upstream failures still report failure")
						}
						require.Zero(t, metrics.AccountSwitchTotal)
						slots.assertReleased(t)
					})
				}
			}
		}
	}
}

func grokImageEditLimitBody(t *testing.T, format string, count int) ([]byte, string) {
	t.Helper()
	if format == "json" {
		images := make([]string, count)
		for index := range images {
			images[index] = fmt.Sprintf("https://example.test/%d.png", index)
		}
		body, err := json.Marshal(map[string]any{"model": "grok-imagine-image-2.0", "prompt": "test", "images": images, "mask": "https://example.test/mask.png"})
		require.NoError(t, err)
		return body, "application/json"
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	require.NoError(t, writer.WriteField("model", "grok-imagine-image-2.0"))
	require.NoError(t, writer.WriteField("prompt", "test"))
	for index := 0; index < count; index++ {
		if index%2 == 0 {
			require.NoError(t, writer.WriteField("image", fmt.Sprintf("https://example.test/%d.png", index)))
		} else {
			part, err := writer.CreateFormFile("image[]", fmt.Sprintf("%d.png", index))
			require.NoError(t, err)
			_, err = part.Write([]byte("\x89PNG\r\n\x1a\nsynthetic"))
			require.NoError(t, err)
		}
	}
	part, err := writer.CreateFormFile("mask", "mask.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\nmask"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buffer.Bytes(), writer.FormDataContentType()
}

func TestGrokImageEditInternalAndRetryableErrorsRemainUpstreamErrors(t *testing.T) {
	enableGrokMediaValidationTestScheduler(t)
	for _, retryable := range []bool{false, true} {
		t.Run(fmt.Sprintf("retryable=%t", retryable), func(t *testing.T) {
			h, slots, _, upstream := newGrokMediaSlotHandler(t, false, false)
			upstream.call = func(*http.Request, int64) (*http.Response, error) {
				if !retryable {
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(iotest.ErrReader(errors.New("internal response read failure")))}, nil
				}
				return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"error":"temporarily unavailable"}`))}, nil
			}
			c, recorder := grokMediaSlotContext(context.Background(), true)
			body, contentType := grokImageEditLimitBody(t, "json", 3)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", contentType)
			h.GrokImages(c)
			require.GreaterOrEqual(t, recorder.Code, http.StatusInternalServerError, recorder.Body.String())
			require.NotEqual(t, "invalid_request_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
			metrics := h.gatewayService.SnapshotOpenAIAccountSchedulerMetrics()
			require.Positive(t, metrics.RuntimeStatsAccountCount, "calls=%d response=%s", upstream.calls, recorder.Body.String())
			if retryable {
				require.Greater(t, upstream.calls, 1)
				require.Positive(t, metrics.AccountSwitchTotal)
			} else {
				require.Equal(t, http.StatusBadGateway, recorder.Code)
				require.Equal(t, 1, upstream.calls)
				require.Zero(t, metrics.AccountSwitchTotal)
			}
			slots.assertReleased(t)
		})
	}
}

type grokMediaValidationSettingsRepo struct {
	service.SettingRepository
}

func (grokMediaSlotRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}

func (*grokMediaValidationSettingsRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func enableGrokMediaValidationTestScheduler(t *testing.T) {
	t.Helper()
	settings := service.NewSettingService(&grokMediaValidationSettingsRepo{}, nil)
	require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{OpenAIAdvancedSchedulerEnabled: true}))
	t.Cleanup(func() {
		require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{}))
	})
}

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
