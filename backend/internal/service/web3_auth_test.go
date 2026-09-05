package service

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/require"
)

type web3ChallengeStoreStub struct {
	challenge *Web3Challenge
	getCalls  int
}

type web3SettingRepositoryStub struct {
	frontendURL         string
	registrationEnabled bool
}

func (s *web3SettingRepositoryStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *web3SettingRepositoryStub) GetValue(_ context.Context, key string) (string, error) {
	if key == SettingKeyFrontendURL && s.frontendURL != "" {
		return s.frontendURL, nil
	}
	if key == SettingKeyRegistrationEnabled && s.registrationEnabled {
		return "true", nil
	}
	return "", ErrSettingNotFound
}

func (s *web3SettingRepositoryStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *web3SettingRepositoryStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *web3SettingRepositoryStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *web3SettingRepositoryStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *web3SettingRepositoryStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func (s *web3ChallengeStoreStub) Save(context.Context, string, Web3Challenge, time.Duration) error {
	return nil
}

func (s *web3ChallengeStoreStub) Get(context.Context, string) (*Web3Challenge, error) {
	s.getCalls++
	return s.challenge, nil
}

func (s *web3ChallengeStoreStub) Consume(context.Context, string, string) (bool, error) {
	return true, nil
}

func (s *web3ChallengeStoreStub) RecordFailure(context.Context, string) error {
	return nil
}

func TestNormalizeWeb3Address(t *testing.T) {
	normalized, checksum, err := NormalizeWeb3Address("  0x52908400098527886e0f7030069857d2e4169ee7  ")
	require.NoError(t, err)
	require.Equal(t, "0x52908400098527886e0f7030069857d2e4169ee7", normalized)
	require.Equal(t, "0x52908400098527886E0F7030069857D2E4169EE7", checksum)

	for _, address := range []string{"", "0x1234", "52908400098527886e0f7030069857d2e4169ee7", "0xzz908400098527886e0f7030069857d2e4169ee7"} {
		_, _, err := NormalizeWeb3Address(address)
		require.ErrorIs(t, err, ErrWeb3AddressInvalid)
	}
}

func TestNormalizeWeb3ChainID(t *testing.T) {
	chainID, err := normalizeWeb3ChainID("0008453")
	require.NoError(t, err)
	require.Equal(t, "8453", chainID)

	for _, value := range []string{"", "0", "-1", "0x1", "mainnet"} {
		_, err := normalizeWeb3ChainID(value)
		require.ErrorIs(t, err, ErrWeb3ChainIDInvalid)
	}
}

func TestNormalizeWeb3Username(t *testing.T) {
	username, err := normalizeWeb3Username("  Alice  ")
	require.NoError(t, err)
	require.Equal(t, "Alice", username)

	_, err = normalizeWeb3Username("   ")
	require.ErrorIs(t, err, ErrWeb3UsernameRequired)

	_, err = normalizeWeb3Username("A")
	require.ErrorIs(t, err, ErrWeb3UsernameTooShort)

	_, err = normalizeWeb3Username(strings.Repeat("用", 101))
	require.ErrorIs(t, err, ErrWeb3UsernameTooLong)

	_, err = normalizeWeb3Username("Alice\nAdmin")
	require.ErrorIs(t, err, ErrWeb3UsernameInvalid)
}

func TestVerifyWeb3RegistrationRejectsUsernameBeforeConsumingChallenge(t *testing.T) {
	cfg := &config.Config{}
	settingService := NewSettingService(&web3SettingRepositoryStub{
		registrationEnabled: true,
	}, cfg)
	challengeStore := &web3ChallengeStoreStub{}
	web3Service := NewWeb3AuthService(nil, challengeStore, &AuthService{
		cfg:            cfg,
		settingService: settingService,
	})

	_, err := web3Service.VerifyRegistration(context.Background(), Web3RegistrationVerifyInput{
		Username: "   ",
	})

	require.ErrorIs(t, err, ErrWeb3UsernameRequired)
	require.Zero(t, challengeStore.getCalls)
}

func TestBuildWeb3SIWEMessage(t *testing.T) {
	issuedAt := time.Date(2026, time.August, 4, 1, 2, 3, 0, time.UTC)
	expiresAt := issuedAt.Add(5 * time.Minute)
	message := buildWeb3SIWEMessage(
		"example.com",
		"0x52908400098527886E0F7030069857D2E4169EE7",
		"Sign in to Sub2API.",
		"https://example.com",
		"1",
		"abcdef1234567890",
		issuedAt,
		expiresAt,
		"request-id",
	)

	require.Contains(t, message, "example.com wants you to sign in with your Ethereum account:")
	require.Contains(t, message, "Chain ID: 1")
	require.Contains(t, message, "Nonce: abcdef1234567890")
	require.Contains(t, message, "Issued At: 2026-08-04T01:02:03Z")
	require.Contains(t, message, "Expiration Time: 2026-08-04T01:07:03Z")
}

func TestVerifyWeb3EOASignature(t *testing.T) {
	privateKey := secp256k1.PrivKeyFromBytes([]byte{
		1, 2, 3, 4, 5, 6, 7, 8,
		9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24,
		25, 26, 27, 28, 29, 30, 31, 32,
	})
	message := "Sign in to Sub2API"
	compact := ecdsa.SignCompact(privateKey, ethereumPersonalSignHash(message), false)
	require.Len(t, compact, 65)
	recoveryID := compact[0] - 27
	require.LessOrEqual(t, recoveryID, byte(1))
	ethereumSignature := append(append([]byte{}, compact[1:]...), recoveryID)

	publicKeyHash := keccak256(privateKey.PubKey().SerializeUncompressed()[1:])
	expectedAddress := "0x" + hex.EncodeToString(publicKeyHash[len(publicKeyHash)-20:])
	signatureHex := "0x" + hex.EncodeToString(ethereumSignature)

	require.True(t, verifyWeb3EOASignature(message, signatureHex, expectedAddress))
	require.True(t, verifyWeb3EOASignature(message, strings.ToUpper(signatureHex[:2])+signatureHex[2:], expectedAddress))
	require.False(t, verifyWeb3EOASignature(message+"!", signatureHex, expectedAddress))
	require.False(t, verifyWeb3EOASignature(message, signatureHex, "0x0000000000000000000000000000000000000000"))
	require.False(t, verifyWeb3EOASignature(message, "0x1234", expectedAddress))
}

func TestWeb3SyntheticEmail(t *testing.T) {
	email := web3SyntheticEmail("0x52908400098527886e0f7030069857d2e4169ee7")
	require.Equal(t, "web3-52908400098527886e0f7030069857d2e4169ee7@web3-connect.invalid", email)
	require.True(t, IsWeb3SyntheticEmail(email))
	require.True(t, isReservedEmail(email))
	require.False(t, IsWeb3SyntheticEmail("user@example.com"))
}

func TestVerifyWeb3ChallengeRejectsDifferentBrowserSession(t *testing.T) {
	service := &Web3AuthService{challengeStore: &web3ChallengeStoreStub{challenge: &Web3Challenge{
		Intent:             web3LoginIntent,
		Message:            "message",
		MessageDigest:      hashWeb3Value("message"),
		BrowserSessionHash: hashWeb3Value("browser-a"),
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
	}}}

	_, err := service.verifyAndConsumeChallenge(context.Background(), web3LoginIntent, Web3VerifyInput{
		ChallengeToken: "token",
		Signature:      "0xinvalid",
		BrowserSession: "browser-b",
	})
	require.ErrorIs(t, err, ErrWeb3ChallengeSessionMismatch)
}

func TestCreateWeb3ChallengeUsesFrontendURLFromSettings(t *testing.T) {
	cfg := &config.Config{}
	settingService := NewSettingService(&web3SettingRepositoryStub{
		frontendURL: "https://app.example.com",
	}, cfg)
	authService := &AuthService{
		cfg:            cfg,
		settingService: settingService,
	}
	web3Service := NewWeb3AuthService(nil, &web3ChallengeStoreStub{}, authService)

	result, err := web3Service.CreateLoginChallenge(context.Background(), Web3ChallengeInput{
		Address:        "0x52908400098527886e0f7030069857d2e4169ee7",
		ChainID:        "1",
		BrowserSession: "browser-session",
	})

	require.NoError(t, err)
	require.Contains(t, result.Message, "app.example.com wants you to sign in")
	require.Contains(t, result.Message, "URI: https://app.example.com")
}
