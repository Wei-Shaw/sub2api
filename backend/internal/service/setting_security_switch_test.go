//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type securitySettingRepoStub struct {
	SettingRepository
	value string
	err   error
	key   string
}

func (s *securitySettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	s.key = key
	return s.value, s.err
}

func TestSecuritySettingsClassifyRepositoryErrors(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		err     error
		enabled bool
	}{
		{name: "enabled", value: "true", enabled: true},
		{name: "disabled", value: "false", enabled: false},
		{name: "missing defaults off", err: ErrSettingNotFound, enabled: false},
		{name: "read failure fails closed", err: errors.New("database unavailable"), enabled: true},
	}

	settings := []struct {
		name string
		key  string
		get  func(*SettingService) bool
	}{
		{
			name: "session binding",
			key:  SettingKeySessionBindingEnabled,
			get: func(service *SettingService) bool {
				return service.IsSessionBindingEnabled(context.Background())
			},
		},
		{
			name: "step-up",
			key:  SettingKeyStepUpEnabled,
			get: func(service *SettingService) bool {
				return service.IsStepUpEnabled(context.Background())
			},
		},
	}

	for _, setting := range settings {
		setting := setting
		for _, tt := range tests {
			tt := tt
			t.Run(setting.name+"/"+tt.name, func(t *testing.T) {
				repo := &securitySettingRepoStub{value: tt.value, err: tt.err}
				service := NewSettingService(repo, nil)

				require.Equal(t, tt.enabled, setting.get(service))
				require.Equal(t, setting.key, repo.key)
			})
		}
	}
}
