package liveattestation

import (
	"context"
	"errors"
)

var (
	ErrUnsupportedPlatform = errors.New("live attestation is only supported when Sub2API runs on macOS; Windows support is not implemented yet")
	ErrChatGPTAppMissing   = errors.New("live attestation requires the official ChatGPT app on the Sub2API server")
)

// Provider 在发起 Live 请求前生成 ChatGPT DeviceCheck attestation。
type Provider interface {
	Check(ctx context.Context) error
	Generate(ctx context.Context) (string, error)
}
