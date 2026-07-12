package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultFalUpscaleEndpoint       = "fal-ai/seedvr/upscale/image"
	defaultFalUpscaleTimeoutSeconds = 300
)

type FalUpscaleSettings struct {
	Endpoint       string
	Token          string
	TimeoutSeconds int
}

func DefaultFalUpscaleSettings() *FalUpscaleSettings {
	return &FalUpscaleSettings{Endpoint: defaultFalUpscaleEndpoint, TimeoutSeconds: defaultFalUpscaleTimeoutSeconds}
}

func (c *FalUpscaleSettings) Configured() bool {
	return c != nil && strings.TrimSpace(c.Endpoint) != "" && strings.TrimSpace(c.Token) != ""
}

func (s *SettingService) GetFalUpscaleSettings(ctx context.Context) *FalUpscaleSettings {
	out := DefaultFalUpscaleSettings()
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyFalUpscaleEndpoint,
		SettingKeyFalUpscaleToken,
		SettingKeyFalUpscaleTimeoutSeconds,
	})
	if err != nil {
		return out
	}
	if endpoint := strings.TrimSpace(values[SettingKeyFalUpscaleEndpoint]); endpoint != "" {
		out.Endpoint = endpoint
	}
	out.Token = strings.TrimSpace(values[SettingKeyFalUpscaleToken])
	if raw := strings.TrimSpace(values[SettingKeyFalUpscaleTimeoutSeconds]); raw != "" {
		if timeout, parseErr := strconv.Atoi(raw); parseErr == nil && timeout > 0 {
			out.TimeoutSeconds = timeout
		}
	}
	return out
}

func (s *SettingService) SetFalUpscaleSettings(ctx context.Context, input *FalUpscaleSettings) error {
	if input == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	timeout := input.TimeoutSeconds
	if timeout <= 0 {
		timeout = defaultFalUpscaleTimeoutSeconds
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		token = s.GetFalUpscaleSettings(ctx).Token
	}
	return s.settingRepo.SetMultiple(ctx, map[string]string{
		SettingKeyFalUpscaleEndpoint:       strings.TrimSpace(input.Endpoint),
		SettingKeyFalUpscaleToken:          token,
		SettingKeyFalUpscaleTimeoutSeconds: strconv.Itoa(timeout),
	})
}
