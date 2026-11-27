import { describe, expect, it } from 'vitest'
import { isOIDCExclusiveMode, resolveExclusiveOIDCEndSessionURL } from '@/utils/oidcExclusive'

describe('resolveExclusiveOIDCEndSessionURL', () => {
  it('returns the configured end-session URL only for exclusive OIDC mode', () => {
    expect(resolveExclusiveOIDCEndSessionURL({
      oidc_oauth_enabled: true,
      oidc_oauth_exclusive: true,
      oidc_oauth_end_session_url: 'https://sso.example.com/end-session/'
    }, 'https://sub2api.example.com')).toBe(
      'https://sso.example.com/end-session/?post_logout_redirect_uri=https%3A%2F%2Fsub2api.example.com%2Flogin%3Flogged_out%3D1'
    )

    expect(resolveExclusiveOIDCEndSessionURL({
      oidc_oauth_enabled: true,
      oidc_oauth_exclusive: false,
      oidc_oauth_end_session_url: 'https://sso.example.com/end-session/'
    })).toBeNull()
  })
})

describe('isOIDCExclusiveMode', () => {
  it('requires both OIDC enablement and the exclusive flag', () => {
    expect(isOIDCExclusiveMode({
      oidc_oauth_enabled: true,
      oidc_oauth_exclusive: true
    })).toBe(true)
    expect(isOIDCExclusiveMode({
      oidc_oauth_enabled: false,
      oidc_oauth_exclusive: true
    })).toBe(false)
  })
})
