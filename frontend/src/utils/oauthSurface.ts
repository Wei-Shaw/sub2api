/**
 * OAuth API surface helpers — admin vs user-owned account OAuth paths.
 *
 * Admin keeps historical shapes (some without `/oauth/` segment).
 * User paths follow the design doc mapping in docs/design/user-oauth-for-owned-accounts.md.
 */

export type OAuthSurface = 'admin' | 'user'

export type OAuthPlatform =
  | 'accounts' // Anthropic / Claude under /accounts
  | 'openai'
  | 'gemini'
  | 'antigravity'
  | 'grok'

/**
 * Returns the API path prefix (no trailing slash) for OAuth endpoints of a platform.
 *
 * Examples:
 * - admin accounts → `/admin/accounts`
 * - user accounts  → `/user/accounts/oauth`
 * - admin openai   → `/admin/openai`
 * - user openai    → `/user/openai`
 * - admin gemini   → `/admin/gemini/oauth`
 * - user gemini    → `/user/gemini/oauth`
 */
export function oauthPrefix(surface: OAuthSurface, platform: OAuthPlatform): string {
  if (surface === 'user') {
    switch (platform) {
      case 'accounts':
        return '/user/accounts/oauth'
      case 'openai':
        return '/user/openai'
      case 'gemini':
        return '/user/gemini/oauth'
      case 'antigravity':
        return '/user/antigravity/oauth'
      case 'grok':
        return '/user/grok/oauth'
    }
  }

  // admin
  switch (platform) {
    case 'accounts':
      return '/admin/accounts'
    case 'openai':
      return '/admin/openai'
    case 'gemini':
      return '/admin/gemini/oauth'
    case 'antigravity':
      return '/admin/antigravity/oauth'
    case 'grok':
      return '/admin/grok/oauth'
  }
}

export interface OAuthComposableOptions {
  surface?: OAuthSurface
}

/** Effective proxy id: user surface always null (never send proxy_id). */
export function effectiveProxyId(
  surface: OAuthSurface,
  proxyId?: number | null
): number | null | undefined {
  if (surface === 'user') return null
  return proxyId
}
