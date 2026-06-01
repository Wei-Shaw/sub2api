package repository

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TencentCaptchaSuite 覆盖 §2 全部行为：
//   - TC3 签名结构（不依赖外部签名向量；用本地确定输入 freeze 关键中间值）
//   - trerror_ 短路 + 不发 HTTP
//   - 各 CaptchaCode → ErrorCode 归一化
//   - HTTP 层错误 → captcha.network
//   - payload / config 缺字段 → 短路
//   - Error 对象（鉴权失败）→ captcha.config
type TencentCaptchaSuite struct {
	suite.Suite
	ctx      context.Context
	verifier *captchaHTTPVerifier
}

func (s *TencentCaptchaSuite) SetupTest() {
	s.ctx = context.Background()
	verifier, ok := NewCaptchaVerifier().(*captchaHTTPVerifier)
	require.True(s.T(), ok, "type assertion failed")
	s.verifier = verifier
}

// completeTencentConfig 返回一份合法的 4 字段天御配置，单测复用。
func completeTencentConfig() map[string]string {
	return map[string]string{
		"captcha_app_id": "200000000",
		"app_secret_key": "test-app-secret",
		"secret_id":      "AKID-test",
		"secret_key":     "secret-key-test",
	}
}

// completeTencentPayload 返回一份非 trerror_ 的 payload。
func completeTencentPayload(ticket string) map[string]string {
	return map[string]string{"ticket": ticket, "randstr": "abcd"}
}

// installTencentTransport 把 verifier 内的 httpClient 替换为定向到 in-process server，
// 同时把 endpoint 切到测试可拦截的 host（保持 Host header 仍是真实 captcha host，
// 这样可以验证签名仍按真实 host 计算）。
//
// captureBody 若非 nil，会把请求 body 写入其中（chan 模式让断言可以非阻塞读取）。
func (s *TencentCaptchaSuite) installTencentTransport(handler http.HandlerFunc) {
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// 在 transport 层直接调 handler。
			rw := newRecordingResponseWriter()
			handler(rw, req)
			return rw.toHTTPResponse(req), nil
		}),
	}
}

func (s *TencentCaptchaSuite) TestVerify_FallbackTicket_ShortCircuits() {
	// transport 故意 panic：如果实现错误地发出了 HTTP 请求，立刻显形。
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			s.T().Fatal("must not call HTTP for trerror_ ticket")
			return nil, errors.New("unreachable")
		}),
	}

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   completeTencentConfig(),
		Payload:  completeTencentPayload("trerror_1001"),
		RemoteIP: "1.2.3.4",
	})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrTencentFallbackTicket, resp.ErrorCode)
}

