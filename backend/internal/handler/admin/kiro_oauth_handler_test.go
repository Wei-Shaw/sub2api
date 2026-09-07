//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestKiroAuthorizeURLRequiresIssuer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewKiroOAuthHandler(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/authorize-url",
		strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.AuthorizeURL(c)
	require.GreaterOrEqual(t, w.Code, 400, "缺少必填参数必须报错")
}

// TestKiroAuthorizeURLIdCEndToEndCompletesViaPastedCallbackURL 是真实账号
// 联调发现的回归：AWS SSO-OIDC 的 client/register 对 IdC（clientType=public）
// 强制要求 redirect_uri 是裸 loopback 地址——sub2api 自建回调页这条路（浏览器
// 整页跳转回我们自己的服务器）在真实账号上直接被 AWS 拒绝
// （invalid_redirect_uri / "Requested client type must use loopback
// interface for redirect"），根本到不了回调页那一步。
//
// 正确流程改成跟已验证可用的参考实现 Kiro-Go 一致：redirect_uri 固定指向
// 一个没有服务监听的 loopback 地址，管理员在浏览器完成登录后手动复制地址栏
// 里"无法连接"页面的完整 URL，粘贴回来交给 CompleteIdC 解析 code/state 并
// 换取 token。这里端到端串联 AuthorizeURL → CompleteIdC，验证整条链路。
func TestKiroAuthorizeURLIdCEndToEndCompletesViaPastedCallbackURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/register":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			// 真实账号测试发现的核心断言：注册请求的 redirectUris 必须是
			// 裸 loopback 地址，不能是我们自己拼的服务端回调地址。
			redirectURIs, _ := body["redirectUris"].([]any)
			require.Len(t, redirectURIs, 1)
			require.Equal(t, "http://127.0.0.1/oauth/callback", redirectURIs[0])
			_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
		case "/token":
			_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600,"profileArn":"arn:x"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := service.NewKiroOAuthService(nil)
	defer svc.Stop()
	svc = svc.WithBaseURLs(
		func(string) string { return srv.URL },
		func(string) string { return srv.URL },
	)
	h := NewKiroOAuthHandler(svc)

	urlW := httptest.NewRecorder()
	urlC, _ := gin.CreateTestContext(urlW)
	urlC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/authorize-url",
		strings.NewReader(`{"issuer_url":"https://d-x.awsapps.com/start","region":"us-east-1"}`))
	urlC.Request.Header.Set("Content-Type", "application/json")
	h.AuthorizeURL(urlC)
	require.Equal(t, http.StatusOK, urlW.Code)

	var urlResp struct {
		Data struct {
			SessionID    string `json:"session_id"`
			AuthorizeURL string `json:"authorize_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(urlW.Body.Bytes(), &urlResp))
	require.NotEmpty(t, urlResp.Data.SessionID)

	parsed, err := url.Parse(urlResp.Data.AuthorizeURL)
	require.NoError(t, err)
	redirectURI := parsed.Query().Get("redirect_uri")
	require.Equal(t, "http://127.0.0.1/oauth/callback", redirectURI)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)

	// 真实场景：管理员在浏览器登录完成后，AWS 把浏览器带到这个打不开的
	// loopback 地址，手动复制地址栏完整 URL 粘贴回来——session_id 是前端
	// 早先从 authorize-url 响应里拿到、自己持有的，不需要（也不可能）从这个
	// URL 里解析出来。
	pastedURL := redirectURI + "?code=authcode&state=" + url.QueryEscape(state)

	completeBody, err := json.Marshal(map[string]string{
		"session_id":   urlResp.Data.SessionID,
		"callback_url": pastedURL,
	})
	require.NoError(t, err)
	completeW := httptest.NewRecorder()
	completeC, _ := gin.CreateTestContext(completeW)
	completeC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/idc/complete", bytes.NewReader(completeBody))
	completeC.Request.Header.Set("Content-Type", "application/json")
	h.CompleteIdC(completeC)

	require.Equal(t, http.StatusOK, completeW.Code, "完成授权必须成功: %s", completeW.Body.String())
	var completeResp struct {
		Data struct {
			Status      string         `json:"status"`
			Credentials map[string]any `json:"credentials"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(completeW.Body.Bytes(), &completeResp))
	require.Equal(t, "ok", completeResp.Data.Status)
	require.Equal(t, "at", completeResp.Data.Credentials["access_token"])
	require.Equal(t, "arn:x", completeResp.Data.Credentials["profile_arn"])
}

