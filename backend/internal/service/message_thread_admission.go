package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// Admission of a thread request asks stores a question: the beta policy, the
// limits of a subscription account (sessions, requests per minute, cost of
// the usage window), and the thread binding. Each can answer, or fail to. The helpers ordinary requests use fold a failure
// into an answer (no rules, room under the limit); that is their long-standing
// behaviour and stays as it is. A thread request rebuilds part of what it
// sends from stored state, so here the three outcomes stay apart:
//
//	nothing configured / not found   → an answer
//	allowed, or refused              → an answer
//	could not be read or understood  → a failure: 503, nothing is sent

// readBetaPolicyStrict reads the beta policy for a thread request. A policy
// that was never stored, or is stored empty, is the default one. A stored
// value that cannot be read, is not one JSON object of the expected shape, or
// holds a rule the settings API could not have written, is an error: the rules
// an operator configured are not known then, and the defaults are not them.
func (s *SettingService) readBetaPolicyStrict(ctx context.Context) (*BetaPolicySettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyBetaPolicySettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultBetaPolicySettings(), nil
		}
		return nil, fmt.Errorf("get beta policy settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return DefaultBetaPolicySettings(), nil
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	var settings *BetaPolicySettings
	if err := decoder.Decode(&settings); err != nil {
		return nil, fmt.Errorf("decode beta policy settings: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode beta policy settings: trailing data")
	}
	if settings == nil {
		return nil, errors.New("decode beta policy settings: not an object")
	}
	// The rules SetBetaPolicySettings accepts. Kept in step with it by
	// TestMessageThreadStrictBetaPolicy.
	actions := map[string]bool{BetaPolicyActionPass: true, BetaPolicyActionFilter: true, BetaPolicyActionBlock: true}
	scopes := map[string]bool{BetaPolicyScopeAll: true, BetaPolicyScopeOAuth: true, BetaPolicyScopeAPIKey: true, BetaPolicyScopeBedrock: true}
	for i, rule := range settings.Rules {
		if rule.BetaToken == "" || !actions[rule.Action] || !scopes[rule.Scope] || (rule.FallbackAction != "" && !actions[rule.FallbackAction]) {
			return nil, fmt.Errorf("beta policy settings: rule[%d] is not a valid rule", i)
		}
	}
	return settings, nil
}

// loadMessageThreadPolicy reads the beta policy once for this attempt. Every
// decision of the attempt, on the client's header and on the outbound one,
// uses that snapshot; without one nothing is sent.
func (s *GatewayService) loadMessageThreadPolicy(ctx context.Context, t *MessageThread) error {
	if t == nil {
		return nil
	}
	t.attempt.policy, t.attempt.policyLoaded = nil, false
	if s.settingService == nil {
		// No settings service at all: there is no policy to read.
		t.attempt.policyLoaded = true
		return nil
	}
	settings, err := s.settingService.readBetaPolicyStrict(ctx)
	if err != nil {
		slog.Warn("message_thread.policy_read_failed", "error", err)
		e := threadStorageError()
		e.Message = "thread: the beta policy is temporarily unavailable. Retry, or send the full conversation without thread."
		return e
	}
	t.attempt.policy, t.attempt.policyLoaded = settings, true
	return nil
}

// betaPolicySettings returns the rules to evaluate: the snapshot of a thread
// attempt when there is one, the stored settings otherwise.
func (s *GatewayService) betaPolicySettings(ctx context.Context) (*BetaPolicySettings, error) {
	if t := threadFrom(ctx); t != nil && t.attempt.policyLoaded {
		return t.attempt.policy, nil
	}
	if s.settingService == nil {
		return nil, nil
	}
	return s.settingService.GetBetaPolicySettings(ctx)
}

// limitAdmission is the answer of one limit of an account.
type limitAdmission int

const (
	// limitNotConfigured: the account does not have this limit (or, for the
	// session limit, the request has no session to count). Nothing is read.
	limitNotConfigured limitAdmission = iota
	// limitAllowed: the limit was evaluated and has room for this request.
	limitAllowed
	// limitDenied: the limit was evaluated and has no room for this request.
	limitDenied
	// limitUnavailable: the limit applies and its data could not be read.
	limitUnavailable
)

// admitThreadSession registers the session of a thread request against the
// account's session limit and reports which of the four answers it got. It is
// checkAndRegisterSession without its fail-open: a limit that is configured
// and cannot be evaluated does not admit.
func (s *GatewayService) admitThreadSession(ctx context.Context, account *Account, sessionID string) (limitAdmission, error) {
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
		return limitNotConfigured, nil
	}
	maxSessions := account.GetMaxSessions()
	if maxSessions <= 0 || sessionID == "" {
		return limitNotConfigured, nil
	}
	if s.sessionLimitCache == nil {
		return limitUnavailable, errors.New("the account has a session limit and no session limit store is configured")
	}
	idleTimeout := time.Duration(account.GetSessionIdleTimeoutMinutes()) * time.Minute
	allowed, err := s.sessionLimitCache.RegisterSession(ctx, account.ID, sessionID, maxSessions, idleTimeout)
	if err != nil {
		return limitUnavailable, err
	}
	if !allowed {
		return limitDenied, nil
	}
	return limitAllowed, nil
}

