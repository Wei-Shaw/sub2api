import { adminAPI } from '@/api/admin'
import type { GeminiOAuthCapabilities } from '@/api/admin/gemini'
import { createOAuthComposable } from './createOAuthComposable'

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

const I18N_PREFIX = 'admin.accounts.oauth.gemini'

export function useGeminiOAuth() {
  return {
    ...createOAuthComposable({
      generateAuthUrl: {
        buildPayload: (...args: any[]) => {
          const [proxyId, projectId, oauthType, tierId] = args
          const payload: Record<string, unknown> = {}
          if (proxyId) payload.proxy_id = proxyId
          const trimmedProjectID = projectId?.trim()
          if (trimmedProjectID) payload.project_id = trimmedProjectID
          if (oauthType) payload.oauth_type = oauthType
          const trimmedTierID = tierId?.trim()
          if (trimmedTierID) payload.tier_id = trimmedTierID
          return payload
        },
        call: (payload) => adminAPI.gemini.generateAuthUrl(payload as any),
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
          if (params.oauthType) payload.oauth_type = params.oauthType
          const trimmedTierID = params.tierId?.trim()
          if (trimmedTierID) payload.tier_id = trimmedTierID
          return payload
        },
        call: (payload) => adminAPI.gemini.exchangeCode(payload as any),
        extractError: (err, t) => {
          const errorMessage = err.message || err.response?.data?.message || ''
          if (errorMessage.includes('missing project_id')) {
            return t(`${I18N_PREFIX}.missingProjectId`)
          }
          return errorMessage || t(`${I18N_PREFIX}.failedToExchangeCode`)
        }
      },
      buildCredentials: (tokenInfo: GeminiTokenInfo): Record<string, unknown> => {
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
      },
      buildExtraInfo: (tokenInfo: GeminiTokenInfo): Record<string, unknown> | undefined => {
        if (!tokenInfo.extra || typeof tokenInfo.extra !== 'object') return undefined
        return tokenInfo.extra
      }
    }),
    getCapabilities: async (): Promise<GeminiOAuthCapabilities | null> => {
      try {
        return await adminAPI.gemini.getCapabilities()
      } catch {
        // Capabilities are optional for older servers; don't block the UI.
        return null
      }
    }
  }
}
