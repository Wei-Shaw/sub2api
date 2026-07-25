//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestUsageLog_SessionIDPersistence proves session_id round-trips from insert to
// read and is omitted (NULL) when absent.
func TestUsageLog_SessionIDPersistence(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: "session-id-" + uuid.NewString() + "@example.com"REDACTED)
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-session-" + uuid.NewString(), Name: "k"REDACTED)
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-session-" + uuid.NewString()REDACTED)

	sessionID := "sess-" + uuid.NewString()

	withSession := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.NewString(),
		Model:        "claude-3",
		InputTokens:  10,
		OutputTokens: 5,
		TotalCost:    1.0,
		ActualCost:   1.0,
		SessionID:    &sessionID,
		CreatedAt:    time.Now().UTC(),
REDACTED
	_, err := repo.Create(ctx, withSession)
REDACTED
	require.NotZero(t, withSession.ID)

	withoutSession := &service.UsageLog{
		UserID:       user.ID,
		APIKeyID:     apiKey.ID,
		AccountID:    account.ID,
		RequestID:    uuid.NewString(),
		Model:        "claude-3",
		InputTokens:  7,
		OutputTokens: 3,
		TotalCost:    0.5,
		ActualCost:   0.5,
		CreatedAt:    time.Now().UTC(),
REDACTED
	_, err = repo.Create(ctx, withoutSession)
REDACTED

	// Round-trip: session id survives insert → read.
	got, err := repo.GetByID(ctx, withSession.ID)
REDACTED
	require.NotNil(t, got.SessionID)
	require.Equal(t, sessionID, *got.SessionID)

	// Omission: absent session id reads back as nil (NULL), not empty string.
	gotNone, err := repo.GetByID(ctx, withoutSession.ID)
REDACTED
	require.Nil(t, gotNone.SessionID)
REDACTED
