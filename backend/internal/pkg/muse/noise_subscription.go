package muse

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

const noiseMaxSubscriptionRecord = 4 << 20

// SubscriptionRecord is either the initial RPC acknowledgement or a consumer
// app event. Its payload is still untrusted and must be matched to the current
// operation/session by the provider, including proactive and approval events.
type SubscriptionRecord struct {
	Raw     json.RawMessage
	Type    string          `json:"type"`
	Event   string          `json:"event"`
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload"`
}

// NoiseSubscriptionDecoder retains only an incomplete NDJSON record. Feed's
// final flag flushes the initial standalone JSON acknowledgement as well as a
// subscription's final record. A transport end flag is not task completion.
// Use one decoder per stream, confined to its single reader.
type NoiseSubscriptionDecoder struct {
	pending []byte
	dead    bool
}

func (d *NoiseSubscriptionDecoder) Feed(data []byte, final bool, emit func(SubscriptionRecord) error) error {
	if d.dead || emit == nil {
		return d.fail()
	}
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		end := len(data)
		if index >= 0 {
			end = index
		}
		if len(d.pending)+end > noiseMaxSubscriptionRecord {
			return d.fail()
		}
		d.pending = append(d.pending, data[:end]...)
		if index < 0 {
			break
		}
		if err := d.emit(emit); err != nil {
			return err
		}
		data = data[index+1:]
	}
	if final {
		return d.emit(emit)
	}
	return nil
}

func (d *NoiseSubscriptionDecoder) emit(emit func(SubscriptionRecord) error) error {
	line := bytes.TrimSpace(d.pending)
	if len(line) == 0 {
		d.pending = d.pending[:0]
		return nil
	}
	if !utf8.Valid(line) || line[0] != '{' {
		return d.fail()
	}
	var record SubscriptionRecord
	if json.Unmarshal(line, &record) != nil {
		return d.fail()
	}
	if record.Type != "" && record.Type != "event" && record.Type != "res" {
		return d.fail()
	}
	if record.Type == "event" && (record.Event == "" || len(record.Event) > 256 || len(record.Payload) == 0) {
		return d.fail()
	}
	if record.Type == "res" && record.Status != "ok" && record.Status != "err" {
		return d.fail()
	}
	record.Raw = append([]byte(nil), line...)
	// Separate ownership from the retained buffer before the next feed.
	d.pending = d.pending[:0]
	if err := emit(record); err != nil {
		d.dead = true
		d.pending = nil
		return err
	}
	return nil
}
func (d *NoiseSubscriptionDecoder) fail() error {
	d.dead = true
	d.pending = nil
	return ErrNoiseProtocol
}

type ChatAcknowledgement struct {
	Channel          string `json:"channel"`
	IsThread         bool   `json:"is_thread"`
	MessageID        string `json:"message_id"`
	ReplyToMessageID string `json:"reply_to_message_id"`
	SessionID        string `json:"session_id"`
}

// ParseChatAcknowledgement binds definite acceptance to the fresh side-chat
// operation. An HTTP success or first output event is not a substitute for it.
func ParseChatAcknowledgement(data []byte, expectedSessionID string) (*ChatAcknowledgement, error) {
	var ack ChatAcknowledgement
	payload, err := NoiseRPCPayload(data)
	if err != nil || !validID(expectedSessionID, 256) || json.Unmarshal(payload, &ack) != nil || !ack.IsThread || ack.SessionID != expectedSessionID || !validID(ack.MessageID, 256) || ack.ReplyToMessageID != ack.MessageID || !validID(ack.Channel, 256) {
		return nil, ErrNoiseProtocol
	}
	return &ack, nil
}

// NoiseRPCPayload recognizes the app's two observed RPC wrappers as well as
// direct acknowledgement objects. Failed or event envelopes are not acceptance.
func NoiseRPCPayload(data []byte) (json.RawMessage, error) {
	if len(data) > noiseMaxSubscriptionRecord || !utf8.Valid(data) {
		return nil, ErrNoiseProtocol
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return nil, ErrNoiseProtocol
	}
	var kind, status string
	if raw, ok := object["type"]; ok {
		if json.Unmarshal(raw, &kind) != nil {
			return nil, ErrNoiseProtocol
		}
	}
	if raw, ok := object["status"]; ok {
		if json.Unmarshal(raw, &status) != nil {
			return nil, ErrNoiseProtocol
		}
	}
	if kind == "event" || kind != "" && kind != "res" {
		return nil, ErrNoiseProtocol
	}
	if status == "err" || (kind == "res" && status != "ok") {
		return nil, ErrNoiseProtocol
	}
	if _, ok := object["error"]; ok {
		return nil, ErrNoiseProtocol
	}
	if raw, ok := object["ok"]; ok {
		var allowed bool
		if json.Unmarshal(raw, &allowed) != nil || !allowed {
			return nil, ErrNoiseProtocol
		}
		if result, ok := object["result"]; ok && len(result) > 0 {
			return result, nil
		}
	}
	if kind == "res" {
		if status != "ok" || len(object["payload"]) == 0 {
			return nil, ErrNoiseProtocol
		}
		return object["payload"], nil
	}
	return append(json.RawMessage(nil), data...), nil
}
