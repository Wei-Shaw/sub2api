//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Runs the file pages against a real Redis server. ZRANGE ... BYLEX REV takes
// its bounds in the opposite order from the forward query.
func TestDeepseekFileStoreOrderedPagesRealRedis(t *testing.T) {
	cache := NewGatewayCache(testRedis(t)).(service.DeepseekFileStore)
	ctx := t.Context()
	now := time.Now().Truncate(time.Second).UTC()
	for i, id := range []string{"c", "b", "a", "d", "e"} {
		f := service.DeepseekFile{ID: id, AccountID: 11, Origin: service.DefaultDeepseekBaseURL, Format: "openai", CreatedAt: now, Purpose: "user_data"}
		if i == 0 {
			f.Purpose = "text_recognition"
		}
		if i == 4 {
			f.ExpiresAt = now.Add(-time.Minute).Unix()
		}
		require.NoError(t, cache.SaveDeepseekFile(ctx, 7, f))
	}
	pageIDs := func(page service.DeepseekFilePage) []string {
		t.Helper()
		files, err := cache.ListDeepseekFiles(ctx, 7, page)
		require.NoError(t, err)
		ids := []string{}
		for _, f := range files {
			ids = append(ids, f.ID)
		}
		return ids
	}
	require.Equal(t, []string{"a", "b", "c", "d"}, pageIDs(service.DeepseekFilePage{Limit: 10}))
	require.Equal(t, []string{"d", "c", "b", "a"}, pageIDs(service.DeepseekFilePage{Limit: 10, Descending: true}))
	require.Equal(t, []string{"a", "b"}, pageIDs(service.DeepseekFilePage{Limit: 1}))
	require.Equal(t, []string{"d"}, pageIDs(service.DeepseekFilePage{Limit: 1, Cursor: "b", Purpose: "user_data"}))
	require.Equal(t, []string{"b", "a"}, pageIDs(service.DeepseekFilePage{Limit: 1, Descending: true, Cursor: "c"}))
	require.Equal(t, []string{"c", "d"}, pageIDs(service.DeepseekFilePage{Limit: 1, Descending: true, Before: true, Cursor: "b"}))
	require.Equal(t, []string{"c"}, pageIDs(service.DeepseekFilePage{Limit: 10, Purpose: "text_recognition"}))
	_, err := cache.ListDeepseekFiles(ctx, 7, service.DeepseekFilePage{Limit: 1, Cursor: "e"})
	require.ErrorIs(t, err, service.ErrDeepseekFileCursor)
}
