package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func withOpenAIIntegrityObservation(t testing.TB, enabled bool) {
	t.Helper()
	previous, _ := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting)
	t.Cleanup(func() {
		if previous == nil {
			previous = &cachedOpenAIAdvancedSchedulerSetting{}
		}
		openAIAdvancedSchedulerSettingCache.Store(previous)
	})
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		requestIntegrityObserveEnabled: enabled,
		expiresAt:                      time.Now().Add(time.Hour).UnixNano(),
	})
}

func TestOpenAIIntegritySnapshotAdmissionAndIsolation(t *testing.T) {
	svc := &OpenAIGatewayService{}
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			withOpenAIIntegrityObservation(t, enabled)
			for _, account := range []*Account{nil, oauth, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {Platform: PlatformGrok, Type: AccountTypeOAuth}} {
				body := []byte(`{"input":"before","nonce":9007199254740993}`)
				snapshot := svc.captureOpenAIRequestIntegritySnapshot(context.Background(), account, body)
				if account == oauth && enabled {
					require.Equal(t, body, snapshot)
					body[10] = 'X'
					require.Equal(t, `{"input":"before","nonce":9007199254740993}`, string(snapshot))
				} else {
					require.Nil(t, snapshot)
				}
			}
			require.Nil(t, svc.captureOpenAIRequestIntegritySnapshot(context.Background(), oauth, nil))
			require.Nil(t, svc.captureOpenAIRequestIntegritySnapshot(context.Background(), oauth, make([]byte, openAIRequestIntegrityMaxObservedBytes+1)))
		})
	}
}

func TestOpenAIIntegritySnapshotDetectsChangesAndSkipsMissingCapture(t *testing.T) {
	withOpenAIIntegrityObservation(t, true)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	snapshot := svc.captureOpenAIRequestIntegritySnapshot(context.Background(), account, []byte(`{"input":"before"}`))
	svc.observeOpenAIRequestIntegrity(context.Background(), c, account, snapshot, []byte(`{"input":"after"}`), "http")
	signature, exists := c.Get(openAIRequestIntegrityDifferencesContextKey)
	require.True(t, exists)
	require.Equal(t, "input|http", signature)
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	svc.observeOpenAIRequestIntegrity(context.Background(), c, account, nil, []byte(`{"input":"after"}`), "http")
	_, exists = c.Get(openAIRequestIntegrityDifferencesContextKey)
	require.False(t, exists)
}

var benchmarkOpenAIIntegritySnapshot []byte

func BenchmarkOpenAIIntegritySnapshotAdmission(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		b.Run(fmt.Sprintf("enabled_%v", enabled), func(b *testing.B) {
			withOpenAIIntegrityObservation(b, enabled)
			svc := &OpenAIGatewayService{}
			for _, kind := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
				account := &Account{Platform: PlatformOpenAI, Type: kind}
				for _, size := range []int{1 << 20, 16 << 20} {
					body := bytes.Repeat([]byte{'x'}, size)
					b.Run(fmt.Sprintf("%s/%dMiB", kind, size>>20), func(b *testing.B) {
						b.ReportAllocs()
						for range b.N {
							benchmarkOpenAIIntegritySnapshot = svc.captureOpenAIRequestIntegritySnapshot(context.Background(), account, body)
						}
					})
				}
			}
		})
	}
}

func TestOpenAIRequestIntegrityTreatsEquivalentInputShapesAsEqual(t *testing.T) {
	original := []byte(`{"model":"gpt","input":"hello"}`)
	forwarded := []byte(`{"model":"mapped","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}],"instructions":"default"}`)
	differences, err := openAIRequestIntegrityDifferences(original, forwarded)
	require.NoError(t, err)
	require.Empty(t, differences)
}

func TestOpenAIRequestIntegrityReportsProtectedFieldNamesOnly(t *testing.T) {
	original := []byte(`{"input":[{"type":"message","role":"user","content":"hello"}],"tools":[{"type":"function","name":"lookup"}],"previous_response_id":"resp_1"}`)
	forwarded := []byte(`{"input":[{"type":"message","role":"user","content":"hello"}],"tools":[],"previous_response_id":"resp_2"}`)
	differences, err := openAIRequestIntegrityDifferences(original, forwarded)
	require.NoError(t, err)
	require.Equal(t, []string{"previous_response_id", "tools"}, differences)
}
