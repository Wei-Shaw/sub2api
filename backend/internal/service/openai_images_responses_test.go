package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesImageResultKeyHashesLargeBase64(t *testing.T) {
	large := strings.Repeat("abcd", 1024*1024)
	key := openAIResponsesImageResultKey("item-1", openAIResponsesImageResult{Result: large, OutputFormat: "png", Size: "1024x1024"})
	require.Contains(t, key, "png|1024x1024|")
	require.Len(t, key, len("png|1024x1024|")+64)
	require.NotContains(t, key, large[:128])

	same := openAIResponsesImageResultKey("item-2", openAIResponsesImageResult{Result: large, OutputFormat: "png", Size: "1024x1024"})
	differentSize := openAIResponsesImageResultKey("item-3", openAIResponsesImageResult{Result: large, OutputFormat: "png", Size: "512x512"})
	require.Equal(t, key, same)
	require.NotEqual(t, key, differentSize)
}
