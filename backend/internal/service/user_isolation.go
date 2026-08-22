package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/sjson"
)

const (
	UserIsolationEnabledExtraKey = "user_isolation_enabled"
	userIsolationDomain          = "sub2api:user-isolation:v1"
)

type userIsolationEndpoint string

const (
	userIsolationEndpointAnthropicMessages userIsolationEndpoint = "anthropic_messages"
	userIsolationEndpointChatCompletions   userIsolationEndpoint = "chat_completions"
	userIsolationEndpointResponses         userIsolationEndpoint = "responses"
)

func (a *Account) IsUserIsolationEnabled() bool {
	return a != nil && a.getExtraBool(UserIsolationEnabledExtraKey)
}

func ValidateUserIsolationAccount(account *Account) error {
	if account == nil || account.Extra == nil {
		return nil
	}
	raw, exists := account.Extra[UserIsolationEnabledExtraKey]
	if !exists {
		return nil
	}
	enabled, ok := raw.(bool)
	if !ok {
		return infraerrors.BadRequest("INVALID_USER_ISOLATION_ENABLED", "user_isolation_enabled must be a boolean")
	}
	if !enabled {
		return nil
	}
	if !supportsManagedUserIsolationAccount(account) {
		return infraerrors.BadRequest("USER_ISOLATION_UNSUPPORTED", fmt.Sprintf("user isolation is not supported for platform %s with protocol %s", account.Platform, account.GetAPIProtocol()))
	}
	return nil
}

func supportsManagedUserIsolationAccount(account *Account) bool {
	if account == nil {
		return false
	}

	switch account.Platform {
	case PlatformAnthropic:
		return account.Type == AccountTypeAPIKey || account.Type == AccountTypeOAuth || account.Type == AccountTypeSetupToken
	case PlatformOpenAI:
		return account.Type == AccountTypeAPIKey || account.Type == AccountTypeOAuth || account.Type == AccountTypeSetupToken
	case PlatformGrok:
		return account.Type == AccountTypeAPIKey || account.Type == AccountTypeOAuth
	case PlatformKimi, PlatformZhipu:
		if account.Type != AccountTypeAPIKey {
			return false
		}
		switch account.GetAPIProtocol() {
		case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolAdaptive:
			return true
		}
	case PlatformDeepseek:
		if account.Type != AccountTypeAPIKey || account.GetAccountMode() == AccountModeCoding {
			return false
		}
		switch account.GetAPIProtocol() {
		case APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses, APIProtocolAdaptive:
			return true
		}
	}
	return false
}

func validateUserIsolationAccountUpdate(account *Account, credentialUpdates, extraUpdates map[string]any) error {
	if account == nil {
		return nil
	}
	candidate := *account
	candidate.Credentials = make(map[string]any, len(account.Credentials)+len(credentialUpdates))
	for key, value := range account.Credentials {
		candidate.Credentials[key] = value
	}
	for key, value := range credentialUpdates {
		candidate.Credentials[key] = value
	}
	candidate.Extra = make(map[string]any, len(account.Extra)+len(extraUpdates))
	for key, value := range account.Extra {
		candidate.Extra[key] = value
	}
	for key, value := range extraUpdates {
		candidate.Extra[key] = value
	}
	return ValidateUserIsolationAccount(&candidate)
}

func applyManagedUserIsolation(
	ctx context.Context,
	cfg *config.Config,
	account *Account,
	endpoint userIsolationEndpoint,
	body []byte,
) ([]byte, error) {
	if !account.IsUserIsolationEnabled() {
		return body, nil
	}

	path, value, err := managedUserIsolationValue(ctx, cfg, account, endpoint)
	if err != nil {
		return nil, err
	}
	if err := validateManagedUserIsolationPayload(body, path); err != nil {
		return nil, fmt.Errorf("validate managed user isolation payload: %w", err)
	}
	updated, err := sjson.SetBytes(body, path, value)
	if err != nil {
		return nil, fmt.Errorf("apply managed user isolation: %w", err)
	}
	return updated, nil
}

func validateManagedUserIsolationPayload(body []byte, path string) error {
	rootKey, nestedKey, _ := strings.Cut(path, ".")
	return validateManagedUserIsolationObject(body, rootKey, nestedKey, true)
}

