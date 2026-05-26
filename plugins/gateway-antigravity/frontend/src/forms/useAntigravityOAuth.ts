/**
 * Antigravity OAuth composable -- plugin-local version.
 *
 * Replaces host's useAntigravityOAuth by calling the plugin's own
 * API module (which uses the host axios instance via SDK).
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  generateAuthUrl,
  exchangeCode as apiExchangeCode,
  refreshAntigravityToken,
  type AntigravityTokenInfo,
} from '../api/antigravity'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'

export function useAntigravityOAuth() {
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const state = ref('')
  const loading = ref(false)
  const error = ref('')

  function resetState() {
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    loading.value = false
    error.value = ''
  }

  async function generateOAuthUrl(proxyId: number | null | undefined): Promise<boolean> {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''
    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId
      const response = await generateAuthUrl(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(
        err, t('admin.accounts.oauth.antigravity.failedToGenerateUrl')
      )
      return false
    } finally {
      loading.value = false
    }
  }

  async function exchangeAuthCode(params: {
    code: string
    sessionId: string
    state: string
    proxyId?: number | null
  }): Promise<AntigravityTokenInfo | null> {
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
        code,
      }
      if (params.proxyId) payload.proxy_id = params.proxyId
      return await apiExchangeCode(payload)
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(
        err, t('admin.accounts.oauth.antigravity.failedToExchangeCode')
      )
      return null
    } finally {
      loading.value = false
    }
  }

  async function validateRefreshToken(
    refreshToken: string,
    proxyId?: number | null
  ): Promise<AntigravityTokenInfo | null> {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.antigravity.pleaseEnterRefreshToken')
      return null
    }
    loading.value = true
    error.value = ''
    try {
      return await refreshAntigravityToken(refreshToken.trim(), proxyId)
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(
        err, t('admin.accounts.oauth.antigravity.failedToValidateRT')
      )
      return null
    } finally {
      loading.value = false
    }
  }

  function buildCredentials(tokenInfo: AntigravityTokenInfo): Record<string, unknown> {
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
      project_id: tokenInfo.project_id,
      email: tokenInfo.email,
    }
  }

  return {
    authUrl, sessionId, state, loading, error,
    resetState, generateAuthUrl: generateOAuthUrl,
    exchangeAuthCode, validateRefreshToken, buildCredentials,
  }
}
