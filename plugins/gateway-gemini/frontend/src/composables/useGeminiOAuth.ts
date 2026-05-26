import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import { getSdk } from '../api/sdk'
import * as geminiApi from '../api/gemini'
import type { GeminiOAuthCapabilities, GeminiTokenInfo } from '../api/gemini'

export type { GeminiTokenInfo }

export function useGeminiOAuth() {
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
      const payload: geminiApi.GeminiAuthUrlRequest = {}
      if (proxyId) payload.proxy_id = proxyId
      const trimmedProjectID = projectId?.trim()
      if (trimmedProjectID) payload.project_id = trimmedProjectID
      if (oauthType) payload.oauth_type = oauthType as geminiApi.GeminiAuthUrlRequest['oauth_type']
      const trimmedTierID = tierId?.trim()
      if (trimmedTierID) payload.tier_id = trimmedTierID

      const response = await geminiApi.generateAuthUrl(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.gemini.failedToGenerateUrl'))
      getSdk().notify.error(error.value)
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
      const payload: geminiApi.GeminiExchangeCodeRequest = {
        session_id: params.sessionId,
        state: params.state,
        code
      }
      if (params.proxyId) payload.proxy_id = params.proxyId
      if (params.oauthType) payload.oauth_type = params.oauthType as geminiApi.GeminiExchangeCodeRequest['oauth_type']
      const trimmedTierID = params.tierId?.trim()
      if (trimmedTierID) payload.tier_id = trimmedTierID

      return await geminiApi.exchangeCode(payload)
    } catch (err: unknown) {
      const errorMessage = extractApiErrorMessage(err, '')
      if (errorMessage.includes('missing project_id')) {
        error.value = t('admin.accounts.oauth.gemini.missingProjectId')
      } else {
        error.value = errorMessage || t('admin.accounts.oauth.gemini.failedToExchangeCode')
      }
      getSdk().notify.error(error.value)
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
      return await geminiApi.getCapabilities()
    } catch {
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