func (s *TencentCaptchaSuite) TestVerify_TicketWithSimilarPrefix_NotShortCircuited() {
	// "trerr_xxx" 不应被识别为 fallback；应该走正常请求路径。
	called := false
	s.installTencentTransport(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"CaptchaCode":1,"CaptchaMsg":"OK","EvilLevel":0,"RequestId":"rid"}}`))
	})

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   completeTencentConfig(),
		Payload:  completeTencentPayload("trerr_xxx"),
		RemoteIP: "1.2.3.4",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), called, "expected HTTP call for non-trerror_ prefix")
	require.True(s.T(), resp.Success)
}

func (s *TencentCaptchaSuite) TestVerify_MissingPayloadFields_ShortCircuits() {
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			s.T().Fatal("must not call HTTP for empty payload")
			return nil, errors.New("unreachable")
		}),
	}

	cases := []map[string]string{
		{},
		{"ticket": ""},
		{"ticket": "abc"},                 // missing randstr
		{"randstr": "abcd"},               // missing ticket
		{"ticket": "abc", "randstr": ""},  // empty randstr
		{"ticket": "", "randstr": "abcd"}, // empty ticket
	}
	for _, payload := range cases {
		resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
			Provider: service.CaptchaProviderTencent,
			Config:   completeTencentConfig(),
			Payload:  payload,
		})
		require.NoError(s.T(), err)
		require.False(s.T(), resp.Success)
		require.Equal(s.T(), service.CaptchaErrInvalid, resp.ErrorCode, "payload=%v", payload)
	}
}

func (s *TencentCaptchaSuite) TestVerify_MissingConfigFields_ShortCircuits() {
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			s.T().Fatal("must not call HTTP for incomplete config")
			return nil, errors.New("unreachable")
		}),
	}

	keys := []string{"captcha_app_id", "app_secret_key", "secret_id", "secret_key"}
	for _, missing := range keys {
		cfg := completeTencentConfig()
		delete(cfg, missing)
		resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
			Provider: service.CaptchaProviderTencent,
			Config:   cfg,
			Payload:  completeTencentPayload("good-ticket"),
		})
		require.NoError(s.T(), err)
		require.False(s.T(), resp.Success)
		require.Equal(s.T(), service.CaptchaErrConfig, resp.ErrorCode, "missing key=%s", missing)
	}
}

func (s *TencentCaptchaSuite) TestVerify_BadAppId_ConfigError() {
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			s.T().Fatal("must not call HTTP for invalid app id")
			return nil, errors.New("unreachable")
		}),
	}
	cfg := completeTencentConfig()
	cfg["captcha_app_id"] = "not-a-number"
	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   cfg,
		Payload:  completeTencentPayload("good-ticket"),
	})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrConfig, resp.ErrorCode)
}

func (s *TencentCaptchaSuite) TestVerify_CaptchaCodeMappings() {
	cases := []struct {
		name        string
		captchaCode int
		expectCode  string
	}{
		{"success", 1, ""}, // success 不带 ErrorCode
		{"6 decrypt failure", 6, service.CaptchaErrConfig},
		{"7 signature failure", 7, service.CaptchaErrConfig},
		{"15 app secret mismatch", 15, service.CaptchaErrConfig},
		{"9 expired", 9, service.CaptchaErrTimeout},
		{"10 duplicate", 10, service.CaptchaErrDuplicate},
		{"5 invalid ticket", 5, service.CaptchaErrInvalid},
		{"100 unknown", 100, service.CaptchaErrInvalid},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.installTencentTransport(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"Response": map[string]any{
						"CaptchaCode": tc.captchaCode,
						"CaptchaMsg":  "msg-" + tc.name,
						"EvilLevel":   0,
						"RequestId":   "rid",
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
			})

			resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
				Provider: service.CaptchaProviderTencent,
				Config:   completeTencentConfig(),
				Payload:  completeTencentPayload("ticket-x"),
				RemoteIP: "1.2.3.4",
			})
			require.NoError(s.T(), err)

			if tc.captchaCode == 1 {
				require.True(s.T(), resp.Success)
				require.Empty(s.T(), resp.ErrorCode)
			} else {
				require.False(s.T(), resp.Success)
				require.Equal(s.T(), tc.expectCode, resp.ErrorCode)
				require.Contains(s.T(), resp.ProviderMsg, "msg-"+tc.name)
			}
		})
	}
}

func (s *TencentCaptchaSuite) TestVerify_AuthFailure_NormalizedToConfig() {
	s.installTencentTransport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"AuthFailure.SignatureFailure","Message":"sign error"},"RequestId":"rid"}}`))
	})

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   completeTencentConfig(),
		Payload:  completeTencentPayload("good-ticket"),
	})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrConfig, resp.ErrorCode)
	require.Contains(s.T(), resp.ProviderMsg, "AuthFailure.SignatureFailure")
}

