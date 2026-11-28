//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type fileOwnerTestUpstream struct {
	status int
	reply  string
	ids    []int64
	bodies []string
	paths  []string
}

func (u *fileOwnerTestUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.ids = append(u.ids, accountID)
	u.paths = append(u.paths, req.URL.RequestURI())
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	u.bodies = append(u.bodies, string(body))
	return &http.Response{StatusCode: u.status, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(u.reply))}, nil
}
func (u *fileOwnerTestUpstream) DoWithTLS(req *http.Request, proxy string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, n)
}

func fileOwnerTestHandler(t *testing.T, cache service.GatewayCache, platform string, baseURL ...string) (*OpenAIGatewayHandler, *fileOwnerTestUpstream) {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	groupID := int64(163)
	owner := service.Account{ID: 11, Platform: platform, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		GroupIDs: []int64{groupID}, Priority: 99, Concurrency: 5,
		Credentials: map[string]any{"api_key": "test", "api_protocol": "adaptive"},
	}
	if len(baseURL) > 0 {
		owner.Credentials["base_url"] = baseURL[0]
	}
	other := owner
	other.ID, other.Priority = 12, 1
	repo := &openAIWSFailoverHandlerAccountRepoStub{accounts: []service.Account{owner, other}}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	upstream := &fileOwnerTestUpstream{status: http.StatusBadRequest, reply: `{"error":{"type":"invalid_request_error","message":"upstream test reply"}}`}
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, cache, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, upstream, nil, nil, nil, nil, nil, nil, nil, nil)
	return &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(nil), SSEPingFormatNone, time.Second), cfg: cfg}, upstream
}

func fileOwnerTestContext(method, path, body, groupPlatform, resolved string, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if resolved != "" {
		c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), resolved))
	}
	groupID := int64(163)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 101, UserID: userID, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Platform: groupPlatform, AllowMessagesDispatch: true}, User: &service.User{ID: userID}})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
	return c, rec
}

