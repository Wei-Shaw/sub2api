package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type grokMediaEligibilityProberStub struct {
	eligible bool
	reason   string
	err      error
	calls    int
REDACTED

func (s *grokMediaEligibilityProberStub) ProbeMediaEligibility(context.Context, int64) (bool, string, error) {
	s.calls++
	return s.eligible, s.reason, s.err
REDACTED

func TestShouldRecordGrokMediaUsage(t *testing.T) {
	tests := []struct {
		name     string
		endpoint service.GrokMediaEndpoint
		model    string
		want     bool
REDACTED{
		{
			name:     "image generation records usage",
			endpoint: service.GrokMediaEndpointImagesGenerations,
			model:    "grok-imagine",
			want:     true,
	REDACTED,
		{
			name:     "image edit records usage",
			endpoint: service.GrokMediaEndpointImagesEdits,
			model:    "grok-imagine-edit",
			want:     true,
	REDACTED,
		{
			name:     "video generation records usage",
			endpoint: service.GrokMediaEndpointVideosGenerations,
			model:    "grok-imagine-video-1.5",
			want:     true,
	REDACTED,
		{
			name:     "video status skips empty model usage",
			endpoint: service.GrokMediaEndpointVideoStatus,
			model:    "",
			want:     false,
	REDACTED,
		{
			name:     "generation skips usage without model",
			endpoint: service.GrokMediaEndpointImagesGenerations,
			model:    " ",
			want:     false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldRecordGrokMediaUsage(tt.endpoint, tt.model))
	REDACTED)
REDACTED
REDACTED

func TestGrokMediaRequiredCapability(t *testing.T) {
	tests := []struct {
		name     string
		endpoint service.GrokMediaEndpoint
		want     service.OpenAIEndpointCapability
REDACTED{
		{name: "image generation", endpoint: service.GrokMediaEndpointImagesGenerations, want: service.OpenAIEndpointCapabilityGrokMediaGenerationREDACTED,
		{name: "image edit", endpoint: service.GrokMediaEndpointImagesEdits, want: service.OpenAIEndpointCapabilityGrokMediaGenerationREDACTED,
		{name: "video generation", endpoint: service.GrokMediaEndpointVideosGenerations, want: service.OpenAIEndpointCapabilityGrokMediaGenerationREDACTED,
		{name: "video edit", endpoint: service.GrokMediaEndpointVideosEdits, want: service.OpenAIEndpointCapabilityGrokMediaGenerationREDACTED,
		{name: "video extension", endpoint: service.GrokMediaEndpointVideosExtensions, want: service.OpenAIEndpointCapabilityGrokMediaGenerationREDACTED,
		{name: "video status preserves lookup", endpoint: service.GrokMediaEndpointVideoStatus, want: ""REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, grokMediaRequiredCapability(tt.endpoint))
	REDACTED)
REDACTED
REDACTED

func TestGrokMediaScheduleModelUsesNormalizedMappedUpstream(t *testing.T) {
	account := &service.Account{
		Platform: service.PlatformGrok,
REDACTED
			"model_mapping": map[string]any{
				"grok-imagine-video-1.5": "wrong-raw-model",
				"grok-imagine-video":     "mapped-video-model",
		REDACTED,
	REDACTED,
REDACTED

	require.Equal(t, "mapped-video-model", grokMediaScheduleModel(account, "grok-imagine-video", nil))
	require.Equal(t, "actual-upstream-model", grokMediaScheduleModel(account, "grok-imagine-video", &service.OpenAIForwardResult{
		UpstreamModel: "actual-upstream-model",
REDACTED))
	require.Equal(t, "mapped-video-model", grokMediaScheduleModel(account, "grok-imagine-video", &service.OpenAIForwardResult{REDACTED))
	require.Equal(t, "grok-imagine-video", grokMediaScheduleModel(nil, " grok-imagine-video ", nil))
REDACTED

func TestEnsureGrokMediaAccountEligibility(t *testing.T) {
	t.Run("non oauth account does not probe", func(t *testing.T) {
		prober := &grokMediaEligibilityProberStub{REDACTED
		h := &OpenAIGatewayHandler{grokMediaEligibilityProber: proberREDACTED
		account := &service.Account{Platform: service.PlatformGrok, Type: service.AccountTypeAPIKeyREDACTED

		eligible, reason, err := h.ensureGrokMediaAccountEligibility(context.Background(), account)

	REDACTED
		require.True(t, eligible)
		require.Equal(t, "non_oauth", reason)
		require.Zero(t, prober.calls)
REDACTED)

	t.Run("unobserved oauth is probed before forwarding", func(t *testing.T) {
		prober := &grokMediaEligibilityProberStub{eligible: true, reason: "eligible"REDACTED
		h := &OpenAIGatewayHandler{grokMediaEligibilityProber: proberREDACTED
		account := &service.Account{ID: 7, Platform: service.PlatformGrok, Type: service.AccountTypeOAuthREDACTED

		eligible, reason, err := h.ensureGrokMediaAccountEligibility(context.Background(), account)

	REDACTED
		require.True(t, eligible)
		require.Equal(t, "eligible", reason)
		require.Equal(t, 1, prober.calls)
REDACTED)

	t.Run("missing prober fails closed", func(t *testing.T) {
		h := &OpenAIGatewayHandler{REDACTED
		account := &service.Account{ID: 8, Platform: service.PlatformGrok, Type: service.AccountTypeOAuthREDACTED

		eligible, reason, err := h.ensureGrokMediaAccountEligibility(context.Background(), account)

	REDACTED
		require.False(t, eligible)
		require.Equal(t, "billing_probe_unavailable", reason)
REDACTED)

	t.Run("probe failure fails closed", func(t *testing.T) {
		probeErr := errors.New("probe failed")
		prober := &grokMediaEligibilityProberStub{reason: "billing_unobserved", err: probeErrREDACTED
		h := &OpenAIGatewayHandler{grokMediaEligibilityProber: proberREDACTED
		account := &service.Account{ID: 9, Platform: service.PlatformGrok, Type: service.AccountTypeOAuthREDACTED

		eligible, reason, err := h.ensureGrokMediaAccountEligibility(context.Background(), account)

		require.ErrorIs(t, err, probeErr)
		require.False(t, eligible)
		require.Equal(t, "billing_unobserved", reason)
REDACTED)
REDACTED
