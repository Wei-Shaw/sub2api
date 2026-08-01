package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
)

const maxCustomMediaModelPatternsLength = 8192

type mediaModelKind uint8

const (
	mediaModelUnknown mediaModelKind = iota
	mediaModelImage
	mediaModelVideo
)

type customMediaModelRules struct {
	imagePatterns []string
	videoPatterns []string
}

var activeCustomMediaModelRules atomic.Pointer[customMediaModelRules]

func init() {
	activeCustomMediaModelRules.Store(&customMediaModelRules{})
}

func normalizeCustomMediaModelPatterns(raw string) string {
	patterns := parseCustomMediaModelPatterns(raw)
	return strings.Join(patterns, "\n")
}

func parseCustomMediaModelPatterns(raw string) []string {
	parts := strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	seen := make(map[string]struct{}, len(parts))
	patterns := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		patterns = append(patterns, part)
	}
	return patterns
}

func ValidateCustomMediaModelPatterns(raw string) error {
	if len(raw) > maxCustomMediaModelPatternsLength {
		return fmt.Errorf("custom media model patterns must be at most %d characters", maxCustomMediaModelPatternsLength)
	}
	for _, pattern := range parseCustomMediaModelPatterns(raw) {
		if len(pattern) > 256 {
			return fmt.Errorf("custom media model pattern %q must be at most 256 characters", pattern)
		}
	}
	return nil
}

func setCustomMediaModelPatterns(imagePatterns, videoPatterns string) {
	activeCustomMediaModelRules.Store(&customMediaModelRules{
		imagePatterns: parseCustomMediaModelPatterns(imagePatterns),
		videoPatterns: parseCustomMediaModelPatterns(videoPatterns),
	})
}

func classifyCustomMediaModel(model string) mediaModelKind {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return mediaModelUnknown
	}
	rules := activeCustomMediaModelRules.Load()
	if rules == nil {
		return mediaModelUnknown
	}
	for _, pattern := range rules.videoPatterns {
		if matchCustomMediaModelPattern(pattern, model) {
			return mediaModelVideo
		}
	}
	for _, pattern := range rules.imagePatterns {
		if matchCustomMediaModelPattern(pattern, model) {
			return mediaModelImage
		}
	}
	return mediaModelUnknown
}

func matchCustomMediaModelPattern(pattern, model string) bool {
	if !strings.Contains(pattern, "*") {
		return model == pattern
	}
	parts := strings.Split(pattern, "*")
	position := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		index := strings.Index(model[position:], part)
		if index < 0 || (i == 0 && index != 0) {
			return false
		}
		position += index + len(part)
	}
	last := parts[len(parts)-1]
	return last == "" || strings.HasSuffix(model, last)
}

func (s *SettingService) LoadCustomMediaModelPatterns(ctx context.Context) error {
	load := func(key string) (string, error) {
		value, err := s.settingRepo.GetValue(ctx, key)
		if errors.Is(err, ErrSettingNotFound) {
			return "", nil
		}
		return value, err
	}
	imagePatterns, err := load(SettingKeyCustomImageModelPatterns)
	if err != nil {
		return fmt.Errorf("load custom image model patterns: %w", err)
	}
	videoPatterns, err := load(SettingKeyCustomVideoModelPatterns)
	if err != nil {
		return fmt.Errorf("load custom video model patterns: %w", err)
	}
	setCustomMediaModelPatterns(imagePatterns, videoPatterns)
	return nil
}
