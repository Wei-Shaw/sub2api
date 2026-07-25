package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAsyncImageTaskRead(t *testing.T) {
	require.True(t, isAsyncImageTaskRead(http.MethodGet, "/v1/images/tasks/imgtask_123"))
	require.True(t, isAsyncImageTaskRead(http.MethodGet, "/images/tasks/imgtask_123"))
	require.False(t, isAsyncImageTaskRead(http.MethodPost, "/v1/images/tasks/imgtask_123"))
	require.False(t, isAsyncImageTaskRead(http.MethodGet, "/v1/images/generations"))
}

func TestIsGrokVideoTaskRead(t *testing.T) {
	require.True(t, isGrokVideoTaskRead(http.MethodGet, "/v1/videos/video-request-123"))
	require.True(t, isGrokVideoTaskRead(http.MethodGet, "/v1/videos/video-request-123/content"))
	require.True(t, isGrokVideoTaskRead(http.MethodGet, "/videos/video-request-123"))
	require.False(t, isGrokVideoTaskRead(http.MethodPost, "/v1/videos/generations"))
	require.False(t, isGrokVideoTaskRead(http.MethodGet, "/v1/images/generations"))
}
