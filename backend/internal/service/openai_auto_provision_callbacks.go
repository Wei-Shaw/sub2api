package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (s *OpenAIAutoProvisionService) updateState(ctx context.Context, mutate func(*autoProvisionState) error) error {
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, autoProvisionStateLockKey, s.instanceID, autoProvisionLockTTL)
	if !ok {
		return errAutoProvisionStateBusy
	}
	defer release()

	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	state, err := s.readState(ctx)
	if err != nil {
		return err
	}
	if err := mutate(normalizeAutoProvisionState(state)); err != nil {
		return err
	}
	return s.writeState(ctx, state)
}

func (s *OpenAIAutoProvisionService) readState(ctx context.Context) (*autoProvisionState, error) {
	if s.settingRepo == nil {
		return newAutoProvisionState(), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, autoProvisionStateKey)
	if errors.Is(err, ErrSettingNotFound) || strings.TrimSpace(raw) == "" {
		return newAutoProvisionState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read auto-provision state: %w", err)
	}
	var state autoProvisionState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, fmt.Errorf("decode auto-provision state: %w", err)
	}
	return normalizeAutoProvisionState(&state), nil
}

func (s *OpenAIAutoProvisionService) writeState(ctx context.Context, state *autoProvisionState) error {
	if s.settingRepo == nil {
		return nil
	}
	encoded, err := json.Marshal(normalizeAutoProvisionState(state))
	if err != nil {
		return fmt.Errorf("encode auto-provision state: %w", err)
	}
	return s.settingRepo.Set(ctx, autoProvisionStateKey, string(encoded))
}

func (s *OpenAIAutoProvisionService) removeReauthorization(ctx context.Context, requestID string) error {
	return s.updateState(ctx, func(state *autoProvisionState) error {
		delete(state.Reauthorizations, requestID)
		return nil
	})
}

func (s *OpenAIAutoProvisionService) recordAutoProvisionError(ctx context.Context, runtimeErr error) {
	if runtimeErr == nil {
		return
	}
	_ = s.updateState(ctx, func(state *autoProvisionState) error {
		state.CheckInProgress = false
		state.LastCheckError = runtimeErr.Error()
		return nil
	})
}

func (s *OpenAIAutoProvisionService) ValidateCallbackSecret(ctx context.Context, provided string) error {
	cfg, err := s.config(ctx)
	if err != nil {
		return err
	}
	secret := strings.TrimSpace(provided)
	if secret == "" || cfg.callbackSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(cfg.callbackSecret)) != 1 {
		return errors.New("invalid automation callback secret")
	}
	return nil
}

func (s *OpenAIAutoProvisionService) HandleProvisionCallback(ctx context.Context, callback OpenAIAutoProvisionCallback) (replay bool, err error) {
	if strings.TrimSpace(callback.Kind) != "registration" {
		return false, errors.New("callback kind must be registration")
	}
	var accepted bool
	err = s.updateState(ctx, func(state *autoProvisionState) error {
		var callbackErr error
		accepted, replay, callbackErr = consumeAutoProvisionCallback(state, callback)
		return callbackErr
	})
	if err == nil && !accepted {
		return false, errors.New("callback was not accepted")
	}
	return replay, err
}

func (s *OpenAIAutoProvisionService) HandleReauthorizationCompletion(ctx context.Context, callback OpenAIAutoProvisionCallback) (replay bool, err error) {
	if strings.TrimSpace(callback.Kind) != "reauthorization" {
		return false, errors.New("callback kind must be reauthorization")
	}
	var accepted bool
	err = s.updateState(ctx, func(state *autoProvisionState) error {
		var callbackErr error
		accepted, replay, callbackErr = consumeAutoProvisionCallback(state, callback)
		return callbackErr
	})
	if err == nil && !accepted {
		return false, errors.New("callback was not accepted")
	}
	return replay, err
}