func TestDeepseekChatFileOwnerHandlers(t *testing.T) {
	cache := testutil.NewRedisGatewayCache(t)
	store := cache.(service.DeepseekFileStore)
	require.NoError(t, store.SaveDeepseekFile(context.Background(), 7, service.DeepseekFile{ID: "own", AccountID: 11, Origin: service.DefaultDeepseekBaseURL}))
	h, up := fileOwnerTestHandler(t, cache, service.PlatformDeepseek)
	for _, tc := range []struct {
		name, path, payload string
		handler             gin.HandlerFunc
	}{
		{"messages", "/v1/messages", `"messages":[{"role":"user","content":[{"type":"document","source":{"type":"file","file_id":"own"}}]}]`, h.Messages},
		{"chat", "/v1/chat/completions", `"messages":[{"role":"user","content":[{"type":"file","file_id":"own"}]}]`, h.ChatCompletions},
		{"responses-shaped chat", "/v1/chat/completions", `"input":[{"role":"user","content":[{"type":"input_file","file_id":"own"}]}]`, h.ChatCompletions},
		{"legacy responses", "/v1/responses", `"messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"own"}}]}]`, h.Responses},
		{"responses", "/v1/responses", `"input":[{"role":"user","content":[{"type":"input_file","file_id":"own"}]}]`, h.Responses},
	} {
		for _, group := range []string{service.PlatformDeepseek, service.PlatformComposite} {
			t.Run(tc.name+"/"+group, func(t *testing.T) {
				before := len(up.ids)
				for _, user := range []int64{8, 7} {
					c, rec := fileOwnerTestContext("POST", tc.path, `{"model":"deepseek-flash","max_tokens":16,`+tc.payload+`}`, group, service.PlatformDeepseek, user)
					c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
					tc.handler(c)
					if user == 8 {
						require.Equal(t, 400, rec.Code, rec.Body.String())
						require.Len(t, up.ids, before)
						require.Equal(t, "Referenced file not found", gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
					} else {
						require.Equal(t, []int64{11}, up.ids[before:], rec.Body.String())
						require.Contains(t, up.bodies[before], `"own"`)
					}
				}
			})
		}
	}
}

func TestNonDeepseekMessagesFileBlocksFollowNormalDispatch(t *testing.T) {
	cache := testutil.NewRedisGatewayCache(t)
	for _, platform := range []string{service.PlatformKimi, service.PlatformZhipu, service.PlatformOpenAI, service.PlatformGrok} {
		t.Run(platform, func(t *testing.T) {
			h, up := fileOwnerTestHandler(t, cache, platform)
			model := "test-model"
			if platform == service.PlatformGrok {
				model = "grok-4.3"
			}
			var baseline int
			for i, content := range []string{`[{"type":"text","text":"hi"}]`, `[{"type":"text","text":"hi"},{"type":"image","source":{"type":"file","file_id":"unregistered"}}]`} {
				c, rec := fileOwnerTestContext("POST", "/v1/messages", `{"model":"`+model+`","max_tokens":16,"messages":[{"role":"user","content":`+content+`}]}`, service.PlatformComposite, platform, 7)
				h.Messages(c)
				if i == 0 {
					baseline = rec.Code
				}
				require.Equal(t, baseline, rec.Code, rec.Body.String())
				require.NotContains(t, rec.Body.String(), "Referenced file")
			}
			require.Len(t, up.ids, 2)
			require.Equal(t, up.ids[0], up.ids[1])
		})
	}
}

func TestDeepseekFilesUserIsolation(t *testing.T) {
	cache := testutil.NewRedisGatewayCache(t)
	store := cache.(service.DeepseekFileStore)
	h, up := fileOwnerTestHandler(t, cache, service.PlatformDeepseek)
	up.status = 200
	for _, beta := range []bool{false, true} {
		for user, id := range map[int64]string{7: "own", 8: "foreign"} {
			require.NoError(t, store.SaveDeepseekFile(context.Background(), user, service.DeepseekFile{
				ID: id, AccountID: 11, Origin: service.DefaultDeepseekBaseURL, Filename: id + ".png", Bytes: 18, CreatedAt: time.Now().UTC(), MIMEType: "image/png", Purpose: "user_data"}))
		}
		request := func(method, id, endpoint string) *httptest.ResponseRecorder {
			c, r := fileOwnerTestContext(method, "/v1/files"+id, "", service.PlatformDeepseek, "", 7)
			if beta {
				c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
			}
			h.DeepseekTools(c, endpoint)
			return r
		}
		before := len(up.ids)
		page := request("GET", "?limit=1", "files_list")
		require.Equal(t, 200, page.Code, page.Body.String())
		require.Equal(t, "own", gjson.GetBytes(page.Body.Bytes(), "data.0.id").String())
		require.NotContains(t, page.Body.String(), "foreign")
		require.NotContains(t, page.Body.String(), "AccountID")
		for _, method := range []string{"GET", "DELETE"} {
			endpoint := "files_" + strings.ToLower(method) + "/"
			unknown := request(method, "/unknown", endpoint+"unknown")
			foreign := request(method, "/foreign", endpoint+"foreign")
			require.Equal(t, 404, foreign.Code, foreign.Body.String())
			require.Equal(t, unknown.Body.String(), foreign.Body.String())
			require.Equal(t, "not_found_error", gjson.GetBytes(foreign.Body.Bytes(), "error.type").String())
			require.Equal(t, beta, gjson.GetBytes(foreign.Body.Bytes(), "type").Exists())
		}
		require.Len(t, up.ids, before)
		for _, method := range []string{"GET", "DELETE"} {
			up.reply = `{"id":"own","object":"file"}`
			if beta {
				up.reply = `{"id":"own","type":"file"}`
			}
			if method == http.MethodDelete {
				up.reply = `{"id":"own","object":"file","deleted":true}`
				if beta {
					up.reply = `{"id":"own","type":"file_deleted"}`
				}
			}
			rec := request(method, "/own?keep=a&keep=b", "files_"+strings.ToLower(method)+"/own")
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Contains(t, up.paths[len(up.paths)-1], "/v1/files/own")
		}
		require.Equal(t, []int64{11, 11}, up.ids[before:])
		require.Equal(t, 404, request("GET", "/own", "files_get/own").Code)
		require.Equal(t, 0, int(gjson.GetBytes(request("GET", "", "files_list").Body.Bytes(), "data.#").Int()))
	}
}

func TestDeepseekFilesDocumentUploadPolicyThroughHandler(t *testing.T) {
	cache := testutil.NewRedisGatewayCache(t)
	for _, anthropic := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			h, up := fileOwnerTestHandler(t, cache, service.PlatformDeepseek)
			h.cfg.Gateway.AllowUnbilledDeepseekDocumentParsing = enabled
			up.status = http.StatusOK
			up.reply = `{"id":"paid","object":"file","created_at":1800000000}`
			if anthropic {
				up.reply = `{"id":"paid","type":"file","created_at":"2026-11-20T00:00:00Z"}`
			}
			body := "--b\r\nContent-Disposition: form-data; name=\"purpose\"\r\n\r\nrich_layout_recognition\r\n--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"report.pdf\"\r\n\r\nPDF\r\n--b--\r\n"
			c, r := fileOwnerTestContext("POST", "/v1/files", body, service.PlatformDeepseek, "", 7)
			c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=b")
			if anthropic {
				c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
			}
			h.DeepseekTools(c, "files_create")
			if enabled {
				require.Equal(t, 200, r.Code, r.Body.String())
				require.Equal(t, []int64{12}, up.ids, r.Body.String())
				files, err := cache.(service.DeepseekFileStore).ListDeepseekFiles(context.Background(), 7, service.DeepseekFilePage{Limit: 20})
				require.NoError(t, err)
				require.Len(t, files, 1)
				require.Equal(t, "rich_layout_recognition", files[0].Purpose)
				require.Equal(t, "report.pdf", files[0].Filename)
				require.Equal(t, int64(3), files[0].Bytes)
				require.Equal(t, body, up.bodies[0])
			} else {
				require.Empty(t, up.ids)
				require.Equal(t, 400, r.Code, r.Body.String())
				require.Equal(t, anthropic, gjson.GetBytes(r.Body.Bytes(), "type").Exists())
				require.Contains(t, r.Body.String(), "/v1/tools/ocr")
			}
		}
	}
}

