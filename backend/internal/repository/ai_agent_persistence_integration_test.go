//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAIAgentEncryptedPersistenceUsesIsolatedPostgresAndRedis(t *testing.T) {
	ctx := context.Background()
	userID := time.Now().UnixNano()
	settings := NewSettingRepository(testEntClient(t))
	require.NoError(t, settings.Set(ctx, "ai_agent_enabled", "true"))
	t.Cleanup(func() {
		_, _ = testEntClient(t).Setting.Delete().Where(setting.KeyEQ("ai_agent_enabled")).Exec(context.Background())
	})
	encryptor, err := NewAESEncryptor(&config.Config{Totp: config.TotpConfig{
		EncryptionKey: strings.Repeat("42", 32),
	}})
	require.NoError(t, err)
	internalAuth, err := service.NewAgentInternalAuth()
	require.NoError(t, err)

	first, err := service.NewAIAgentService(settings, encryptor, &config.Config{}, internalAuth)
	require.NoError(t, err)
	created, err := first.CreateConversation(ctx, userID)
	require.NoError(t, err)
	require.NotEmpty(t, created.Conversation.ID)

	storageKey := fmt.Sprintf("ai_agent_conversations_%d_encrypted", userID)
	t.Cleanup(func() {
		_, _ = testEntClient(t).Setting.Delete().Where(setting.KeyEQ(storageKey)).Exec(context.Background())
	})
	ciphertext, err := settings.GetValue(ctx, storageKey)
	require.NoError(t, err)
	require.NotContains(t, ciphertext, created.Conversation.ID)
	require.NotContains(t, ciphertext, "messages")

	second, err := service.NewAIAgentService(settings, encryptor, &config.Config{}, internalAuth)
	require.NoError(t, err)
	restored, err := second.Conversations(ctx, userID)
	require.NoError(t, err)
	require.Len(t, restored.Conversations, 1)
	require.Equal(t, created.Conversation.ID, restored.Conversations[0].ID)

	require.NoError(t, second.DeleteConversation(ctx, userID, created.Conversation.ID))
	afterDelete, err := second.Conversations(ctx, userID)
	require.NoError(t, err)
	require.Empty(t, afterDelete.Conversations)

	redis := testRedis(t)
	redisKey := "agent-destructive-isolation-canary"
	require.NoError(t, redis.Set(ctx, redisKey, created.Conversation.ID, time.Minute).Err())
	require.NoError(t, redis.Del(ctx, redisKey).Err())
	exists, err := redis.Exists(ctx, redisKey).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}
