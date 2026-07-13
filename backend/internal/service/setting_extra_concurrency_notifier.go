package service

import (
	"context"
)

type ExtraConcurrencySettingsNotifier interface {
	PublishExtraConcurrencySettingsState(ctx context.Context, enabled bool) error
	SerializeExtraConcurrencySettingsUpdate(
		ctx context.Context,
		enabled bool,
		reserveFence func(context.Context) (int64, error),
		update func(context.Context, int64) error,
	) error
	SubscribeExtraConcurrencySettingsInvalidation(ctx context.Context, handler func()) error
}

func (s *SettingService) SetExtraConcurrencySettingsNotifier(notifier ExtraConcurrencySettingsNotifier) {
	if s == nil {
		return
	}
	s.extraConcurrencyNotifier = notifier
}

func (s *SettingService) StartExtraConcurrencySettingsSubscriber(ctx context.Context) error {
	if s == nil || s.extraConcurrencyNotifier == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.extraConcurrencyNotifier.SubscribeExtraConcurrencySettingsInvalidation(ctx, func() {
		s.InvalidateExtraConcurrencyRuntimeSettings()
	})
}
