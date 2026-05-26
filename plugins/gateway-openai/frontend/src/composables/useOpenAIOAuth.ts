/**
 * OpenAI OAuth composable — plugin version.
 * Replaces host @/composables/useOpenAIOAuth with direct axios calls.
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import { getClient } from '../api/client'
import { getSdk } from '../api/sdk'

export interface OpenAITokenInfo {
  access_token?: string
  refresh_token?: string
  client_id?: string
  id_token?: string
  token_type?: string
  expires_in?: number
  expires_at?: number
  scope?: string
  email?: string
  name?: string
  plan_type?: string
  privacy_mode?: string
  chatgpt_account_id?: string
  chatgpt_user_id?: string
  organization_id?: string
  [key: string]: unknown
}

export function useOpenAIOAuth() {
  const { t } = useI18n()
  const endpointPrefix = '/admin/openai'

  const authUrl = ref('')
  const sessionId = ref('')
  const oauthState = ref('')
  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    oauthState.value = ''
    loading.value = false
    error.value = ''
  }

  const generateAuthUrl = async (
    proxyId?: number | null,
    redirectUri?: string,
  ): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    oauthState.value = ''
    error.value = ''
    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId
      if (redirectUri) payload.redirect_uri = redirectUri
      const { data } = await getClient().post<{
        auth_url: string; session_id: string
      }>(`${endpointPrefix}/generate-auth-url`, payload)
      authUrl.value = data.auth_url
      sessionId.value = data.session_id
      try {
        const parsed = new URL(data.auth_url)
        oauthState.value = parsed.searchParams.get('state') || ''
      } catch { oauthState.value = '' }
      return true
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.openai.failedToGenerateUrl'))
      getSdk().notify.error(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  const exchangeAuthCode = async (
    code: string,
    currentSessionId: string,
    state: string,
    _proxyId?: number | null,
  ): Promise<OpenAITokenInfo | null> => {
    if (!code.trim() || !currentSessionId || !state.trim()) {
      error.value = 'Missing auth code, session ID, or state'
      return null
    }
    loading.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {
        session_id: currentSessionId,
        code: code.trim(),
        state: state.trim(),
      }
      const { data } = await getClient().post<OpenAITokenInfo>(
        `${endpointPrefix}/exchange-code`, payload,
      )
      return data
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.openai.failedToExchangeCode'))
      getSdk().notify.error(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null,
    clientId?: string,
  ): Promise<OpenAITokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = 'Missing refresh token'
      return null
    }
    loading.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {
        refresh_token: refreshToken.trim(),
      }
      if (proxyId) payload.proxy_id = proxyId
      if (clientId) payload.client_id = clientId
      const { data } = await getClient().post<OpenAITokenInfo>(
        `${endpointPrefix}/refresh-token`, payload,
      )
      return data
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.openai.failedToValidateRT'))
      getSdk().notify.error(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: OpenAITokenInfo): Record<string, unknown> => {
    const creds: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      expires_at: tokenInfo.expires_at,
    }
    if (tokenInfo.refresh_token) creds.refresh_token = tokenInfo.refresh_token
    if (tokenInfo.id_token) creds.id_token = tokenInfo.id_token
    if (tokenInfo.email) creds.email = tokenInfo.email
    if (tokenInfo.chatgpt_account_id) creds.chatgpt_account_id = tokenInfo.chatgpt_account_id
    if (tokenInfo.chatgpt_user_id) creds.chatgpt_user_id = tokenInfo.chatgpt_user_id
    if (tokenInfo.organization_id) creds.organization_id = tokenInfo.organization_id
    if (tokenInfo.plan_type) creds.plan_type = tokenInfo.plan_type
    if (tokenInfo.client_id) creds.client_id = tokenInfo.client_id
    return creds
  }

  const buildExtraInfo = (
    tokenInfo: OpenAITokenInfo,
  ): Record<string, string> | undefined => {
    const extra: Record<string, string> = {}
    if (tokenInfo.email) extra.email = tokenInfo.email
    if (tokenInfo.name) extra.name = tokenInfo.name
    if (tokenInfo.privacy_mode) extra.privacy_mode = tokenInfo.privacy_mode
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  return {
    authUrl, sessionId, oauthState, loading, error,
    resetState, generateAuthUrl, exchangeAuthCode,
    validateRefreshToken, buildCredentials, buildExtraInfo,
  }
}