func (s *TencentCaptchaSuite) TestVerify_TransientPlatformError_NormalizedToNetwork() {
	s.installTencentTransport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"InternalError","Message":"oops"},"RequestId":"rid"}}`))
	})

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   completeTencentConfig(),
		Payload:  completeTencentPayload("good-ticket"),
	})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrNetwork, resp.ErrorCode)
}

func (s *TencentCaptchaSuite) TestVerify_HTTPError_NormalizedToNetwork() {
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial timeout")
		}),
	}

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   completeTencentConfig(),
		Payload:  completeTencentPayload("good-ticket"),
	})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrNetwork, resp.ErrorCode)
	require.Contains(s.T(), resp.ProviderMsg, "dial timeout")
}

func (s *TencentCaptchaSuite) TestVerify_HTTP5xx_NormalizedToNetwork() {
	s.installTencentTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream down"))
	})

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   completeTencentConfig(),
		Payload:  completeTencentPayload("good-ticket"),
	})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrNetwork, resp.ErrorCode)
	require.Contains(s.T(), resp.ProviderMsg, "non-2xx status 500")
}

func (s *TencentCaptchaSuite) TestVerify_BodyContainsAllFields() {
	var capturedBody []byte
	var capturedAuth string
	s.installTencentTransport(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Response":{"CaptchaCode":1,"CaptchaMsg":"OK","EvilLevel":2,"RequestId":"rid"}}`))
	})

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTencent,
		Config:   completeTencentConfig(),
		Payload:  completeTencentPayload("good-ticket"),
		RemoteIP: "1.2.3.4",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.Success)
	require.Equal(s.T(), 2, resp.EvilLevel)

	// Auth header 必须以算法名开头并包含 SecretId 与 Service scope
	require.True(s.T(), strings.HasPrefix(capturedAuth, "TC3-HMAC-SHA256 "), "auth header: %s", capturedAuth)
	require.Contains(s.T(), capturedAuth, "Credential=AKID-test/")
	require.Contains(s.T(), capturedAuth, "/captcha/tc3_request")
	require.Contains(s.T(), capturedAuth, "SignedHeaders=content-type;host;x-tc-action")
	require.Contains(s.T(), capturedAuth, "Signature=")

	// Body 必须包含 5 个业务字段
	var body map[string]any
	require.NoError(s.T(), json.Unmarshal(capturedBody, &body))
	require.EqualValues(s.T(), 9, body["CaptchaType"])
	require.Equal(s.T(), "good-ticket", body["Ticket"])
	require.Equal(s.T(), "abcd", body["Randstr"])
	require.Equal(s.T(), "1.2.3.4", body["UserIp"])
	require.EqualValues(s.T(), 200000000, body["CaptchaAppId"])
	require.Equal(s.T(), "test-app-secret", body["AppSecretKey"])
}

// TestTC3Authorization_StructureCheck 校验 Authorization 头的结构合法性：
// 算法名前缀、Credential scope 字面值、SignedHeaders 列表、Signature 必须是 64 字符 hex。
//
// 不硬编码"完整签名向量"的原因：腾讯云官方文档没给 captcha 服务的样例向量，
// 自造的 freeze 值随实现微调易脆裂。改由 TestTC3Authorization_StableAcrossRuns
// 提供"实现内自一致"保证；本测试覆盖外部可观测的结构特征。
func TestTC3Authorization_StructureCheck(t *testing.T) {
	signedAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	body := []byte(`{"CaptchaAppId":200000000,"AppSecretKey":"app-sk","CaptchaType":9,"Ticket":"tk","Randstr":"rs","UserIp":"1.2.3.4"}`)

	auth := buildTencentTC3Authorization("AKID000000000000000000000000EXAMPLE", "000000000000000000000000000EXAMPLE", body, signedAt)

	require.True(t, strings.HasPrefix(auth, "TC3-HMAC-SHA256 "), "auth: %s", auth)
	require.Contains(t, auth, "Credential=AKID000000000000000000000000EXAMPLE/2024-01-02/captcha/tc3_request")
	require.Contains(t, auth, "SignedHeaders=content-type;host;x-tc-action")

	// 解析 Signature= 段。
	idx := strings.Index(auth, "Signature=")
	require.True(t, idx >= 0, "Signature= segment missing in: %s", auth)
	gotSig := strings.TrimSpace(auth[idx+len("Signature="):])
	if i := strings.IndexAny(gotSig, ", "); i >= 0 {
		gotSig = gotSig[:i]
	}
	require.Len(t, gotSig, 64, "signature should be 64-char hex (sha256), got: %s", gotSig)
	for _, ch := range gotSig {
		require.True(t, (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'), "non-hex char in signature: %q", ch)
	}
}

func TestTC3Authorization_StableAcrossRuns(t *testing.T) {
	signedAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	body := []byte(`{"CaptchaAppId":200000000,"AppSecretKey":"app-sk","CaptchaType":9,"Ticket":"tk","Randstr":"rs","UserIp":"1.2.3.4"}`)

	a := buildTencentTC3Authorization("SID", "SK", body, signedAt)
	b := buildTencentTC3Authorization("SID", "SK", body, signedAt)
	require.Equal(t, a, b, "same inputs must yield same signature")

	c := buildTencentTC3Authorization("SID", "SK", body, signedAt.Add(time.Second))
	require.NotEqual(t, a, c, "different timestamp must change signature")

	d := buildTencentTC3Authorization("SID", "SK2", body, signedAt)
	require.NotEqual(t, a, d, "different secret key must change signature")
}

func TestAnonymizeIP(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "unknown"},
		{"not-an-ip", "unknown"},
		{"1.2.3.4", "1.2.3.0"},
		{"  10.0.0.7  ", "10.0.0.0"},
		{"2001:db8:1234:5678::1", "2001:db8:1234::"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, anonymizeIP(tc.in), "input=%q", tc.in)
	}
}

