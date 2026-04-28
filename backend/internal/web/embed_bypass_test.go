package web

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldBypassEmbeddedFrontendGeneratedImages(t *testing.T) {
	require.True(t, shouldBypassEmbeddedFrontend("/sub2api/generated-images/foo.png"))
}

func TestShouldBypassEmbeddedFrontendKeepsAdjacentFrontendPaths(t *testing.T) {
	for _, path := range []string{
		"/sub2api/generated-image/foo.png",
		"/sub2api/generated-images",
		"/generated-images/foo.png",
	} {
		require.False(t, shouldBypassEmbeddedFrontend(path), path)
	}
}
