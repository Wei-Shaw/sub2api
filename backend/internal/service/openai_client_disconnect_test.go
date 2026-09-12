package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIFailureAfterDisconnectClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"upstream_524", &UpstreamFailoverError{StatusCode: 524}, true},
		{"already_health_handled", &UpstreamFailoverError{StatusCode: 524, AccountHealthHandled: true}, true},
		{"capacity", &UpstreamFailoverError{StatusCode: 503, RequestScopedTransient: true, ResponseBody: []byte(`{"error":{"message":"Our servers are currently overloaded. Please try again later."}}`)}, true},
		{"provider_scope", &UpstreamFailoverError{StatusCode: 503, Scope: GatewayFailureScopeProvider}, false},
		{"request_scope", &UpstreamFailoverError{StatusCode: 503, Scope: GatewayFailureScopeRequest}, false},
		{"request_transient", &UpstreamFailoverError{StatusCode: 503, RequestScopedTransient: true}, false},
		{"client_canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"transport", errors.New("connection closed"), false},
		{"bad_request", &UpstreamFailoverError{StatusCode: 400}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldReportOpenAIFailureAfterDisconnect(tc.err))
		})
	}
}
