import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { CodeBuddyTokenInfo } from '@/api/admin/codebuddy'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'

export function useCodeBuddyOAuth() {
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

  const generateAuthUrl = async (proxyId: number | null | undefined): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId

      const response = await adminAPI.codebuddy.generateAuthUrl(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.codebuddy.failedToGenerateUrl'))
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  // CodeBuddy 用 state 换 token（无需 authorization code）。
  const exchangeState = async (params: {
    state: string
    sessionId?: string
    proxyId?: number | null
  }): Promise<CodeBuddyTokenInfo | null> => {
    const stateValue = params.state?.trim()
    if (!stateValue) {
      error.value = t('admin.accounts.oauth.codebuddy.missingExchangeParams')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const payload: Record<string, unknown> = {
        state: stateValue
      }
      if (params.sessionId) payload.session_id = params.sessionId
      if (params.proxyId) payload.proxy_id = params.proxyId

      return await adminAPI.codebuddy.exchangeState(payload as any)
    } catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.codebuddy.errors',
        t('admin.accounts.oauth.codebuddy.failedToExchangeState')
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
  ): Promise<CodeBuddyTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.codebuddy.pleaseEnterRefreshToken')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      return await adminAPI.codebuddy.refreshCodeBuddyToken(refreshToken.trim(), proxyId)
    } catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.codebuddy.errors',
        t('admin.accounts.oauth.codebuddy.failedToValidateRT')
      )
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: CodeBuddyTokenInfo): Record<string, unknown> => {
    const credentials: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      token_type: tokenInfo.token_type || 'Bearer',
      expires_at: tokenInfo.expires_at,
      domain: tokenInfo.domain,
      scope: tokenInfo.scope,
      uid: tokenInfo.uid,
      nickname: tokenInfo.nickname
    }
    if (tokenInfo.refresh_token) credentials.refresh_token = tokenInfo.refresh_token
    if (tokenInfo.enabled_models) credentials.models = tokenInfo.enabled_models
    return Object.fromEntries(
      Object.entries(credentials).filter(([, value]) => value !== undefined && value !== '')
    )
  }

  return {
    authUrl,
    sessionId,
    state,
    loading,
    error,
    resetState,
    generateAuthUrl,
    exchangeState,
    validateRefreshToken,
    buildCredentials
  }
}
