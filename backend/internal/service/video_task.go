package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// VideoTaskBillingCache is the durable queue used by the async video billing
// reconciler. The production implementation is Redis; keeping this optional
// avoids widening GatewayCache and breaking lightweight gateway test doubles.
type VideoTaskBillingCache interface {
	SetVideoTaskBilling(ctx context.Context, key string, payload []byte, nextPoll time.Time, ttl time.Duration) error
	ClaimDueVideoTask(ctx context.Context, now time.Time) (key string, payload []byte, err error)
	DeleteVideoTaskBilling(ctx context.Context, key string) error
}

// VideoTaskPlatform identifies the upstream protocol that owns a video task.
// Task IDs are only meaningful together with the platform and caller identity.
const (
	VideoTaskPlatformOpenAI = PlatformOpenAI
	VideoTaskPlatformGrok   = PlatformGrok
)

// VideoTaskSessionHash derives the sticky-session key used by asynchronous
// video status/content requests. The caller identity is part of the key so a
// task ID cannot be used to discover another user's account binding.
func VideoTaskSessionHash(platform, requestID string, userID, apiKeyID int64) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	requestID = strings.TrimSpace(requestID)
	if platform == "" || requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return fmt.Sprintf("video-task:%s:%s", platform,
		DeriveSessionHashFromSeed(fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID)))
}

// BindVideoTaskAccount records the account that successfully created a video
// task. Subsequent status/content requests must use this binding instead of
// selecting another account from the group.
func (s *OpenAIGatewayService) BindVideoTaskAccount(
	ctx context.Context,
	groupID *int64,
	platform, requestID string,
	userID, apiKeyID, accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("video task binding cache is unavailable")
	}
	key := VideoTaskSessionHash(platform, requestID, userID, apiKeyID)
	if key == "" || accountID <= 0 {
		return fmt.Errorf("video task binding is invalid")
	}
	// Keep the account binding longer than the normal OpenAI sticky-session
	// window. The upstream video job may remain queryable for many hours.
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), key, accountID, 24*time.Hour)
}

// ResolveVideoTaskAccount returns the account bound at task creation time.
func (s *OpenAIGatewayService) ResolveVideoTaskAccount(
	ctx context.Context,
	groupID *int64,
	platform, requestID string,
	userID, apiKeyID int64,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("video task binding cache is unavailable")
	}
	key := VideoTaskSessionHash(platform, requestID, userID, apiKeyID)
	if key == "" {
		return 0, fmt.Errorf("video task binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), key)
}

func videoTaskStorageID(platform, requestID string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	requestID = strings.TrimSpace(requestID)
	// Preserve the existing Grok Redis key format while allowing new providers
	// to share the same cache namespace safely.
	if platform == VideoTaskPlatformGrok {
		return requestID
	}
	return platform + ":" + requestID
}

// StableVideoTaskBillingRequestID is the durable usage/dedup key shared by
// asynchronous video status polls and content downloads.
func StableVideoTaskBillingRequestID(platform, requestID string) string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	requestID = strings.TrimSpace(requestID)
	if platform == "" || requestID == "" {
		return ""
	}
	if strings.HasPrefix(requestID, platform+"-video:") {
		return requestID
	}
	return platform + "-video:" + requestID
}

// The gateway cache predates the cross-provider task abstraction and exposes
// these operations under the Grok-specific names. Keep the provider-neutral
// service boundary here so callers do not encode that storage detail.
func (s *OpenAIGatewayService) StoreVideoTaskPendingBilling(
	ctx context.Context,
	platform, requestID string,
	userID, apiKeyID int64,
	pending GrokVideoPendingBilling,
) error {
	pending.RequestID = strings.TrimSpace(requestID)
	pending.UserID, pending.APIKeyID = userID, apiKeyID
	if pending.Platform == "" {
		pending.Platform = strings.ToLower(strings.TrimSpace(platform))
	}
	if err := s.StoreGrokVideoPendingBilling(ctx, videoTaskStorageID(platform, requestID), userID, apiKeyID, pending); err != nil {
		return err
	}
	// StoreGrokVideoPendingBilling preserves the legacy Redis key namespace. For
	// the worker payload, rewrite the task id to the actual upstream id (the
	// OpenAI namespace key is intentionally prefixed, but the HTTP id is not).
	if queue, ok := s.cache.(VideoTaskBillingCache); ok && pending.HoldID != "" {
		payload, err := json.Marshal(pending)
		if err != nil {
			return err
		}
		return queue.SetVideoTaskBilling(ctx, grokVideoPendingBillingKey(requestID, userID, apiKeyID), payload, time.Now(), 30*24*time.Hour)
	}
	return nil
}

func (s *OpenAIGatewayService) LoadVideoTaskPendingBilling(
	ctx context.Context,
	platform, requestID string,
	userID, apiKeyID int64,
) (*GrokVideoPendingBilling, error) {
	return s.LoadGrokVideoPendingBilling(ctx, videoTaskStorageID(platform, requestID), userID, apiKeyID)
}

func (s *OpenAIGatewayService) ClaimVideoTaskBilling(
	ctx context.Context,
	platform, requestID string,
	userID, apiKeyID int64,
) (bool, error) {
	return s.ClaimGrokVideoBilling(ctx, videoTaskStorageID(platform, requestID), userID, apiKeyID)
}

func (s *OpenAIGatewayService) ReleaseVideoTaskBilling(
	ctx context.Context,
	platform, requestID string,
	userID, apiKeyID int64,
) error {
	return s.ReleaseGrokVideoBilling(ctx, videoTaskStorageID(platform, requestID), userID, apiKeyID)
}
