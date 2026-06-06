import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { XAITokenInfo } from '@/api/admin/xai'

export function useXAIOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

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
    redirectUri?: string
  ): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId
      if (redirectUri?.trim()) payload.redirect_uri = redirectUri.trim()

      const response = await adminAPI.xai.generateAuthUrl(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.xai.failedToGenerateUrl', 'Failed to generate xAI auth URL')
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
  }): Promise<XAITokenInfo | null> => {
    const code = params.code?.trim()
    if (!code || !params.sessionId || !params.state) {
      error.value = t('admin.accounts.oauth.xai.missingExchangeParams', 'Missing xAI OAuth exchange parameters')
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
      if (params.proxyId) payload.proxy_id = params.proxyId

      return await adminAPI.xai.exchangeCode(payload as any)
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.xai.failedToExchangeCode', 'Failed to exchange xAI authorization code')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<XAITokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.xai.pleaseEnterRefreshToken', 'Please enter xAI refresh token')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      return await adminAPI.xai.refreshXAIToken(refreshToken.trim(), proxyId)
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.xai.failedToValidateRT', 'Failed to validate xAI refresh token')
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: XAITokenInfo): Record<string, unknown> => {
    let expiresAt: string | undefined
    if (typeof tokenInfo.expires_at === 'number' && Number.isFinite(tokenInfo.expires_at)) {
      expiresAt = Math.floor(tokenInfo.expires_at).toString()
    } else if (typeof tokenInfo.expires_at === 'string' && tokenInfo.expires_at.trim()) {
      expiresAt = tokenInfo.expires_at.trim()
    }

    return {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token,
      id_token: tokenInfo.id_token,
      token_type: tokenInfo.token_type,
      expires_at: expiresAt,
      expires_in: tokenInfo.expires_in,
      email: tokenInfo.email,
      sub: tokenInfo.sub,
      base_url: tokenInfo.base_url || 'https://api.x.ai/v1',
      redirect_uri: tokenInfo.redirect_uri,
      token_endpoint: tokenInfo.token_endpoint,
      auth_kind: tokenInfo.auth_kind || 'oauth',
      type: tokenInfo.type || 'xai'
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
    validateRefreshToken,
    buildCredentials
  }
}
