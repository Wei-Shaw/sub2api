package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// SetTLSFingerprintProfileService 注入账号级 TLS 指纹解析服务，供 doOpenAIUpstream 在
// 决定出站 TLS Profile 时优先尊重账号已显式配置的选择（见 resolveOpenAICodexTLSProfile）。
func (s *OpenAIGatewayService) SetTLSFingerprintProfileService(svc *TLSFingerprintProfileService) {
	s.tlsFPProfileService = svc
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
//
// 未被插件接管时统一走 DoWithTLS：resolveOpenAICodexTLSProfile 决定的 profile 为 nil 时，
// DoWithTLS 退化为普通 Do 行为（httpUpstreamService 既有保证），因此这里不需要再区分
// Do/DoWithTLS 两条调用路径。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	var explicitProfile *tlsfingerprint.Profile
	if s.tlsFPProfileService != nil {
		explicitProfile = s.tlsFPProfileService.ResolveTLSProfile(account)
	}
	return s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency,
		resolveOpenAICodexTLSProfile(explicitProfile, account))
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
