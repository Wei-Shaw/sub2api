import { adminAPI } from '@/api/admin'
import type { AntigravityTokenInfo } from '@/api/admin/antigravity'
import { createOAuthComposable } from './createOAuthComposable'

const I18N_PREFIX = 'admin.accounts.oauth.antigravity'

export function useAntigravityOAuth() {
  return createOAuthComposable({
    generateAuthUrl: {
      buildPayload: (...args: any[]) => {
        const [proxyId] = args
        const payload: Record<string, unknown> = {}
        if (proxyId) payload.proxy_id = proxyId
        return payload
      },
      call: (payload) => adminAPI.antigravity.generateAuthUrl(payload as any),
      extractError: (err, t) =>
        err.response?.data?.detail || t(`${I18N_PREFIX}.failedToGenerateUrl`)
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
      call: (payload) => adminAPI.antigravity.exchangeCode(payload as any),
      extractError: (err, t) =>
        err.response?.data?.detail || t(`${I18N_PREFIX}.failedToExchangeCode`)
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
        return adminAPI.antigravity.refreshAntigravityToken(refreshToken.trim(), proxyId)
      },
      extractError: (err, t) =>
        err.response?.data?.detail || t(`${I18N_PREFIX}.failedToValidateRT`),
      showErrorOnFailure: false
    },
    buildCredentials: (
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

      return {
        access_token: tokenInfo.access_token,
        refresh_token: refreshToken,
        token_type: tokenInfo.token_type,
        expires_at: expiresAt,
        project_id: tokenInfo.project_id,
        email: tokenInfo.email
      }
    }
  })
}
