package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsGrokVideoStatusBillable(t *testing.T) {
	t.Parallel()
	require.False(t, IsGrokVideoStatusBillable(nil))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"pending"REDACTED`)))
	require.False(t, IsGrokVideoStatusBillable([]byte(`{"status":"completed"REDACTED`)))
	require.True(t, IsGrokVideoStatusBillable([]byte(`{"status":"completed","video":{"url":"https://vidgen.x.ai/x.mp4"REDACTEDREDACTED`)))
	require.True(t, IsGrokVideoStatusBillable([]byte(`{"url":"https://example.com/v.mp4"REDACTED`)))
	require.True(t, IsGrokVideoStatusBillable([]byte(`{"download_url":"/v1/videos/task/content"REDACTED`)))
REDACTED

func TestExtractGrokVideoBillingFromStatusBodyPrefersUpstreamParams(t *testing.T) {
	t.Parallel()
	pending := &GrokVideoPendingBilling{
		Model:                "pending-model",
		BillingModel:         "pending-billing",
		UpstreamModel:        "pending-upstream",
		VideoResolution:      VideoBillingResolution480P,
		VideoDurationSeconds: 8,
REDACTED
	body := []byte(`{
		"status":"done",
		"model":"status-model",
		"video":{"url":"https://vidgen.x.ai/signed.mp4","duration":12,"resolution":"720p"REDACTED
REDACTED`)
	result := ExtractGrokVideoBillingFromStatusBody(body, pending, "req-1")
	require.NotNil(t, result)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, "status-model", result.Model)
	require.Equal(t, VideoBillingResolution720P, result.VideoResolution)
	require.Equal(t, 12, result.VideoDurationSeconds)
REDACTED

func TestExtractGrokVideoBillingFromStatusBodyFallsBackToPending(t *testing.T) {
	t.Parallel()
	pending := &GrokVideoPendingBilling{
		Model:                "create-model",
		BillingModel:         "create-billing",
		UpstreamModel:        "create-upstream",
		VideoResolution:      VideoBillingResolution1080P,
		VideoDurationSeconds: 10,
REDACTED
	body := []byte(`{"status":"completed","video":{"url":"https://vidgen.x.ai/signed.mp4"REDACTEDREDACTED`)
	result := ExtractGrokVideoBillingFromStatusBody(body, pending, "req-2")
	require.NotNil(t, result)
	require.Equal(t, "create-billing", result.BillingModel)
	require.Equal(t, "create-upstream", result.UpstreamModel)
	require.Equal(t, VideoBillingResolution1080P, result.VideoResolution)
	require.Equal(t, 10, result.VideoDurationSeconds)
REDACTED

func TestGrokMediaUsageFromResponseVideoCreateDoesNotBill(t *testing.T) {
	t.Parallel()
	info := GrokMediaRequestInfo{Model: "grok-imagine-video", Resolution: "720p", DurationSeconds: 10REDACTED
	meta := grokMediaUsageFromResponse(GrokMediaEndpointVideosGenerations, info, []byte(`{"request_id":"v1"REDACTED`))
	require.Equal(t, "v1", meta.ResponseID)
	require.Equal(t, 0, meta.VideoCount)
	require.Equal(t, 10, meta.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution720P, meta.VideoResolution)
REDACTED

func TestGrokMediaUsageFromResponseVideoStatusBillsOnURL(t *testing.T) {
	t.Parallel()
	meta := grokMediaUsageFromResponse(
		GrokMediaEndpointVideoStatus,
		GrokMediaRequestInfo{REDACTED,
		[]byte(`{"status":"completed","video":{"url":"https://vidgen.x.ai/a.mp4","duration":9,"resolution":"480p"REDACTEDREDACTED`),
	)
	require.Equal(t, 1, meta.VideoCount)
	require.Equal(t, 9, meta.VideoDurationSeconds)
	require.Equal(t, VideoBillingResolution480P, meta.VideoResolution)

	pendingOnly := grokMediaUsageFromResponse(
		GrokMediaEndpointVideoStatus,
		GrokMediaRequestInfo{REDACTED,
		[]byte(`{"status":"completed"REDACTED`),
	)
	require.Equal(t, 0, pendingOnly.VideoCount)
REDACTED
