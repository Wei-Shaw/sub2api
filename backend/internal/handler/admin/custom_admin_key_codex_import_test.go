package admin

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedCodexMemoryService struct {
	*codexImportMemoryAdminService
	fresh   *service.Account
	err     error
	lookups []service.ScopedCodexIdentity
}

func (s *scopedCodexMemoryService) ScopedAdminCodexCandidates(_ context.Context, identity service.ScopedCodexIdentity) ([]service.Account, error) {
	s.lookups = append(s.lookups, identity)
	return s.accounts, nil
}
func (s *scopedCodexMemoryService) ScopedAdminCreateCodex(ctx context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.CreateAccount(ctx, input)
}
func (s *scopedCodexMemoryService) ScopedAdminUpdateCodex(ctx context.Context, id int64, input *service.UpdateAccountInput, prepare func(*service.Account, *service.UpdateAccountInput) error) (*service.Account, error) {
	current := s.fresh
	if current == nil {
		var err error
		current, err = s.GetAccount(ctx, id)
		if err != nil {
			return nil, err
		}
	}
	if err := prepare(current, input); err != nil {
		return nil, err
	}
	return s.UpdateAccount(ctx, id, input)
}

func TestScopedCodexImportCapturedRequestAndSafeResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	memory := &scopedCodexMemoryService{codexImportMemoryAdminService: newCodexImportMemoryAdminService(nil)}
	h := &AccountHandler{adminService: memory}
	ctx := adminaccess.WithPrincipal(context.Background(), &adminaccess.Principal{KeyID: "fixture", Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.create", "accounts.credentials", "accounts.schedule", "accounts.network"}, AllGroups: true, AccountTypes: []string{"oauth"}}}}})
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Request = c.Request.WithContext(ctx) })
	r.Use(h.ScopedAdminKeyAccounts)
	r.POST("/api/v1/admin/accounts/import/codex-session", func(c *gin.Context) { t.Fatal("不能回退到完整管理员导入") })
	content := `{"access_token":"private-access-fixture","refresh_token":"private-refresh-fixture","email":"test@example.com"}`
	req := map[string]any{"content": content, "name": "推送账户", "notes": "测试", "group_ids": []int64{2, 14}, "concurrency": 40, "priority": 0, "expires_at": nil, "auto_pause_on_expired": true, "proxy_id": nil, "rate_multiplier": 1, "update_existing": true, "extra": map[string]any{"codex_fingerprint_mode": "off", "openai_oauth_responses_websockets_v2_enabled": true, "openai_oauth_responses_websockets_v2_mode": "shared"}}
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/admin/accounts/import/codex-session", strings.NewReader(string(raw))))
		return w
	}
	w := call()
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"created":1`)
	require.NotContains(t, w.Body.String(), "private-")
	require.Len(t, memory.createdAccounts, 1)
	input := memory.createdAccounts[0]
	require.Equal(t, []int64{2, 14}, input.GroupIDs)
	require.Equal(t, 40, input.Concurrency)
	require.True(t, input.SkipDefaultGroupBind)
	require.Equal(t, "private-refresh-fixture", input.Credentials["refresh_token"])
	require.Equal(t, "off", input.Extra["codex_fingerprint_mode"])
	memory.err = infraerrors.BadRequest("UPSTREAM_SECRET", "private-refresh-fixture")
	// update_existing=false 沿用原接口语义：新建；错误也必须脱敏。
	req["update_existing"] = false
	raw, err = json.Marshal(req)
	require.NoError(t, err)
	w = call()
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"failed":1`)
	require.NotContains(t, w.Body.String(), "private-")
}

