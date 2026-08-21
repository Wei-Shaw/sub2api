package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const openAIAPIKeyHealthBreakerSettingsCacheTTL = 30 * time.Second

type cachedOpenAIAPIKeyHealthBreakerSettings struct {
	settings  OpenAIAPIKeyHealthBreakerSettings
	expiresAt time.Time
REDACTED

func normalizeOpenAIAPIKeyHealthBreakerSettings(settings *OpenAIAPIKeyHealthBreakerSettings) *OpenAIAPIKeyHealthBreakerSettings {
	if settings == nil {
		return DefaultOpenAIAPIKeyHealthBreakerSettings()
REDACTED
	result := *settings
	if result.WindowMinutes < 1 {
		result.WindowMinutes = 1
REDACTED else if result.WindowMinutes > 60 {
		result.WindowMinutes = 60
REDACTED
	if result.FailureThreshold < 1 {
		result.FailureThreshold = 1
REDACTED else if result.FailureThreshold > 10000 {
		result.FailureThreshold = 10000
REDACTED
	if result.CooldownMinutes < 1 {
		result.CooldownMinutes = 1
REDACTED else if result.CooldownMinutes > 60 {
		result.CooldownMinutes = 60
REDACTED
	return &result
REDACTED

func (s *SettingService) GetOpenAIAPIKeyHealthBreakerSettings(ctx context.Context) (*OpenAIAPIKeyHealthBreakerSettings, error) {
	if s == nil || s.settingRepo == nil {
		return DefaultOpenAIAPIKeyHealthBreakerSettings(), nil
REDACTED
	if cached, ok := s.openAIAPIKeyHealthBreakerCache.Load().(*cachedOpenAIAPIKeyHealthBreakerSettings); ok && cached != nil && time.Now().Before(cached.expiresAt) {
		result := cached.settings
		return &result, nil
REDACTED

	settings := DefaultOpenAIAPIKeyHealthBreakerSettings()
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIAPIKeyHealthBreakerSettings)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, fmt.Errorf("get OpenAI API key health breaker settings: %w", err)
REDACTED
	if err == nil && strings.TrimSpace(value) != "" {
		var stored OpenAIAPIKeyHealthBreakerSettings
		if json.Unmarshal([]byte(value), &stored) == nil {
			settings = normalizeOpenAIAPIKeyHealthBreakerSettings(&stored)
	REDACTED
REDACTED
	s.openAIAPIKeyHealthBreakerCache.Store(&cachedOpenAIAPIKeyHealthBreakerSettings{
		settings:  *settings,
		expiresAt: time.Now().Add(openAIAPIKeyHealthBreakerSettingsCacheTTL),
REDACTED)
	result := *settings
	return &result, nil
REDACTED
