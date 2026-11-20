package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
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
	response, err := s.doOpenAIAccountTestUpstreamOnce(request, proxyURL, account, useTLSFallback)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		return response, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	_ = response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || !recoverableOpenAI401(body) || s.openaiGatewayService == nil ||
		s.openaiGatewayService.openAITokenProvider == nil || request.GetBody == nil {
		return response, nil
	}
	owner := account
	if account.IsCredentialShadow() {
		owner, err = resolveCredentialAccount(request.Context(), s.accountRepo, account)
		if err != nil || owner == nil || !sameProxy(account, owner) {
			return response, nil
		}
	}
	rejected := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	fresh, refreshErr := s.openaiGatewayService.openAITokenProvider.refreshRejectedAccessToken(request.Context(), owner, rejected, true)
	if refreshErr != nil {
		if refreshErr.Error() == openAIReauthorizationRequired {
			_ = response.Body.Close()
			return nil, refreshErr
		}
		return response, nil
	}
	retry := request.Clone(request.Context())
	retry.Body, err = request.GetBody()
	if err != nil {
		return response, nil
	}
	retry.Header.Set("Authorization", "Bearer "+fresh.GetOpenAIAccessToken())
	setOpenAIChatGPTAccountHeaders(retry.Header, fresh)
	_ = response.Body.Close()
	// Reuse the exact payload and transport. No generation has been accepted;
	// a second 401 returns to the ordinary probe error path without recursion.
	retried, err := s.doOpenAIAccountTestUpstreamOnce(retry, proxyURL, account, useTLSFallback)
	if err == nil && retried.StatusCode == http.StatusUnauthorized {
		body, readErr := io.ReadAll(io.LimitReader(retried.Body, 2<<20))
		_ = retried.Body.Close()
		retried.Body = io.NopCloser(bytes.NewReader(body))
		if readErr == nil {
			s.handleOpenAIAccountTest401(request.Context(), fresh, body)
		}
	}
	return retried, err
}

func (s *AccountTestService) doOpenAIAccountTestUpstreamOnce(
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

func (s *AccountTestService) handleOpenAIAccountTest401(ctx context.Context, account *Account, body []byte) {
	if s.accountRepo == nil {
		return
	}
	owner := account
	if account.IsCredentialShadow() {
		var err error
		owner, err = resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil || owner == nil {
			return
		}
	}
	if owner.IsOpenAIOAuth() {
		if recoverableOpenAI401(body) && strings.TrimSpace(owner.GetOpenAIRefreshToken()) != "" &&
			!owner.IsOpenAIPersonalAccessToken() && !owner.IsOpenAIAgentIdentity() {
			// Keep active accounts eligible for background refresh and the next
			// scheduled probe. Never clear an existing error on a failed probe.
			return
		}
		if repo, ok := s.accountRepo.(OpenAIOAuthRecoveryRepository); ok {
			_, _ = repo.SetOpenAIOAuthRecoveryStateIfUnchanged(ctx, owner, openAIReauthorizationRequired)
			return
		}
	}
	_ = s.accountRepo.SetError(ctx, owner.ID, "Authentication failed (401): "+string(body))
}
