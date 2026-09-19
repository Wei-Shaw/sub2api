package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const codexTicketWatchdogExtraKey = "codex_ticket_watchdog"

// Persist only a small reason/timestamp summary, never response bodies or STATE.
type CodexTicketWatchdogStatus struct {
	Enabled         bool       `json:"enabled"`
	TriggerCount    int64      `json:"trigger_count"`
	LastReason      string     `json:"last_reason,omitempty"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
}

func codexTicketWatchdogStatusOf(account *Account, enabled bool) CodexTicketWatchdogStatus {
	var status CodexTicketWatchdogStatus
	if account != nil {
		if raw, err := json.Marshal(account.Extra[codexTicketWatchdogExtraKey]); err == nil {
			_ = json.Unmarshal(raw, &status)
		}
	}
	if status.LastReason != "model_mismatch" && status.LastReason != "state_312" {
		status = CodexTicketWatchdogStatus{}
	}
	status.Enabled = enabled
	return status
}

// Bound at the exact injection point. Client-supplied headers cannot opt a
// request into the watchdog, and an old response cannot revoke a newer ticket.
type codexTicketReceipt struct {
	accountID        int64
	model            string
	revision         string
	fixedFingerprint string
	stateHash        [32]byte
	capturedAt       time.Time
}

type codexTicketReceiptContextKey struct{}

func receiptForCodexTicket(ticket *openAICodexTicket) codexTicketReceipt {
	return codexTicketReceipt{ticket.AccountID, ticket.Model, ticket.ConfigRevision,
		ticket.FixedProxyFingerprint, sha256.Sum256([]byte(ticket.State)), ticket.CapturedAt}
}

func (r codexTicketReceipt) matches(ticket *openAICodexTicket) bool {
	if ticket == nil {
		return false
	}
	other := receiptForCodexTicket(ticket)
	return r.accountID == other.accountID && r.model == other.model && r.revision == other.revision &&
		r.fixedFingerprint == other.fixedFingerprint && r.stateHash == other.stateHash && r.capturedAt.Equal(other.capturedAt)
}

func (s *OpenAIGatewayService) codexTicketRejectedByWatchdog(ticket *openAICodexTicket) bool {
	if s == nil || ticket == nil {
		return false
	}
	raw, ok := s.openaiCodexWatchdogRevoked.Load(openAICodexTicketKey(ticket.AccountID, ticket.Model))
	if !ok {
		return false
	}
	through, ok := raw.(time.Time)
	return ok && !ticket.CapturedAt.After(through)
}

func (s *OpenAIGatewayService) applyOpenAICodexTicketToRequest(ctx context.Context, account *Account, model string, req *http.Request) error {
	receipt, err := s.applyOpenAICodexTicketWithReceipt(ctx, account, model, req.Header)
	if err != nil {
		return err
	}
	*req = *req.WithContext(context.WithValue(req.Context(), codexTicketReceiptContextKey{}, receipt))
	return nil
}

func (s *OpenAIGatewayService) observeCodexTicketResponse(req *http.Request, resp *http.Response) {
	receipt, _ := req.Context().Value(codexTicketReceiptContextKey{}).(*codexTicketReceipt)
	if receipt == nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}
	var once sync.Once
	trigger := func(reason string) {
		once.Do(func() { s.invalidateCodexTicketFromResponse(*receipt, reason) })
	}
	// 312 is an experimental refresh signal, not an asserted upstream revocation protocol.
	if state := strings.TrimSpace(resp.Header.Get(openAICodexTurnStateHeader)); len(state) == 312 && validCodexTicketState(state) {
		trigger("state_312")
	}
	if resp.Body != nil {
		resp.Body = &codexTicketWatchdogBody{ReadCloser: resp.Body, model: receipt.model, trigger: trigger}
	}
}

func (s *OpenAIGatewayService) invalidateCodexTicketFromResponse(receipt codexTicketReceipt, reason string) {
	// Do not depend on the downstream connection remaining alive after completion.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !s.openAICodexTicketEnabledContext(ctx) {
		return
	}
	s.openaiCodexAccountMu.Lock()
	account, err := s.codexTicketAccountByID(ctx, receipt.accountID)
	if err != nil || s.openaiCodexAccountStopping || !s.openAICodexTicketEnabledContext(ctx) || !codexAccountTicketEligible(account) {
		s.openaiCodexAccountMu.Unlock()
		return
	}
	ac := codexAccountTicketConfigOf(account)
	current := s.lookupOpenAICodexTicket(account, receipt.model)
	if !ac.Enabled || !receipt.matches(current) {
		s.openaiCodexAccountMu.Unlock()
		return
	}
	key := openAICodexTicketKey(account.ID, receipt.model)
	// Keep the rejected identity in memory even if persisting the invalidation
	// fails, so a stale scheduler/account snapshot cannot restore it.
	through := receipt.capturedAt
	if previous, ok := s.openaiCodexWatchdogRevoked.Load(key); ok {
		if previousTime, ok := previous.(time.Time); ok && previousTime.After(through) {
			through = previousTime
		}
	}
	s.openaiCodexWatchdogRevoked.Store(key, through)
	s.openaiCodexTickets.Delete(key)
	status := codexTicketWatchdogStatusOf(account, true)
	status.TriggerCount++
	status.LastReason = reason
	now := time.Now()
	status.LastTriggeredAt = &now
	persistErr := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		openAICodexTicketExtraKey(receipt.model): nil,
		codexTicketWatchdogExtraKey:              status,
	})
	s.openaiCodexAccountMu.Unlock()
	if persistErr != nil {
		logger.L().Warn("codex ticket watchdog invalidation persistence failed", zap.Int64("account_id", account.ID))
	}
	// Reuse any running harvest; preserve failure cooldown and bounded attempts.
	// Successful jobs have no cooldown, so the first signal starts recovery now.
	s.startCodexAccountTicketJob(ctx, account.ID, false)
}

const codexTicketWatchdogBufferLimit = 1024 * 1024

// Transparent incremental observer. It never changes response bytes, returns a
// synthetic error, reads ahead, or retries a business request. Oversized/invalid
// frames are ignored rather than interpreted as a routing failure.
type codexTicketWatchdogBody struct {
	io.ReadCloser
	model     string
	trigger   func(string)
	mode      byte
	buffer    []byte
	data      []byte
	overflow  bool
	skipLine  bool
	eventName string
	mu        sync.Mutex
	finished  bool
}

func (b *codexTicketWatchdogBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return n, err
	}
	if n > 0 {
		b.observe(p[:n])
	}
	if err == io.EOF {
		b.finishLocked()
	}
	return n, err
}

func (b *codexTicketWatchdogBody) Close() error {
	// Unblock a concurrent underlying Read without holding the parser lock.
	err := b.ReadCloser.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finishLocked()
	return err
}

// Some streaming consumers stop at the completed data line without reading
// the following blank line or EOF. Finalize only bytes already observed.
func (b *codexTicketWatchdogBody) finishLocked() {
	if b.finished {
		return
	}
	b.finished = true
	if b.mode == 'j' && !b.overflow {
		b.observeJSON(b.buffer)
	} else if b.mode == 's' {
		if len(b.buffer) > 0 && !b.skipLine {
			b.line(b.buffer)
		}
		b.flushEvent()
	}
}

func (b *codexTicketWatchdogBody) observe(p []byte) {
	if b.mode == 0 {
		trimmed := bytes.TrimSpace(p)
		if len(trimmed) == 0 {
			return
		}
		b.mode = 's'
		if trimmed[0] == '{' {
			b.mode = 'j'
		}
	}
	if b.mode == 'j' {
		if !b.overflow && len(b.buffer)+len(p) <= codexTicketWatchdogBufferLimit {
			b.buffer = append(b.buffer, p...)
		} else {
			b.overflow = true
			b.buffer = nil
		}
		return
	}
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		part := p
		if end >= 0 {
			part = p[:end]
		}
		if !b.skipLine {
			if len(b.buffer)+len(part) > codexTicketWatchdogBufferLimit {
				b.skipLine, b.overflow = true, true
				b.buffer = nil
			} else {
				b.buffer = append(b.buffer, part...)
			}
		}
		if end < 0 {
			return
		}
		if !b.skipLine {
			b.line(b.buffer)
		}
		b.buffer = b.buffer[:0]
		b.skipLine = false
		p = p[end+1:]
	}
}

func (b *codexTicketWatchdogBody) line(line []byte) {
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if len(line) == 0 {
		b.flushEvent()
		return
	}
	if bytes.HasPrefix(line, []byte("event:")) {
		b.eventName = ""
		if strings.TrimSpace(string(line[6:])) == "response.completed" {
			b.eventName = "response.completed"
		}
	}
	if !b.overflow && bytes.HasPrefix(line, []byte("data:")) {
		value := bytes.TrimPrefix(line[5:], []byte{' '})
		if len(b.data)+len(value)+1 > codexTicketWatchdogBufferLimit {
			b.overflow = true
			b.data = nil
		} else {
			b.data = append(b.data, value...)
			b.data = append(b.data, '\n')
		}
	}
}

func (b *codexTicketWatchdogBody) flushEvent() {
	if !b.overflow {
		b.observeJSON(b.data)
	}
	b.data = b.data[:0]
	b.eventName = ""
	b.overflow = false
}

func (b *codexTicketWatchdogBody) observeJSON(raw []byte) {
	if !gjson.ValidBytes(raw) {
		return
	}
	root := gjson.ParseBytes(raw)
	response := root
	eventType := root.Get("type").String()
	completedEvent := eventType == "response.completed" || (eventType == "" && b.eventName == "response.completed")
	if completedEvent {
		response = root.Get("response")
	} else if root.Get("type").Exists() || root.Get("object").String() != "response" {
		return
	}
	status := response.Get("status").String()
	if status != "completed" && (!completedEvent || status != "") {
		return
	}
	actual := response.Get("model")
	if actual.Type != gjson.String {
		return
	}
	model := strings.TrimSpace(actual.String())
	if model != "" && model != b.model {
		b.trigger("model_mismatch")
	}
}
