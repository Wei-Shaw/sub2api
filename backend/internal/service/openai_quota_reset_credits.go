package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

type openAIRateLimitResetCreditDetailPayload struct {
	ExpiresAt      string `json:"expires_at,omitempty"`
	ExpiresAtCamel string `json:"expiresAt,omitempty"`
	ResetType      string `json:"reset_type,omitempty"`
	ResetTypeCamel string `json:"resetType,omitempty"`
	Status         string `json:"status,omitempty"`
REDACTED

type openAIRateLimitResetCreditDetailsPayload struct {
	AvailableCount        json.RawMessage `json:"available_count,omitempty"`
	AvailableCountCamel   json.RawMessage `json:"availableCount,omitempty"`
	Credits               json.RawMessage `json:"credits,omitempty"`
	RateLimitResetCredits json.RawMessage `json:"rate_limit_reset_credits,omitempty"`
	Items                 json.RawMessage `json:"items,omitempty"`
	Data                  json.RawMessage `json:"data,omitempty"`
REDACTED

type openAIRateLimitResetCreditDetails struct {
	AvailableCount       *int
	AvailableCreditCount int
	CreditListPresent    bool
	Credits              []OpenAIRateLimitResetCreditDetail
REDACTED

func parseOpenAIRateLimitResetCreditDetails(body []byte) (openAIRateLimitResetCreditDetails, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return openAIRateLimitResetCreditDetails{REDACTED, nil
REDACTED

	var rawCredits []*openAIRateLimitResetCreditDetailPayload
	var availableCount *int
	var creditListPresent bool
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &rawCredits); err != nil {
			return openAIRateLimitResetCreditDetails{REDACTED, err
	REDACTED
		creditListPresent = true
REDACTED else {
		var payload openAIRateLimitResetCreditDetailsPayload
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return openAIRateLimitResetCreditDetails{REDACTED, err
	REDACTED
		availableCount = parseOpenAIResetCreditAvailableCount(payload.AvailableCount, payload.AvailableCountCamel)
		var err error
		rawCredits, creditListPresent, err = firstPresentResetCreditPayload(
			payload.Credits,
			payload.RateLimitResetCredits,
			payload.Items,
			payload.Data,
		)
		if err != nil {
			return openAIRateLimitResetCreditDetails{AvailableCount: availableCountREDACTED, err
	REDACTED
REDACTED

	credits := make([]OpenAIRateLimitResetCreditDetail, 0, len(rawCredits))
	availableCreditCount := 0
	for _, raw := range rawCredits {
		if raw == nil {
			continue
	REDACTED
		resetType := strings.TrimSpace(raw.ResetType)
		if resetType == "" {
			resetType = strings.TrimSpace(raw.ResetTypeCamel)
	REDACTED
		if resetType != "" && !strings.EqualFold(resetType, "codex_rate_limits") {
			continue
	REDACTED
		if status := strings.TrimSpace(raw.Status); status != "" && !strings.EqualFold(status, "available") {
			continue
	REDACTED
		availableCreditCount++
		expiresAt := strings.TrimSpace(raw.ExpiresAt)
		if expiresAt == "" {
			expiresAt = strings.TrimSpace(raw.ExpiresAtCamel)
	REDACTED
		if expiresAt == "" {
			continue
	REDACTED
		credits = append(credits, OpenAIRateLimitResetCreditDetail{ExpiresAt: expiresAtREDACTED)
REDACTED
	return openAIRateLimitResetCreditDetails{
		AvailableCount:       availableCount,
		AvailableCreditCount: availableCreditCount,
		CreditListPresent:    creditListPresent,
		Credits:              credits,
REDACTED, nil
REDACTED

func parseOpenAIResetCreditAvailableCount(values ...json.RawMessage) *int {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
	REDACTED

		var count int
		if trimmed[0] == '"' {
			var text string
			if err := json.Unmarshal(trimmed, &text); err != nil {
				continue
		REDACTED
			parsed, err := strconv.Atoi(strings.TrimSpace(text))
			if err != nil {
				continue
		REDACTED
			count = parsed
	REDACTED else if err := json.Unmarshal(trimmed, &count); err != nil {
			continue
	REDACTED
		if count >= 0 {
			return &count
	REDACTED
REDACTED
	return nil
REDACTED

func firstPresentResetCreditPayload(values ...json.RawMessage) ([]*openAIRateLimitResetCreditDetailPayload, bool, error) {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
	REDACTED
		var credits []*openAIRateLimitResetCreditDetailPayload
		if err := json.Unmarshal(trimmed, &credits); err != nil {
			return nil, false, err
	REDACTED
		return credits, true, nil
REDACTED
	return nil, false, nil
REDACTED
