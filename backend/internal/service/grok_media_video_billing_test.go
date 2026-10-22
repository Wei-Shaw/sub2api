package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGrokVideoE2EDurationFromCreatedAt(t *testing.T) {
	t.Parallel()
	created := time.Now().UTC().Add(-45 * time.Second)
	d := GrokVideoE2EDuration(created.Format(time.RFC3339Nano), time.Now().UTC())
	require.GreaterOrEqual(t, d, 44*time.Second)
	require.LessOrEqual(t, d, 47*time.Second)

	require.Equal(t, time.Duration(0), GrokVideoE2EDuration("", time.Now()))
	require.Equal(t, time.Duration(0), GrokVideoE2EDuration("not-a-time", time.Now()))
	// Future CreatedAt clamps to zero (clock skew).
	require.Equal(t, time.Duration(0), GrokVideoE2EDuration(time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), time.Now()))
}

func TestGrokVideoPendingCreatedAtStampOnStoreShape(t *testing.T) {
	t.Parallel()
	// GrokVideoPendingCreatedAtNow must be parseable by GrokVideoE2EDuration.
	stamp := GrokVideoPendingCreatedAtNow()
	require.NotEmpty(t, stamp)
	d := GrokVideoE2EDuration(stamp, time.Now().UTC().Add(2*time.Second))
	require.GreaterOrEqual(t, d, time.Second)
	require.LessOrEqual(t, d, 3*time.Second)
}

func TestIsGrokVideoStatusBillable(t *testing.T) {
	t.Parallel()
	// Official xAI success: status=done + video.url
	require.True(t, IsGrokVideoStatusBillable([]byte(`{
		"status":"done",
		"model":"grok-imagine-video-1.5",
		"video":{"url":"https://vidgen.x.ai/x.mp4","duration":8,"respect_moderation":true}
	}`)))

	// Official non-success states
	require.False(t, IsGrokVideoStatusBillable(nil))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"pending"}`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"queued","video_url":"https://example.com/v.mp4"}`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"running","video_url":"https://example.com/v.mp4"}`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"expired"}`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"failed","error":{"code":"task_failed"}}`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"cancelled","video_url":"https://example.com/v.mp4"}`)))
	// Success status without any URL is not billable.
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"done"}`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"completed"}`)))
	// URL alone (no status) is not enough.
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"url":"https://example.com/v.mp4"}`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"download_url":"/v1/videos/task/content"}`)))
}

// [OI]/Ark relays report "completed" and put the file in a top-level field instead
// of xAI's video.url. Both vocabularies must be billable or finished videos are free.
func TestIsGrokVideoStatusBillableAcceptsRelayShapes(t *testing.T) {
	t.Parallel()
	const url = "https://cos.example.com/media/abc.mp4"
	cases := map[string]string{
		"completed + video_url":     `{"status":"completed","video_url":"` + url + `"}`,
		"completed + url":           `{"status":"completed","url":"` + url + `"}`,
		"completed + result_url":    `{"status":"completed","result_url":"` + url + `"}`,
		"completed + metadata.url":  `{"status":"completed","metadata":{"url":"` + url + `"}}`,
		"completed + content field": `{"status":"completed","content":{"video_url":"` + url + `"}}`,
		"completed + data array":    `{"status":"completed","data":[{"url":"` + url + `"}]}`,
		"succeeded + video_url":     `{"status":"succeeded","video_url":"` + url + `"}`,
		"uppercase status":          `{"status":"COMPLETED","video_url":"` + url + `"}`,
	}
	for name, body := range cases {
		require.True(t, IsGrokVideoStatusBillable([]byte(body)), name)
	}
}

