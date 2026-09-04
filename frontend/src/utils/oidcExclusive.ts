type OIDCExclusivePublicSettings = {
  oidc_oauth_enabled?: boolean
  oidc_oauth_exclusive?: boolean
  oidc_oauth_end_session_url?: string
}

export function isOIDCExclusiveMode(
  settings: OIDCExclusivePublicSettings | null | undefined
): boolean {
  return settings?.oidc_oauth_enabled === true && settings.oidc_oauth_exclusive === true
}

export function resolveExclusiveOIDCEndSessionURL(
  settings: OIDCExclusivePublicSettings | null | undefined,
  appOrigin = window.location.origin
): string | null {
  if (!isOIDCExclusiveMode(settings)) return null
  const value = settings?.oidc_oauth_end_session_url?.trim()
  if (!value) return null

  try {
    const endSessionURL = new URL(value)
    const returnURL = new URL('/login?logged_out=1', appOrigin)
    endSessionURL.searchParams.set('post_logout_redirect_uri', returnURL.toString())
    return endSessionURL.toString()
  } catch {
    return null
  }
}