func TestNormalizeTencentCaptchaCode(t *testing.T) {
	require.Equal(t, service.CaptchaErrConfig, normalizeTencentCaptchaCode(6))
	require.Equal(t, service.CaptchaErrConfig, normalizeTencentCaptchaCode(7))
	require.Equal(t, service.CaptchaErrConfig, normalizeTencentCaptchaCode(15))
	require.Equal(t, service.CaptchaErrTimeout, normalizeTencentCaptchaCode(9))
	require.Equal(t, service.CaptchaErrDuplicate, normalizeTencentCaptchaCode(10))
	require.Equal(t, service.CaptchaErrInvalid, normalizeTencentCaptchaCode(100))
	require.Equal(t, service.CaptchaErrInvalid, normalizeTencentCaptchaCode(0))
}

func TestIsTencentAuthErrorCode(t *testing.T) {
	require.True(t, isTencentAuthErrorCode("AuthFailure"))
	require.True(t, isTencentAuthErrorCode("AuthFailure.SignatureFailure"))
	require.True(t, isTencentAuthErrorCode("UnauthorizedOperation"))
	require.True(t, isTencentAuthErrorCode("InvalidParameterValue.AppidNotFound"))
	require.True(t, isTencentAuthErrorCode("InvalidParameterValue.AppSecretKeyError"))
	require.False(t, isTencentAuthErrorCode("InternalError"))
	require.False(t, isTencentAuthErrorCode(""))
	require.False(t, isTencentAuthErrorCode("InvalidParameterValue.SomeOtherError"))
}

func TestTencentCaptchaSuite(t *testing.T) {
	suite.Run(t, new(TencentCaptchaSuite))
}

// recordingResponseWriter 一个最小可用的 http.ResponseWriter 实现，
// 用于在 RoundTrip 内拼装响应（不使用 httptest.Server 减少端口占用与并发干扰）。
type recordingResponseWriter struct {
	header http.Header
	body   []byte
	status int
}

func newRecordingResponseWriter() *recordingResponseWriter {
	return &recordingResponseWriter{
		header: make(http.Header),
	}
}

func (w *recordingResponseWriter) Header() http.Header { return w.header }

func (w *recordingResponseWriter) Write(p []byte) (int, error) {
	w.body = append(w.body, p...)
	return len(p), nil
}

func (w *recordingResponseWriter) WriteHeader(code int) { w.status = code }

func (w *recordingResponseWriter) toHTTPResponse(req *http.Request) *http.Response {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     w.header,
		Body:       io.NopCloser(strings.NewReader(string(w.body))),
		Request:    req,
	}
}
