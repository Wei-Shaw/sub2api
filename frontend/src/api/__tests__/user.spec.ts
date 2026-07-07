import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('user api oauth binding urls', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api/v1')
  })

  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('builds third-party bind urls against the bind start endpoint', async () => {
    const { buildOAuthBindingStartURL } = await import('@/api/user')

    expect(buildOAuthBindingStartURL('linuxdo', { redirectTo: '/settings/profile' })).toBe(
      'https://api.example.com/api/v1/auth/oauth/linuxdo/bind/start?redirect=%2Fsettings%2Fprofile&intent=bind_current_user'
    )
    expect(
      buildOAuthBindingStartURL('wechat', {
        redirectTo: '/settings/profile',
        wechatOAuthSettings: {
          wechat_oauth_open_enabled: true,
          wechat_oauth_mp_enabled: false,
          wechat_oauth_mobile_enabled: false
        }
      })
    ).toBe(
      'https://api.example.com/api/v1/auth/oauth/wechat/bind/start?redirect=%2Fsettings%2Fprofile&intent=bind_current_user&mode=open'
    )
  })
})
