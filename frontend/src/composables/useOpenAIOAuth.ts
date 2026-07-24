import { adminAPI } from '@/api/admin'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { createOAuthComposable, type OAuthAuthUrlResponse } from './createOAuthComposable'

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
  subscription_expires_at?: string
  privacy_mode?: string
  // OpenAI specific IDs (extracted from ID Token)
  chatgpt_account_id?: string
  chatgpt_user_id?: string
  organization_id?: string
  [key: string]: unknown
}

export type OpenAIOAuthPlatform = 'openai'

const I18N_PREFIX = 'admin.accounts.oauth.openai'
const ENDPOINT_PREFIX = '/admin/openai'

export function useOpenAIOAuth() {
  const instance = createOAuthComposable({
    generateAuthUrl: {
      buildPayload: (...args: any[]) => {
        const [proxyId, redirectUri] = args
        const payload: Record<string, unknown> = {}
        if (proxyId) {
          payload.proxy_id = proxyId
        }
        if (redirectUri) {
          payload.redirect_uri = redirectUri
        }
        return payload
      },
      call: (payload) =>
        adminAPI.accounts.generateAuthUrl(
          `${ENDPOINT_PREFIX}/generate-auth-url`,
          payload as any
        ) as Promise<OAuthAuthUrlResponse>,
      extractError: (err, t) =>
        extractApiErrorMessage(err, t(`${I18N_PREFIX}.failedToGenerateUrl`)),
      onSuccess: (response, setState) => {
        try {
          const parsed = new URL(response.auth_url)
          setState(parsed.searchParams.get('state') || '')
        } catch {
          setState('')
        }
      }
    },
    exchangeAuthCode: {
      validate: (_t, ...args: any[]) => {
        const [code, currentSessionId, state] = args
        if (!code.trim() || !currentSessionId || !state.trim()) {
          return 'Missing auth code, session ID, or state'
        }
        return null
      },
      buildPayload: (...args: any[]) => {
        const [code, currentSessionId, state, proxyId] = args
        const payload: { session_id: string; code: string; state: string; proxy_id?: number } = {
          session_id: currentSessionId,
          code: code.trim(),
          state: state.trim()
        }
        if (proxyId) {
          payload.proxy_id = proxyId
        }
        return payload
      },
      call: (payload) =>
        adminAPI.accounts.exchangeCode(`${ENDPOINT_PREFIX}/exchange-code`, payload as any),
      extractError: (err, t) =>
        extractI18nErrorMessage(
          err,
          t,
          `${I18N_PREFIX}.errors`,
          t(`${I18N_PREFIX}.failedToExchangeCode`)
        )
    },
    validateRefreshToken: {
      validate: (_t, ...args: any[]) => {
        const [refreshToken] = args
        if (!refreshToken.trim()) {
          return 'Missing refresh token'
        }
        return null
      },
      call: (...args: any[]) => {
        const [refreshToken, proxyId, clientId] = args
        return adminAPI.accounts.refreshOpenAIToken(
          refreshToken.trim(),
          proxyId,
          `${ENDPOINT_PREFIX}/refresh-token`,
          clientId
        )
      },
      extractError: (err, t) =>
        extractI18nErrorMessage(err, t, `${I18N_PREFIX}.errors`, t(`${I18N_PREFIX}.failedToValidateRT`)),
      showErrorOnFailure: true
    },
    buildCredentials: (tokenInfo: OpenAITokenInfo): Record<string, unknown> => {
      const creds: Record<string, unknown> = {
        access_token: tokenInfo.access_token,
        expires_at: tokenInfo.expires_at
      }

      // 仅在返回了新的 refresh_token 时才写入，防止用空值覆盖已有令牌
      if (tokenInfo.refresh_token) {
        creds.refresh_token = tokenInfo.refresh_token
      }
      if (tokenInfo.id_token) {
        creds.id_token = tokenInfo.id_token
      }
      if (tokenInfo.email) {
        creds.email = tokenInfo.email
      }
      if (tokenInfo.chatgpt_account_id) {
        creds.chatgpt_account_id = tokenInfo.chatgpt_account_id
      }
      if (tokenInfo.chatgpt_user_id) {
        creds.chatgpt_user_id = tokenInfo.chatgpt_user_id
      }
      if (tokenInfo.organization_id) {
        creds.organization_id = tokenInfo.organization_id
      }
      if (tokenInfo.plan_type) {
        creds.plan_type = tokenInfo.plan_type
      }
      if (tokenInfo.subscription_expires_at) {
        creds.subscription_expires_at = tokenInfo.subscription_expires_at
      }
      if (tokenInfo.client_id) {
        creds.client_id = tokenInfo.client_id
      }

      return creds
    },
    buildExtraInfo: (tokenInfo: OpenAITokenInfo): Record<string, string> | undefined => {
      const extra: Record<string, string> = {}
      if (tokenInfo.email) {
        extra.email = tokenInfo.email
      }
      if (tokenInfo.name) {
        extra.name = tokenInfo.name
      }
      if (tokenInfo.privacy_mode) {
        extra.privacy_mode = tokenInfo.privacy_mode
      }
      return Object.keys(extra).length > 0 ? extra : undefined
    }
  })

  // OpenAI exposes the oauth state as `oauthState` (parsed from the auth URL),
  // not `state` like the other providers.
  const { state, ...rest } = instance
  return { ...rest, oauthState: state }
}