type mappedFileStore struct {
	stickyWritebackCache
	service.DeepseekFileStore
}

func (*mappedFileStore) LookupDeepseekFiles(_ context.Context, userID int64, ids []string) (map[string]service.DeepseekFile, error) {
	files := map[string]service.DeepseekFile{}
	for _, id := range ids {
		if userID == 7 && id == "own" {
			files[id] = service.DeepseekFile{ID: id, AccountID: 11, Origin: service.DefaultDeepseekBaseURL, Format: "openai"}
		}
	}
	return files, nil
}

func TestDeepseekMappedOpenAIFileOwnerHandler(t *testing.T) {
	h, up := fileOwnerTestHandler(t, &mappedFileStore{}, service.PlatformOpenAI, service.DefaultDeepseekBaseURL)
	body := `{"model":"deepseek-flash","messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"own"}}]}]}`
	for _, user := range []int64{8, 7} {
		c, rec := fileOwnerTestContext("POST", "/v1/chat/completions", body, service.PlatformOpenAI, "", user)
		h.ChatCompletions(c)
		if user == 8 {
			require.Equal(t, 400, rec.Code, rec.Body.String())
			require.Empty(t, up.ids)
		} else {
			require.Equal(t, []int64{11}, up.ids, rec.Body.String())
			require.Contains(t, up.bodies[0], "own")
		}
	}
}
