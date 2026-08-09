//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

type tokenVersionUserRepoStub struct {
	*userRepoStub
}

func (s *tokenVersionUserRepoStub) GetTokenVersion(_ context.Context, userID int64) (int64, error) {
	user, err := s.GetByID(context.Background(), userID)
	if err != nil {
		return 0, err
	}
	return user.TokenVersion, nil
}

func (s *tokenVersionUserRepoStub) IncrementTokenVersion(_ context.Context, userID int64) (int64, error) {
	user, err := s.GetByID(context.Background(), userID)
	if err != nil {
		return 0, err
	}
	user.TokenVersion++
	user.TokenVersionResolved = true
	return user.TokenVersion, nil
}

type tokenVersionRaceCache struct {
	mu              sync.Mutex
	tokens          map[string]*RefreshTokenData
	consumed        map[string]*RefreshTokenData
	revokedFamilies map[string]struct{}
	onStore         func()
	storeErr        error
	consumeErr      error
	storeCalls      int
	revokedUserIDs  []int64
}

func newTokenVersionRaceCache() *tokenVersionRaceCache {
	return &tokenVersionRaceCache{
		tokens:          make(map[string]*RefreshTokenData),
		consumed:        make(map[string]*RefreshTokenData),
		revokedFamilies: make(map[string]struct{}),
	}
}

func (s *tokenVersionRaceCache) StoreRefreshToken(_ context.Context, tokenHash string, data *RefreshTokenData, _ time.Duration) error {
	s.mu.Lock()
	s.storeCalls++
	if s.storeErr != nil {
		err := s.storeErr
		s.mu.Unlock()
		return err
	}
	if _, revoked := s.revokedFamilies[data.FamilyID]; revoked {
		s.mu.Unlock()
		return ErrRefreshTokenReused
	}
	hook := s.onStore
	s.onStore = nil
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, revoked := s.revokedFamilies[data.FamilyID]; revoked {
		return ErrRefreshTokenReused
	}
	cloned := *data
	s.tokens[tokenHash] = &cloned
	return nil
}

func (s *tokenVersionRaceCache) GetRefreshToken(_ context.Context, tokenHash string) (*RefreshTokenData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.tokens[tokenHash]
	if !ok {
		return nil, ErrRefreshTokenNotFound
	}
	cloned := *data
	return &cloned, nil
}

func (s *tokenVersionRaceCache) ConsumeRefreshToken(_ context.Context, tokenHash string) (*RefreshTokenData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.consumeErr != nil {
		return nil, s.consumeErr
	}
	if data, ok := s.tokens[tokenHash]; ok {
		cloned := *data
		delete(s.tokens, tokenHash)
		s.consumed[tokenHash] = &cloned
		return &cloned, nil
	}
	if data, ok := s.consumed[tokenHash]; ok {
		cloned := *data
		s.revokedFamilies[data.FamilyID] = struct{}{}
		for childHash, child := range s.tokens {
			if child.FamilyID == data.FamilyID {
				delete(s.tokens, childHash)
			}
		}
		return &cloned, ErrRefreshTokenReused
	}
	return nil, ErrRefreshTokenNotFound
}

func (s *tokenVersionRaceCache) AcknowledgeRefreshTokenReplay(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.consumed, tokenHash)
	return nil
}

func (s *tokenVersionRaceCache) DeleteRefreshToken(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	delete(s.tokens, tokenHash)
	s.mu.Unlock()
	return nil
}

func (s *tokenVersionRaceCache) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = make(map[string]*RefreshTokenData)
	s.revokedUserIDs = append(s.revokedUserIDs, userID)
	return nil
}

func (s *tokenVersionRaceCache) DeleteTokenFamily(_ context.Context, familyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokedFamilies[familyID] = struct{}{}
	for tokenHash, data := range s.tokens {
		if data.FamilyID == familyID {
			delete(s.tokens, tokenHash)
		}
	}
	return nil
}
func (s *tokenVersionRaceCache) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}
func (s *tokenVersionRaceCache) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}
func (s *tokenVersionRaceCache) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
}
func (s *tokenVersionRaceCache) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *tokenVersionRaceCache) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return true, nil
}

