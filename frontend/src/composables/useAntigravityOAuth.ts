import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { apiClient } from '@/api/client'
import type { AntigravityTokenInfo } from '@/api/admin/antigravity'
import {
  type OAuthComposableOptions,
  type OAuthSurface,
  effectiveProxyId,
  oauthPrefix
} from '@/utils/oauthSurface'

export function useAntigravityOAuth(opts?: OAuthComposableOptions) {
  const appStore = useAppStore()
  const { t } = useI18n()
  const surface: OAuthSurface = opts?.surface ?? 'admin'
  const prefix = oauthPrefix(surface, 'antigravity')

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
        response = await adminAPI.antigravity.generateAuthUrl(payload as any)
      }
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value =
        err.response?.data?.detail || t('admin.accounts.oauth.antigravity.failedToGenerateUrl')
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
  }): Promise<AntigravityTokenInfo | null> => {
    const code = params.code?.trim()
    if (!code || !params.sessionId || !params.state) {
      error.value = t('admin.accounts.oauth.antigravity.missingExchangeParams')
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
        const { data } = await apiClient.post<AntigravityTokenInfo>(
          `${prefix}/exchange-code`,
          payload
        )
        return data
      }
      const tokenInfo = await adminAPI.antigravity.exchangeCode(payload as any)
      return tokenInfo as AntigravityTokenInfo
    } catch (err: any) {
      error.value =
        err.response?.data?.detail || t('admin.accounts.oauth.antigravity.failedToExchangeCode')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<AntigravityTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.antigravity.pleaseEnterRefreshToken')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const effectiveProxy = effectiveProxyId(surface, proxyId)
      if (surface === 'user') {
        const { data } = await apiClient.post<AntigravityTokenInfo>(`${prefix}/refresh-token`, {
          refresh_token: refreshToken.trim()
        })
        return data
      }
      const tokenInfo = await adminAPI.antigravity.refreshAntigravityToken(
        refreshToken.trim(),
        effectiveProxy
      )
      return tokenInfo as AntigravityTokenInfo
    } catch (err: any) {
      error.value =
        err.response?.data?.detail || t('admin.accounts.oauth.antigravity.failedToValidateRT')
      // Don't show global error toast for batch validation to avoid spamming
      // appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (
    tokenInfo: AntigravityTokenInfo,
    fallbackRefreshToken?: string
  ): Record<string, unknown> => {
    let expiresAt: string | undefined
    if (typeof tokenInfo.expires_at === 'number' && Number.isFinite(tokenInfo.expires_at)) {
      expiresAt = Math.floor(tokenInfo.expires_at).toString()
    } else if (typeof tokenInfo.expires_at === 'string' && tokenInfo.expires_at.trim()) {
      expiresAt = tokenInfo.expires_at.trim()
    }
    const refreshToken = tokenInfo.refresh_token?.trim()
      ? tokenInfo.refresh_token
      : fallbackRefreshToken

    const creds: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      refresh_token: refreshToken,
      token_type: tokenInfo.token_type,
      expires_at: expiresAt,
      project_id: tokenInfo.project_id,
      email: tokenInfo.email
    }
    const planType =
      typeof tokenInfo.plan_type === 'string' ? tokenInfo.plan_type.trim() : ''
    if (planType) creds.plan_type = planType
    const tierId = typeof tokenInfo.tier_id === 'string' ? tokenInfo.tier_id.trim() : ''
    if (tierId) creds.tier_id = tierId
    return creds
  }

  /** 列表 Private/Pro 角标依赖的 extra 字段 */
  const buildExtraInfo = (tokenInfo: AntigravityTokenInfo): Record<string, unknown> | undefined => {
    const extra: Record<string, unknown> = {}
    const privacy =
      typeof tokenInfo.privacy_mode === 'string' ? tokenInfo.privacy_mode.trim() : ''
    if (privacy) extra.privacy_mode = privacy
    const tierId = typeof tokenInfo.tier_id === 'string' ? tokenInfo.tier_id.trim() : ''
    if (tierId) {
      extra.load_code_assist = {
        currentTier: { id: tierId },
        paidTier: { id: tierId }
      }
    }
    return Object.keys(extra).length > 0 ? extra : undefined
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