// The live iliu.ai body that was silently unbilled: status=completed, top-level URLs,
// no video.duration, no video.url.
func TestExtractGrokVideoBillingFromIliuStatusBody(t *testing.T) {
	t.Parallel()
	const url = "https://iliu-ai-1446793020.cos.ap-guangzhou.myqcloud.com/media/abc.mp4"
	body := []byte(`{"id":"task_E7ofFSxw0PvWPhjuNRpx5B8YqW6kMlcP","task_id":"task_E7ofFSxw0PvWPhjuNRpx5B8YqW6kMlcP",
		"object":"video","model":"seedance-2.5-M","status":"completed","progress":100,
		"metadata":{"url":"` + url + `"},"video_url":"` + url + `","result_url":"` + url + `","url":"` + url + `",
		"content":{"video_url":"` + url + `"},"data":[{"url":"` + url + `"}]}`)
	pending := &GrokVideoPendingBilling{
		Model:                "seedance-2.5-M",
		BillingModel:         "seedance-2.5-M",
		UpstreamModel:        "seedance-2.5-M",
		VideoResolution:      VideoBillingResolution720P,
		VideoDurationSeconds: 6,
	}
	result := ExtractGrokVideoBillingFromStatusBody(body, pending, "task_E7ofFSxw0PvWPhjuNRpx5B8YqW6kMlcP")
	require.NotNil(t, result)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, "seedance-2.5-M", result.Model)
	require.Equal(t, "seedance-2.5-M", result.BillingModel)
	// Status carries no duration/resolution — both come from the create-time snapshot.
	require.Equal(t, 6, result.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
	require.Equal(t, "task_E7ofFSxw0PvWPhjuNRpx5B8YqW6kMlcP", result.ResponseID)
}

func TestGrokVideoStatusDurationAndResolutionFallbacks(t *testing.T) {
	t.Parallel()
	// Top-level duration and resolution are accepted when the status reports them.
	body := []byte(`{"status":"completed","video_url":"https://example.com/v.mp4","duration":10,"resolution":"1080p"}`)
	require.Equal(t, 10, grokVideoStatusDurationSeconds(body))
	require.Equal(t, "1080p", grokVideoStatusResolution(body))

	// video.duration still wins over the top-level field (xAI official shape).
	official := []byte(`{"status":"done","video":{"url":"https://example.com/v.mp4","duration":12},"duration":3}`)
	require.Equal(t, 12, grokVideoStatusDurationSeconds(official))

	// Absent fields report unknown, never a fabricated value.
	require.Equal(t, 0, grokVideoStatusDurationSeconds([]byte(`{"status":"completed"}`)))
	require.Equal(t, "", grokVideoStatusResolution([]byte(`{"status":"completed"}`)))

	// Snapshot resolution is authoritative over a status-reported one.
	pending := &GrokVideoPendingBilling{VideoResolution: VideoBillingResolution480P, VideoDurationSeconds: 5}
	result := ExtractGrokVideoBillingFromStatusBody(
		[]byte(`{"status":"completed","video_url":"https://example.com/v.mp4","resolution":"1080p","duration":9}`),
		pending, "req",
	)
	require.NotNil(t, result)
	require.Equal(t, VideoBillingResolution480P, result.VideoResolution)
	// Status duration still wins over the snapshot when present.
	require.Equal(t, 9, result.VideoDurationSeconds)
}

func TestExtractGrokVideoBillingFromStatusBodyPrefersUpstreamParams(t *testing.T) {
	t.Parallel()
	pending := &GrokVideoPendingBilling{
		Model:                "pending-model",
		BillingModel:         "pending-billing",
		UpstreamModel:        "pending-upstream",
		VideoResolution:      VideoBillingResolution720P,
		VideoDurationSeconds: 8,
	}
	// Official completed body from docs.x.ai Video Generation.
	body := []byte(`{
		"status":"done",
		"model":"grok-imagine-video-1.5",
		"video":{"url":"https://vidgen.x.ai/signed.mp4","duration":12,"respect_moderation":true}
	}`)
	result := ExtractGrokVideoBillingFromStatusBody(body, pending, "req-1")
	require.NotNil(t, result)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, "grok-imagine-video-1.5", result.Model)
	// Resolution is not in official status response — use create-time request.
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
	// Duration prefers official video.duration.
	require.Equal(t, 12, result.VideoDurationSeconds)
}

