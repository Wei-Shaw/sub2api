package service

import (
	"context"
	"net/http"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (response *http.Response, err error) {
	// Capture quota at header arrival, before a long response body is consumed.
	// Parsing remaining_seconds after streaming shifts the inferred reset time.
	defer func() {
		if err == nil && response != nil && account.UsesOpenAICodexProtocol() && (account.Platform == PlatformOpenAI || account.Platform == "") && !account.IsShadow() {
			if snapshot := ParseCodexRateLimitHeaders(response.Header); snapshot != nil {
				s.updateCodexUsageSnapshot(request.Context(), account.ID, snapshot)
				if s.accountRepo != nil {
					responseRequest := response.Request
					if responseRequest == nil {
						responseRequest = request
					}
					response.Request = responseRequest.WithContext(context.WithValue(responseRequest.Context(), capturedCodexObservationKey{}, &capturedCodexObservation{accountID: account.ID, snapshot: snapshot}))
				}
			}
		}
	}()
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}

// doOpenAIAccountTestUpstream 让 OpenAI OAuth 账号测试与真实转发使用同一插件路径。
// API Key 和未命中插件的账号保持各自原有的 HTTPUpstream 行为。
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback {
		return s.httpUpstream.DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}