func validateManagedUserIsolationObject(body []byte, managedKey, nestedKey string, requireObject bool) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode request object: %w", err)
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		if requireObject {
			return fmt.Errorf("request body must be a JSON object")
		}
		return nil
	}

	seenManagedKey := false
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode request key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("request contains a non-string JSON key")
		}

		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return fmt.Errorf("decode request field %q: %w", key, err)
		}
		if !strings.EqualFold(key, managedKey) {
			continue
		}
		if key != managedKey {
			return fmt.Errorf("request contains non-canonical user isolation key %q", key)
		}
		if seenManagedKey {
			return fmt.Errorf("request contains duplicate user isolation key %q", key)
		}
		seenManagedKey = true
		if nestedKey != "" {
			if err := validateManagedUserIsolationObject(raw, nestedKey, "", false); err != nil {
				return err
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("decode request object boundary: %w", err)
	}
	if requireObject {
		var trailing json.RawMessage
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return fmt.Errorf("request contains trailing JSON data")
			}
			return fmt.Errorf("decode trailing request data: %w", err)
		}
	}
	return nil
}

func applyManagedUserIsolationToMap(
	ctx context.Context,
	cfg *config.Config,
	account *Account,
	endpoint userIsolationEndpoint,
	payload map[string]any,
) error {
	if !account.IsUserIsolationEnabled() {
		return nil
	}

	path, value, err := managedUserIsolationValue(ctx, cfg, account, endpoint)
	if err != nil {
		return err
	}
	if strings.Contains(path, ".") {
		return fmt.Errorf("managed user isolation path %q cannot be applied to a flat payload", path)
	}
	payload[path] = value
	return nil
}

func managedUserIsolationValue(
	ctx context.Context,
	cfg *config.Config,
	account *Account,
	endpoint userIsolationEndpoint,
) (string, string, error) {
	path, ok := resolveUserIsolationPath(account, endpoint)
	if !ok {
		return "", "", fmt.Errorf("user isolation is enabled but unsupported for platform %s, account type %s, endpoint %s", account.Platform, account.Type, endpoint)
	}
	if cfg == nil || strings.TrimSpace(cfg.JWT.Secret) == "" {
		return "", "", fmt.Errorf("user isolation is enabled but JWT secret is unavailable")
	}
	userID, _ := ctx.Value(ctxkey.UserID).(int64)
	if userID <= 0 {
		return "", "", fmt.Errorf("user isolation is enabled but authenticated user ID is unavailable")
	}
	return path, deriveManagedUserIsolationID(cfg.JWT.Secret, account, userID), nil
}

func resolveUserIsolationPath(account *Account, endpoint userIsolationEndpoint) (string, bool) {
	if !supportsManagedUserIsolationAccount(account) {
		return "", false
	}

	switch account.Platform {
	case PlatformAnthropic:
		if endpoint == userIsolationEndpointAnthropicMessages {
			return "metadata.user_id", true
		}
	case PlatformOpenAI:
		if endpoint == userIsolationEndpointResponses ||
			(account.Type == AccountTypeAPIKey && endpoint == userIsolationEndpointChatCompletions) {
			return "safety_identifier", true
		}
	case PlatformGrok:
		if endpoint == userIsolationEndpointResponses || endpoint == userIsolationEndpointChatCompletions {
			return "user", true
		}
	case PlatformKimi:
		switch endpoint {
		case userIsolationEndpointChatCompletions:
			return "safety_identifier", true
		case userIsolationEndpointAnthropicMessages:
			return "metadata.user_id", true
		}
	case PlatformZhipu:
		switch endpoint {
		case userIsolationEndpointChatCompletions:
			return "user_id", true
		case userIsolationEndpointAnthropicMessages:
			return "metadata.user_id", true
		}
	case PlatformDeepseek:
		switch endpoint {
		case userIsolationEndpointChatCompletions:
			return "user_id", true
		case userIsolationEndpointAnthropicMessages:
			return "metadata.user_id", true
		case userIsolationEndpointResponses:
			return "user", true
		}
	}
	return "", false
}

func deriveManagedUserIsolationID(secret string, account *Account, userID int64) string {
	message := strings.Join([]string{
		userIsolationDomain,
		account.Platform,
		strconv.FormatInt(account.ID, 10),
		strconv.FormatInt(userID, 10),
	}, "|")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	return "u1_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
