package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoTaskSessionHashScopesTaskBinding(t *testing.T) {
	got := VideoTaskSessionHash(VideoTaskPlatformOpenAI, "video_123", 7, 9)
	require.Equal(t, "video-task:openai:f643de2fa99d201b", got)
	require.NotEqual(t, got, VideoTaskSessionHash(VideoTaskPlatformGrok, "video_123", 7, 9))
	require.NotEqual(t, got, VideoTaskSessionHash(VideoTaskPlatformOpenAI, "video_123", 8, 9))
	require.NotEqual(t, got, VideoTaskSessionHash(VideoTaskPlatformOpenAI, "video_123", 7, 10))
}

func TestVideoTaskSessionHashRejectsIncompleteIdentity(t *testing.T) {
	require.Empty(t, VideoTaskSessionHash(VideoTaskPlatformOpenAI, "", 7, 9))
	require.Empty(t, VideoTaskSessionHash(VideoTaskPlatformOpenAI, "video_123", 0, 9))
	require.Empty(t, VideoTaskSessionHash(VideoTaskPlatformOpenAI, "video_123", 7, 0))
}