// TestKiroCompleteIdCDiscoversProfileArnWhenTokenResponseOmitsIt 覆盖真实
// IdC 行为（与上面端到端测试里 mock 的 "profileArn":"arn:x" 不同——那是
// 为了同一个测试文件不必再起一个 mock server 而顺手带上的简化，真实 IdC
// 的 /token 响应里没有这个字段）：CompleteIdC 在 BuildAccountCredentials
// 拿到的 creds 里没有 profile_arn 时，必须尝试一次 ListAvailableProfiles
// 自动发现（DiscoverProfileArn），发现成功就直接把新账号的 credentials
// 补上，不需要用户再跑一次 KiroCredentialFields.vue 的手填流程。
func TestKiroCompleteIdCDiscoversProfileArnWhenTokenResponseOmitsIt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/register":
			_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
		case "/token":
			// 真实 IdC /token 响应形态：没有 profileArn 字段。
			_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600}`))
		case "/ListAvailableProfiles":
			require.Equal(t, "Bearer at", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"profiles": []map[string]string{
					{"arn": "arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456", "profileName": "default"},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := service.NewKiroOAuthService(nil)
	defer svc.Stop()
	svc = svc.WithBaseURLs(
		func(string) string { return srv.URL },
		func(string) string { return srv.URL },
	)
	svc = svc.WithListProfilesHost(func(string) string { return srv.URL })
	h := NewKiroOAuthHandler(svc)

	urlW := httptest.NewRecorder()
	urlC, _ := gin.CreateTestContext(urlW)
	urlC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/authorize-url",
		strings.NewReader(`{"issuer_url":"https://d-x.awsapps.com/start","region":"us-east-1"}`))
	urlC.Request.Header.Set("Content-Type", "application/json")
	h.AuthorizeURL(urlC)
	require.Equal(t, http.StatusOK, urlW.Code)

	var urlResp struct {
		Data struct {
			SessionID    string `json:"session_id"`
			AuthorizeURL string `json:"authorize_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(urlW.Body.Bytes(), &urlResp))
	parsed, err := url.Parse(urlResp.Data.AuthorizeURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	pastedURL := parsed.Query().Get("redirect_uri") + "?code=authcode&state=" + url.QueryEscape(state)

	completeBody, err := json.Marshal(map[string]string{
		"session_id":   urlResp.Data.SessionID,
		"callback_url": pastedURL,
	})
	require.NoError(t, err)
	completeW := httptest.NewRecorder()
	completeC, _ := gin.CreateTestContext(completeW)
	completeC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/idc/complete", bytes.NewReader(completeBody))
	completeC.Request.Header.Set("Content-Type", "application/json")
	h.CompleteIdC(completeC)

	require.Equal(t, http.StatusOK, completeW.Code, "完成授权必须成功: %s", completeW.Body.String())
	var completeResp struct {
		Data struct {
			Credentials map[string]any `json:"credentials"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(completeW.Body.Bytes(), &completeResp))
	require.Equal(t, "arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456",
		completeResp.Data.Credentials["profile_arn"],
		"token 响应没带 profile_arn 时必须靠自动发现补上")
}

// TestKiroCompleteIdCDiscoveryFailureDoesNotBlockAccountCreation 覆盖发现
// 失败（这里模拟 Builder ID 会撞到的真实 403）时账号创建本身不能被阻断——
// 这只是一次锦上添花的尝试，account creation 的成败不应该依赖它。
func TestKiroCompleteIdCDiscoveryFailureDoesNotBlockAccountCreation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/register":
			_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
		case "/token":
			_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt","expiresIn":3600}`))
		case "/ListAvailableProfiles":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"AWS Builder ID is not supported for this operation"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := service.NewKiroOAuthService(nil)
	defer svc.Stop()
	svc = svc.WithBaseURLs(
		func(string) string { return srv.URL },
		func(string) string { return srv.URL },
	)
	svc = svc.WithListProfilesHost(func(string) string { return srv.URL })
	h := NewKiroOAuthHandler(svc)

	urlW := httptest.NewRecorder()
	urlC, _ := gin.CreateTestContext(urlW)
	urlC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/authorize-url",
		strings.NewReader(`{"issuer_url":"https://d-x.awsapps.com/start","region":"us-east-1"}`))
	urlC.Request.Header.Set("Content-Type", "application/json")
	h.AuthorizeURL(urlC)

	var urlResp struct {
		Data struct {
			SessionID    string `json:"session_id"`
			AuthorizeURL string `json:"authorize_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(urlW.Body.Bytes(), &urlResp))
	parsed, err := url.Parse(urlResp.Data.AuthorizeURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	pastedURL := parsed.Query().Get("redirect_uri") + "?code=authcode&state=" + url.QueryEscape(state)

	completeBody, err := json.Marshal(map[string]string{
		"session_id":   urlResp.Data.SessionID,
		"callback_url": pastedURL,
	})
	require.NoError(t, err)
	completeW := httptest.NewRecorder()
	completeC, _ := gin.CreateTestContext(completeW)
	completeC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/idc/complete", bytes.NewReader(completeBody))
	completeC.Request.Header.Set("Content-Type", "application/json")
	h.CompleteIdC(completeC)

	require.Equal(t, http.StatusOK, completeW.Code, "发现失败不应该阻断账号创建本身: %s", completeW.Body.String())
	var completeResp struct {
		Data struct {
			Credentials map[string]any `json:"credentials"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(completeW.Body.Bytes(), &completeResp))
	require.NotContains(t, completeResp.Data.Credentials, "profile_arn", "发现失败时不应该写入任何 profile_arn")
}

