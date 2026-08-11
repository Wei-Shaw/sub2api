package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const AccountSchedulingThresholdReasonSource = "account_scheduling_threshold"

const (
	defaultTempUnschedReasonErrorMessage          = "temporary scheduling block reason unavailable"
	defaultAccountSchedulingThresholdErrorMessage = "account scheduling threshold reached"
)

type tempUnschedReasonPayload struct {
	Source           string  `json:"source,omitempty"`
	Platform         string  `json:"platform,omitempty"`
	Window           string  `json:"window,omitempty"`
	Scope            string  `json:"scope,omitempty"`
	ThresholdPercent int     `json:"threshold_percent,omitempty"`
	UsedPercent      float64 `json:"used_percent,omitempty"`
	UntilUnix        int64   `json:"until_unix,omitempty"`
	TriggeredAtUnix  int64   `json:"triggered_at_unix,omitempty"`
	ErrorMessage     string  `json:"error_message"`
REDACTED

type AccountSchedulingThresholdReasonInput struct {
	Platform         string
	Window           string
	Scope            string
	ThresholdPercent int
	UsedPercent      float64
	Until            time.Time
	Now              time.Time
REDACTED

func BuildTempUnschedReasonPayload(source string, errorMessage string) string {
	payload := tempUnschedReasonPayload{
		Source:       strings.TrimSpace(source),
		ErrorMessage: normalizeTempUnschedReasonErrorMessage(errorMessage, defaultTempUnschedReasonErrorMessage),
REDACTED

	raw, err := json.Marshal(payload)
	if err != nil {
		return payload.ErrorMessage
REDACTED
	return string(raw)
REDACTED

func BuildAccountSchedulingThresholdReason(errorMessage string) string {
	return BuildTempUnschedReasonPayload(
		AccountSchedulingThresholdReasonSource,
		normalizeTempUnschedReasonErrorMessage(errorMessage, defaultAccountSchedulingThresholdErrorMessage),
	)
REDACTED

func BuildDetailedAccountSchedulingThresholdReason(input AccountSchedulingThresholdReasonInput) string {
	triggeredAt := input.Now
	if triggeredAt.IsZero() {
		triggeredAt = time.Now().UTC()
REDACTED
	payload := tempUnschedReasonPayload{
		Source:           AccountSchedulingThresholdReasonSource,
		Platform:         strings.TrimSpace(input.Platform),
		Window:           strings.TrimSpace(input.Window),
		Scope:            strings.TrimSpace(input.Scope),
		ThresholdPercent: input.ThresholdPercent,
		UsedPercent:      input.UsedPercent,
		TriggeredAtUnix:  triggeredAt.Unix(),
		ErrorMessage:     buildAccountSchedulingThresholdErrorMessage(input),
REDACTED
	if !input.Until.IsZero() {
		payload.UntilUnix = input.Until.UTC().Unix()
REDACTED

	raw, err := json.Marshal(payload)
	if err != nil {
		return payload.ErrorMessage
REDACTED
	return string(raw)
REDACTED

func IsAccountSchedulingThresholdReason(rawReason string) bool {
	payload, ok := parseTempUnschedReasonPayload(rawReason)
	if !ok {
		return false
REDACTED
	return payload.Source == AccountSchedulingThresholdReasonSource
REDACTED

func parseTempUnschedReasonPayload(rawReason string) (tempUnschedReasonPayload, bool) {
	rawReason = strings.TrimSpace(rawReason)
	if rawReason == "" {
		return tempUnschedReasonPayload{REDACTED, false
REDACTED

	var payload tempUnschedReasonPayload
	if err := json.Unmarshal([]byte(rawReason), &payload); err != nil {
		return tempUnschedReasonPayload{REDACTED, false
REDACTED
	payload.Source = strings.TrimSpace(payload.Source)
	payload.ErrorMessage = strings.TrimSpace(payload.ErrorMessage)
	return payload, true
REDACTED

func normalizeTempUnschedReasonErrorMessage(errorMessage string, fallback string) string {
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage != "" {
		return errorMessage
REDACTED

	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback
REDACTED
	return defaultTempUnschedReasonErrorMessage
REDACTED

func buildAccountSchedulingThresholdErrorMessage(input AccountSchedulingThresholdReasonInput) string {
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		platform = "account"
REDACTED

	target := strings.TrimSpace(input.Window)
	if scope := strings.TrimSpace(input.Scope); scope != "" {
		if target == "" {
			target = scope
	REDACTED else {
			target = target + "/" + scope
	REDACTED
REDACTED
	if target == "" {
		target = "usage window"
REDACTED

	threshold := input.ThresholdPercent
	if threshold <= 0 {
		threshold = 100
REDACTED

	untilText := "the window reset"
	if !input.Until.IsZero() {
		untilText = input.Until.UTC().Format(time.RFC3339)
REDACTED

	return fmt.Sprintf(
		"%s scheduling threshold reached for %s: %.1f%% used >= %d%%; paused until %s",
		platform,
		target,
		input.UsedPercent,
		threshold,
		untilText,
	)
REDACTED

func tempUnschedStateFromStoredReason(rawReason string, fallbackUntilUnix int64) *TempUnschedState {
	state := &TempUnschedState{
		UntilUnix: fallbackUntilUnix,
		RuleIndex: -1,
REDACTED

	rawReason = strings.TrimSpace(rawReason)
	if rawReason == "" {
		state.ErrorMessage = defaultTempUnschedReasonErrorMessage
		return state
REDACTED

	parsed := TempUnschedState{RuleIndex: -1REDACTED
	if err := json.Unmarshal([]byte(rawReason), &parsed); err == nil {
		if fallbackUntilUnix > parsed.UntilUnix {
			parsed.UntilUnix = fallbackUntilUnix
	REDACTED
		if strings.TrimSpace(parsed.ErrorMessage) == "" {
			if IsAccountSchedulingThresholdReason(rawReason) {
				parsed.ErrorMessage = defaultAccountSchedulingThresholdErrorMessage
		REDACTED else {
				parsed.ErrorMessage = rawReason
		REDACTED
	REDACTED
		return &parsed
REDACTED

	state.ErrorMessage = rawReason
	return state
REDACTED
