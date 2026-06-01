package repository

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CaptchaVerifierSuite struct {
	suite.Suite
	ctx      context.Context
	verifier *captchaHTTPVerifier
	received chan url.Values
}

func (s *CaptchaVerifierSuite) SetupTest() {
	s.ctx = context.Background()
	s.received = make(chan url.Values, 1)
	verifier, ok := NewCaptchaVerifier().(*captchaHTTPVerifier)
	require.True(s.T(), ok, "type assertion failed")
	s.verifier = verifier
}

func (s *CaptchaVerifierSuite) setupTransport(provider string, handler http.HandlerFunc) {
	s.verifier.verifyURLs[provider] = "http://in-process/" + provider
	s.verifier.httpClient = &http.Client{
		Transport: newInProcessTransport(handler, nil),
	}
}

// verifyTurnstile 是测试常用的最小调用：组装 token + secret 并触发 Verify。
// 抽出 helper 以避免每个用例重复构造 VerifyRequest。
func (s *CaptchaVerifierSuite) verifyTurnstile(token, secret, remoteIP string) (*service.VerifyResult, error) {
	return s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTurnstile,
		Config:   map[string]string{"secret_key": secret},
		Payload:  map[string]string{"token": token},
		RemoteIP: remoteIP,
	})
}

func (s *CaptchaVerifierSuite) TestVerify_SendsFormAndDecodesJSON() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		s.received <- values

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{Success: true})
	}))

	resp, err := s.verifyTurnstile("token", "sk", "1.1.1.1")
	require.NoError(s.T(), err, "Verify")
	require.NotNil(s.T(), resp)
	require.True(s.T(), resp.Success, "expected success result")

	select {
	case values := <-s.received:
		require.Equal(s.T(), "sk", values.Get("secret"))
		require.Equal(s.T(), "token", values.Get("response"))
		require.Equal(s.T(), "1.1.1.1", values.Get("remoteip"))
	default:
		require.Fail(s.T(), "expected server to receive request")
	}
}

func (s *CaptchaVerifierSuite) TestVerify_ContentType() {
	var contentType string
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{Success: true})
	}))

	_, err := s.verifyTurnstile("token", "sk", "1.1.1.1")
	require.NoError(s.T(), err)
	require.True(s.T(), strings.HasPrefix(contentType, "application/x-www-form-urlencoded"), "unexpected content-type: %s", contentType)
}

func (s *CaptchaVerifierSuite) TestVerify_EmptyRemoteIP_NotSent() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		s.received <- values

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{Success: true})
	}))

	_, err := s.verifyTurnstile("token", "sk", "")
	require.NoError(s.T(), err)

	select {
	case values := <-s.received:
		require.Equal(s.T(), "", values.Get("remoteip"), "remoteip should be empty or not sent")
	default:
		require.Fail(s.T(), "expected server to receive request")
	}
}

func (s *CaptchaVerifierSuite) TestVerify_RequestError_NormalizedToNetwork() {
	s.verifier.verifyURLs[service.CaptchaProviderTurnstile] = "http://in-process/turnstile"
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}),
	}

	resp, err := s.verifyTurnstile("token", "sk", "1.1.1.1")
	require.NoError(s.T(), err, "Verify should not propagate transport errors")
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrNetwork, resp.ErrorCode)
}

func (s *CaptchaVerifierSuite) TestVerify_InvalidJSON_NormalizedToNetwork() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "not-valid-json")
	}))

	resp, err := s.verifyTurnstile("token", "sk", "1.1.1.1")
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrNetwork, resp.ErrorCode)
}

func (s *CaptchaVerifierSuite) TestVerify_SuccessFalse_InvalidResponse() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-response"},
		})
	}))

	resp, err := s.verifyTurnstile("token", "sk", "1.1.1.1")
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrInvalid, resp.ErrorCode)
	require.Contains(s.T(), resp.ProviderMsg, "invalid-input-response")
}

func (s *CaptchaVerifierSuite) TestVerify_SuccessFalse_InvalidSecret() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-secret"},
		})
	}))

	resp, err := s.verifyTurnstile("token", "sk", "1.1.1.1")
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrConfig, resp.ErrorCode)
}

func (s *CaptchaVerifierSuite) TestVerify_SuccessFalse_TimeoutOrDuplicate() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"timeout-or-duplicate"},
		})
	}))

	resp, err := s.verifyTurnstile("token", "sk", "1.1.1.1")
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.CaptchaErrTimeout, resp.ErrorCode)
}

