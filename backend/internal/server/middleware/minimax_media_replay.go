package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Only a verified existing receipt may bypass admission for a new paid request.
// The header alone never grants a billing or model-allowlist exemption.
const miniMaxMediaReplayContextKey = "sub2api.minimax_media.verified_replay"

type miniMaxReplayLookup interface {
	Replay(context.Context, *service.APIKey, string, string, []byte) (*service.MiniMaxMediaTask, error)
}

// ProvideAPIKeyAuthMiddleware wires the optional media receipt lookup without
// changing the constructor used by unrelated gateway integrations and tests.
func ProvideAPIKeyAuthMiddleware(keys *service.APIKeyService, subscriptions *service.SubscriptionService, cfg *config.Config, media *service.MiniMaxMediaService) APIKeyAuthMiddleware {
	return APIKeyAuthMiddleware(apiKeyAuthWithSubscription(keys, subscriptions, cfg, media))
}

func IsMiniMaxMediaReplay(c *gin.Context) bool {
	verified, exists := c.Get(miniMaxMediaReplayContextKey)
	return exists && verified == true
}

func miniMaxMediaCreateKind(method, path string) string {
	if method != http.MethodPost {
		return ""
	}
	switch path {
	case "/v1/t2a_v2":
		return "speech"
	case "/v1/image_generation":
		return "image"
	case "/v1/voice_clone":
		return "voice_clone"
	case "/v1/voice_design":
		return "voice_design"
	case "/v2/video_generation":
		return "video"
	}
	return ""
}

func prepareMiniMaxMediaReplay(c *gin.Context, key *service.APIKey, lookups []miniMaxReplayLookup) bool {
	kind := miniMaxMediaCreateKind(c.Request.Method, c.Request.URL.Path)
	if kind == "" || len(lookups) == 0 || lookups[0] == nil || key.Group == nil || (key.Group.Platform != service.PlatformMiniMax && key.Group.Platform != service.PlatformComposite) {
		return true
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		status := http.StatusBadRequest
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
		}
		AbortWithError(c, status, "INVALID_MEDIA_REQUEST", "Unable to read media request")
		return false
	}
	requestmodel.ResetRequestBody(c.Request, body)
	c.Request = c.Request.WithContext(service.WithMiniMaxRequestHash(c.Request.Context(), body))
	if c.GetHeader("Idempotency-Key") == "" {
		return true
	}
	task, err := lookups[0].Replay(c.Request.Context(), key, kind, c.GetHeader("Idempotency-Key"), body)
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "MEDIA_RECEIPT_UNAVAILABLE"
		if errors.Is(err, service.ErrMiniMaxMediaConflict) {
			status, code = http.StatusConflict, "MEDIA_IDEMPOTENCY_CONFLICT"
		}
		AbortWithError(c, status, code, "Unable to replay media request; inspect the original task receipt")
		return false
	}
	if task != nil {
		c.Set(miniMaxMediaReplayContextKey, true)
	}
	return true
}
