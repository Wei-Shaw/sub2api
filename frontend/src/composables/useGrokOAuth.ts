import { adminAPI } from '@/api/admin'
import type { GrokTokenInfo } from '@/api/admin/grok'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { createOAuthComposable } from './createOAuthComposable'

const I18N_PREFIX = 'admin.accounts.oauth.grok'
const GROK_CLI_BASE_URL = 'https://cli-chat-proxy.grok.com/v1'

export function useGrokOAuth() {
  return createOAuthComposable({
    generateAuthUrl: {
      buildPayload: (...args: any[]) => {
        const [proxyId] = args
        const payload: Record<string, unknown> = {}
        if (proxyId) payload.proxy_id = proxyId
        return payload
      },
      call: (payload) => adminAPI.grok.generateAuthUrl(payload as any),
      extractError: (err, t) =>
        extractApiErrorMessage(err, t(`${I18N_PREFIX}.failedToGenerateUrl`))
    },
    exchangeAuthCode: {
      validate: (t, ...args: any[]) => {
        const [params] = args
        const code = params.code?.trim()
        if (!code || !params.sessionId || !params.state) {
          return t(`${I18N_PREFIX}.missingExchangeParams`)
        }
        return null
      },
      buildPayload: (...args: any[]) => {
        const [params] = args
        const code = params.code?.trim()
        const payload: Record<string, unknown> = {
          session_id: params.sessionId,
          state: params.state,
          code
        }
        if (params.proxyId) payload.proxy_id = params.proxyId
        return payload
      },
      call: (payload) => adminAPI.grok.exchangeCode(payload as any),
      extractError: (err, t) =>
        extractI18nErrorMessage(
          err,
          t,
          `${I18N_PREFIX}.errors`,
          t(`${I18N_PREFIX}.failedToExchangeCode`)
        )
    },
    validateRefreshToken: {
      validate: (t, ...args: any[]) => {
        const [refreshToken] = args
        if (!refreshToken.trim()) {
          return t(`${I18N_PREFIX}.pleaseEnterRefreshToken`)
        }
        return null
      },
      call: (...args: any[]) => {
        const [refreshToken, proxyId] = args
        return adminAPI.grok.refreshGrokToken(refreshToken.trim(), proxyId)
      },
      extractError: (err, t) =>
        extractI18nErrorMessage(err, t, `${I18N_PREFIX}.errors`, t(`${I18N_PREFIX}.failedToValidateRT`)),
      showErrorOnFailure: false
    },
    buildCredentials: (tokenInfo: GrokTokenInfo): Record<string, unknown> => {
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
        base_url: GROK_CLI_BASE_URL
      }
      if (tokenInfo.refresh_token) credentials.refresh_token = tokenInfo.refresh_token
      if (tokenInfo.id_token) credentials.id_token = tokenInfo.id_token
      return Object.fromEntries(
        Object.entries(credentials).filter(([, value]) => value !== undefined && value !== '')
      )
    },
    buildExtraInfo: (tokenInfo: GrokTokenInfo): Record<string, unknown> => {
      const extra: Record<string, unknown> = {}
      if (tokenInfo.email) extra.email = tokenInfo.email
      if (tokenInfo.subscription_tier) extra.subscription_tier = tokenInfo.subscription_tier
      if (tokenInfo.entitlement_status) extra.entitlement_status = tokenInfo.entitlement_status
      return extra
    }
  })
}
