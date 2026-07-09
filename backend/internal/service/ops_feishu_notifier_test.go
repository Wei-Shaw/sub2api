//go:build unit

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignFeishuWebhook(t *testing.T) {
	t.Parallel()

	got := signFeishuWebhook("1700000000", "secret")

	require.NotEmpty(t, got)
	require.NotContains(t, got, "secret")
}

func TestSendOpsAlertFeishuWebhook(t *testing.T) {
	t.Parallel()

	var received opsFeishuWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	t.Cleanup(server.Close)

	value := 12.5
	err := sendOpsAlertFeishuWebhook(t.Context(), OpsFeishuAlertConfig{
		WebhookURL: server.URL,
		Secret:     "secret",
	}, &OpsAlertRule{
		Name:       "High upstream error rate",
		Severity:   "warning",
		MetricType: "upstream_error_rate",
		Operator:   ">",
		Threshold:  5,
	}, &OpsAlertEvent{
		Status:      OpsAlertStatusFiring,
		Title:       "Alert firing",
		Description: "upstream_error_rate > 5",
		MetricValue: &value,
		FiredAt:     time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC),
	})

	require.NoError(t, err)
	require.Equal(t, "text", received.MsgType)
	require.NotEmpty(t, received.Timestamp)
	require.NotEmpty(t, received.Sign)
	require.Contains(t, received.Content.Text, "High upstream error rate")
	require.Contains(t, received.Content.Text, "upstream_error_rate")
}

func TestSendOpsAlertFeishuWebhookRejectsNonZeroCode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":19021,"msg":"bad sign"}`))
	}))
	t.Cleanup(server.Close)

	err := sendOpsAlertFeishuWebhook(t.Context(), OpsFeishuAlertConfig{WebhookURL: server.URL}, &OpsAlertRule{}, &OpsAlertEvent{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "19021")
}
