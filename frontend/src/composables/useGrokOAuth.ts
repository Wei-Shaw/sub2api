import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { apiClient } from '@/api/client'
import type { GrokTokenInfo } from '@/api/admin/grok'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import {
  type OAuthComposableOptions,
  type OAuthSurface,
  effectiveProxyId,
  oauthPrefix
} from '@/utils/oauthSurface'

export function useGrokOAuth(opts?: OAuthComposableOptions) {
  const appStore = useAppStore()
  const { t } = useI18n()
  const surface: OAuthSurface = opts?.surface ?? 'admin'
  const prefix = oauthPrefix(surface, 'grok')

  const authUrl = ref('')
  const sessionId = ref('')
  const state = ref('')
  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    loading.value = false
    error.value = ''
  }

  const generateAuthUrl = async (proxyId: number | null | undefined): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {}
      const effectiveProxy = effectiveProxyId(surface, proxyId)
      if (effectiveProxy) payload.proxy_id = effectiveProxy

      let response: { auth_url: string; session_id: string; state: string }
      if (surface === 'user') {
        const { data } = await apiClient.post<{
          auth_url: string
          session_id: string
          state: string
        }>(`${prefix}/auth-url`, payload)
        response = data
      } else {
        response = await adminAPI.grok.generateAuthUrl(payload)
      }
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.grok.failedToGenerateUrl'))
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  const exchangeAuthCode = async (params: {
    code: string
    sessionId: string
    state: string
    proxyId?: number | null
  }): Promise<GrokTokenInfo | null> => {
    const code = params.code?.trim()
    if (!code || !params.sessionId || !params.state) {
      error.value = t('admin.accounts.oauth.grok.missingExchangeParams')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const payload: Record<string, unknown> = {
        session_id: params.sessionId,
        state: params.state,
        code
      }
      const effectiveProxy = effectiveProxyId(surface, params.proxyId)
      if (effectiveProxy) payload.proxy_id = effectiveProxy

      if (surface === 'user') {
        const { data } = await apiClient.post<GrokTokenInfo>(`${prefix}/exchange-code`, payload)
        return data
      }
      return await adminAPI.grok.exchangeCode(payload as any)
    } catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.grok.errors',
        t('admin.accounts.oauth.grok.failedToExchangeCode')
      )
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<GrokTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.grok.pleaseEnterRefreshToken')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const effectiveProxy = effectiveProxyId(surface, proxyId)
      if (surface === 'user') {
        const { data } = await apiClient.post<GrokTokenInfo>(`${prefix}/refresh-token`, {
          refresh_token: refreshToken.trim()
        })
        return data
      }
      return await adminAPI.grok.refreshGrokToken(refreshToken.trim(), effectiveProxy)
    } catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.grok.errors',
        t('admin.accounts.oauth.grok.failedToValidateRT')
      )
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: GrokTokenInfo): Record<string, unknown> => {
    const credentials: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      token_type: tokenInfo.token_type,
      expires_at: tokenInfo.expires_at,
      client_id: tokenInfo.client_id,
      scope: tokenInfo.scope,
      email: tokenInfo.email,
      sub: tokenInfo.sub,
      team_id: tokenInfo.team_id,
      subscription_tier: tokenInfo.subscription_tier,
      entitlement_status: tokenInfo.entitlement_status,
      base_url: 'https://cli-chat-proxy.grok.com/v1'
    }
    if (tokenInfo.refresh_token) credentials.refresh_token = tokenInfo.refresh_token
    if (tokenInfo.id_token) credentials.id_token = tokenInfo.id_token
    return Object.fromEntries(Object.entries(credentials).filter(([, value]) => value !== undefined && value !== ''))
  }

  const buildExtraInfo = (tokenInfo: GrokTokenInfo): Record<string, unknown> => {
    const extra: Record<string, unknown> = {}
    if (tokenInfo.email) extra.email = tokenInfo.email
    if (tokenInfo.subscription_tier) extra.subscription_tier = tokenInfo.subscription_tier
    if (tokenInfo.entitlement_status) extra.entitlement_status = tokenInfo.entitlement_status
    return extra
  }

  return {
    authUrl,
    sessionId,
    state,
    loading,
    error,
    resetState,
    generateAuthUrl,
    exchangeAuthCode,
    validateRefreshToken,
    buildCredentials,
    buildExtraInfo
  }
}
