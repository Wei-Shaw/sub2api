package admin

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// liveAccountModel is the common response shape consumed by the account test
// dialog, regardless of the account's upstream platform.
type liveAccountModel struct {
	ID          string `json:"id"`
	Object      string `json:"object,omitempty"`
	Type        string `json:"type,omitempty"`
	OwnedBy     string `json:"owned_by,omitempty"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at,omitempty"`
}

const (
	liveAccountModelsTTL     = 5 * time.Minute
	liveAccountModelsTimeout = 5 * time.Second
)

type liveAccountModelsEntry struct {
	models    []liveAccountModel
	fetchedAt time.Time
	// key distinguishes snapshots taken against different upstreams, so
	// changing an account's base URL invalidates the cached list immediately.
	key string
}

// Opening the test dialog repeatedly should not hammer the account's upstream.
var liveAccountModelsCache sync.Map // accountID -> liveAccountModelsEntry

// tryLiveAccountModels returns the account's real upstream model list. It
// reports ok=false when live discovery is unavailable so the caller can retain
// the existing static per-platform fallback.
func (h *AccountHandler) tryLiveAccountModels(ctx context.Context, account *service.Account, refresh bool) ([]liveAccountModel, bool) {
	if h == nil || h.accountTestService == nil || account == nil {
		return nil, false
	}

	cacheKey := liveAccountModelsCacheKey(account)
	if !refresh {
		if cached, ok := liveAccountModelsCache.Load(account.ID); ok {
			if entry, ok := cached.(liveAccountModelsEntry); ok &&
				entry.key == cacheKey &&
				time.Since(entry.fetchedAt) < liveAccountModelsTTL &&
				len(entry.models) > 0 {
				return entry.models, true
			}
		}
	}

	fetchCtx, cancel := context.WithTimeout(ctx, liveAccountModelsTimeout)
	defer cancel()

	modelIDs, err := h.accountTestService.FetchUpstreamSupportedModels(fetchCtx, account)
	if err != nil || len(modelIDs) == 0 {
		return nil, false
	}

	models := make([]liveAccountModel, 0, len(modelIDs))
	for _, id := range modelIDs {
		if id == "" {
			continue
		}
		models = append(models, liveAccountModel{
			ID:          id,
			Object:      "model",
			Type:        "model",
			DisplayName: id,
		})
	}
	if len(models) == 0 {
		return nil, false
	}

	liveAccountModelsCache.Store(account.ID, liveAccountModelsEntry{
		models:    models,
		fetchedAt: time.Now(),
		key:       cacheKey,
	})
	return models, true
}

func liveAccountModelsCacheKey(account *service.Account) string {
	if account == nil {
		return ""
	}
	return account.Platform + "\x00" + account.Type + "\x00" + accountUpstreamBaseURL(account)
}

func accountUpstreamBaseURL(account *service.Account) string {
	switch {
	case account.IsOpenAI():
		return account.GetOpenAIBaseURL()
	case account.IsGemini():
		return account.GetGeminiBaseURL("")
	case account.IsGrok():
		return account.GetGrokBaseURL()
	default:
		return account.GetBaseURL()
	}
}

func invalidateLiveAccountModels(accountID int64) {
	liveAccountModelsCache.Delete(accountID)
}
