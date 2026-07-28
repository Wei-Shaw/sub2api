//go:build unit

package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRetainGrokMediaVideoTerminalOwnerForStatusAndContent(t *testing.T) {
	groupID := int64(7)
	repo := &grokCredentialVideoOwnerRepo{}
	require.NoError(t, repo.Bind(context.Background(), service.GrokMediaVideoRequestOwner{
		RequestID: "video-terminal", UserID: 41, APIKeyID: 51, GroupID: groupID,
		AccountID: 63, ExpiresAt: time.Now().Add(4 * 24 * time.Hour),
	}))
	gateway := &service.OpenAIGatewayService{}
	gateway.SetGrokMediaVideoRequestOwnerRepository(repo)
	h := &OpenAIGatewayHandler{gatewayService: gateway}
	terminal := &service.OpenAIForwardResult{GrokMediaVideoTerminal: true}

	require.NoError(t, h.retainGrokMediaVideoTerminalOwner(
		context.Background(), service.GrokMediaEndpointVideoStatus, terminal,
		&groupID, "video-terminal", 41, 51,
	))
	owner := repo.owners[grokCredentialVideoOwnerKey("video-terminal", 41, 51, groupID)]
	require.NotNil(t, owner.TerminalAt)
	require.WithinDuration(t, time.Now().Add(7*24*time.Hour), owner.ExpiresAt, 2*time.Second)

	owner.TerminalAt = nil
	repo.owners[grokCredentialVideoOwnerKey("video-terminal", 41, 51, groupID)] = owner
	require.NoError(t, h.retainGrokMediaVideoTerminalOwner(
		context.Background(), service.GrokMediaEndpointVideoStatus,
		&service.OpenAIForwardResult{}, &groupID, "video-terminal", 41, 51,
	))
	require.Nil(t, repo.owners[grokCredentialVideoOwnerKey("video-terminal", 41, 51, groupID)].TerminalAt)

	require.NoError(t, h.retainGrokMediaVideoTerminalOwner(
		context.Background(), service.GrokMediaEndpointVideoContent, terminal,
		&groupID, "video-terminal", 41, 51,
	))
	require.NotNil(t, repo.owners[grokCredentialVideoOwnerKey("video-terminal", 41, 51, groupID)].TerminalAt)
}

func TestReleaseMismatchedGrokMediaVideoOwner(t *testing.T) {
	releases := 0
	selection := &service.AccountSelectionResult{
		Account:     &service.Account{ID: 12},
		Acquired:    true,
		ReleaseFunc: func() { releases++ },
	}

	require.True(t, releaseMismatchedGrokMediaVideoOwner(selection, 11))
	require.Equal(t, 1, releases)
	require.Nil(t, selection.ReleaseFunc)
	require.True(t, releaseMismatchedGrokMediaVideoOwner(selection, 11))
	require.Equal(t, 1, releases)
}

func TestReleaseMismatchedGrokMediaVideoOwnerKeepsMatchingSlot(t *testing.T) {
	releases := 0
	selection := &service.AccountSelectionResult{
		Account:     &service.Account{ID: 11},
		Acquired:    true,
		ReleaseFunc: func() { releases++ },
	}

	require.False(t, releaseMismatchedGrokMediaVideoOwner(selection, 11))
	require.Zero(t, releases)
	require.NotNil(t, selection.ReleaseFunc)
}