func TestExtractGrokVideoBillingFromStatusBodyFallsBackToPending(t *testing.T) {
	t.Parallel()
	pending := &GrokVideoPendingBilling{
		Model:                "create-model",
		BillingModel:         "create-billing",
		UpstreamModel:        "create-upstream",
		VideoResolution:      VideoBillingResolution1080P,
		VideoDurationSeconds: 10,
	}
	// done + video.url, but no model/duration in body.
	body := []byte(`{"status":"done","video":{"url":"https://vidgen.x.ai/signed.mp4"}}`)
	result := ExtractGrokVideoBillingFromStatusBody(body, pending, "req-2")
	require.NotNil(t, result)
	require.Equal(t, "create-billing", result.BillingModel)
	require.Equal(t, "create-upstream", result.UpstreamModel)
	require.Equal(t, VideoBillingResolution1080P, result.VideoResolution)
	require.Equal(t, 10, result.VideoDurationSeconds)
}

func TestExtractGrokVideoBillingRejectsNonDoneStatus(t *testing.T) {
	t.Parallel()
	pending := &GrokVideoPendingBilling{Model: "m", VideoDurationSeconds: 8, VideoResolution: "720p"}
	require.Nil(t, ExtractGrokVideoBillingFromStatusBody(
		[]byte(`{"status":"pending","video":{"url":"https://vidgen.x.ai/x.mp4","duration":8}}`),
		pending, "req",
	))
	require.Nil(t, ExtractGrokVideoBillingFromStatusBody(
		[]byte(`{"status":"running","video_url":"https://vidgen.x.ai/x.mp4","duration":8}`),
		pending, "req",
	))
	require.Nil(t, ExtractGrokVideoBillingFromStatusBody(
		[]byte(`{"status":"failed","video_url":"https://vidgen.x.ai/x.mp4","duration":8}`),
		pending, "req",
	))
	// A finished status with no URL anywhere is still unbillable.
	require.Nil(t, ExtractGrokVideoBillingFromStatusBody(
		[]byte(`{"status":"completed","progress":100}`),
		pending, "req",
	))
}

func TestGrokMediaUsageFromResponseVideoCreateDoesNotBill(t *testing.T) {
	t.Parallel()
	info := GrokMediaRequestInfo{Model: "grok-imagine-video", Resolution: "720p", DurationSeconds: 10}
	meta := grokMediaUsageFromResponse(GrokMediaEndpointVideosGenerations, info, []byte(`{"request_id":"v1"}`))
	require.Equal(t, "v1", meta.ResponseID)
	require.Equal(t, 0, meta.VideoCount)
	require.Equal(t, 10, meta.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution720P, meta.VideoResolution)
}

func TestGrokMediaUsageFromResponseVideoStatusBillsOnOfficialDone(t *testing.T) {
	t.Parallel()
	meta := grokMediaUsageFromResponse(
		GrokMediaEndpointVideoStatus,
		GrokMediaRequestInfo{},
		[]byte(`{"status":"done","model":"grok-imagine-video-1.5","video":{"url":"https://vidgen.x.ai/a.mp4","duration":9}}`),
	)
	require.Equal(t, 1, meta.VideoCount)
	require.Equal(t, 9, meta.VideoDurationSeconds)
	require.Equal(t, "grok-imagine-video-1.5", meta.Model)

	// Official non-done must not set billable units.
	pendingOnly := grokMediaUsageFromResponse(
		GrokMediaEndpointVideoStatus,
		GrokMediaRequestInfo{},
		[]byte(`{"status":"pending"}`),
	)
	require.Equal(t, 0, pendingOnly.VideoCount)

	// Relay shape: status=completed with a top-level URL is billable too.
	completed := grokMediaUsageFromResponse(
		GrokMediaEndpointVideoStatus,
		GrokMediaRequestInfo{},
		[]byte(`{"status":"completed","model":"seedance-2.5-M","video_url":"https://example.com/a.mp4"}`),
	)
	require.Equal(t, 1, completed.VideoCount)
	require.Equal(t, "seedance-2.5-M", completed.Model)
}
