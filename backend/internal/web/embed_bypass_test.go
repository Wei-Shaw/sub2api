package web

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldBypassEmbeddedFrontendGeneratedImages(t *testing.T) {
	require.True(t, shouldBypassEmbeddedFrontend("/sub2api/generated-images/foo.png"))
}

func TestShouldBypassEmbeddedFrontendConfigGuides(t *testing.T) {
	for _, path := range []string{
		"/config-guides/omp-openai/manifest.json",
		"/config-guides/opencode-openai/opencode.json",
	} {
		require.True(t, shouldBypassEmbeddedFrontend(path), path)
	}
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
