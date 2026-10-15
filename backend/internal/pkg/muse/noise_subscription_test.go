//go:build unit

package muse

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoiseSubscriptionFragmentsAndStandaloneAck(t *testing.T) {
	var decoder NoiseSubscriptionDecoder
	var records []SubscriptionRecord
	emit := func(r SubscriptionRecord) error { records = append(records, r); return nil }
	require.NoError(t, decoder.Feed([]byte(`{"channel":"sidechat","is_thread":true,"message_id":"user-1","reply_to_message_id":"user-1","session_id":"op-1"}`), true, emit))
	ack, err := ParseChatAcknowledgement(records[0].Raw, "op-1")
	require.NoError(t, err)
	require.Equal(t, "user-1", ack.MessageID)
	require.NoError(t, decoder.Feed([]byte("{\"type\":\"event\",\"event\":\"delta.text_append\",\"payload\":{\"text\":\"你好\"}}\n")[:45], false, emit))
	line := []byte("{\"type\":\"event\",\"event\":\"delta.text_append\",\"payload\":{\"text\":\"你好\"}}\n")
	require.NoError(t, decoder.Feed(line[45:], false, emit))
	require.Len(t, records, 2)
	require.Equal(t, "delta.text_append", records[1].Event)
	require.Contains(t, string(records[1].Payload), "你好")
	require.Contains(t, string(records[0].Raw), "user-1")
}

func TestNoiseSubscriptionRejectsMalformedAndOversizedRecords(t *testing.T) {
	for _, data := range [][]byte{[]byte(`[]`), []byte(`{"type":"event","payload":{}}`), []byte("{\"text\":\"\xff\"}"), []byte(strings.Repeat("x", noiseMaxSubscriptionRecord+1)), []byte("not-json\n")} {
		var decoder NoiseSubscriptionDecoder
		err := decoder.Feed(data, true, func(SubscriptionRecord) error { return nil })
		require.ErrorIs(t, err, ErrNoiseProtocol)
		require.ErrorIs(t, decoder.Feed([]byte("{}\n"), false, func(SubscriptionRecord) error { return nil }), ErrNoiseProtocol)
	}
}

func TestNoiseSubscriptionConsumerFailurePoisonsStream(t *testing.T) {
	var decoder NoiseSubscriptionDecoder
	stop := errors.New("operation fenced")
	err := decoder.Feed([]byte("{}\n{}\n"), false, func(SubscriptionRecord) error { return stop })
	require.ErrorIs(t, err, stop)
	require.ErrorIs(t, decoder.Feed(nil, true, func(SubscriptionRecord) error { return nil }), ErrNoiseProtocol)
}

func TestChatAcknowledgementRejectsOtherTasksAndImplicitAcceptance(t *testing.T) {
	for _, data := range []string{`{"ok":true}`, `{"channel":"sidechat","is_thread":true,"message_id":"user-1","reply_to_message_id":"user-1","session_id":"another-op"}`, `{"channel":"main","is_thread":false,"message_id":"user-1","reply_to_message_id":"user-1","session_id":"op-1"}`, `{"channel":"sidechat","is_thread":true,"message_id":"user-1","reply_to_message_id":"different-user","session_id":"op-1"}`} {
		_, err := ParseChatAcknowledgement([]byte(data), "op-1")
		require.ErrorIs(t, err, ErrNoiseProtocol)
	}
}

func TestNoiseRPCWrappersPreserveAcceptanceAndRejections(t *testing.T) {
	ack := `{"channel":"sidechat","is_thread":true,"message_id":"user-1","reply_to_message_id":"user-1","session_id":"op-1"}`
	for _, body := range []string{ack, `{"ok":true,"result":` + ack + `}`, `{"type":"res","status":"ok","payload":` + ack + `}`} {
		parsed, err := ParseChatAcknowledgement([]byte(body), "op-1")
		require.NoError(t, err)
		require.Equal(t, "user-1", parsed.MessageID)
	}
	for _, body := range []string{`{"type":"res","status":"err","payload":` + ack + `}`, `{"type":"res","status":"err","ok":true,"result":` + ack + `}`, `{"ok":false,"result":` + ack + `}`, `{"type":"event","event":"message.user","payload":` + ack + `}`} {
		_, err := ParseChatAcknowledgement([]byte(body), "op-1")
		require.ErrorIs(t, err, ErrNoiseProtocol)
	}
	var decoder NoiseSubscriptionDecoder
	count := 0
	err := decoder.Feed([]byte("{\"type\":\"res\",\"status\":\"ok\",\"payload\":{\"agent_id\":\"agent-1\"}}\n"), false, func(record SubscriptionRecord) error {
		count++
		payload, e := NoiseRPCPayload(record.Raw)
		require.JSONEq(t, `{"agent_id":"agent-1"}`, string(payload))
		return e
	})
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
