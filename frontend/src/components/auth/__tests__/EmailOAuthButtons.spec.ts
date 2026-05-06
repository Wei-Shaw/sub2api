import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmailOAuthButtons from '@/components/auth/EmailOAuthButtons.vue'

const routeState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))

const locationState = vi.hoisted(() => ({
  current: { href: 'http://localhost/register?aff=AFF123' } as { href: string },
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      if (key === 'auth.emailOAuth.signIn') {
        return `使用 ${params?.providerName ?? ''} 登录`
      }
      return key
    },
  }),
}))

describe('EmailOAuthButtons', () => {
  beforeEach(() => {
    routeState.query = { redirect: '/billing?plan=pro', aff: 'AFF123' }
    locationState.current = { href: 'http://localhost/register?aff=AFF123' }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    })
    window.localStorage.clear()
    window.sessionStorage.clear()
  })

  it('passes the affiliate code to the email oauth start URL', async () => {
    const wrapper = mount(EmailOAuthButtons, {
      props: {
        githubEnabled: true,
        googleEnabled: false,
      },
      global: {
        stubs: {
          GitHubMark: true,
          GoogleMark: true,
        },
      },
    })

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toBe(
      '/api/v1/auth/oauth/github/start?redirect=%2Fbilling%3Fplan%3Dpro&aff_code=AFF123'
    )
    expect(window.sessionStorage.getItem('oauth_aff_code')).toBe('AFF123')
  })

  it('uses a full-width descriptive button when only GitHub is enabled', () => {
    const wrapper = mount(EmailOAuthButtons, {
      props: {
        githubEnabled: true,
        googleEnabled: false,
      },
      global: {
        stubs: {
          GitHubMark: true,
          GoogleMark: true,
        },
      },
    })

    expect(wrapper.find('.grid').classes()).not.toContain('sm:grid-cols-2')
    expect(wrapper.get('button').text()).toContain('使用 GitHub 登录')
  })

  it('uses compact labels and two columns when GitHub and Google are both enabled', () => {
    const wrapper = mount(EmailOAuthButtons, {
      props: {
        githubEnabled: true,
        googleEnabled: true,
      },
      global: {
        stubs: {
          GitHubMark: true,
          GoogleMark: true,
        },
      },
    })

    expect(wrapper.find('.grid').classes()).toContain('sm:grid-cols-2')
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)
    expect(buttons[0].text()).toContain('GitHub')
    expect(buttons[0].text()).not.toContain('使用 GitHub 登录')
    expect(buttons[1].text()).toContain('Google')
    expect(buttons[1].text()).not.toContain('使用 Google 登录')
  })
})
