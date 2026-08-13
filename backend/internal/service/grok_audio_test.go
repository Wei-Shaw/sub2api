package service

import (
	"context"
	"io"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestBuildGrokVoiceURL_UsesAPIDefaultForCLIProxyBase(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
REDACTED
			"base_url": xai.DefaultCLIBaseURL,
	REDACTED,
REDACTED
	url, err := buildGrokVoiceURL(account, nil, "tts")
REDACTED
	require.Equal(t, xai.DefaultBaseURL+"/tts", url)

	url, err = buildGrokVoiceURL(account, nil, "realtime")
REDACTED
	require.Equal(t, xai.DefaultBaseURL+"/realtime", url)
REDACTED

func TestBuildGrokVoiceURL_EmptyBaseFallsBackToAPI(t *testing.T) {
	account := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
REDACTEDREDACTED,
REDACTED
	url, err := buildGrokVoiceURL(account, nil, "stt")
REDACTED
	require.Equal(t, xai.DefaultBaseURL+"/stt", url)
REDACTED

func TestBuildGrokVoiceURL_RequiresEndpoint(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	_, err := buildGrokVoiceURL(account, nil, "  ")
REDACTED
REDACTED

func TestBuildGrokVoiceURL_EncodesCustomVoicePathSegments(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Type: AccountTypeOAuthREDACTED
	got, err := buildGrokVoiceURL(account, nil, "custom-voices/nlbqfwie/audio")
REDACTED
	require.Equal(t, xai.DefaultBaseURL+"/custom-voices/nlbqfwie/audio", got)

	_, err = buildGrokVoiceURL(account, nil, "custom-voices/../audio")
REDACTED
REDACTED

func TestForwardGrokVoice_RejectsNonGrok(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	_, err := svc.ForwardGrokVoice(context.Background(), nil, &Account{Platform: PlatformOpenAIREDACTED, "tts", []byte(`{REDACTED`), "application/json")
REDACTED
	require.Contains(t, err.Error(), "not supported")
REDACTED

func TestAwaitGrokRealtimeAudioObservedReadsFlagAfterRelayExits(t *testing.T) {
	errCh := make(chan error, 1)
	var observed atomic.Bool
	go func() {
		observed.Store(true)
		errCh <- io.EOF
REDACTED()
	got, err := awaitGrokRealtimeAudioObserved(errCh, &observed)
	require.ErrorIs(t, err, io.EOF)
	require.True(t, got, "audioObserved must be read after the relay returns, not before <-errCh")
REDACTED

func TestGrokRealtimeEventHasAudio(t *testing.T) {
	require.False(t, grokRealtimeEventHasAudio([]byte(`{"type":"session.created"REDACTED`)))
	require.False(t, grokRealtimeEventHasAudio([]byte(`{"type":"response.audio_transcript.delta","delta":"hi"REDACTED`)))
	require.False(t, grokRealtimeEventHasAudio([]byte(`{"type":"response.audio.delta","delta":""REDACTED`)))
	require.True(t, grokRealtimeEventHasAudio([]byte(`{"type":"response.audio.delta","delta":"abc"REDACTED`)))
	require.True(t, grokRealtimeEventHasAudio([]byte(`{"type":"response.output_audio.delta","audio":"abc"REDACTED`)))
REDACTED

func TestForwardGrokVoice_RejectsUnknownEndpoint(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	_, err := svc.ForwardGrokVoice(context.Background(), nil, &Account{Platform: PlatformGrokREDACTED, "unknown", []byte(`{REDACTED`), "application/json")
REDACTED
	require.Contains(t, err.Error(), "unsupported")
REDACTED
