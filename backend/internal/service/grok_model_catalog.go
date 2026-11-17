package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	grokCatalogStatusSuccess            = "success"
	grokCatalogStatusFailed             = "failed"
	grokCatalogStatusConfiguredFallback = "configured_fallback"
)

var (
	errGrokCatalogInvalidResponse    = errors.New("grok catalog: invalid response")
	errGrokCatalogEmpty              = errors.New("grok catalog: empty catalog")
	errGrokCatalogInvalidModelsURL   = errors.New("grok catalog: invalid models url")
	errGrokCatalogInvalidObservation = errors.New("grok catalog: invalid observation")
)

type GrokCatalogScope struct {
	AccountID   int64  `json:"account_id"`
	AccountType string `json:"account_type"`
	Endpoint    string `json:"endpoint"`
	Fingerprint string `json:"fingerprint"`
}

type GrokCatalogAttempt struct {
	StartedAt  string            `json:"started_at"`
	FinishedAt string            `json:"finished_at"`
	Status     string            `json:"status"`
	ErrorCode  string            `json:"error_code,omitempty"`
	Scope      *GrokCatalogScope `json:"scope,omitempty"`
}

type GrokObservedModelsSnapshot struct {
	Models           []string            `json:"models"`
	FetchedAt        string              `json:"fetched_at"`
	Source           string              `json:"source,omitempty"`
	Scope            *GrokCatalogScope   `json:"scope,omitempty"`
	SuccessStartedAt string              `json:"success_started_at,omitempty"`
	LastAttempt      *GrokCatalogAttempt `json:"last_attempt,omitempty"`
}

type GrokCatalogObservation struct {
	Models  []string
	Attempt GrokCatalogAttempt
}