func (s *CaptchaVerifierSuite) TestVerify_DispatchesProviderURLs() {
	var requestedPaths []string
	s.verifier.verifyURLs[service.CaptchaProviderTurnstile] = "http://in-process/turnstile"
	s.verifier.verifyURLs[service.CaptchaProviderHcaptcha] = "http://in-process/hcaptcha"
	s.verifier.httpClient = &http.Client{
		Transport: newInProcessTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestedPaths = append(requestedPaths, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{Success: true})
		}), nil),
	}

	_, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTurnstile,
		Config:   map[string]string{"secret_key": "sk"},
		Payload:  map[string]string{"token": "token"},
	})
	require.NoError(s.T(), err)
	_, err = s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderHcaptcha,
		Config:   map[string]string{"secret_key": "sk"},
		Payload:  map[string]string{"token": "token"},
	})
	require.NoError(s.T(), err)

	require.Equal(s.T(), []string{"/turnstile", "/hcaptcha"}, requestedPaths)
}

func (s *CaptchaVerifierSuite) TestVerify_UnsupportedProvider() {
	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: "unknown",
		Payload:  map[string]string{"token": "tok"},
	})
	require.NoError(s.T(), err, "unsupported provider should be normalized, not errored")
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrConfig, resp.ErrorCode)
	require.Contains(s.T(), resp.ProviderMsg, "unsupported")
}

func (s *CaptchaVerifierSuite) TestVerify_MissingTokenInPayload_ShortCircuits() {
	// 不设置 transport：如果错误地发了 HTTP 请求，会因为没有 transport 替换而走真实网络。
	// 这里通过传一个明显无效的 URL 让"被错误调用"立刻失败，从而区分行为。
	s.verifier.verifyURLs[service.CaptchaProviderTurnstile] = "http://127.0.0.1:1/should-not-call"

	resp, err := s.verifier.Verify(s.ctx, service.VerifyRequest{
		Provider: service.CaptchaProviderTurnstile,
		Config:   map[string]string{"secret_key": "sk"},
		Payload:  map[string]string{}, // empty
	})
	require.NoError(s.T(), err)
	require.False(s.T(), resp.Success)
	require.Equal(s.T(), service.CaptchaErrInvalid, resp.ErrorCode)
}

func (s *CaptchaVerifierSuite) TestValidateProviderConfig_TurnstileBadSecret() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-secret"},
		})
	}))

	err := s.verifier.ValidateProviderConfig(s.ctx, service.CaptchaProviderTurnstile, map[string]string{"secret_key": "bad"})
	require.ErrorIs(s.T(), err, service.ErrTurnstileInvalidSecretKey)
}

func (s *CaptchaVerifierSuite) TestValidateProviderConfig_HCaptchaBadSecret() {
	s.setupTransport(service.CaptchaProviderHcaptcha, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"missing-input-secret"},
		})
	}))

	err := s.verifier.ValidateProviderConfig(s.ctx, service.CaptchaProviderHcaptcha, map[string]string{"secret_key": "bad"})
	require.ErrorIs(s.T(), err, service.ErrCaptchaInvalidSecretKey)
}

func (s *CaptchaVerifierSuite) TestValidateProviderConfig_TurnstileGoodSecret() {
	s.setupTransport(service.CaptchaProviderTurnstile, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.CaptchaVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-response"}, // token 错而非 secret 错
		})
	}))

	err := s.verifier.ValidateProviderConfig(s.ctx, service.CaptchaProviderTurnstile, map[string]string{"secret_key": "ok"})
	require.NoError(s.T(), err, "non-secret error codes should be treated as 'secret key is fine'")
}

func (s *CaptchaVerifierSuite) TestValidateProviderConfig_TencentBypassesNetwork() {
	// 故意把 URL 设成会失败的：如果实现错误地发起 HTTP 请求会暴露。
	s.verifier.verifyURLs[service.CaptchaProviderTencent] = "http://127.0.0.1:1/should-not-call"

	err := s.verifier.ValidateProviderConfig(s.ctx, service.CaptchaProviderTencent, map[string]string{
		"captcha_app_id": "200000000",
		"app_secret_key": "x",
		"secret_id":      "y",
		"secret_key":     "z",
	})
	require.NoError(s.T(), err, "tencent_captcha should always succeed without network call")
}

func (s *CaptchaVerifierSuite) TestValidateProviderConfig_UnknownProviderRejected() {
	err := s.verifier.ValidateProviderConfig(s.ctx, "geetest", map[string]string{"secret_key": "x"})
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "unsupported")
}

func TestCaptchaVerifierSuite(t *testing.T) {
	suite.Run(t, new(CaptchaVerifierSuite))
}
