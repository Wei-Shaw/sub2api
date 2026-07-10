package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newAPIKeyVideoGatewayC1AliveRouter mirrors newAPIKeyVideoGatewayTestRouter but also
// returns the *VideoGatewayService so the test can drive the worker SYNCHRONOUSLY
// (svc.ProcessRunnableTasks) instead of relying on a background goroutine. The handler
// and the test tick share the SAME service + repo, so a task advanced by a tick is
// visible to the next HTTP GET — exactly the cross-component wiring the C1 skeleton needs.
func newAPIKeyVideoGatewayC1AliveRouter(repo *apiKeyVideoGatewayMemoryRepo) (*gin.Engine, *service.VideoGatewayService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := &config.Config{}
	cfg.Gateway.MaxBodySize = 1 << 20
	videoSvc := service.NewVideoGatewayService(repo, apiKeyVideoGatewayNoopEncryptor{}, cfg)
	videoHandler := handler.NewVideoHandler(videoSvc)
	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			Video:         videoHandler,
		},
		apiKeyVideoGatewayAuthMiddleware(),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	return router, videoSvc
}

// c1AliveTaskEnvelope carries the keys the C1 live proof asserts — including
// aspect_ratio (the 竖屏 carry), which the shared apiKeyVideoGatewayTaskEnvelope omits —
// alongside the QCanvas normalizeTaskRecord contract keys.
type c1AliveTaskEnvelope struct {
	Code int `json:"code"`
	Data struct {
		ID                        int64  `json:"id"`
		Provider                  string `json:"provider"`
		Status                    string `json:"status"`
		AspectRatio               string `json:"aspect_ratio"`
		ResultURL                 string `json:"result_url"`
		ErrorMessage              string `json:"error_message"`
		UpstreamTaskID            string `json:"upstream_task_id"`
		MockOnly                  bool   `json:"mock_only"`
		ProviderBoundary          string `json:"provider_boundary"`
		RealProviderDispatchCount int    `json:"real_provider_dispatch_count"`
	} `json:"data"`
}