// AdmissionFault reports that a store an admission decision of this request
// depended on could not be read.
func (t *MessageThread) AdmissionFault() bool { return t != nil && t.admissionFault }

// CountRPMOnSuccess uses the row admitted immediately before a thread was
// sent. Ordinary requests keep using the selected account, as before.
func (t *MessageThread) CountRPMOnSuccess(selected *Account) bool {
	if t != nil {
		return t.attempt.countRPM
	}
	return accountRPMEnabled(selected)
}

func accountRPMEnabled(account *Account) bool {
	return account != nil && account.IsAnthropicOAuthOrSetupToken() && account.GetBaseRPM() > 0
}

// registerThreadSessionForSend is the last admission step. RegisterSession
// atomically creates or refreshes a session, but grants no release rights.
func (s *GatewayService) registerThreadSessionForSend(ctx context.Context, t *MessageThread, account *Account) error {
	admission, err := s.admitThreadSession(ctx, account, t.session)
	if err := t.limitDecision(account, "sessions", admission, err); err != nil {
		return err
	}
	t.attempt.countRPM = accountRPMEnabled(account)
	return nil
}

// threadZoneAdmission turns the zone of a soft limit into an answer. reserved
// tells a request that belongs to a session already on the account: the zone
// above the limit is kept for those, and nothing is served past the ceiling.
func threadZoneAdmission(zone WindowCostSchedulability, reserved bool) limitAdmission {
	switch zone {
	case WindowCostSchedulable:
		return limitAllowed
	case WindowCostStickyOnly:
		if reserved {
			return limitAllowed
		}
	}
	return limitDenied
}

// admitThreadRPM evaluates the account's requests-per-minute limit for a
// thread request from the counter itself, not from what the scheduler
// prefetched. A minute without a counter is a count of zero; a counter that
// cannot be read is not. The thresholds are the scheduler's, and so is the
// soft nature of the limit: nothing is locked between this read and the
// increment that follows a served request.
func (s *GatewayService) admitThreadRPM(ctx context.Context, account *Account, reserved bool) (limitAdmission, error) {
	if !accountRPMEnabled(account) {
		return limitNotConfigured, nil
	}
	if s.rpmCache == nil {
		return limitUnavailable, errors.New("the account has a request rate limit and no rate store is configured")
	}
	count, err := s.rpmCache.GetRPM(ctx, account.ID)
	if err != nil {
		return limitUnavailable, err
	}
	return threadZoneAdmission(account.CheckRPMSchedulability(count), reserved), nil
}

// admitThreadWindowCost evaluates the account's window cost limit for a
// thread request. The cached cost is an answer; on a miss, or when the cache
// cannot be read, the durable state behind it is the authority, and an
// account it has no row for has spent nothing. When neither can be read there
// is no cost to compare.
func (s *GatewayService) admitThreadWindowCost(ctx context.Context, account *Account, reserved bool) (limitAdmission, error) {
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() || account.GetWindowCostLimit() <= 0 {
		return limitNotConfigured, nil
	}
	var cacheErr error
	if s.sessionLimitCache != nil {
		cost, hit, err := s.sessionLimitCache.GetWindowCost(ctx, account.ID)
		if err == nil && hit {
			return threadZoneAdmission(account.CheckWindowCostSchedulability(cost), reserved), nil
		}
		cacheErr = err
	}
	if s.usageLogRepo == nil {
		return limitUnavailable, errors.Join(errors.New("the account has a window cost limit and no window cost state is configured"), cacheErr)
	}
	stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, account.ID, account.GetCurrentWindowStartTime())
	if err != nil {
		return limitUnavailable, errors.Join(err, cacheErr)
	}
	if stats == nil {
		return limitUnavailable, errors.New("empty account window statistics")
	}
	cost := stats.StandardCost
	if s.sessionLimitCache != nil {
		_ = s.sessionLimitCache.SetWindowCost(ctx, account.ID, cost)
	}
	return threadZoneAdmission(account.CheckWindowCostSchedulability(cost), reserved), nil
}

