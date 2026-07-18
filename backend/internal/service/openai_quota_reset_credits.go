package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type openAIRateLimitResetCreditDetailPayload struct {
	ID             string `json:"id,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	ExpiresAtCamel string `json:"expiresAt,omitempty"`
	ResetType      string `json:"reset_type,omitempty"`
	ResetTypeCamel string `json:"resetType,omitempty"`
	Status         string `json:"status,omitempty"`
}

type openAIRateLimitResetCreditDetailsPayload struct {
	AvailableCount        json.RawMessage `json:"available_count,omitempty"`
	AvailableCountCamel   json.RawMessage `json:"availableCount,omitempty"`
	Credits               json.RawMessage `json:"credits,omitempty"`
	RateLimitResetCredits json.RawMessage `json:"rate_limit_reset_credits,omitempty"`
	Items                 json.RawMessage `json:"items,omitempty"`
	Data                  json.RawMessage `json:"data,omitempty"`
}

type openAIRateLimitResetCreditDetails struct {
	AvailableCount       *int
	AvailableCreditCount int
	IncompleteCredits    int
	CreditListPresent    bool
	Credits              []OpenAIRateLimitResetCreditDetail
	Candidates           []OpenAIQuotaResetCreditCandidate
}

// OpenAIQuotaResetCreditCandidate is internal scheduling metadata. It must not
// be embedded in the public quota response because ID is an opaque upstream
// capability used only by the exact-credit consume call.
type OpenAIQuotaResetCreditCandidate struct {
	ID           string
	Status       string
	ExpiresAt    time.Time
	ExpiresAtRaw string
}

func parseOpenAIRateLimitResetCreditDetails(body []byte) (openAIRateLimitResetCreditDetails, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return openAIRateLimitResetCreditDetails{}, nil
	}

	var rawCredits []*openAIRateLimitResetCreditDetailPayload
	var availableCount *int
	var creditListPresent bool
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &rawCredits); err != nil {
			return openAIRateLimitResetCreditDetails{}, err
		}
		creditListPresent = true
	} else {
		var payload openAIRateLimitResetCreditDetailsPayload
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return openAIRateLimitResetCreditDetails{}, err
		}
		availableCount = parseOpenAIResetCreditAvailableCount(payload.AvailableCount, payload.AvailableCountCamel)
		var err error
		rawCredits, creditListPresent, err = firstPresentResetCreditPayload(
			payload.Credits,
			payload.RateLimitResetCredits,
			payload.Items,
			payload.Data,
		)
		if err != nil {
			return openAIRateLimitResetCreditDetails{AvailableCount: availableCount}, err
		}
	}

	credits := make([]OpenAIRateLimitResetCreditDetail, 0, len(rawCredits))
	candidates := make([]OpenAIQuotaResetCreditCandidate, 0, len(rawCredits))
	availableCreditCount := 0
	incompleteCredits := 0
	for _, raw := range rawCredits {
		if raw == nil {
			incompleteCredits++
			continue
		}
		resetType := strings.TrimSpace(raw.ResetType)
		if resetType == "" {
			resetType = strings.TrimSpace(raw.ResetTypeCamel)
		}
		if resetType != "" && !strings.EqualFold(resetType, "codex_rate_limits") {
			continue
		}
		if status := strings.TrimSpace(raw.Status); status != "" && !strings.EqualFold(status, "available") {
			continue
		}
		availableCreditCount++
		expiresAt := strings.TrimSpace(raw.ExpiresAt)
		if expiresAt == "" {
			expiresAt = strings.TrimSpace(raw.ExpiresAtCamel)
		}
		if expiresAt == "" {
			incompleteCredits++
			continue
		}
		credits = append(credits, OpenAIRateLimitResetCreditDetail{ExpiresAt: expiresAt})
		expiresAtTime, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			incompleteCredits++
			continue
		}
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			incompleteCredits++
			continue
		}
		candidates = append(candidates, OpenAIQuotaResetCreditCandidate{
			ID:           id,
			Status:       strings.TrimSpace(raw.Status),
			ExpiresAt:    expiresAtTime,
			ExpiresAtRaw: expiresAt,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ExpiresAt.Equal(candidates[j].ExpiresAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].ExpiresAt.Before(candidates[j].ExpiresAt)
	})
	return openAIRateLimitResetCreditDetails{
		AvailableCount:       availableCount,
		AvailableCreditCount: availableCreditCount,
		IncompleteCredits:    incompleteCredits,
		CreditListPresent:    creditListPresent,
		Credits:              credits,
		Candidates:           candidates,
	}, nil
}

func schedulableOpenAIResetCredits(details openAIRateLimitResetCreditDetails) ([]OpenAIQuotaResetCreditCandidate, error) {
	if !details.CreditListPresent {
		return nil, fmt.Errorf("reset credit details are unavailable")
	}
	if details.IncompleteCredits > 0 || len(details.Candidates) != details.AvailableCreditCount {
		return nil, fmt.Errorf("available reset credit details are incomplete")
	}
	if details.AvailableCount != nil && *details.AvailableCount != details.AvailableCreditCount {
		return nil, fmt.Errorf("available reset credit count does not match credit details")
	}
	return append([]OpenAIQuotaResetCreditCandidate(nil), details.Candidates...), nil
}

func parseOpenAIResetCreditAvailableCount(values ...json.RawMessage) *int {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}

		var count int
		if trimmed[0] == '"' {
			var text string
			if err := json.Unmarshal(trimmed, &text); err != nil {
				continue
			}
			parsed, err := strconv.Atoi(strings.TrimSpace(text))
			if err != nil {
				continue
			}
			count = parsed
		} else if err := json.Unmarshal(trimmed, &count); err != nil {
			continue
		}
		if count >= 0 {
			return &count
		}
	}
	return nil
}

func firstPresentResetCreditPayload(values ...json.RawMessage) ([]*openAIRateLimitResetCreditDetailPayload, bool, error) {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}
		var credits []*openAIRateLimitResetCreditDetailPayload
		if err := json.Unmarshal(trimmed, &credits); err != nil {
			return nil, false, err
		}
		return credits, true, nil
	}
	return nil, false, nil
}
