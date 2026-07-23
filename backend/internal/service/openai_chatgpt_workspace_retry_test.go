package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIsChatGPTWorkspaceAccountMismatch(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{
			name:   "declared workspace code",
			status: http.StatusConflict,
			body:   `{"error":{"code":"sa_server_user_does_not_belong_to_workspace","message":"User does not belong to declared workspace"}}`,
			want:   true,
		},
		{
			name:   "nested detail code",
			status: http.StatusConflict,
			body:   `{"detail":{"code":"sa_server_user_does_not_belong_to_workspace"}}`,
			want:   true,
		},
		{
			name:   "nested detail error code",
			status: http.StatusConflict,
			body:   `{"detail":{"error_code":"sa_server_user_does_not_belong_to_workspace"}}`,
			want:   true,
		},
		{
			name:   "workspace membership message",
			status: http.StatusConflict,
			body:   `{"error":{"message":"User user-123 does not belong to declared workspace ws-456"}}`,
			want:   true,
		},
		{
			name:   "same body with bad request status",
			status: http.StatusBadRequest,
			body:   `{"error":{"code":"sa_server_user_does_not_belong_to_workspace"}}`,
			want:   false,
		},
		{
			name:   "unrelated conflict",
			status: http.StatusConflict,
			body:   `{"error":{"code":"content_policy_violation","message":"request rejected"}}`,
			want:   false,
		},
		{
			name:   "malformed body",
			status: http.StatusConflict,
			body:   "not-json",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isChatGPTWorkspaceAccountMismatch(tt.status, []byte(tt.body)))
		})
	}
}

func TestOpenAIGatewayServiceRetriesWorkspaceHeaderMismatchWithoutHeader(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"Reply OK","input":[]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusConflict, `{"error":{"code":"sa_server_user_does_not_belong_to_workspace","message":"User user-123 does not belong to declared workspace ws-456"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"id":"resp-workspace-retry","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          5110,
		Name:        "workspace-header-retry",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "stale-workspace",
		},
	}

	result, err := svc.Forward(context.Background(), newOpenAIRejectedFieldTestContext(body), account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "stale-workspace", upstream.requests[0].Header.Get("chatgpt-account-id"))
	require.Empty(t, upstream.requests[1].Header.Get("chatgpt-account-id"))
}

func TestOpenAIGatewayServiceDoesNotRetryUnrelatedConflict(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"Reply OK","input":[]}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusConflict,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"content_policy_violation","message":"request rejected"}}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          5111,
		Name:        "workspace-header-no-retry",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "workspace-id",
		},
	}

	_, err := svc.Forward(context.Background(), newOpenAIRejectedFieldTestContext(body), account, body)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "workspace-id", upstream.requests[0].Header.Get("chatgpt-account-id"))
}

func TestOpenAIGatewayServiceBlocksAccountAfterWorkspaceRetryFails(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"Reply OK","input":[]}`)
	workspaceError := `{"detail":{"error_code":"sa_server_user_does_not_belong_to_workspace","error":"User user-123 does not belong to declared workspace ws-456"}}`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusConflict, workspaceError),
		newOpenAIRejectedFieldTestResponse(http.StatusConflict, workspaceError),
	}}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          5112,
		Name:        "workspace-header-block",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "workspace-id"},
	}

	_, err := svc.Forward(context.Background(), newOpenAIRejectedFieldTestContext(body), account, body)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.Len(t, upstream.requests, 2)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}
