package upstreamstation

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type testEncryptor struct{}

func (testEncryptor) Encrypt(plaintext string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}
func (testEncryptor) Decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", errors.New("invalid ciphertext")
	}
	return string(data), nil
}

func TestCredentialCodecRoundTrip(t *testing.T) {
	t.Parallel()

	codec := NewCredentialCodec(testEncryptor{})
	want := Credentials{
		Username: "boss@example.com",
		Password: "secret",
		Extra:    map[string]any{"tenant": "alpha"},
	}

	ciphertext, err := codec.Encrypt(want)
	require.NoError(t, err)
	require.NotContains(t, ciphertext, "boss@example.com")
	require.NotContains(t, ciphertext, "secret")

	got, err := codec.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

type registryTestConnector struct {
	typeName string
	detect   bool
	calls    int
}

func (c *registryTestConnector) Type() string { return c.typeName }
func (c *registryTestConnector) Detect(context.Context, string) (bool, error) {
	c.calls++
	return c.detect, nil
}
func (*registryTestConnector) Authenticate(context.Context, *Station, Credentials) (*Session, error) {
	return nil, nil
}
func (*registryTestConnector) GetBalance(context.Context, string, *Session) (float64, error) {
	return 0, nil
}
func (*registryTestConnector) ListGroups(context.Context, string, *Session) ([]RemoteGroup, error) {
	return nil, nil
}
func (*registryTestConnector) GetRechargeMultiplier(context.Context, string, *Session) (float64, error) {
	return 0, nil
}

func TestConnectorRegistryResolvesExplicitAndAutoTypes(t *testing.T) {
	t.Parallel()

	newAPI := &registryTestConnector{typeName: SiteTypeNewAPI}
	sub2api := &registryTestConnector{typeName: SiteTypeSub2API, detect: true}
	registry := NewConnectorRegistry(newAPI, sub2api)

	explicit, err := registry.Resolve(context.Background(), &Station{SiteType: SiteTypeNewAPI})
	require.NoError(t, err)
	require.Same(t, newAPI, explicit)
	require.Zero(t, newAPI.calls)

	auto, err := registry.Resolve(context.Background(), &Station{SiteType: SiteTypeAuto, BaseURL: "https://station.example"})
	require.NoError(t, err)
	require.Same(t, sub2api, auto)
	require.Equal(t, 1, newAPI.calls)
	require.Equal(t, 1, sub2api.calls)
}
