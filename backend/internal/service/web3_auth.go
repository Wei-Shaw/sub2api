package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

const (
	Web3SyntheticEmailDomain = "@web3-connect.invalid"
	web3ChallengeTTL         = 5 * time.Minute
	web3LoginIntent          = "login"
	web3RegisterIntent       = "register"
)

var (
	ErrWeb3AddressInvalid           = infraerrors.BadRequest("WEB3_ADDRESS_INVALID", "invalid EVM address")
	ErrWeb3ChainIDInvalid           = infraerrors.BadRequest("WEB3_CHAIN_ID_INVALID", "invalid EVM chain ID")
	ErrWeb3ChallengeNotFound        = infraerrors.BadRequest("WEB3_CHALLENGE_NOT_FOUND", "web3 challenge not found")
	ErrWeb3ChallengeExpired         = infraerrors.BadRequest("WEB3_CHALLENGE_EXPIRED", "web3 challenge has expired")
	ErrWeb3ChallengeConsumed        = infraerrors.Conflict("WEB3_CHALLENGE_CONSUMED", "web3 challenge has already been consumed")
	ErrWeb3ChallengeSessionMismatch = infraerrors.Unauthorized("WEB3_CHALLENGE_SESSION_MISMATCH", "web3 challenge browser session mismatch")
	ErrWeb3ChallengeIntentMismatch  = infraerrors.BadRequest("WEB3_CHALLENGE_INTENT_MISMATCH", "web3 challenge intent mismatch")
	ErrWeb3SignatureInvalid         = infraerrors.Unauthorized("WEB3_SIGNATURE_INVALID", "invalid wallet signature")
	ErrWeb3IdentityNotFound         = infraerrors.NotFound("WEB3_IDENTITY_NOT_FOUND", "wallet is not registered")
	ErrWeb3IdentityExists           = infraerrors.Conflict("WEB3_IDENTITY_EXISTS", "wallet is already registered")
	ErrWeb3UsernameRequired         = infraerrors.BadRequest("WEB3_USERNAME_REQUIRED", "username is required")
	ErrWeb3UsernameTooShort         = infraerrors.BadRequest("WEB3_USERNAME_TOO_SHORT", "username must contain at least 2 characters")
	ErrWeb3UsernameTooLong          = infraerrors.BadRequest("WEB3_USERNAME_TOO_LONG", "username must contain at most 100 characters")
	ErrWeb3UsernameInvalid          = infraerrors.BadRequest("WEB3_USERNAME_INVALID", "username contains invalid characters")
	ErrWeb3FrontendURLNotConfigured = infraerrors.ServiceUnavailable("WEB3_FRONTEND_URL_NOT_CONFIGURED", "web3 authentication frontend URL is not configured")
)

type Web3Challenge struct {
	Intent             string    `json:"intent"`
	Address            string    `json:"address"`
	ChecksumAddress    string    `json:"checksum_address"`
	ChainID            string    `json:"chain_id"`
	Nonce              string    `json:"nonce"`
	Message            string    `json:"message"`
	MessageDigest      string    `json:"message_digest"`
	BrowserSessionHash string    `json:"browser_session_hash"`
	FailedAttempts     int       `json:"failed_attempts"`
	CreatedAt          time.Time `json:"created_at"`
	ExpiresAt          time.Time `json:"expires_at"`
}

type Web3ChallengeInput struct {
	Address        string
	ChainID        string
	BrowserSession string
}