// TestKiroCompleteIdCSurfacesAuthorizeError 覆盖管理员在 AWS 门户拒绝/取消
// 授权时的路径：粘贴回来的 URL 带的是 ?error=... 而不是 ?code=...，
// CompleteIdC 必须报错，不能把 error 参数当 code 传下去。
func TestKiroCompleteIdCSurfacesAuthorizeError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := service.NewKiroOAuthService(nil)
	defer svc.Stop()
	h := NewKiroOAuthHandler(svc)

	body, err := json.Marshal(map[string]string{
		"session_id":   "sid",
		"callback_url": "http://127.0.0.1/oauth/callback?error=access_denied&error_description=User+declined",
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/idc/complete", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CompleteIdC(c)

	require.GreaterOrEqual(t, w.Code, 400)
}

func TestKiroCompleteIdCRequiresSessionIDAndCallbackURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewKiroOAuthHandler(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/idc/complete", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CompleteIdC(c)

	require.GreaterOrEqual(t, w.Code, 400)
}

// TestKiroDevicePollPendingIsNotAnError 是「pending 不是错误」这条契约的
// 唯一守卫：DeviceStart 拿到会话后，AWS 端在真正授权完成前对 /token 轮询
// 恒答 authorization_pending，DevicePoll 必须把它翻译成 200 +
// {"status":"pending","interval":N}，不能让前端轮询被当成失败中止。
//
// KiroOAuthService 的 oidcBase/socialBase 字段未导出，本包（admin）拿不到
// 直接赋值的机会；WithBaseURLs 是专为这类跨包 handler 测试开的口子（见
// kiro_oauth_service.go 上的注释），生产环境的 wire 从不调用它。
func TestKiroDevicePollPendingIsNotAnError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/register":
			_, _ = w.Write([]byte(`{"clientId":"cid","clientSecret":"csec"}`))
		case "/device_authorization":
			_, _ = w.Write([]byte(`{"deviceCode":"dc","userCode":"ABCD-EFGH",
				"verificationUri":"https://view.awsapps.com/start/#/device",
				"verificationUriComplete":"https://view.awsapps.com/start/#/device?user_code=ABCD-EFGH",
				"expiresIn":600,"interval":5}`))
		case "/token":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	svc := service.NewKiroOAuthService(nil)
	defer svc.Stop()
	svc = svc.WithBaseURLs(
		func(string) string { return srv.URL },
		func(string) string { return srv.URL },
	)
	h := NewKiroOAuthHandler(svc)

	// DeviceStart 拿一个真实会话，好让 DevicePoll 能查到 client_id/device_code。
	startW := httptest.NewRecorder()
	startC, _ := gin.CreateTestContext(startW)
	startC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/device/start",
		strings.NewReader(`{"region":"us-east-1"}`))
	startC.Request.Header.Set("Content-Type", "application/json")
	h.DeviceStart(startC)
	require.Equal(t, http.StatusOK, startW.Code)

	var startResp struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(startW.Body.Bytes(), &startResp))
	require.NotEmpty(t, startResp.Data.SessionID)

	// AWS 端尚未完成授权：/token 恒答 authorization_pending。
	pollBody, err := json.Marshal(map[string]string{"session_id": startResp.Data.SessionID})
	require.NoError(t, err)
	pollW := httptest.NewRecorder()
	pollC, _ := gin.CreateTestContext(pollW)
	pollC.Request = httptest.NewRequest(http.MethodPost, "/admin/kiro/oauth/device/poll", bytes.NewReader(pollBody))
	pollC.Request.Header.Set("Content-Type", "application/json")
	h.DevicePoll(pollC)

	require.Equal(t, http.StatusOK, pollW.Code, "pending 必须是 200，不能是错误状态码")
	var pollResp struct {
		Data struct {
			Status   string `json:"status"`
			Interval int    `json:"interval"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(pollW.Body.Bytes(), &pollResp))
	require.Equal(t, "pending", pollResp.Data.Status)
	require.Positive(t, pollResp.Data.Interval)
}
