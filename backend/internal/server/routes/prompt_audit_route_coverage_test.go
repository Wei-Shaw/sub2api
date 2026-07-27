package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEveryGatewayPOSTRouteIsClassifiedForPromptAuditCoverage(t *testing.T) {
	routeSource, err := os.ReadFile("gateway.go")
REDACTED
	pattern := regexp.MustCompile(`(?:gateway|gemini|r|codexDirect|antigravityV1|antigravityV1Beta)\.POST\("([^"]+)"`)
	matches := pattern.FindAllStringSubmatch(string(routeSource), -1)
	actual := map[string]struct{REDACTED{REDACTED
	for _, match := range matches {
		actual[match[1]] = struct{REDACTED{REDACTED
REDACTED

	audited := map[string][]string{
		"/messages":                 {"gateway_handler.go", "openai_gateway_handler.go"REDACTED,
		"/responses":                {"gateway_handler_responses.go", "openai_gateway_handler.go"REDACTED,
		"/responses/*subpath":       {"gateway_handler_responses.go", "openai_gateway_handler.go"REDACTED,
		"/chat/completions":         {"gateway_handler_chat_completions.go", "openai_chat_completions.go"REDACTED,
		"/embeddings":               {"openai_embeddings.go"REDACTED,
		"/alpha/search":             {"openai_alpha_search.go"REDACTED,
		"/live":                     {"openai_live.go"REDACTED,
		"/realtime/calls":           {"openai_live.go"REDACTED,
		"/images/generations":       {"openai_images.go", "grok_media.go"REDACTED,
		"/images/edits":             {"openai_images.go", "grok_media.go"REDACTED,
		"/images/generations/async": {"image_task_handler.go"REDACTED,
		"/images/edits/async":       {"image_task_handler.go"REDACTED,
		"/images/batches":           {"batch_image_handler.go"REDACTED,
		"/videos/generations":       {"grok_media.go"REDACTED,
		"/videos/edits":             {"grok_media.go"REDACTED,
		"/videos/extensions":        {"grok_media.go"REDACTED,
		"/models/*modelAction":      {"gemini_v1beta_handler.go"REDACTED,
REDACTED
	excluded := map[string]string{
		"/messages/count_tokens":     "tokenization only; it does not execute a model request",
		"/images/batches/:id/cancel": "control-plane cancellation with no user prompt",
REDACTED

	unclassified := make([]string, 0)
	for route := range actual {
		if _, ok := audited[route]; ok {
			continue
	REDACTED
		if _, ok := excluded[route]; ok {
			continue
	REDACTED
		unclassified = append(unclassified, route)
REDACTED
	sort.Strings(unclassified)
	require.Empty(t, unclassified, "new gateway POST routes must be audited or explicitly classified with a no-prompt reason")

	for route, files := range audited {
		_, exists := actual[route]
		require.Truef(t, exists, "stale prompt-audit route manifest entry %s", route)
		for _, filename := range files {
			source, readErr := os.ReadFile(filepath.Join("..", "..", "handler", filename))
			require.NoError(t, readErr)
			require.Containsf(t, string(source), "checkSecurityAudit", "%s route handler %s bypasses Coordinator", route, filename)
	REDACTED
REDACTED

	for route, reason := range excluded {
		require.NotEmpty(t, strings.TrimSpace(reason))
		_, exists := actual[route]
		require.Truef(t, exists, "stale excluded route %s", route)
REDACTED
REDACTED

func TestResponsesWebSocketHasFirstAndSubsequentTurnPromptGates(t *testing.T) {
	routeSource, err := os.ReadFile("gateway.go")
REDACTED
	require.GreaterOrEqual(t, strings.Count(string(routeSource), `.GET("/responses"`), 2)
	handlerSource, err := os.ReadFile(filepath.Join("..", "..", "handler", "openai_gateway_handler.go"))
REDACTED
	require.Contains(t, string(handlerSource), `checkSecurityAuditStage`)
	require.Contains(t, string(handlerSource), `"first_turn"`)
	require.Contains(t, string(handlerSource), `"subsequent_turn"`)
	wsStart := strings.Index(string(handlerSource), `func (h *OpenAIGatewayHandler) ResponsesWebSocket`)
	require.NotEqual(t, -1, wsStart)
	wsSource := string(handlerSource)[wsStart:]
	require.Less(t,
		strings.Index(wsSource, `"first_turn"`),
		strings.Index(wsSource, `TryAcquireUserSlotForAPIKey`),
		"the first response.create gate must precede per-request user/account slots",
	)
REDACTED

func TestPromptAuditAdminRoutesRejectUnauthenticatedAndNonAdminRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		PromptAudit: securityaudit.NewPromptAdminHandler(nil),
REDACTEDREDACTED
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
			return
	REDACTED
		servermiddleware.AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Admin access required")
REDACTED)
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() REDACTED)
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() REDACTED)
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil)

	for _, tc := range []struct {
		name       string
		auth       string
		wantStatus int
REDACTED{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorizedREDACTED,
		{name: "non-admin", auth: "Bearer user-token", wantStatus: http.StatusForbiddenREDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/prompt-audit/config", nil)
			if tc.auth != "" {
				request.Header.Set("Authorization", tc.auth)
		REDACTED
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.wantStatus, recorder.Code)
	REDACTED)
REDACTED
REDACTED
