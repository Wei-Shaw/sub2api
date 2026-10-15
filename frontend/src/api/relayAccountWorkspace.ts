import { apiClient } from './client'
import type { AccountWorkspaceAPI, AccountWorkspace } from '@/composables/useAccountWorkspace'
import type { Account } from '@/types'
import type { GeminiTokenInfo, GeminiOAuthCapabilities, GeminiAuthUrlResponse } from './admin/gemini'
import type { AntigravityTokenInfo, AntigravityAuthUrlResponse } from './admin/antigravity'
import type { GrokTokenInfo, GrokAuthUrlResponse, GrokOAuthCapabilities, GrokQuotaProbeResult, GrokQuotaResetResult, GrokSSOToOAuthResponse } from './admin/grok'
import { getGrokSSOImportTimeout } from './admin/grok'

type AccountResult<K extends keyof AccountWorkspaceAPI['accounts']> = Awaited<ReturnType<AccountWorkspaceAPI['accounts'][K]>>
const get = async <T>(path: string, params?: Record<string, unknown>) => (await apiClient.get<T>(path, { params })).data
const post = async <T>(path: string, body?: unknown, timeout?: number) => (await apiClient.post<T>(path, body, { timeout })).data
const put = async <T>(path: string, body: unknown) => (await apiClient.put<T>(path, body)).data

// OAuth composables supply Admin paths; only the scope changes, not the flow.
function relayEndpoint(endpoint: string): string {
  if (!endpoint.startsWith('/admin/accounts/') && !endpoint.startsWith('/admin/openai/')) {
    throw new Error('Unsupported account authorization endpoint')
  }
  return endpoint.replace(/^\/admin\//, '/relay/')
}

export const relayAccountWorkspace: AccountWorkspace = {
  isRelay: true,
  basePath: '/relay',
  api: {
    accounts: {
      create: input => post<Account>('/relay/accounts', input),
      update: (id, input) => put<Account>(`/relay/accounts/${id}`, input),
      generateAuthUrl: (endpoint, config) => post<AccountResult<'generateAuthUrl'>>(relayEndpoint(endpoint), config),
      exchangeCode: (endpoint, input) => post<AccountResult<'exchangeCode'>>(relayEndpoint(endpoint), input),
      refreshOpenAIToken: (refreshToken, _proxyId, endpoint = '/admin/openai/refresh-token', clientId) =>
        post<AccountResult<'refreshOpenAIToken'>>(relayEndpoint(endpoint), { refresh_token: refreshToken, client_id: clientId }),
      syncUpstreamModels: id => post<AccountResult<'syncUpstreamModels'>>(`/relay/accounts/${id}/models/sync-upstream`),
      syncUpstreamModelsPreview: input => post<AccountResult<'syncUpstreamModelsPreview'>>('/relay/accounts/models/sync-upstream-preview', input),
      getAntigravityDefaultModelMapping: () => get<Record<string, string>>('/relay/accounts/antigravity/default-model-mapping'),
      applyOAuthCredentials: (id, input) => post<Account>(`/relay/accounts/${id}/apply-oauth-credentials`, input),
      clearError: id => post<Account>(`/relay/accounts/${id}/clear-error`),
      getAvailableModels: id => get<AccountResult<'getAvailableModels'>>(`/relay/accounts/${id}/models`),
      getStats: (id, days = 30) => get<AccountResult<'getStats'>>(`/relay/accounts/${id}/stats`, { days }),
      getUsage: (id, source, force) => get<AccountResult<'getUsage'>>(`/relay/accounts/${id}/usage`, { source, force }),
      refreshCredentials: id => post<AccountResult<'refreshCredentials'>>(`/relay/accounts/${id}/refresh`),
      getOpenCodeGoUsage: id => get<AccountResult<'getOpenCodeGoUsage'>>(`/relay/accounts/${id}/opencode-go-usage`),
      setOpenCodeGoUsageAutoRefresh: (id, enabled) => put<AccountResult<'setOpenCodeGoUsageAutoRefresh'>>(`/relay/accounts/${id}/opencode-go-usage/auto-refresh`, { enabled }),
      refreshOpenCodeGoUsage: id => post<AccountResult<'refreshOpenCodeGoUsage'>>(`/relay/accounts/${id}/opencode-go-usage/refresh`),
      getGrokMediaEligibility: id => get<AccountResult<'getGrokMediaEligibility'>>(`/relay/accounts/${id}/grok-media-eligibility`),
      updateGrokMediaEligibility: (id, mode) => put<AccountResult<'updateGrokMediaEligibility'>>(`/relay/accounts/${id}/grok-media-eligibility`, { mode }),
      checkMixedChannelRisk: input => post<AccountResult<'checkMixedChannelRisk'>>('/relay/accounts/check-mixed-channel', input),
      probeUpstreamBilling: id => post<AccountResult<'probeUpstreamBilling'>>(`/relay/accounts/${id}/upstream-billing-probe`),
      importCodexSession: input => post<AccountResult<'importCodexSession'>>('/relay/accounts/import/codex-session', input, 120000),
      createOpenAICodexPAT: input => post<Account>('/relay/openai/create-from-codex-pat', input),
    },
    gemini: {
      generateAuthUrl: input => post<GeminiAuthUrlResponse>('/relay/gemini/oauth/auth-url', input),
      exchangeCode: input => post<GeminiTokenInfo>('/relay/gemini/oauth/exchange-code', input),
      getCapabilities: () => get<GeminiOAuthCapabilities>('/relay/gemini/oauth/capabilities'),
    },
    antigravity: {
      generateAuthUrl: input => post<AntigravityAuthUrlResponse>('/relay/antigravity/oauth/auth-url', input),
      exchangeCode: input => post<AntigravityTokenInfo>('/relay/antigravity/oauth/exchange-code', input),
      refreshAntigravityToken: refreshToken => post<AntigravityTokenInfo>('/relay/antigravity/oauth/refresh-token', { refresh_token: refreshToken }),
    },
    grok: {
      generateAuthUrl: input => post<GrokAuthUrlResponse>('/relay/grok/oauth/auth-url', input),
      exchangeCode: input => post<GrokTokenInfo>('/relay/grok/oauth/exchange-code', input),
      getCapabilities: () => get<GrokOAuthCapabilities>('/relay/grok/oauth/capabilities'),
      refreshGrokToken: refreshToken => post<GrokTokenInfo>('/relay/grok/oauth/refresh-token', { refresh_token: refreshToken }),
      validateSSOToken: ssoToken => post<GrokTokenInfo>('/relay/grok/oauth/sso-token', { sso_token: ssoToken }, 120000),
      authorizePassword: emailAndPassword => {
        const separator = emailAndPassword.indexOf('----')
        return post<GrokTokenInfo>('/relay/grok/oauth/password', {
          email: (separator < 0 ? emailAndPassword : emailAndPassword.slice(0, separator)).trim(),
          password: separator < 0 ? '' : emailAndPassword.slice(separator + 4),
        }, 120000)
      },
      createFromSSO: input => post<GrokSSOToOAuthResponse>('/relay/grok/sso-to-oauth', input, getGrokSSOImportTimeout(input.sso_tokens.length)),
      queryQuota: id => get<GrokQuotaProbeResult>(`/relay/grok/accounts/${id}/quota`),
      resetQuota: id => post<GrokQuotaResetResult>(`/relay/grok/accounts/${id}/reset-quota`),
    },
  },
}
