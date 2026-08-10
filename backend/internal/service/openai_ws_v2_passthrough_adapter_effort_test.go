//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWSPassthroughUsageMeta_InitFromFirstFrame_MappedModelCandidate(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"sol","reasoning":{"effort":"max"}}`)

	meta := newOpenAIWSPassthroughUsageMeta("sol", body)
	meta.initFromFirstFrame(&Account{Platform: PlatformOpenAI}, body, "gpt-5.6-sol")

	got := meta.reasoningEffort.Load()
	require.NotNil(t, got, "reasoning effort should be set")
	require.Equal(t, "max", *got, "mapped model gpt-5.6-sol should preserve max")
}

func TestWSPassthroughUsageMeta_InitFromFirstFrame_NonGPT56FallsBackToXHigh(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.4","reasoning":{"effort":"max"}}`)

	meta := newOpenAIWSPassthroughUsageMeta("gpt-5.4", body)
	meta.initFromFirstFrame(&Account{Platform: PlatformOpenAI}, body, "gpt-5.4")

	got := meta.reasoningEffort.Load()
	require.NotNil(t, got)
	require.Equal(t, "xhigh", *got, "non-5.6 model should normalize max to xhigh")
}

func TestWSPassthroughUsageMeta_UpdateFromResponseCreate_MappedModelCandidate(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"sol","reasoning":{"effort":"max"}}`)

	meta := newOpenAIWSPassthroughUsageMeta("sol", body)
	meta.updateFromResponseCreate(&Account{Platform: PlatformOpenAI}, body, "gpt-5.6-sol", "sol")

	got := meta.reasoningEffort.Load()
	require.NotNil(t, got)
	require.Equal(t, "max", *got, "mapped model should preserve max on multi-turn update")
}

func TestWSPassthroughUsageMeta_UltraRequiresOpenAIAccount(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.6-sol","reasoning":{"effort":"ultra"}}`)

	openAI := newOpenAIWSPassthroughUsageMeta("gpt-5.6-sol", body)
	openAI.initFromFirstFrame(&Account{Platform: PlatformOpenAI}, body, "gpt-5.6-sol")
	require.NotNil(t, openAI.reasoningEffort.Load())
	require.Equal(t, "ultra", *openAI.reasoningEffort.Load())

	grok := newOpenAIWSPassthroughUsageMeta("gpt-5.6-sol", body)
	grok.initFromFirstFrame(&Account{Platform: PlatformGrok}, body, "gpt-5.6-sol")
	require.Nil(t, grok.reasoningEffort.Load())
}
