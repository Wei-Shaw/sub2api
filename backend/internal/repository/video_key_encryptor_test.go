package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewVideoKeyEncryptorReportsDedicatedOperatorSetting(t *testing.T) {
	_, err := NewVideoKeyEncryptor(&config.Config{})
	require.Error(t, err)
	require.ErrorContains(t, err, "VIDEO_GATEWAY_ENCRYPTION_KEY")
	require.ErrorContains(t, err, "dedicated")
}

func TestNewVideoKeyEncryptorRejectsMalformedKeyAndRoundTrips(t *testing.T) {
	_, err := NewVideoKeyEncryptor(&config.Config{VideoGateway: config.VideoGatewayConfig{EncryptionKey: "not-hex"}})
	require.ErrorContains(t, err, "invalid video gateway encryption key")

	encryptor, err := NewVideoKeyEncryptor(&config.Config{VideoGateway: config.VideoGatewayConfig{EncryptionKey: strings.Repeat("11", 32)}})
	require.NoError(t, err)
	ciphertext, err := encryptor.Encrypt("test-only-provider-credential")
	require.NoError(t, err)
	plaintext, err := encryptor.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "test-only-provider-credential", plaintext)
}
