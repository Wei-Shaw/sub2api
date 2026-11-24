package routes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

var threadCompositeBetaDigest = func() string {
	sum := sha256.Sum256([]byte("message-threads-2026-08-12"))
	return hex.EncodeToString(sum[:])
}()

type threadCompositeStore struct {
	service.GatewayCache
	bindings map[string]service.MessageThreadBinding
}

func (s threadCompositeStore) ReadMessageThread(_ context.Context, scope, id string) (*service.MessageThreadBinding, error) {
	if binding, ok := s.bindings[scope+"/"+id]; ok {
		return &binding, nil
	}
	return nil, nil
}

func (s threadCompositeStore) BindMessageThread(context.Context, string, string, service.MessageThreadBinding) error {
	return nil
}

// A composite route rewrites the model in the request body before the handler
// runs. A message thread compares the model the client wrote: two public names
// for one upstream model are a client change, and a route that now leads
// somewhere else is not.
func TestMessageThreadComparesThePublicModelBehindACompositeRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	route := func(id int64, public, upstream string) service.CompositeModelRoute {
		return service.CompositeModelRoute{
			ID: id, GroupID: 1, PublicModel: public, MatchType: service.CompositeRouteMatchExact,
			TargetPlatform: service.PlatformAnthropic, UpstreamModel: upstream,
			Endpoint: service.CompositeRouteEndpointAny, Priority: 100, Enabled: true,
		}
	}
	gateway := service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		threadCompositeStore{bindings: map[string]service.MessageThreadBinding{"5:6:1/msg_1": {
			AccountID: 7, Identity: strings.Repeat("a", 64), Model: "claude-sonnet-4-5", Wire: strings.Repeat("b", 32),
			Beta: "message-threads-2026-08-12", ClientModel: "team/fast", ContextTokens: 100,
			// sha256 of the client's beta set, "message-threads-2026-08-12".
			ClientBeta: threadCompositeBetaDigest, ClientFixed: map[string]string{},
		}}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	run := func(routes []service.CompositeModelRoute, model string) (upstreamModel string, thread *service.MessageThread, err error) {
		router := gin.New()
		router.Use(gin.HandlerFunc(servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformComposite}})
			c.Next()
		})))
		router.Use(compositeTargetPlatformMiddleware(service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: routes})))
		router.POST("/v1/messages", func(c *gin.Context) {
			body, readErr := io.ReadAll(c.Request.Body)
			require.NoError(t, readErr)
			groupID := int64(1)
			upstreamModel = gjson.GetBytes(body, "model").String()
			thread, err = gateway.PrepareMessageThread(c.Request.Context(), body, c.Request.Header, upstreamModel, 5, 6, &groupID, true)
			c.Status(http.StatusNoContent)
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
			`{"model":"`+model+`","messages":[{"role":"user","content":"next"}],"thread":{"type":"continue","previous_message_id":"msg_1"}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Anthropic-Beta", "message-threads-2026-08-12")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNoContent, w.Code)
		return upstreamModel, thread, err
	}
	routes := []service.CompositeModelRoute{route(1, "team/fast", "claude-sonnet-4-5"), route(2, "team/alias", "claude-sonnet-4-5")}

	// The name the thread was created with.
	upstream, thread, err := run(routes, "team/fast")
	require.NoError(t, err)
	require.True(t, thread.IsContinue())
	require.Equal(t, "claude-sonnet-4-5", upstream, "the handler sees the rewritten model")

	// Another public name for the same upstream model: the body the handler
	// sees is identical, the client's request is not.
	upstream, _, err = run(routes, "team/alias")
	require.Equal(t, "claude-sonnet-4-5", upstream)
	var threadErr *service.MessageThreadError
	require.ErrorAs(t, err, &threadErr)
	require.Equal(t, http.StatusBadRequest, threadErr.Status)
	require.Equal(t, "thread_fingerprint_mismatch", threadErr.Code)
	require.Contains(t, threadErr.Message, "model")

	// The route now leads to another model. The client changed nothing, so it
	// is not told that it did; the thread ends later, when the request that is
	// about to be sent no longer matches the one that created it.
	upstream, thread, err = run([]service.CompositeModelRoute{route(1, "team/fast", "claude-opus-4-8")}, "team/fast")
	require.NoError(t, err)
	require.True(t, thread.IsContinue())
	require.Equal(t, "claude-opus-4-8", upstream)
}
