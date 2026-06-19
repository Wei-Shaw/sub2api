import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// 受控 stub：同意页只依赖 `@/api/oidcConsent` 的两个方法。
const getConsentInfo = vi.fn()
const submitConsentDecision = vi.fn()

vi.mock('@/api/oidcConsent', () => ({
  getConsentInfo: (...args: unknown[]) => getConsentInfo(...args),
  submitConsentDecision: (...args: unknown[]) => submitConsentDecision(...args)
}))

// i18n 直出 key，te() 恒为 true，使 scope title/description 走 i18n 分支渲染出文本。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, te: () => true })
  }
})

import ConsentView from '../ConsentView.vue'

// jsdom 的 window.location.assign 不可重定义，整体替换为可断言的 mock。
const assignMock = vi.fn()

function makeRouter(consent: string): Router {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/oauth/consent', component: ConsentView },
      { path: '/login', component: { template: '<div />' } },
      { path: '/dashboard', component: { template: '<div />' } }
    ]
  })
  router.push({ path: '/oauth/consent', query: { consent } })
  return router
}

async function mountConsent(consent = 'consent-token') {
  const router = makeRouter(consent)
  await router.isReady()
  const wrapper = mount(ConsentView, {
    global: {
      plugins: [router],
      stubs: {
        AuthLayout: { template: '<div><slot /></div>' }
      }
    }
  })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.clearAllMocks()
  getConsentInfo.mockResolvedValue({
    client_id: 'rp_demo',
    client_name: 'Demo RP',
    scopes: [
      { scope: 'openid', sensitive: false },
      { scope: 'profile', sensitive: false }
    ]
  })
  submitConsentDecision.mockResolvedValue({ redirect: 'https://rp.example.com/cb?code=abc' })
  assignMock.mockReset()
  Object.defineProperty(window, 'location', {
    configurable: true,
    writable: true,
    value: { assign: assignMock, href: 'http://localhost/oauth/consent' }
  })
})

describe('ConsentView', () => {
  it('渲染每个 scope 的标题与描述', async () => {
    const { wrapper } = await mountConsent()

    expect(wrapper.find('[data-testid="consent-scope-openid"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="consent-scope-profile"]').exists()).toBe(true)
    // te()=true → scopeTitle/scopeDescription 返回 i18n key 字符串
    const html = wrapper.html()
    expect(html).toContain('oidc.consent.scopes.openid.title')
    expect(html).toContain('oidc.consent.scopes.openid.description')
    expect(html).toContain('oidc.consent.scopes.profile.title')
  })

  it('Allow 调用 submitConsentDecision("allow") 并整页跳转到 redirect', async () => {
    const { wrapper } = await mountConsent()

    await wrapper.find('[data-testid="consent-allow"]').trigger('click')
    await flushPromises()

    expect(submitConsentDecision).toHaveBeenCalledTimes(1)
    expect(submitConsentDecision).toHaveBeenCalledWith('consent-token', 'allow')
    expect(assignMock).toHaveBeenCalledWith('https://rp.example.com/cb?code=abc')
  })

  it('Deny 调用 submitConsentDecision("deny") 并整页跳转', async () => {
    submitConsentDecision.mockResolvedValue({
      redirect: 'https://rp.example.com/cb?error=access_denied'
    })
    const { wrapper } = await mountConsent()

    await wrapper.find('[data-testid="consent-deny"]').trigger('click')
    await flushPromises()

    expect(submitConsentDecision).toHaveBeenCalledTimes(1)
    expect(submitConsentDecision).toHaveBeenCalledWith('consent-token', 'deny')
    expect(assignMock).toHaveBeenCalledWith('https://rp.example.com/cb?error=access_denied')
  })

  it('请求包含 sub2api:balance 时展示红色敏感 scope 警示', async () => {
    getConsentInfo.mockResolvedValue({
      client_id: 'rp_demo',
      client_name: 'Demo RP',
      scopes: [
        { scope: 'openid', sensitive: false },
        { scope: 'sub2api:balance', sensitive: true }
      ]
    })
    const { wrapper } = await mountConsent()

    expect(wrapper.find('[data-testid="consent-sensitive-warning"]').exists()).toBe(true)
    // 敏感 scope 行使用红色文字 class
    const sensitiveRow = wrapper.find('[data-testid="consent-scope-sub2api:balance"]')
    expect(sensitiveRow.exists()).toBe(true)
    expect(sensitiveRow.html()).toContain('text-red-600')
  })

  it('无敏感 scope 时不渲染警示横幅', async () => {
    const { wrapper } = await mountConsent()
    expect(wrapper.find('[data-testid="consent-sensitive-warning"]').exists()).toBe(false)
  })

  it('401 时引导登录并保留回跳路径', async () => {
    getConsentInfo.mockRejectedValue({ status: 401, error: 'login_required' })
    const { router } = await mountConsent()
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toContain('/oauth/consent')
  })
})
