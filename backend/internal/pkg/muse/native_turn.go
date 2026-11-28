package muse

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type nativeTranscript struct {
	AgentID   string `json:"agent_id"`
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
	Messages  []struct {
		ID      string `json:"id"`
		Role    string `json:"role"`
		Status  string `json:"status"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
	Metadata struct {
		Model string `json:"model"`
	} `json:"metadata"`
}

type nativePayload struct {
	SessionID      string           `json:"session_id"`
	AgentID        string           `json:"agent_id"`
	ParentAgentID  string           `json:"parent_agent_id"`
	RootAgentID    string           `json:"root_agent_id"`
	TaskID         string           `json:"task_id"`
	MessageID      string           `json:"message_id"`
	IsThread       bool             `json:"is_thread"`
	MessageSeq     int64            `json:"message_seq"`
	ChatEventSeq   int64            `json:"chat_event_seq"`
	Seq            int64            `json:"seq"`
	Status         string           `json:"status"`
	Text           string           `json:"text"`
	Role           string           `json:"role"`
	ActiveSubagent *int             `json:"active_subagent_count"`
	Transcript     nativeTranscript `json:"transcript"`
}

type nativeMessage struct {
	id           string
	canonicalSeq int64
	started      bool
	version      int64
	text         string
	pending      map[int64]nativePayload
	done         *nativePayload
}

// The app may replay completed transcripts without their deltas, and replay
// records may follow a newer task snapshot. Correlation uses the isolated side
// chat/root agent, the sole acknowledged user message, canonical adjacency,
// and a terminal task snapshot for that request. Task termination can precede
// delivery of the final transcript; both records remain necessary for results.
type nativeTurn struct {
	sessionID, userID, rootAgent, providerID, operationID, model string
	userSeq                                                      int64
	message                                                      *nativeMessage
	taskStatus                                                   string
	taskSeq                                                      int64
	activeSubagents                                              int
	sequence                                                     int64
	createdAt                                                    int64
	outputStarted                                                bool
	seen                                                         map[string][32]byte
	retained                                                     int
	emit                                                         func(Event) error
	result                                                       *Result
}

func newNativeTurn(sessionID, userID, model, providerID, operationID string, emit func(Event) error) *nativeTurn {
	return &nativeTurn{sessionID: sessionID, userID: userID, model: model, providerID: providerID, operationID: operationID, emit: emit, createdAt: time.Now().Unix(), seen: map[string][32]byte{}}
}

func (t *nativeTurn) emitData(data apicompat.ResponsesStreamEvent) error {
	if t.emit == nil {
		return nil
	}
	t.sequence++
	data.SequenceNumber = int(t.sequence - 1)
	return t.emit(Event{OperationID: t.operationID, Sequence: t.sequence, ProviderTurnID: t.providerID, Data: data})
}

func (t *nativeTurn) baseResponse(status string) *apicompat.ResponsesResponse {
	return &apicompat.ResponsesResponse{ID: t.providerID, Object: "response", CreatedAt: t.createdAt, Model: t.model, Status: status, Output: []apicompat.ResponsesOutput{}}
}

func (t *nativeTurn) record(record SubscriptionRecord) error {
	if t.result != nil {
		return io.EOF
	}
	if record.Type != "event" {
		payload, err := NoiseRPCPayload(record.Raw)
		if err != nil {
			return err
		}
		var ack struct {
			SessionID  string `json:"session_id"`
			AgentID    string `json:"agent_id"`
			Subscribed bool   `json:"subscribed"`
			IsThread   bool   `json:"is_thread"`
			IsPrimary  bool   `json:"is_primary"`
		}
		if json.Unmarshal(payload, &ack) != nil || !ack.Subscribed || !ack.IsThread || ack.IsPrimary || ack.SessionID != t.sessionID || !validID(ack.AgentID, 256) || t.rootAgent != "" {
			return ErrNoiseProtocol
		}
		t.rootAgent = ack.AgentID
		return t.emitData(apicompat.ResponsesStreamEvent{Type: "response.created", Response: t.baseResponse("in_progress")})
	}
	if t.rootAgent == "" {
		return ErrNoiseProtocol
	}
	if record.Event == "approvals.snapshot" {
		var approvals struct {
			Pending []json.RawMessage `json:"pending_approvals"`
		}
		if json.Unmarshal(record.Payload, &approvals) != nil || len(approvals.Pending) > 0 {
			return ErrOwnerReview
		}
		return nil
	}
	var payload nativePayload
	if json.Unmarshal(record.Payload, &payload) != nil {
		return ErrNoiseProtocol
	}
	if payload.SessionID != t.sessionID || !payload.IsThread {
		return nil
	}
	var envelope struct {
		Seq int64 `json:"seq"`
	}
	if json.Unmarshal(record.Raw, &envelope) != nil || envelope.Seq <= 0 {
		return ErrNoiseProtocol
	}
	key := record.Event + ":" + payload.MessageID + ":" + jsonNumber(envelope.Seq)
	hash := sha256.Sum256(record.Payload)
	if previous, ok := t.seen[key]; ok {
		if previous != hash {
			return ErrNoiseProtocol
		}
		return nil
	}
	if len(t.seen) >= 8192 || t.retained+len(record.Payload) > 16<<20 {
		return ErrNoiseProtocol
	}
	t.seen[key] = hash
	t.retained += len(record.Payload)
	switch record.Event {
	case "message.user":
		if payload.Role != "user" || payload.ParentAgentID != t.rootAgent || !validID(payload.MessageID, 256) {
			return ErrNoiseProtocol
		}
		if t.userID != "" && payload.MessageID != t.userID {
			return ErrOwnerReview
		}
		if payload.ChatEventSeq > 0 {
			payload.Seq = payload.ChatEventSeq
		}
		if payload.Seq <= 0 || (t.userSeq != 0 && t.userSeq != payload.Seq) {
			return ErrNoiseProtocol
		}
		t.userID = payload.MessageID
		t.userSeq = payload.Seq
	case "task.status":
		if payload.AgentID != t.rootAgent {
			return nil
		}
		if payload.RootAgentID != t.rootAgent || payload.TaskID != t.rootAgent || payload.ActiveSubagent == nil || *payload.ActiveSubagent < 0 {
			return ErrNoiseProtocol
		}
		if envelope.Seq > t.taskSeq {
			t.taskSeq = envelope.Seq
			t.taskStatus = payload.Status
			t.activeSubagents = *payload.ActiveSubagent
		}
	case "delta.message_start", "delta.text_append", "delta.message_done":
		if payload.ParentAgentID != t.rootAgent || (payload.AgentID != "" && payload.AgentID != t.rootAgent) {
			return nil
		}
		if payload.MessageSeq <= 0 || payload.Seq <= 0 || !validID(payload.MessageID, 256) {
			return ErrNoiseProtocol
		}
		if t.userSeq == 0 {
			return ErrOwnerReview
		}
		// Later/proactive messages do not belong to this single-request turn.
		if payload.MessageSeq != t.userSeq+1 {
			return nil
		}
		if t.message == nil {
			t.message = &nativeMessage{id: payload.MessageID, canonicalSeq: payload.MessageSeq, pending: map[int64]nativePayload{}}
		}
		m := t.message
		if m.id != payload.MessageID || m.canonicalSeq != payload.MessageSeq {
			return ErrOwnerReview
		}
		switch record.Event {
		case "delta.message_start":
			if payload.AgentID != t.rootAgent || payload.Seq != 1 || m.started {
				return ErrNoiseProtocol
			}
			m.started = true
			m.version = 1
		case "delta.text_append":
			if len(payload.Text) > 1<<20 {
				return ErrNoiseProtocol
			}
			if old, ok := m.pending[payload.Seq]; ok && old.Text != payload.Text {
				return ErrNoiseProtocol
			}
			m.pending[payload.Seq] = payload
		case "delta.message_done":
			if payload.AgentID != t.rootAgent || payload.Transcript.AgentID != t.rootAgent || !validID(payload.Transcript.MessageID, 256) || payload.Status != payload.Transcript.Status {
				return ErrNoiseProtocol
			}
			if m.done != nil {
				return ErrNoiseProtocol
			}
			m.done = &payload
		}
		if err := t.flushDeltas(); err != nil {
			return err
		}
	default:
		if strings.HasPrefix(record.Event, "delta.") && payload.MessageSeq == t.userSeq+1 && payload.ParentAgentID == t.rootAgent {
			return ErrOwnerReview
		}
	}
	return t.finish()
}

func jsonNumber(value int64) string { b, _ := json.Marshal(value); return string(b) }

func (t *nativeTurn) startOutput() error {
	if t.outputStarted {
		return nil
	}
	t.outputStarted = true
	item := &apicompat.ResponsesOutput{Type: "message", ID: t.message.id, Role: "assistant", Status: "in_progress", Content: []apicompat.ResponsesContentPart{}}
	if err := t.emitData(apicompat.ResponsesStreamEvent{Type: "response.output_item.added", Item: item}); err != nil {
		return err
	}
	return t.emitData(apicompat.ResponsesStreamEvent{Type: "response.content_part.added", ItemID: t.message.id, Part: &apicompat.ResponsesContentPart{Type: "output_text"}})
}

func (t *nativeTurn) appendText(text string) error {
	if text == "" {
		return nil
	}
	if len(t.message.text)+len(text) > 1<<20 {
		return ErrNoiseProtocol
	}
	if err := t.startOutput(); err != nil {
		return err
	}
	t.message.text += text
	return t.emitData(apicompat.ResponsesStreamEvent{Type: "response.output_text.delta", ItemID: t.message.id, Delta: text})
}

func (t *nativeTurn) flushDeltas() error {
	m := t.message
	if m == nil || !m.started {
		return nil
	}
	for {
		next, ok := m.pending[m.version+1]
		if !ok {
			break
		}
		delete(m.pending, m.version+1)
		m.version++
		if err := t.appendText(next.Text); err != nil {
			return err
		}
	}
	return nil
}

func (t *nativeTurn) finish() error {
	if t.message == nil || t.message.done == nil || t.userID == "" || t.userSeq == 0 || t.activeSubagents != 0 || t.taskSeq <= t.userSeq {
		return nil
	}
	m := t.message
	done := m.done
	status := ""
	switch {
	case t.taskStatus == "completed" && done.Status == "completed":
		status = "completed"
	case (t.taskStatus == "interrupted" || t.taskStatus == "cancelled" || t.taskStatus == "canceled") && (done.Status == "interrupted" || done.Status == "cancelled" || done.Status == "canceled"):
		status = "incomplete"
	case t.taskStatus == "failed" && done.Status == "failed":
		status = "failed"
	default:
		return nil
	}
	if len(done.Transcript.Messages) == 0 || len(done.Transcript.Messages) > 64 || !validID(done.Transcript.Metadata.Model, 200) {
		return ErrOwnerReview
	}
	text := ""
	matched := false
	for _, message := range done.Transcript.Messages {
		if message.Role != "assistant" || message.ID != m.id {
			continue
		}
		if matched {
			return ErrOwnerReview
		}
		matched = true
		for _, part := range message.Content {
			if part.Type == "text" {
				text += part.Text
				if len(text) > 1<<20 {
					return ErrNoiseProtocol
				}
			}
		}
	}
	if !matched {
		return ErrOwnerReview
	}
	// A revision cannot silently contradict bytes already emitted to an API
	// client. Reconciliation, which emits nothing, uses the full final transcript.
	if !strings.HasPrefix(text, m.text) {
		return ErrOwnerReview
	}
	if err := t.appendText(strings.TrimPrefix(text, m.text)); err != nil {
		return err
	}
	if err := t.startOutput(); err != nil {
		return err
	}
	item := apicompat.ResponsesOutput{Type: "message", ID: m.id, Role: "assistant", Status: status, Content: []apicompat.ResponsesContentPart{{Type: "output_text", Text: text}}}
	if err := t.emitData(apicompat.ResponsesStreamEvent{Type: "response.output_text.done", ItemID: m.id, Text: text}); err != nil {
		return err
	}
	if err := t.emitData(apicompat.ResponsesStreamEvent{Type: "response.content_part.done", ItemID: m.id, Part: &item.Content[0]}); err != nil {
		return err
	}
	if err := t.emitData(apicompat.ResponsesStreamEvent{Type: "response.output_item.done", Item: &item}); err != nil {
		return err
	}
	response := t.baseResponse(status)
	response.Model = done.Transcript.Metadata.Model
	response.Output = []apicompat.ResponsesOutput{item}
	if status == "incomplete" {
		response.IncompleteDetails = &apicompat.ResponsesIncompleteDetails{Reason: "cancelled"}
	}
	if status == "failed" {
		response.Error = &apicompat.ResponsesError{Code: "muse_remote_failed", Message: "Muse task failed"}
	}
	if err := t.emitData(apicompat.ResponsesStreamEvent{Type: "response." + status, Response: response}); err != nil {
		return err
	}
	t.result = &Result{ProviderTurnID: t.providerID, Response: response}
	return io.EOF
}

func (t *nativeTurn) confirmedInterruption() bool {
	return t.userSeq > 0 && t.userID != "" && t.rootAgent != "" && t.taskSeq > t.userSeq && t.activeSubagents == 0 && (t.taskStatus == "interrupted" || t.taskStatus == "cancelled" || t.taskStatus == "canceled")
}

func (p *NativeProvider) Cancel(ctx context.Context, session Session, ref string) (bool, error) {
	reference, err := parseNativeReference(ref)
	if err != nil {
		return false, err
	}
	conn, err := p.ownedConnection(ctx, session, reference)
	if err != nil {
		return false, err
	}
	defer conn.close()
	state := newNativeTurn(reference.sideChat, "", "", ref, "", nil)
	subID, err := conn.send(ctx, "POST", "/chat/subscribe", map[string]any{"session_id": reference.sideChat, "after_stream_seq": 0, "after_chat_event_seq": 0, "capabilities": []string{"chat_cancel", "delta_stream"}})
	if err != nil {
		return false, err
	}
	var decoder NoiseSubscriptionDecoder
	var cancelID int64
	var cancelBody []byte
	cancelStatus := 0
	acknowledged := false
	for {
		event, err := conn.read(ctx)
		if err != nil {
			return false, err
		}
		if event.Kind == "reset" {
			return false, ErrNoiseProtocol
		}
		if event.StreamID == subID {
			if state.result != nil {
				continue
			}
			if event.Kind == "response" && event.Status != 200 {
				return false, ErrNoiseProtocol
			}
			err = decoder.Feed(event.Body, event.EndBody, state.record)
			if err != nil && err != io.EOF {
				return false, err
			}
			if state.confirmedInterruption() && (acknowledged || cancelID == 0) {
				return true, nil
			}
			if state.result != nil {
				if state.result.Response.Status != "incomplete" {
					return false, nil
				}
				if acknowledged || cancelID == 0 {
					return true, nil
				}
				continue
			}
			if event.EndBody {
				return false, ErrOwnerReview
			}
			if cancelID == 0 && state.userSeq > 0 && state.rootAgent != "" && state.taskStatus == "running" {
				cancelID, err = conn.send(ctx, "POST", "/chat/cancel", map[string]string{"session_id": reference.sideChat})
				if err != nil {
					return false, err
				}
			}
		} else if cancelID != 0 && event.StreamID == cancelID {
			if event.Kind == "response" {
				cancelStatus = event.Status
			}
			cancelBody = append(cancelBody, event.Body...)
			if len(cancelBody) > 64<<10 {
				return false, ErrNoiseProtocol
			}
			if event.EndBody {
				payload, err := NoiseRPCPayload(cancelBody)
				if err != nil || cancelStatus != 200 {
					return false, ErrNoiseProtocol
				}
				var ack struct {
					Cancelled bool `json:"cancelled"`
				}
				if json.Unmarshal(payload, &ack) != nil || !ack.Cancelled {
					return false, ErrOwnerReview
				}
				acknowledged = true
				if state.confirmedInterruption() {
					return true, nil
				}
				if state.result != nil {
					return state.result.Response.Status == "incomplete", nil
				}
			}
		} else {
			return false, ErrNoiseProtocol
		}
	}
}
