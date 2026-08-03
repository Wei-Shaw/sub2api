package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplayCyberPolicyRequest_NotFoundWhenNoPayload(t *testing.T) {
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		repo, nil, nil, nil, nil, nil, nil,
	)

	result, err := svc.ReplayCyberPolicyRequest(context.Background(), 123)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "未留存")
}

func TestReplayCyberPolicyRequest_ErrorsWhenExtractedTextEmpty(t *testing.T) {
	repo := &contentModerationTestRepo{}
	require.NoError(t, repo.CreateRequestPayload(context.Background(), &CyberPolicyRequestPayload{
		ModerationLogID: 5,
		Protocol:        ContentModerationProtocolOpenAIChat,
		RequestBody:     `{"messages":[]}`,
	}))
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		repo, nil, nil, nil, nil, nil, nil,
	)

	result, err := svc.ReplayCyberPolicyRequest(context.Background(), 5)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "可审核")
}

func TestReplayCyberPolicyRequest_ErrorsWhenNoAPIKeyConfigured(t *testing.T) {
	repo := &contentModerationTestRepo{}
	require.NoError(t, repo.CreateRequestPayload(context.Background(), &CyberPolicyRequestPayload{
		ModerationLogID: 6,
		Protocol:        ContentModerationProtocolOpenAIChat,
		RequestBody:     `{"messages":[{"role":"user","content":"hello there"}]}`,
	}))
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		repo, nil, nil, nil, nil, nil, nil,
	)

	result, err := svc.ReplayCyberPolicyRequest(context.Background(), 6)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "API Key")
}

func TestGetCyberPolicyRequest_ReturnsStoredPayload(t *testing.T) {
	repo := &contentModerationTestRepo{}
	require.NoError(t, repo.CreateRequestPayload(context.Background(), &CyberPolicyRequestPayload{
		ModerationLogID: 8,
		Protocol:        ContentModerationProtocolOpenAIResponses,
		RequestBody:     `{"input":"hi"}`,
		BodyBytes:       14,
	}))
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		repo, nil, nil, nil, nil, nil, nil,
	)

	payload, err := svc.GetCyberPolicyRequest(context.Background(), 8)

	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, `{"input":"hi"}`, payload.RequestBody)
	require.Equal(t, ContentModerationProtocolOpenAIResponses, payload.Protocol)
}

func TestGetCyberPolicyRequest_NotFound(t *testing.T) {
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{}},
		repo, nil, nil, nil, nil, nil, nil,
	)

	payload, err := svc.GetCyberPolicyRequest(context.Background(), 404)

	require.Error(t, err)
	require.Nil(t, payload)
}