func TestRefreshInFlightCannotCrossRevokeAllTokenVersionBoundary(t *testing.T) {
	ctx := context.Background()
	user := &User{
		ID:                   42,
		Email:                "race@example.com",
		Role:                 RoleUser,
		Status:               StatusActive,
		TokenVersion:         4,
		TokenVersionResolved: true,
	}
	repo := &tokenVersionUserRepoStub{userRepoStub: &userRepoStub{user: user}}
	cache := newTokenVersionRaceCache()
	svc := NewAuthService(nil, repo, nil, cache, &config.Config{JWT: config.JWTConfig{
		Secret:                 "test-token-version-race-secret",
		ExpireHour:             1,
		RefreshTokenExpireDays: 7,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)

	original, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)

	// Revoke precisely after the old refresh token passed its version check but
	// while the replacement pair is being minted.
	cache.onStore = func() {
		require.NoError(t, svc.RevokeAllUserTokens(ctx, user.ID))
	}
	replacement, err := svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.NoError(t, err)
	require.Equal(t, int64(5), user.TokenVersion)
	require.Equal(t, []int64{42}, cache.revokedUserIDs)

	accessClaims, err := svc.ValidateToken(replacement.AccessToken)
	require.NoError(t, err)
	require.Equal(t, int64(4), accessClaims.TokenVersion, "in-flight refresh must retain its authenticated pre-revocation stamp")
	require.NotEqual(t, user.TokenVersion, accessClaims.TokenVersion)

	cache.mu.Lock()
	storedReplacement := cache.tokens[hashToken(replacement.RefreshToken)]
	cache.mu.Unlock()
	require.NotNil(t, storedReplacement)
	require.Equal(t, int64(4), storedReplacement.TokenVersion)

	_, err = svc.RefreshTokenPair(ctx, replacement.RefreshToken)
	require.ErrorIs(t, err, ErrTokenRevoked)
}

func TestRefreshTokenPairFailsClosedWhenReplacementStoreFails(t *testing.T) {
	ctx := context.Background()
	user := &User{
		ID:                   81,
		Email:                "store-failure@example.com",
		Role:                 RoleUser,
		Status:               StatusActive,
		TokenVersion:         2,
		TokenVersionResolved: true,
	}
	repo := &tokenVersionUserRepoStub{userRepoStub: &userRepoStub{user: user}}
	cache := newTokenVersionRaceCache()
	svc := NewAuthService(nil, repo, nil, cache, &config.Config{JWT: config.JWTConfig{
		Secret:                 "test-refresh-store-failure-secret",
		ExpireHour:             1,
		RefreshTokenExpireDays: 7,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)

	original, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	cache.mu.Lock()
	cache.storeErr = errors.New("redis write failed")
	storeCallsBefore := cache.storeCalls
	cache.mu.Unlock()

	replacement, err := svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.Error(t, err)
	require.Nil(t, replacement, "a pair with unpersisted refresh state must never escape")
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, storeCallsBefore+1, cache.storeCalls)
	require.Empty(t, cache.tokens)
}

func TestRefreshTokenPairFailsClosedWhenAtomicConsumeFails(t *testing.T) {
	ctx := context.Background()
	user := &User{
		ID:                   82,
		Email:                "consume-failure@example.com",
		Role:                 RoleUser,
		Status:               StatusActive,
		TokenVersion:         3,
		TokenVersionResolved: true,
	}
	repo := &tokenVersionUserRepoStub{userRepoStub: &userRepoStub{user: user}}
	cache := newTokenVersionRaceCache()
	svc := NewAuthService(nil, repo, nil, cache, &config.Config{JWT: config.JWTConfig{
		Secret:                 "test-refresh-consume-failure-secret",
		ExpireHour:             1,
		RefreshTokenExpireDays: 7,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)

	original, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	cache.mu.Lock()
	cache.consumeErr = errors.New("redis unavailable")
	storeCallsBefore := cache.storeCalls
	cache.mu.Unlock()

	replacement, err := svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.ErrorIs(t, err, ErrServiceUnavailable)
	require.Nil(t, replacement)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, storeCallsBefore, cache.storeCalls, "consume errors must not mint/store a child")
	require.Contains(t, cache.tokens, hashToken(original.RefreshToken), "failed consume must not delete the parent")
}

func TestRefreshTokenReplayAdvancesPersistentVersionAndInvalidatesWinner(t *testing.T) {
	ctx := context.Background()
	user := &User{
		ID:                   83,
		Email:                "refresh-replay@example.com",
		Role:                 RoleUser,
		Status:               StatusActive,
		TokenVersion:         10,
		TokenVersionResolved: true,
	}
	repo := &tokenVersionUserRepoStub{userRepoStub: &userRepoStub{user: user}}
	cache := newTokenVersionRaceCache()
	svc := NewAuthService(nil, repo, nil, cache, &config.Config{JWT: config.JWTConfig{
		Secret:                 "test-refresh-replay-secret",
		ExpireHour:             1,
		RefreshTokenExpireDays: 7,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)

	original, err := svc.GenerateTokenPair(ctx, user, "")
	require.NoError(t, err)
	winner, err := svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.NoError(t, err)

	replayed, err := svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.ErrorIs(t, err, ErrRefreshTokenReused)
	require.Nil(t, replayed)
	require.Equal(t, int64(11), user.TokenVersion)
	require.Equal(t, []int64{user.ID}, cache.revokedUserIDs)

	claims, err := svc.ValidateToken(winner.AccessToken)
	require.NoError(t, err)
	require.Equal(t, int64(10), claims.TokenVersion)
	require.NotEqual(t, user.TokenVersion, claims.TokenVersion)
	_, err = svc.RefreshToken(ctx, winner.AccessToken)
	require.ErrorIs(t, err, ErrTokenRevoked, "replay must invalidate the winner's stateless access token")

	cache.mu.Lock()
	require.Empty(t, cache.tokens, "replay must remove the winner's child refresh state")
	_, familyRevoked := cache.revokedFamilies[claims.SessionID]
	cache.mu.Unlock()
	require.True(t, familyRevoked)

	_, err = svc.RefreshTokenPair(ctx, original.RefreshToken)
	require.ErrorIs(t, err, ErrRefreshTokenInvalid)
	require.Equal(t, int64(11), user.TokenVersion, "an acknowledged replay must not repeatedly revoke later logins")
}

func TestPersistentTokenVersionMigrationRequiresOneFreshLogin(t *testing.T) {
	svc := NewAuthService(nil, &userRepoStub{}, nil, nil, &config.Config{JWT: config.JWTConfig{
		Secret:     "test-token-version-migration-secret",
		ExpireHour: 1,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)
	legacyUser := &User{
		ID:           77,
		Email:        "legacy-session@example.com",
		PasswordHash: "legacy-password-hash",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 0,
	}
	now := time.Now()
	legacyUnsignedClaims := &JWTClaims{
		UserID:       legacyUser.ID,
		Email:        legacyUser.Email,
		Role:         legacyUser.Role,
		TokenVersion: resolvedTokenVersion(legacyUser),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	legacyToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, legacyUnsignedClaims).
		SignedString([]byte("test-token-version-migration-secret"))
	require.NoError(t, err)
	legacyClaims, err := svc.ValidateToken(legacyToken)
	require.ErrorIs(t, err, ErrTokenRevoked)
	require.NotNil(t, legacyClaims)
	require.Equal(t, 0, legacyClaims.TokenVersionEpoch)

	// Migration 221 initializes the durable stamp to zero. That intentionally
	// rejects pre-migration fingerprint claims and forces one fresh login.
	migratedUser := *legacyUser
	migratedUser.TokenVersion = 0
	migratedUser.TokenVersionResolved = true
	require.NotEqual(t, currentTokenVersionEpoch, legacyClaims.TokenVersionEpoch)

	freshToken, err := svc.GenerateToken(context.Background(), &migratedUser)
	require.NoError(t, err)
	freshClaims, err := svc.ValidateToken(freshToken)
	require.NoError(t, err)
	require.Equal(t, currentTokenVersionEpoch, freshClaims.TokenVersionEpoch)
	require.Equal(t, migratedUser.TokenVersion, freshClaims.TokenVersion)
}

func TestRefreshTokenPairRejectsLegacyTokenVersionEpoch(t *testing.T) {
	ctx := context.Background()
	user := &User{
		ID:                   78,
		Email:                "legacy-refresh@example.com",
		Role:                 RoleUser,
		Status:               StatusActive,
		TokenVersion:         0,
		TokenVersionResolved: true,
	}
	repo := &tokenVersionUserRepoStub{userRepoStub: &userRepoStub{user: user}}
	cache := newTokenVersionRaceCache()
	legacyRefresh := "rt_legacy_refresh_token"
	cache.tokens[hashToken(legacyRefresh)] = &RefreshTokenData{
		UserID:       user.ID,
		TokenVersion: user.TokenVersion,
		FamilyID:     "legacy-family",
		CreatedAt:    time.Now().Add(-time.Minute),
		ExpiresAt:    time.Now().Add(time.Hour),
		// TokenVersionEpoch intentionally omitted, as in pre-migration Redis data.
	}
	svc := NewAuthService(nil, repo, nil, cache, &config.Config{JWT: config.JWTConfig{
		Secret:                 "test-legacy-refresh-epoch-secret",
		ExpireHour:             1,
		RefreshTokenExpireDays: 7,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := svc.RefreshTokenPair(ctx, legacyRefresh)
	require.ErrorIs(t, err, ErrTokenRevoked)
}

func TestValidateTokenNeverTreatsInvalidSignatureAsRefreshableExpiry(t *testing.T) {
	user := &User{
		ID:                   79,
		Email:                "signature-check@example.com",
		Role:                 RoleUser,
		Status:               StatusActive,
		TokenVersion:         0,
		TokenVersionResolved: true,
	}
	svc := NewAuthService(nil, &userRepoStub{user: user}, nil, nil, &config.Config{JWT: config.JWTConfig{
		Secret:     "real-signing-secret",
		ExpireHour: 1,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)
	now := time.Now()
	claims := &JWTClaims{
		UserID:            user.ID,
		Email:             user.Email,
		Role:              user.Role,
		TokenVersion:      user.TokenVersion,
		TokenVersionEpoch: currentTokenVersionEpoch,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Hour)),
		},
	}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("attacker-secret"))
	require.NoError(t, err)

	parsed, err := svc.ValidateToken(forged)
	require.ErrorIs(t, err, ErrInvalidToken)
	require.Nil(t, parsed)
	_, err = svc.RefreshToken(context.Background(), forged)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateTokenRejectsUnmintedHMACAlgorithms(t *testing.T) {
	svc := NewAuthService(nil, &userRepoStub{}, nil, nil, &config.Config{JWT: config.JWTConfig{
		Secret:     "single-algorithm-secret",
		ExpireHour: 1,
	}}, nil, nil, nil, nil, nil, nil, nil, nil)
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, &JWTClaims{
		UserID:            80,
		Email:             "algorithm@example.com",
		Role:              RoleUser,
		TokenVersionEpoch: currentTokenVersionEpoch,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}).SignedString([]byte("single-algorithm-secret"))
	require.NoError(t, err)

	_, err = svc.ValidateToken(token)
	require.ErrorIs(t, err, ErrInvalidToken)
}