func TestScopedCodexImportRevalidatesIdentityAndMergesFreshCredentials(t *testing.T) {
	token := buildCodexAccessToken(t, "account", "user", time.Now().Add(time.Hour))
	stale := service.Account{ID: 1, Credentials: map[string]any{"access_token": token, "refresh_token": "stale"}, Extra: map[string]any{"quota_used": 1}}
	fresh := stale
	fresh.Credentials = map[string]any{"access_token": token, "refresh_token": "latest", "base_url": "https://example.com"}
	fresh.Extra = map[string]any{"quota_used": 9}
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "锁内合并", true: "身份已更换"}[changed], func(t *testing.T) {
			current := fresh
			if changed {
				current.Credentials = map[string]any{"access_token": "different", "refresh_token": "other"}
			}
			memory := &scopedCodexMemoryService{codexImportMemoryAdminService: newCodexImportMemoryAdminService([]service.Account{stale}), fresh: &current}
			h := &AccountHandler{adminService: memory}
			req := scopedCodexSessionImportRequest{CodexSessionImportRequest: CodexSessionImportRequest{Content: token, GroupIDs: []int64{1}}}
			entries, err := parseCodexSessionImportEntries(req.CodexSessionImportRequest)
			require.NoError(t, err)
			result, err := h.importScopedCodexSessions(context.Background(), memory, req, entries)
			require.NoError(t, err)
			if changed {
				require.Equal(t, 1, result.Failed)
				require.Empty(t, memory.updatedAccounts)
				return
			}
			require.Equal(t, 1, result.Updated)
			require.Len(t, memory.updatedAccounts, 1)
			input := memory.updatedAccounts[0].input
			require.Equal(t, "latest", input.Credentials["refresh_token"])
			require.Equal(t, "https://example.com", input.Credentials["base_url"])
			require.Equal(t, 9, input.Extra["quota_used"])
			require.Nil(t, input.ExpiresAt)
			require.Nil(t, input.AutoPauseOnExpired)
		})
	}
}

func TestScopedCodexRequestRejectsUnopenedFields(t *testing.T) {
	for _, body := range []string{`{"content":"x"}`, `{"group_ids":[1],"confirm_mixed_channel_risk":true}`, `{"group_ids":[1],"extra":{"quota_used":0}}`, `{"group_ids":[1],"credential_extras":{"refresh_token":"x"}}`, `{"group_ids":[1],"extra":{"openai_oauth_responses_websockets_v2_enabled":"true"}}`} {
		var req scopedCodexSessionImportRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		require.Error(t, validateScopedCodexRequest(req), body)
	}
}

func TestScopedCodexImportIdempotencySeparatesKeysAndPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := service.DefaultIdempotencyCoordinator()
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	repo := newMemoryIdempotencyRepoStub()
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(repo, service.DefaultIdempotencyConfig()))
	memory := &scopedCodexMemoryService{codexImportMemoryAdminService: newCodexImportMemoryAdminService(nil)}
	h := &AccountHandler{adminService: memory}
	principal := &adminaccess.Principal{KeyID: "aaaaaaaaaaaaaaaaaaaaaaaa", Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.create", "accounts.credentials", "accounts.schedule", "accounts.network"}, GroupIDs: []int64{2}, AccountTypes: []string{"oauth"}}}}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if principal != nil {
			c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), principal))
		}
	})
	r.Use(h.ScopedAdminKeyAccounts)
	r.POST("/api/v1/admin/accounts/import/codex-session", h.ImportCodexSession)
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/admin/accounts/import/codex-session", strings.NewReader(`{"content":"{\"access_token\":\"fixture-access\",\"refresh_token\":\"fixture-refresh\"}","group_ids":[2],"update_existing":false}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "same-import")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "fixture-")
		return w
	}
	first := call()
	require.Len(t, memory.createdAccounts, 1)
	replay := call()
	require.Equal(t, "true", replay.Header().Get("X-Idempotency-Replayed"))
	require.JSONEq(t, first.Body.String(), replay.Body.String())
	require.Len(t, memory.createdAccounts, 1)
	principal.KeyID = "bbbbbbbbbbbbbbbbbbbbbbbb"
	other := call()
	require.Empty(t, other.Header().Get("X-Idempotency-Replayed"))
	require.Len(t, memory.createdAccounts, 2)
	principal.Policy.Rules[0].Permissions = []string{"accounts.create"}
	memory.err = infraerrors.Forbidden("SCOPED_CODEX_FORBIDDEN", "权限已收回")
	restricted := call()
	require.Empty(t, restricted.Header().Get("X-Idempotency-Replayed"))
	require.Contains(t, restricted.Body.String(), `"failed":1`)
	require.Len(t, memory.createdAccounts, 2)
	principal = nil
	memory.err = nil
	full := call()
	require.Empty(t, full.Header().Get("X-Idempotency-Replayed"))
	require.Len(t, memory.createdAccounts, 3)
	for _, record := range repo.data {
		require.LessOrEqual(t, len(record.Scope), 128, "生产数据库 scope 最大长度")
	}
}