// TestAPIKeyVideoGatewayC1MockAliveReachesSucceeded is the C1 端到端 mock 活体 proof
// (keyless, ¥0, in-process). It upgrades the skeleton from "contract proven" to "live
// run once": it drives the EXACT QCanvas §5 request shape through the REAL gin router +
// handler + service + worker, advancing the always-on mock task all the way to
// SUCCEEDED with a playable result_url, and asserts:
//   - every key QCanvas's normalizeTaskRecord reads (id/status/result_url/error_message
//     /provider) is present and correct on the POLLED response;
//   - the 竖屏 9:16 aspect ratio survives QCanvas → Sub2API → record → response — the
//     param carry that B1 fixed on the Seedance request side, here proven end-to-end on
//     the wire for the mock path (proves the param reaches the candidate, NOT that Ark
//     renders portrait — that is the Stage-2 real shot);
//   - the real Seedance door stays SHUT the whole time (mock_only=true,
//     real_provider_dispatch_count=0, provider_boundary=api-key-video-mock-only,
//     repo.realProviderTaskCount()==0).
//
// The cross-process §5 curl + browser candidate are the Stage-2 runbook (they need a
// running server + an api-key + Postgres); this is their keyless in-process equivalent —
// the same router/handler/service/worker code path, exercised over real HTTP in one
// process with zero external dependencies.
func TestAPIKeyVideoGatewayC1MockAliveReachesSucceeded(t *testing.T) {
	repo := newAPIKeyVideoGatewayMemoryRepo()
	repo.seedProvider(service.VideoProviderMock, true, map[string]any{"priority": 10, "key_status": "normal", "health_status": "healthy"})
	router, svc := newAPIKeyVideoGatewayC1AliveRouter(repo)

	// The exact body shape QCanvas's createSub2ApiVideoMockTask POSTs, with a PORTRAIT
	// 9:16 ratio so we can prove the 竖屏 param survives every hop to the candidate.
	createBody := []byte(`{
		"provider":"mock",
		"task_type":"text_to_video",
		"model":"mock-video",
		"prompt":"QCanvas C1 alive 竖屏 clip",
		"aspect_ratio":"9:16",
		"metadata":{"nodeId":"n1","nodeLabel":"shot-1"}
	}`)
	createRec := apiKeyVideoGatewayRequest(router, http.MethodPost, "/v1/video/tasks", createBody)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	var created c1AliveTaskEnvelope
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	require.Equal(t, service.VideoProviderMock, created.Data.Provider)
	require.Equal(t, service.VideoStatusQueued, created.Data.Status, "mock task starts queued, before the worker advances it")
	require.Equal(t, "9:16", created.Data.AspectRatio, "portrait ratio must be accepted on create")
	require.True(t, created.Data.MockOnly)
	require.Equal(t, 0, created.Data.RealProviderDispatchCount)
	taskID := created.Data.ID
	require.NotZero(t, taskID)
	t.Logf("POST /v1/video/tasks (mock, 9:16) → 201 id=%d status=%s aspect_ratio=%s mock_only=%v real_dispatch=%d",
		taskID, created.Data.Status, created.Data.AspectRatio, created.Data.MockOnly, created.Data.RealProviderDispatchCount)

	// Drive the worker synchronously (no background goroutine → deterministic, no flake).
	// Each tick advances the mock task one hop: queued → submitted → running → succeeded.
	ctx := context.Background()
	var final c1AliveTaskEnvelope
	ticks := 0
	for i := 0; i < 20; i++ {
		require.NoError(t, svc.ProcessRunnableTasks(ctx, 10, 15*time.Minute), "worker tick %d", i)
		ticks++
		getRec := apiKeyVideoGatewayRequest(router, http.MethodGet, fmt.Sprintf("/v1/video/tasks/%d", taskID), nil)
		require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
		final = c1AliveTaskEnvelope{}
		require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &final))
		t.Logf("  GET /v1/video/tasks/%d after tick %d → status=%s result_url=%q", taskID, ticks, final.Data.Status, final.Data.ResultURL)
		if service.IsTerminalVideoStatus(final.Data.Status) {
			break
		}
	}
	t.Logf("LIVE candidate after %d tick(s): status=%s result_url=%s aspect_ratio=%s mock_only=%v real_dispatch=%d boundary=%s",
		ticks, final.Data.Status, final.Data.ResultURL, final.Data.AspectRatio, final.Data.MockOnly, final.Data.RealProviderDispatchCount, final.Data.ProviderBoundary)

	// --- the live result the candidate pool consumes ---
	require.Equal(t, service.VideoStatusSucceeded, final.Data.Status, "mock chain must reach succeeded")
	require.Equal(t, fmt.Sprintf("/api/v1/video/mock-assets/%d.svg", taskID), final.Data.ResultURL,
		"succeeded response must carry the playable result_url QCanvas maps to resultUrl")
	require.Empty(t, final.Data.ErrorMessage)
	require.Equal(t, service.VideoProviderMock, final.Data.Provider)
	require.NotEmpty(t, final.Data.UpstreamTaskID, "succeeded task must carry the (mock) upstream id")

	// --- 竖屏 9:16 carried QCanvas → Sub2API → record → response, end to end ---
	require.Equal(t, "9:16", final.Data.AspectRatio, "portrait ratio must survive to the polled candidate")

	// --- the real Seedance door never opened on the mock path ---
	require.True(t, final.Data.MockOnly)
	require.Equal(t, 0, final.Data.RealProviderDispatchCount)
	require.Equal(t, "api-key-video-mock-only", final.Data.ProviderBoundary)
	require.Equal(t, 0, repo.realProviderTaskCount(), "no real provider task may ever be created on the mock path")
}