func NewGrokCatalogScope(accountID int64, accountType, modelsURL string) (*GrokCatalogScope, error) {
	parsed, err := url.Parse(modelsURL)
	if err != nil {
		return nil, errGrokCatalogInvalidModelsURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if parsed.Hostname() == "" || (scheme != "http" && scheme != "https") {
		return nil, errGrokCatalogInvalidModelsURL
	}
	endpoint := scheme + "://" + strings.ToLower(parsed.Host) + strings.TrimRight(parsed.EscapedPath(), "/")
	canonical := endpoint
	if parsed.RawQuery != "" {
		canonical += "?" + parsed.RawQuery
	}
	return &GrokCatalogScope{
		AccountID:   accountID,
		AccountType: accountType,
		Endpoint:    endpoint,
		Fingerprint: grokCatalogFingerprint(canonical, accountID, accountType),
	}, nil
}

func grokCatalogFingerprint(canonicalURL string, accountID int64, accountType string) string {
	payload := canonicalURL + "\n" + strconv.FormatInt(accountID, 10) + "\n" + accountType
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func MergeGrokCatalogObservation(previous *GrokObservedModelsSnapshot, observation GrokCatalogObservation) (*GrokObservedModelsSnapshot, bool, error) {
	if err := validateGrokCatalogObservation(observation); err != nil {
		return nil, false, err
	}
	started, err := time.Parse(time.RFC3339Nano, observation.Attempt.StartedAt)
	if err != nil {
		return nil, false, errGrokCatalogInvalidObservation
	}

	next := cloneGrokObservedSnapshot(previous)
	if next == nil {
		next = &GrokObservedModelsSnapshot{}
	}
	changed := false
	if shouldReplaceGrokCatalogAttempt(next.LastAttempt, started) {
		next.LastAttempt = cloneGrokCatalogAttempt(&observation.Attempt)
		changed = true
	}
	if observation.Attempt.Status == grokCatalogStatusSuccess && shouldReplaceGrokCatalogSuccess(next, started) {
		applyGrokCatalogSuccess(next, observation)
		changed = true
	}
	return next, changed, nil
}

func validateGrokCatalogObservation(observation GrokCatalogObservation) error {
	switch observation.Attempt.Status {
	case grokCatalogStatusSuccess, grokCatalogStatusFailed, grokCatalogStatusConfiguredFallback:
	default:
		return errGrokCatalogInvalidObservation
	}
	switch observation.Attempt.ErrorCode {
	case "", "configuration", "http_error", "transport", "invalid_response", "empty_catalog", "too_large", "persistence":
	default:
		return errGrokCatalogInvalidObservation
	}
	started, err := time.Parse(time.RFC3339Nano, observation.Attempt.StartedAt)
	if err != nil {
		return errGrokCatalogInvalidObservation
	}
	finished, err := time.Parse(time.RFC3339Nano, observation.Attempt.FinishedAt)
	if err != nil {
		return errGrokCatalogInvalidObservation
	}
	if finished.Before(started) {
		return errGrokCatalogInvalidObservation
	}
	if observation.Attempt.Status == grokCatalogStatusSuccess {
		if observation.Attempt.Scope == nil || !grokCatalogHasModelID(observation.Models) {
			return errGrokCatalogInvalidObservation
		}
	}
	return nil
}

func grokCatalogHasModelID(models []string) bool {
	for _, model := range models {
		if strings.TrimSpace(model) != "" {
			return true
		}
	}
	return false
}

func shouldReplaceGrokCatalogAttempt(current *GrokCatalogAttempt, started time.Time) bool {
	if current == nil {
		return true
	}
	previous, err := time.Parse(time.RFC3339Nano, current.StartedAt)
	if err != nil {
		return true
	}
	return started.After(previous)
}

func shouldReplaceGrokCatalogSuccess(current *GrokObservedModelsSnapshot, started time.Time) bool {
	if current == nil || current.SuccessStartedAt == "" {
		return true
	}
	previous, err := time.Parse(time.RFC3339Nano, current.SuccessStartedAt)
	if err != nil {
		return true
	}
	return started.After(previous)
}

func applyGrokCatalogSuccess(dst *GrokObservedModelsSnapshot, observation GrokCatalogObservation) {
	dst.Models = append([]string(nil), observation.Models...)
	dst.FetchedAt = observation.Attempt.FinishedAt
	dst.SuccessStartedAt = observation.Attempt.StartedAt
	dst.Scope = cloneGrokCatalogScope(observation.Attempt.Scope)
}

func cloneGrokObservedSnapshot(src *GrokObservedModelsSnapshot) *GrokObservedModelsSnapshot {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Models != nil {
		dst.Models = append([]string(nil), src.Models...)
	}
	dst.Scope = cloneGrokCatalogScope(src.Scope)
	dst.LastAttempt = cloneGrokCatalogAttempt(src.LastAttempt)
	return &dst
}

func cloneGrokCatalogAttempt(src *GrokCatalogAttempt) *GrokCatalogAttempt {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Scope = cloneGrokCatalogScope(src.Scope)
	return &dst
}

func cloneGrokCatalogScope(src *GrokCatalogScope) *GrokCatalogScope {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

func parseGrokCatalogModelIDs(body []byte) ([]string, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !json.Valid(body) {
		return nil, errGrokCatalogInvalidResponse
	}

	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errGrokCatalogInvalidResponse
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, errGrokCatalogInvalidResponse
	}

	var entries []json.RawMessage
	switch raw[0] {
	case '[':
		items, err := decodeGrokCatalogArray(raw)
		if err != nil {
			return nil, err
		}
		entries = items
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, errGrokCatalogInvalidResponse
		}
		if errRaw, ok := obj["error"]; ok && !bytes.Equal(bytes.TrimSpace(errRaw), []byte("null")) {
			return nil, errGrokCatalogInvalidResponse
		}
		sawCatalog := false
		if data, ok := obj["data"]; ok {
			sawCatalog = true
			items, err := decodeGrokCatalogArray(data)
			if err != nil {
				return nil, err
			}
			entries = append(entries, items...)
		}
		if models, ok := obj["models"]; ok {
			sawCatalog = true
			items, err := decodeGrokCatalogArray(models)
			if err != nil {
				return nil, err
			}
			entries = append(entries, items...)
		}
		if !sawCatalog {
			return nil, errGrokCatalogInvalidResponse
		}
	default:
		return nil, errGrokCatalogInvalidResponse
	}

	ids, err := grokCatalogIDsFromEntries(entries)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errGrokCatalogEmpty
	}
	return ids, nil
}

func decodeGrokCatalogArray(raw json.RawMessage) ([]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil, errGrokCatalogInvalidResponse
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errGrokCatalogInvalidResponse
	}
	return items, nil
}

func grokCatalogIDsFromEntries(entries []json.RawMessage) ([]string, error) {
	models := make([]string, 0, len(entries))
	for _, raw := range entries {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			return nil, errGrokCatalogInvalidResponse
		}
		switch raw[0] {
		case '"':
			var id string
			if err := json.Unmarshal(raw, &id); err != nil {
				return nil, errGrokCatalogInvalidResponse
			}
			models = append(models, id)
		case '{':
			var entry upstreamModelEntry
			if err := json.Unmarshal(raw, &entry); err != nil {
				return nil, errGrokCatalogInvalidResponse
			}
			models = append(models, grokUpstreamModelEntryID(entry))
		default:
			return nil, errGrokCatalogInvalidResponse
		}
	}
	return dedupeAndSortModelIDs(models), nil
}