func TestScopedCodexImportProxyModeForwarding(t *testing.T) {
	for _, tc := range []struct {
		body, mode string
		id         int64
	}{
		{`"proxy_id":7`, "", 7}, {`"proxy_mode":"specified","proxy_id":7`, "specified", 7},
		{`"random_proxy":true`, "random", 0}, {`"proxy_mode":"random"`, "random", 0},
		{`"proxy_mode":"least_used"`, "least_used", 0}, {`"proxy_mode":"direct"`, "direct", 0},
	} {
		t.Run(tc.body, func(t *testing.T) {
			memory := &scopedCodexMemoryService{codexImportMemoryAdminService: newCodexImportMemoryAdminService(nil)}
			h := &AccountHandler{adminService: memory}
			var req scopedCodexSessionImportRequest
			require.NoError(t, json.Unmarshal([]byte(`{"content":"{\"access_token\":\"fixture\",\"refresh_token\":\"refresh\"}","group_ids":[1],`+tc.body+`}`), &req))
			require.NoError(t, validateScopedCodexRequest(req))
			entries, err := parseCodexSessionImportEntries(req.CodexSessionImportRequest)
			require.NoError(t, err)
			result, err := h.importScopedCodexSessions(context.Background(), memory, req, entries)
			require.NoError(t, err)
			require.Equal(t, 1, result.Created)
			require.Len(t, memory.createdAccounts, 1)
			require.Equal(t, tc.mode, memory.createdAccounts[0].ProxyMode)
			if tc.id > 0 {
				require.Equal(t, tc.id, *memory.createdAccounts[0].ProxyID)
			}
		})
	}
}

func TestScopedCodexImportLooksUpOnlyValidatedEntryIdentity(t *testing.T) {
	token := buildCodexAccessToken(t, "workspace", "member", time.Now().Add(time.Hour))
	for _, tc := range []struct {
		name        string
		content     string
		update      bool
		wantLookups int
		wantSkipped int
		onlyAccess  bool
	}{
		{"有效身份与批内重复", token + "\n" + token, true, 1, 1, true},
		{"无效内容无需读取候选", `{"refresh_token":"fixture"}`, true, 0, 0, false},
		{"禁用更新无需读取候选", token, false, 0, 0, true},
		{"有refresh保留身份匹配", `{"access_token":"` + token + `","refresh_token":"fixture"}`, true, 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			memory := &scopedCodexMemoryService{codexImportMemoryAdminService: newCodexImportMemoryAdminService(nil)}
			req := scopedCodexSessionImportRequest{CodexSessionImportRequest: CodexSessionImportRequest{Content: tc.content, GroupIDs: []int64{1}, UpdateExisting: &tc.update}}
			entries, err := parseCodexSessionImportEntries(req.CodexSessionImportRequest)
			require.NoError(t, err)
			result, err := (&AccountHandler{adminService: memory}).importScopedCodexSessions(context.Background(), memory, req, entries)
			require.NoError(t, err)
			require.Equal(t, tc.wantSkipped, result.Skipped)
			require.Len(t, memory.lookups, tc.wantLookups)
			if tc.wantLookups > 0 {
				lookup := memory.lookups[0]
				require.Equal(t, "workspace", lookup.AccountID)
				require.Equal(t, "member", lookup.UserID)
				require.Equal(t, token, lookup.AccessToken)
				require.Equal(t, tc.onlyAccess, lookup.AccessTokenOnly)
			}
		})
	}
}
