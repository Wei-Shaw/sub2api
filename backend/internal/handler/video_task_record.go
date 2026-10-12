package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type videoTaskRecorder interface {
	BeginVideoTaskRecord(context.Context, *service.APIKey, *service.Account, *service.UserSubscription, service.GrokVideoPendingBilling) (*service.VideoTask, error)
	AcceptVideoTaskRecord(context.Context, *service.VideoTask, string, string) error
	ObserveVideoTaskRecord(context.Context, string, int64, int64, int64, string) error
}

// Buffer JSON only, never video downloads. Successful creation is not exposed
// until its identity has been saved. Retain native response formats verbatim.
type videoRecordWriter struct {
	gin.ResponseWriter
	header  http.Header
	body    bytes.Buffer
	status  int
	written bool
}

func (w *videoRecordWriter) Header() http.Header { return w.header }
func (w *videoRecordWriter) Status() int         { return w.status }
func (w *videoRecordWriter) Written() bool       { return w.written }
func (w *videoRecordWriter) Size() int {
	if !w.written {
		return -1
	}
	return w.body.Len()
}
func (w *videoRecordWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}
func (w *videoRecordWriter) WriteHeaderNow() { w.written = true }
func (w *videoRecordWriter) Write(p []byte) (int, error) {
	w.WriteHeaderNow()
	return w.body.Write(p)
}
func (w *videoRecordWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *videoRecordWriter) Flush()                            { w.WriteHeaderNow() }
func (w *videoRecordWriter) publish() {
	if !w.written {
		return
	}
	for k := range w.ResponseWriter.Header() {
		w.ResponseWriter.Header().Del(k)
	}
	for k, v := range w.header {
		w.ResponseWriter.Header()[k] = v
	}
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write(w.body.Bytes())
}

func forwardRecordedVideo(c *gin.Context, recorder videoTaskRecorder, endpoint service.GrokMediaEndpoint,
	key *service.APIKey, account *service.Account, subscription *service.UserSubscription,
	pending service.GrokVideoPendingBilling, taskID string,
	forward func() (*service.OpenAIForwardResult, error),
) (*service.OpenAIForwardResult, error) {
	create := isGrokVideoCreateEndpoint(endpoint)
	if !create && !endpoint.IsVideoLookupRequest() {
		return forward()
	}
	var record *service.VideoTask
	if create {
		var err error
		record, err = recorder.BeginVideoTaskRecord(c.Request.Context(), key, account, subscription, pending)
		if err != nil {
			service.MarkResponseCommitted(c)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "video_task_unavailable", "message": "Video task tracking is unavailable; no video was submitted"}})
			return nil, err
		}
	}
	original := c.Writer
	var buffered *videoRecordWriter
	if endpoint != service.GrokMediaEndpointVideoContent {
		buffered = &videoRecordWriter{ResponseWriter: original, header: original.Header().Clone(), status: http.StatusOK}
		c.Writer = buffered
		defer func() { c.Writer = original }()
	}
	result, err := forward()
	c.Writer = original
	if err != nil {
		if buffered != nil {
			buffered.publish()
		}
		if create {
			var limited *service.UpstreamFailoverError
			if errors.As(err, &limited) && (limited.IsCredentialFailure() || limited.StatusCode == http.StatusTooManyRequests) {
				// Pre-submit credential failure and explicit throttling retain
				// their existing bounded failover without changing billing.
				return result, err
			}
			// A provider error is NOT proof that no billable task was created.
			// Keep the pre-submit row and prevent ambiguous automatic failover.
			var unknown *service.OpenAIVideoCreateUnknownError
			if errors.As(err, &unknown) {
				return result, err
			}
			return result, &service.OpenAIVideoCreateUnknownError{Cause: err}
		}
		return result, err
	}
	state := service.VideoTaskPending
	if result != nil && result.VideoCount > 0 {
		state = service.VideoTaskCompleted
	}
	if buffered != nil {
		switch gjson.GetBytes(buffered.body.Bytes(), "status").String() {
		case "failed", "cancelled", "canceled":
			state = service.VideoTaskFailed
		case "succeeded":
			if endpoint.IsSeedance() {
				state = service.VideoTaskCompleted
			}
		}
	}
	// Finish the record even after client disconnection, without retaining its
	// cancellation. No network call or billing occurs in these bounded writes.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Second)
	defer cancel()
	if create {
		if result == nil || result.ResponseID == "" {
			return nil, &service.OpenAIVideoCreateUnknownError{Cause: errors.New("video response missing task id")}
		}
		if result.VideoResolution != "" {
			record.Pending.VideoResolution = result.VideoResolution
		}
		if result.VideoDurationSeconds > 0 {
			record.Pending.VideoDurationSeconds = result.VideoDurationSeconds
		}
		record.Pending.UpstreamModel = result.UpstreamModel
		if err := recorder.AcceptVideoTaskRecord(saveCtx, record, result.ResponseID, state); err != nil {
			return nil, &service.OpenAIVideoCreateUnknownError{TaskID: result.ResponseID, Cause: err}
		}
	} else if err := recorder.ObserveVideoTaskRecord(saveCtx, taskID, key.UserID, key.ID, account.ID, state); err != nil {
		// A status bookkeeping failure must not suppress the existing usage
		// settlement, especially after content has already been streamed.
		logger.LegacyPrintf("handler.video_task", "video task observation deferred user=%d key=%d account=%d", key.UserID, key.ID, account.ID)
	}
	if buffered != nil {
		buffered.publish()
	}
	return result, nil
}