type Web3ChallengeResult struct {
	ChallengeToken string    `json:"challenge_token"`
	Message        string    `json:"message"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type Web3VerifyInput struct {
	ChallengeToken string
	Signature      string
	BrowserSession string
}

type Web3RegistrationVerifyInput struct {
	Web3VerifyInput
	Username       string
	InvitationCode string
	PromoCode      string
	AffiliateCode  string
}

type Web3LoginResult struct {
	User    *User
	Address string
}

type Web3UserCreateInput struct {
	Email        string
	PasswordHash string
	Username     string
	Role         string
	Status       string
	Balance      float64
	Concurrency  int
	RPMLimit     int
	Address      string
}

type Web3IdentityRepository interface {
	GetUserIDByAddress(ctx context.Context, address string) (int64, error)
	GetAddressByUserID(ctx context.Context, userID int64) (string, bool, error)
	ExistsByAddress(ctx context.Context, address string) (bool, error)
	CreateUserWithIdentity(ctx context.Context, input Web3UserCreateInput) (int64, error)
}

type Web3ChallengeStore interface {
	Save(ctx context.Context, tokenHash string, challenge Web3Challenge, ttl time.Duration) error
	Get(ctx context.Context, tokenHash string) (*Web3Challenge, error)
	Consume(ctx context.Context, tokenHash, expectedDigest string) (bool, error)
	RecordFailure(ctx context.Context, tokenHash string) error
}

type Web3AuthService struct {
	identityRepository Web3IdentityRepository
	challengeStore     Web3ChallengeStore
	authService        *AuthService
}

func NewWeb3AuthService(
	identityRepository Web3IdentityRepository,
	challengeStore Web3ChallengeStore,
	authService *AuthService,
) *Web3AuthService {
	return &Web3AuthService{
		identityRepository: identityRepository,
		challengeStore:     challengeStore,
		authService:        authService,
	}
}

func (s *Web3AuthService) CreateLoginChallenge(ctx context.Context, input Web3ChallengeInput) (*Web3ChallengeResult, error) {
	return s.createChallenge(ctx, web3LoginIntent, input)
}

func (s *Web3AuthService) CreateRegistrationChallenge(ctx context.Context, input Web3ChallengeInput) (*Web3ChallengeResult, error) {
	if s == nil || s.authService == nil || !s.authService.IsRegistrationEnabled(ctx) {
		return nil, ErrRegDisabled
	}
	return s.createChallenge(ctx, web3RegisterIntent, input)
}

func (s *Web3AuthService) createChallenge(ctx context.Context, intent string, input Web3ChallengeInput) (*Web3ChallengeResult, error) {
	if s == nil || s.challengeStore == nil || s.authService == nil || s.authService.cfg == nil {
		return nil, ErrServiceUnavailable
	}
	if strings.TrimSpace(input.BrowserSession) == "" {
		return nil, ErrWeb3ChallengeSessionMismatch
	}
	normalizedAddress, checksumAddress, err := NormalizeWeb3Address(input.Address)
	if err != nil {
		return nil, err
	}
	chainID, err := normalizeWeb3ChainID(input.ChainID)
	if err != nil {
		return nil, err
	}
	frontendURL := s.authService.cfg.Server.FrontendURL
	if s.authService.settingService != nil {
		frontendURL = s.authService.settingService.GetFrontendURL(ctx)
	}
	domain, uri, err := web3FrontendIdentity(frontendURL)
	if err != nil {
		return nil, err
	}
	token, err := randomWeb3Token(32)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	nonce, err := randomWeb3Nonce(16)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(web3ChallengeTTL)
	tokenHash := hashWeb3Value(token)
	statement := "Sign in to Sub2API. This request does not trigger a blockchain transaction or cost gas."
	if intent == web3RegisterIntent {
		statement = "Register a Sub2API account with this wallet. This request does not trigger a blockchain transaction or cost gas."
	}
	message := buildWeb3SIWEMessage(domain, checksumAddress, statement, uri, chainID, nonce, now, expiresAt, tokenHash[:16])
	challenge := Web3Challenge{
		Intent:             intent,
		Address:            normalizedAddress,
		ChecksumAddress:    checksumAddress,
		ChainID:            chainID,
		Nonce:              nonce,
		Message:            message,
		MessageDigest:      hashWeb3Value(message),
		BrowserSessionHash: hashWeb3Value(input.BrowserSession),
		CreatedAt:          now,
		ExpiresAt:          expiresAt,
	}
	if err := s.challengeStore.Save(ctx, tokenHash, challenge, web3ChallengeTTL); err != nil {
		return nil, ErrServiceUnavailable
	}
	return &Web3ChallengeResult{ChallengeToken: token, Message: message, ExpiresAt: expiresAt}, nil
}

func (s *Web3AuthService) VerifyLogin(ctx context.Context, input Web3VerifyInput) (*Web3LoginResult, error) {
	challenge, err := s.verifyAndConsumeChallenge(ctx, web3LoginIntent, input)
	if err != nil {
		return nil, err
	}
	userID, err := s.identityRepository.GetUserIDByAddress(ctx, challenge.Address)
	if err != nil {
		if errors.Is(err, ErrWeb3IdentityNotFound) {
			return nil, ErrWeb3IdentityNotFound
		}
		return nil, ErrServiceUnavailable
	}
	user, err := s.authService.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive() {
		return nil, ErrUserNotActive
	}
	return &Web3LoginResult{User: user, Address: challenge.ChecksumAddress}, nil
}

func (s *Web3AuthService) VerifyRegistration(ctx context.Context, input Web3RegistrationVerifyInput) (*User, error) {
	if s == nil || s.authService == nil || !s.authService.IsRegistrationEnabled(ctx) {
		return nil, ErrRegDisabled
	}
	username, err := normalizeWeb3Username(input.Username)
	if err != nil {
		return nil, err
	}
	challenge, err := s.verifyAndConsumeChallenge(ctx, web3RegisterIntent, input.Web3VerifyInput)
	if err != nil {
		return nil, err
	}
	invitation, err := s.validateRegistrationInvitation(ctx, input.InvitationCode)
	if err != nil {
		return nil, err
	}
	exists, err := s.identityRepository.ExistsByAddress(ctx, challenge.Address)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	if exists {
		return nil, ErrWeb3IdentityExists
	}
	randomPassword, err := randomHexString(32)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	passwordHash, err := s.authService.HashPassword(randomPassword)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	grantPlan := s.authService.resolveSignupGrantPlan(ctx, "email")
	defaultRPMLimit := 0
	if s.authService.settingService != nil {
		defaultRPMLimit = s.authService.settingService.GetDefaultUserRPMLimit(ctx)
	}
	userID, err := s.identityRepository.CreateUserWithIdentity(ctx, Web3UserCreateInput{
		Email:        web3SyntheticEmail(challenge.Address),
		PasswordHash: passwordHash,
		Username:     username,
		Role:         RoleUser,
		Status:       StatusActive,
		Balance:      grantPlan.Balance,
		Concurrency:  grantPlan.Concurrency,
		RPMLimit:     defaultRPMLimit,
		Address:      challenge.Address,
	})
	if err != nil {
		if errors.Is(err, ErrWeb3IdentityExists) {
			return nil, ErrWeb3IdentityExists
		}
		return nil, ErrServiceUnavailable
	}
	user, err := s.authService.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	s.authService.postAuthUserBootstrap(ctx, user, "email", true)
	s.authService.assignSubscriptions(ctx, user.ID, grantPlan.Subscriptions, "auto assigned by web3 signup defaults")
	_ = s.authService.snapshotPlatformQuotaDefaults(ctx, user.ID, &grantPlan)
	s.authService.bindOAuthAffiliate(ctx, user.ID, input.AffiliateCode)
	if invitation != nil {
		if err := s.authService.redeemRepo.Use(ctx, invitation.ID, user.ID); err != nil {
			logger.LegacyPrintf("service.web3_auth", "[Web3Auth] Failed to mark invitation as used: user_id=%d err=%v", user.ID, err)
		}
	}
	user = s.authService.applyOAuthSignupPromoCode(ctx, user, input.PromoCode)
	return user, nil
}

func (s *Web3AuthService) verifyAndConsumeChallenge(ctx context.Context, intent string, input Web3VerifyInput) (*Web3Challenge, error) {
	if s == nil || s.challengeStore == nil {
		return nil, ErrServiceUnavailable
	}
	token := strings.TrimSpace(input.ChallengeToken)
	if token == "" {
		return nil, ErrWeb3ChallengeNotFound
	}
	tokenHash := hashWeb3Value(token)
	challenge, err := s.challengeStore.Get(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if challenge.Intent != intent {
		return nil, ErrWeb3ChallengeIntentMismatch
	}
	if strings.TrimSpace(input.BrowserSession) == "" || subtle.ConstantTimeCompare(
		[]byte(challenge.BrowserSessionHash),
		[]byte(hashWeb3Value(input.BrowserSession)),
	) != 1 {
		return nil, ErrWeb3ChallengeSessionMismatch
	}
	if time.Now().UTC().After(challenge.ExpiresAt) {
		return nil, ErrWeb3ChallengeExpired
	}
	if hashWeb3Value(challenge.Message) != challenge.MessageDigest || !verifyWeb3EOASignature(challenge.Message, input.Signature, challenge.Address) {
		_ = s.challengeStore.RecordFailure(ctx, tokenHash)
		return nil, ErrWeb3SignatureInvalid
	}
	consumed, err := s.challengeStore.Consume(ctx, tokenHash, challenge.MessageDigest)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	if !consumed {
		return nil, ErrWeb3ChallengeConsumed
	}
	return challenge, nil
}

func (s *Web3AuthService) validateRegistrationInvitation(ctx context.Context, invitationCode string) (*RedeemCode, error) {
	if s.authService.settingService == nil || !s.authService.settingService.IsInvitationCodeEnabled(ctx) {
		return nil, nil
	}
	invitationCode = strings.TrimSpace(invitationCode)
	if invitationCode == "" {
		return nil, ErrInvitationCodeRequired
	}
	code, err := s.authService.redeemRepo.GetByCode(ctx, invitationCode)
	if err != nil || code.Type != RedeemTypeInvitation || !code.CanUse() {
		return nil, ErrInvitationCodeInvalid
	}
	return code, nil
}

func NormalizeWeb3Address(address string) (string, string, error) {
	address = strings.TrimSpace(address)
	if len(address) != 42 || !strings.EqualFold(address[:2], "0x") {
		return "", "", ErrWeb3AddressInvalid
	}
	lowerHex := strings.ToLower(address[2:])
	decoded, err := hex.DecodeString(lowerHex)
	if err != nil || len(decoded) != 20 {
		return "", "", ErrWeb3AddressInvalid
	}
	normalized := "0x" + lowerHex
	return normalized, checksumWeb3Address(lowerHex), nil
}

func normalizeWeb3ChainID(chainID string) (string, error) {
	chainID = strings.TrimSpace(chainID)
	if chainID == "" {
		return "", ErrWeb3ChainIDInvalid
	}
	value, ok := new(big.Int).SetString(chainID, 10)
	if !ok || value.Sign() <= 0 {
		return "", ErrWeb3ChainIDInvalid
	}
	return value.String(), nil
}

func normalizeWeb3Username(username string) (string, error) {
	username = strings.TrimSpace(username)
	length := utf8.RuneCountInString(username)
	if length == 0 {
		return "", ErrWeb3UsernameRequired
	}
	if length < 2 {
		return "", ErrWeb3UsernameTooShort
	}
	if length > 100 {
		return "", ErrWeb3UsernameTooLong
	}
	for _, character := range username {
		if unicode.IsControl(character) {
			return "", ErrWeb3UsernameInvalid
		}
	}
	return username, nil
}

func web3FrontendIdentity(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return "", "", ErrWeb3FrontendURLNotConfigured
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.Host, parsed.String(), nil
}

func buildWeb3SIWEMessage(domain, address, statement, uri, chainID, nonce string, issuedAt, expiresAt time.Time, requestID string) string {
	return fmt.Sprintf(
		"%s wants you to sign in with your Ethereum account:\n%s\n\n%s\n\nURI: %s\nVersion: 1\nChain ID: %s\nNonce: %s\nIssued At: %s\nExpiration Time: %s\nRequest ID: %s",
		domain,
		address,
		statement,
		uri,
		chainID,
		nonce,
		issuedAt.UTC().Format(time.RFC3339),
		expiresAt.UTC().Format(time.RFC3339),
		requestID,
	)
}

func verifyWeb3EOASignature(message, signatureHex, expectedAddress string) bool {
	signatureHex = strings.TrimSpace(signatureHex)
	signatureHex = strings.TrimPrefix(signatureHex, "0x")
	signatureHex = strings.TrimPrefix(signatureHex, "0X")
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != 65 {
		return false
	}
	if signature[64] == 27 || signature[64] == 28 {
		signature[64] -= 27
	}
	if signature[64] > 1 {
		return false
	}
	compactSignature := make([]byte, 65)
	compactSignature[0] = 27 + signature[64]
	copy(compactSignature[1:], signature[:64])
	publicKey, _, err := ecdsa.RecoverCompact(compactSignature, ethereumPersonalSignHash(message))
	if err != nil {
		return false
	}
	serialized := publicKey.SerializeUncompressed()
	publicKeyHash := keccak256(serialized[1:])
	recovered := "0x" + hex.EncodeToString(publicKeyHash[len(publicKeyHash)-20:])
	return recovered == strings.ToLower(strings.TrimSpace(expectedAddress))
}

func checksumWeb3Address(lowerHex string) string {
	hash := keccak256([]byte(lowerHex))
	checksum := []byte(lowerHex)
	for index, char := range checksum {
		if char < 'a' || char > 'f' {
			continue
		}
		nibble := hash[index/2]
		if index%2 == 0 {
			nibble >>= 4
		} else {
			nibble &= 0x0f
		}
		if nibble >= 8 {
			checksum[index] = char - ('a' - 'A')
		}
	}
	return "0x" + string(checksum)
}

func ethereumPersonalSignHash(message string) []byte {
	prefix := "\x19Ethereum Signed Message:\n" + strconv.Itoa(len([]byte(message)))
	return keccak256([]byte(prefix + message))
}

func keccak256(input []byte) []byte {
	hasher := sha3.NewLegacyKeccak256()
	_, _ = hasher.Write(input)
	return hasher.Sum(nil)
}

func web3SyntheticEmail(address string) string {
	return "web3-" + strings.TrimPrefix(strings.ToLower(address), "0x") + Web3SyntheticEmailDomain
}

func IsWeb3SyntheticEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(email)), Web3SyntheticEmailDomain)
}

func randomWeb3Token(byteLength int) (string, error) {
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomWeb3Nonce(byteLength int) (string, error) {
	return randomWeb3Token(byteLength)
}

func hashWeb3Value(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