// admitMessageThreadLimits is the admission of a thread request on the account
// that was selected for it, for the soft limits the scheduler evaluated on
// the way: window cost and requests per minute. The scheduler reads them in
// batches and carries on when it cannot; here each configured limit is read
// again for this one account, and the three outcomes stay apart.
//
// A continue request has one account. No room there ends the thread for now
// (the client replays), and data that cannot be read is a failure; in both
// cases nothing is sent anywhere. A create request passes the account over,
// and a selection that ends without an account because of unread data is
// reported as a failure.
func (s *GatewayService) admitMessageThreadLimits(ctx context.Context, t *MessageThread, account *Account) error {
	if t == nil || account == nil {
		return nil
	}
	reserved := t.IsContinue() || (t.stickyAccountID > 0 && t.stickyAccountID == account.ID)
	for _, limit := range []struct {
		name  string
		admit func(context.Context, *Account, bool) (limitAdmission, error)
	}{
		{"window_cost", s.admitThreadWindowCost},
		{"rpm", s.admitThreadRPM},
	} {
		admission, err := limit.admit(ctx, account, reserved)
		if decision := t.limitDecision(account, limit.name, admission, err); decision != nil {
			return decision
		}
	}
	return nil
}

func (t *MessageThread) limitDecision(account *Account, name string, admission limitAdmission, err error) error {
	switch admission {
	case limitDenied:
		if t.IsContinue() {
			return threadNotFoundError()
		}
		t.limitDenied = true
		return threadAccountPassedOverError()
	case limitUnavailable:
		slog.Warn("message_thread.limit_unavailable", "limit", name, "account_id", account.ID, "error", err)
		if t.IsContinue() {
			e := threadStorageError()
			e.Message = "thread: a limit of the account that holds this thread is temporarily unavailable. Retry, or send the full conversation without thread."
			return e
		}
		t.admissionFault = true
		return threadAccountPassedOverError()
	}
	return nil
}

// threadAccountPassedOverError tells the handler that a create request may
// not use the selected account and may try another.
func threadAccountPassedOverError() *MessageThreadError {
	e := threadStorageError()
	e.Message = "thread: no account can take this request right now. Retry, or send the full conversation without thread."
	e.passedOver = true
	return e
}

// IsMessageThreadAccountPassedOver reports that the selected account may not
// serve this create request for a reason that says nothing about other
// accounts: its limit has no room, or its limit could not be evaluated.
func IsMessageThreadAccountPassedOver(err error) bool {
	var te *MessageThreadError
	return errors.As(err, &te) && te.passedOver
}

// LimitDenied reports that an account was passed over because a limit of it
// had no room for this request.
func (t *MessageThread) LimitDenied() bool { return t != nil && t.limitDenied }

// SetStickyAccount records the account the request's session is bound to, if
// any. A thread created by that session is one of the account's sessions.
func (t *MessageThread) SetStickyAccount(accountID int64) {
	if t != nil {
		t.stickyAccountID = accountID
	}
}

// NewMessageThreadNoRoomError is the answer to a create request when every
// account that could serve it is at a limit.
func NewMessageThreadNoRoomError() *MessageThreadError {
	e := threadStorageError()
	e.Message = "thread: no account has room for this request right now. Retry later, or send the full conversation without thread."
	return e
}

// messageThreadRecordVersion is the version of the stored form of a binding.
// A record of any other version is not one this code can route by.
const messageThreadRecordVersion = 1

// messageThreadRecord is the stored form of a binding. Every field is written
// on every record, so a field that is missing when a record is read back is
// damage and not a default.
type messageThreadRecord struct {
	Version int `json:"v"`
	MessageThreadBinding
}

var messageThreadRecordFields = []string{"v", "a", "i", "m", "b", "w", "cm", "cb", "cf", "t", "s", "mi", "cc", "ht", "bp", "st"}

// EncodeMessageThreadBinding returns the stored form of a binding. It refuses
// a binding this version would not accept back.
func EncodeMessageThreadBinding(binding MessageThreadBinding) ([]byte, error) {
	if binding.ClientFixed == nil {
		binding.ClientFixed = map[string]string{}
	}
	if !binding.Valid() {
		return nil, errors.New("message thread binding is incomplete")
	}
	return json.Marshal(messageThreadRecord{Version: messageThreadRecordVersion, MessageThreadBinding: binding})
}

// DecodeMessageThreadBinding reads a stored binding back. It accepts exactly
// what EncodeMessageThreadBinding writes: this version, every field present
// and none of them null. The caller then checks Valid.
func DecodeMessageThreadBinding(raw []byte) (*MessageThreadBinding, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode message thread binding: %w", err)
	}
	if len(fields) != len(messageThreadRecordFields) {
		return nil, errors.New("decode message thread binding: unexpected set of fields")
	}
	for _, name := range messageThreadRecordFields {
		value, present := fields[name]
		if !present || string(value) == "null" {
			return nil, fmt.Errorf("decode message thread binding: field %q is missing", name)
		}
	}
	var record messageThreadRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("decode message thread binding: %w", err)
	}
	if record.Version != messageThreadRecordVersion {
		return nil, fmt.Errorf("decode message thread binding: version %d", record.Version)
	}
	return &record.MessageThreadBinding, nil
}
