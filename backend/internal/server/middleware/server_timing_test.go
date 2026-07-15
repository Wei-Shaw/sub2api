package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/gin-gonic/gin"
)

func runServerTimingRequest(
	t *testing.T,
	enabled bool,
	path string,
	adminMarker string,
	userMarker string,
	role string,
	handler gin.HandlerFunc,
) *httptest.ResponseRecorder {
REDACTED
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ServerTiming(enabled))
	engine.Any("/*path", func(c *gin.Context) {
		if role != "" {
			c.Set(string(ContextKeyUserRole), role)
	REDACTED
		handler(c)
REDACTED)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if adminMarker != "" {
		request.Header.Set(servertiming.AdminUIHeader, adminMarker)
REDACTED
	if userMarker != "" {
		request.Header.Set(servertiming.UserUIHeader, userMarker)
REDACTED
	engine.ServeHTTP(recorder, request)
	return recorder
REDACTED

func TestServerTimingScopesAndRoleGate(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		path        string
		adminMarker string
		userMarker  string
		role        string
		wantHeader  bool
REDACTED{
		{name: "disabled", enabled: false, path: "/api/v1/admin/users", role: "admin"REDACTED,
		{name: "admin API path", enabled: true, path: "/api/v1/admin/users", role: "admin", wantHeader: trueREDACTED,
		{name: "shared API marked by admin UI", enabled: true, path: "/api/v1/groups/available", adminMarker: "1", role: "admin", wantHeader: trueREDACTED,
		{name: "user role on allowlisted path", enabled: true, path: "/api/v1/groups/available", role: "user", wantHeader: trueREDACTED,
		{name: "user role with user UI marker on allowlisted path", enabled: true, path: "/api/v1/keys", userMarker: "1", role: "user", wantHeader: trueREDACTED,
		{name: "user role cannot use admin marker on non-user path", enabled: true, path: "/api/v1/settings/public", adminMarker: "1", role: "user"REDACTED,
		{name: "user marker alone does not authorize non-user path", enabled: true, path: "/api/v1/settings/public", userMarker: "1", role: "user"REDACTED,
		{name: "unauthenticated public request", enabled: true, path: "/api/v1/settings/public", adminMarker: "1"REDACTED,
		{name: "unauthenticated user path", enabled: true, path: "/api/v1/keys"REDACTED,
		{name: "unmarked shared API still scopes by path for admin", enabled: true, path: "/api/v1/groups/available", role: "admin", wantHeader: trueREDACTED,
		{name: "invalid admin marker on non-scoped path", enabled: true, path: "/api/v1/settings/public", adminMarker: "true", role: "admin"REDACTED,
		{name: "admin prefix boundary", enabled: true, path: "/api/v1/administrator", role: "admin"REDACTED,
		{name: "auth me path", enabled: true, path: "/api/v1/auth/me", role: "user", wantHeader: trueREDACTED,
		{name: "payment user path", enabled: true, path: "/api/v1/payment/plans", role: "user", wantHeader: trueREDACTED,
		{name: "payment public excluded", enabled: true, path: "/api/v1/payment/public/orders/verify", userMarker: "1", role: "user"REDACTED,
		{name: "payment webhook excluded", enabled: true, path: "/api/v1/payment/webhook/stripe", userMarker: "1", role: "user"REDACTED,
		{name: "channel monitors path", enabled: true, path: "/api/v1/channel-monitors/1/status", role: "user", wantHeader: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := runServerTimingRequest(t, tt.enabled, tt.path, tt.adminMarker, tt.userMarker, tt.role, func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
		REDACTED)
			header := recorder.Header().Get(servertiming.HeaderName)
			if tt.wantHeader && header == "" {
				t.Fatalf("%s header missing", servertiming.HeaderName)
		REDACTED
			if !tt.wantHeader && header != "" {
				t.Fatalf("unexpected %s header: %q", servertiming.HeaderName, header)
		REDACTED
			if header != "" && (!strings.Contains(header, "total;dur=") || !strings.Contains(header, `cache;desc="bypass"`)) {
				t.Fatalf("incomplete timing header: %q", header)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestIsUserTimingPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
REDACTED{
		{"/api/v1/auth/me", trueREDACTED,
		{"/api/v1/auth/revoke-all-sessions", trueREDACTED,
		{"/api/v1/auth/oauth/bind-token", trueREDACTED,
		{"/api/v1/auth/login", falseREDACTED,
		{"/api/v1/user", trueREDACTED,
		{"/api/v1/user/profile", trueREDACTED,
		{"/api/v1/user/totp/status", trueREDACTED,
		{"/api/v1/keys", trueREDACTED,
		{"/api/v1/keys/12", trueREDACTED,
		{"/api/v1/groups/available", trueREDACTED,
		{"/api/v1/groups/rates", trueREDACTED,
		{"/api/v1/groups", falseREDACTED,
		{"/api/v1/channels/available", trueREDACTED,
		{"/api/v1/channels", falseREDACTED,
		{"/api/v1/usage/stats", trueREDACTED,
		{"/api/v1/announcements", trueREDACTED,
		{"/api/v1/redeem/history", trueREDACTED,
		{"/api/v1/subscriptions/active", trueREDACTED,
		{"/api/v1/channel-monitors", trueREDACTED,
		{"/api/v1/payment/config", trueREDACTED,
		{"/api/v1/payment/orders/my", trueREDACTED,
		{"/api/v1/payment/public/orders/verify", falseREDACTED,
		{"/api/v1/payment/webhook/easypay", falseREDACTED,
		{"/api/v1/admin/users", falseREDACTED,
		{"/api/v1/settings/public", falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isUserTimingPath(tt.path); got != tt.want {
				t.Fatalf("isUserTimingPath(%q) = %v, want %v", tt.path, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestServerTimingCollectorIsRequestScoped(t *testing.T) {
	active := false
	recorder := runServerTimingRequest(t, true, "/api/v1/keys", "1", "", "admin", func(c *gin.Context) {
		active = servertiming.Active(c.Request.Context())
		c.Status(http.StatusNoContent)
REDACTED)
	if !active {
		t.Fatal("collector was not attached to marked request context")
REDACTED
	if recorder.Header().Get(servertiming.HeaderName) == "" {
		t.Fatal("timing header missing from status-only response")
REDACTED
REDACTED

func TestServerTimingCollectorForUserUIMarker(t *testing.T) {
	active := false
	// Use a non-allowlisted path so collection depends on the user UI marker.
	recorder := runServerTimingRequest(t, true, "/api/v1/settings/public", "", "1", "admin", func(c *gin.Context) {
		active = servertiming.Active(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
REDACTED)
	if !active {
		t.Fatal("collector was not attached for user UI marker")
REDACTED
	// Admin role may emit even when the path is not user-allowlisted.
	if recorder.Header().Get(servertiming.HeaderName) == "" {
		t.Fatal("admin timing header missing for user-UI-marked request")
REDACTED
REDACTED

func TestServerTimingFinalizesBeforeEarlyCommit(t *testing.T) {
	recorder := runServerTimingRequest(t, true, "/api/v1/admin/stream", "", "", "admin", func(c *gin.Context) {
		c.Status(http.StatusAccepted)
		c.Writer.WriteHeaderNow()
REDACTED)
	if got := recorder.Header().Get(servertiming.HeaderName); got == "" {
		t.Fatal("timing header was not written before response commit")
REDACTED
REDACTED

func TestServerTimingFinalizesOnFlush(t *testing.T) {
	recorder := runServerTimingRequest(t, true, "/api/v1/admin/export", "", "", "admin", func(c *gin.Context) {
		c.Writer.Flush()
REDACTED)
	if got := recorder.Header().Get(servertiming.HeaderName); got == "" {
		t.Fatal("timing header was not written before stream flush")
REDACTED
REDACTED

func TestServerTimingStatusResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
REDACTED{
		{name: "not modified", status: http.StatusNotModifiedREDACTED,
		{name: "internal error", status: http.StatusInternalServerErrorREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := runServerTimingRequest(t, true, "/api/v1/admin/test", "", "", "admin", func(c *gin.Context) {
				c.Status(tt.status)
		REDACTED)
			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
		REDACTED
			if got := recorder.Header().Get(servertiming.HeaderName); got == "" {
				t.Fatalf("timing header missing from status %d response", tt.status)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestServerTimingResponseWriterUnwraps(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	baseWriter := c.Writer
	writer := &serverTimingResponseWriter{ResponseWriter: baseWriterREDACTED
	if got := writer.Unwrap(); got != baseWriter {
		t.Fatalf("Unwrap() = %T, want original Gin writer", got)
REDACTED
REDACTED

func TestServerTimingCacheOutcome(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		value      string
		want       string
REDACTED{
		{name: "snapshot hit", headerName: snapshotCacheHeader, value: "hit", want: "hit"REDACTED,
		{name: "usage miss", headerName: usageCacheHeader, value: "MISS", want: "miss"REDACTED,
		{name: "invalid", headerName: snapshotCacheHeader, value: "stale", want: "bypass"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := runServerTimingRequest(t, true, "/api/v1/admin/dashboard", "", "", "admin", func(c *gin.Context) {
				c.Header(tt.headerName, tt.value)
				c.JSON(http.StatusOK, gin.H{"ok": trueREDACTED)
		REDACTED)
			want := `cache;desc="` + tt.want + `"`
			if got := recorder.Header().Get(servertiming.HeaderName); !strings.Contains(got, want) {
				t.Fatalf("timing header %q does not contain %q", got, want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestServerTimingResponseHeaderForWebSocket(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/ws/qps", nil)
	collector := servertiming.New(time.Now())
	c.Request = c.Request.WithContext(servertiming.WithCollector(c.Request.Context(), collector))
	c.Set(string(ContextKeyUserRole), "admin")

	header := ServerTimingResponseHeader(c)
	if header.Get(servertiming.HeaderName) == "" {
		t.Fatal("WebSocket response header missing timing value")
REDACTED

	c.Set(string(ContextKeyUserRole), "user")
	if got := ServerTimingResponseHeader(c); got != nil {
		t.Fatalf("non-admin WebSocket received timing header: %#v", got)
REDACTED
REDACTED