func (s *OpenAIAutoProvisionService) HandleReauthorizationCallback(ctx context.Context, callback OpenAIAutoReauthorizationCallback) (replay bool, err error) {
	if strings.TrimSpace(callback.RequestID) == "" || strings.TrimSpace(callback.EventID) == "" || callback.AccountID <= 0 ||
		strings.TrimSpace(callback.Email) == "" || strings.TrimSpace(callback.SessionID) == "" ||
		strings.TrimSpace(callback.CallbackURL) == "" {
		return false, errors.New("request_id, event_id, account_id, email, session_id, and callback_url are required")
	}
	code, state, err := parseReauthorizationCallbackURL(callback.CallbackURL)
	if err != nil {
		return false, err
	}

	var pending autoReauthorizationPending
	err = s.updateState(ctx, func(state *autoProvisionState) error {
		if _, ok := state.ProcessedEvents[callback.EventID]; ok {
			replay = true
			return nil
		}
		candidate, ok := state.Reauthorizations[callback.RequestID]
		if !ok || candidate.AccountID != callback.AccountID || !strings.EqualFold(candidate.Email, callback.Email) {
			return errors.New("reauthorization callback does not match a pending request")
		}
		pending = candidate
		return nil
	})
	if err != nil || replay {
		return replay, err
	}
	if s.oauth == nil || s.accounts == nil || s.admin == nil {
		return false, errors.New("openai OAuth reauthorization dependencies are unavailable")
	}

	account, err := s.accounts.GetByID(ctx, callback.AccountID)
	if err != nil {
		return false, fmt.Errorf("load account for reauthorization: %w", err)
	}
	if account == nil || !account.IsOpenAIOAuth() || account.IsCredentialShadow() {
		return false, errors.New("reauthorization target is not a primary OpenAI OAuth account")
	}
	if accountEmail := strings.TrimSpace(account.GetCredential("email")); accountEmail != "" && !strings.EqualFold(accountEmail, pending.Email) {
		return false, errors.New("reauthorization account email does not match pending request")
	}

	tokenInfo, err := s.oauth.ExchangeCode(ctx, &OpenAIExchangeCodeInput{
		SessionID: callback.SessionID, Code: code, State: state, RedirectURI: callback.RedirectURI,
	})
	if err != nil {
		return false, fmt.Errorf("exchange reauthorization code: %w", err)
	}
	if err := validateReauthorizationIdentity(account, pending.Email, tokenInfo); err != nil {
		return false, err
	}
	credentials := make(map[string]any, len(account.Credentials)+8)
	for key, value := range account.Credentials {
		credentials[key] = value
	}
	for key, value := range s.oauth.BuildAccountCredentials(tokenInfo) {
		credentials[key] = value
	}
	if _, err := s.admin.UpdateAccount(ctx, account.ID, &UpdateAccountInput{Credentials: credentials}); err != nil {
		return false, fmt.Errorf("persist reauthorization credentials: %w", err)
	}
	if _, err := s.admin.ClearAccountError(ctx, account.ID); err != nil {
		return false, fmt.Errorf("clear account error after reauthorization: %w", err)
	}

	err = s.updateState(ctx, func(state *autoProvisionState) error {
		delete(state.Reauthorizations, callback.RequestID)
		now := time.Now().UTC()
		state.LastCallbackAt = &now
		state.LastCallbackKind = "reauthorization"
		state.LastCallbackStatus = "oauth_callback_received"
		state.LastCheckError = ""
		state.ProcessedEvents[callback.EventID] = now
		return nil
	})
	return false, err
}

func parseReauthorizationCallbackURL(raw string) (code, state string, err error) {
	value := strings.TrimSpace(raw)
	parsed, parseErr := url.Parse(value)
	if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", "", errors.New("reauthorization callback_url must be an absolute HTTP(S) URL without credentials or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", errors.New("reauthorization callback_url must use http or https")
	}
	query := parsed.Query()
	code = strings.TrimSpace(query.Get("code"))
	state = strings.TrimSpace(query.Get("state"))
	if code == "" || state == "" {
		return "", "", errors.New("reauthorization callback_url must contain code and state")
	}
	return code, state, nil
}

func validateReauthorizationIdentity(account *Account, expectedEmail string, tokenInfo *OpenAITokenInfo) error {
	if tokenInfo == nil || strings.TrimSpace(tokenInfo.AccessToken) == "" {
		return errors.New("reauthorization did not return an access token")
	}
	if strings.TrimSpace(tokenInfo.Email) == "" || !strings.EqualFold(tokenInfo.Email, expectedEmail) {
		return errors.New("reauthorization identity does not match the target account")
	}
	expectedAccountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if expectedAccountID != "" && strings.TrimSpace(tokenInfo.ChatGPTAccountID) != "" && expectedAccountID != strings.TrimSpace(tokenInfo.ChatGPTAccountID) {
		return errors.New("reauthorization ChatGPT account identity does not match the target account")
	}
	return nil
}
