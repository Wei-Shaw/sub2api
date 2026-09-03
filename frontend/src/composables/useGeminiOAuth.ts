import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { apiClient } from '@/api/client'
import type { GeminiOAuthCapabilities } from '@/api/admin/gemini'
import {
  type OAuthComposableOptions,
  type OAuthSurface,
  effectiveProxyId,
  oauthPrefix
} from '@/utils/oauthSurface'

export interface GeminiTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  scope?: string
  expires_at?: number | string
  project_id?: string
  oauth_type?: string
  tier_id?: string
  extra?: Record<string, unknown>
  [key: string]: unknown
}

export function useGeminiOAuth(opts?: OAuthComposableOptions) {
  const appStore = useAppStore()
  const { t } = useI18n()
  const surface: OAuthSurface = opts?.surface ?? 'admin'
  const prefix = oauthPrefix(surface, 'gemini')

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

  const generateAuthUrl = async (
    proxyId: number | null | undefined,
    projectId?: string | null,
    oauthType?: string,
    tierId?: string
  ): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {}
      const effectiveProxy = effectiveProxyId(surface, proxyId)
      if (effectiveProxy) payload.proxy_id = effectiveProxy
      const trimmedProjectID = projectId?.trim()
      if (trimmedProjectID) payload.project_id = trimmedProjectID
      if (oauthType) payload.oauth_type = oauthType
      const trimmedTierID = tierId?.trim()
      if (trimmedTierID) payload.tier_id = trimmedTierID

      let response: { auth_url: string; session_id: string; state: string }
      if (surface === 'user') {
        const { data } = await apiClient.post<{
          auth_url: string
          session_id: string
          state: string
        }>(`${prefix}/auth-url`, payload)
        response = data
      } else {
        response = await adminAPI.gemini.generateAuthUrl(payload as any)
      }
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.gemini.failedToGenerateUrl')
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
    oauthType?: string
    tierId?: string
  }): Promise<GeminiTokenInfo | null> => {
    const code = params.code?.trim()
    if (!code || !params.sessionId || !params.state) {
      error.value = t('admin.accounts.oauth.gemini.missingExchangeParams')
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
      if (params.oauthType) payload.oauth_type = params.oauthType
      const trimmedTierID = params.tierId?.trim()
      if (trimmedTierID) payload.tier_id = trimmedTierID

      if (surface === 'user') {
        const { data } = await apiClient.post<GeminiTokenInfo>(`${prefix}/exchange-code`, payload)
        return data
      }
      const tokenInfo = await adminAPI.gemini.exchangeCode(payload as any)
      return tokenInfo as GeminiTokenInfo
    } catch (err: any) {
      // Check for specific missing project_id error
      const errorMessage = err.message || err.response?.data?.message || ''
      if (errorMessage.includes('missing project_id')) {
        error.value = t('admin.accounts.oauth.gemini.missingProjectId')
      } else {
        error.value = errorMessage || t('admin.accounts.oauth.gemini.failedToExchangeCode')
      }
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: GeminiTokenInfo): Record<string, unknown> => {
    let expiresAt: string | undefined
    if (typeof tokenInfo.expires_at === 'number' && Number.isFinite(tokenInfo.expires_at)) {
      expiresAt = Math.floor(tokenInfo.expires_at).toString()
    } else if (typeof tokenInfo.expires_at === 'string' && tokenInfo.expires_at.trim()) {
      expiresAt = tokenInfo.expires_at.trim()
    }

    return {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token,
      token_type: tokenInfo.token_type,
      expires_at: expiresAt,
      scope: tokenInfo.scope,
      project_id: tokenInfo.project_id,
      oauth_type: tokenInfo.oauth_type,
      tier_id: tokenInfo.tier_id
    }
  }

  const buildExtraInfo = (tokenInfo: GeminiTokenInfo): Record<string, unknown> | undefined => {
    if (!tokenInfo.extra || typeof tokenInfo.extra !== 'object') return undefined
    return tokenInfo.extra
  }

  const getCapabilities = async (): Promise<GeminiOAuthCapabilities | null> => {
    try {
      if (surface === 'user') {
        const { data } = await apiClient.get<GeminiOAuthCapabilities>(`${prefix}/capabilities`)
        return data
      }
      return await adminAPI.gemini.getCapabilities()
    } catch (err: any) {
      // Capabilities are optional for older servers; don't block the UI.
      return null
    }
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
    buildCredentials,
    buildExtraInfo,
    getCapabilities
  }
}
