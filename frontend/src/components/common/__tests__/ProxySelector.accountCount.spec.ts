import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ProxySelector from '../ProxySelector.vue'
import enResources from '@/i18n/locales/en/admin/resources'
import zhResources from '@/i18n/locales/zh/admin/resources'
import type { Proxy } from '@/types'

vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy: vi.fn() } } }))
enableAutoUnmount(afterEach)

async function openSelector(locale: 'zh' | 'en', accountCounts: Array<number | undefined>): Promise<VueWrapper> {
  // Real vue-i18n with the real locale files: pluralization and interpolation
  // are exercised exactly as in the app.
  const i18n = createI18n({
    legacy: false,
    locale,
    missingWarn: false,
    fallbackWarn: false,
    messages: {
      en: { admin: { proxies: enResources.proxies } },
      zh: { admin: { proxies: zhResources.proxies } }
    }
  })
  const proxies: Proxy[] = accountCounts.map((count: number | undefined, i: number) => ({
    id: i + 1,
    name: `Proxy ${i + 1}`,
    host: '127.0.0.1',
    port: 11000 + i,
    protocol: 'socks5h',
    username: null,
    status: 'active',
    expires_at: null,
    account_count: count
  } as Proxy))
  const wrapper = mount(ProxySelector, {
    props: { modelValue: null, proxies },
    global: { plugins: [i18n], stubs: { Icon: true } }
  })
  await wrapper.get('.select-trigger').trigger('click')
  return wrapper
}

function proxyRows(wrapper: VueWrapper) {
  // First option is "no proxy".
  return wrapper.findAll('.select-option').slice(1)
}

describe('ProxySelector account counts', () => {
  it('shows the bound account count on the right side of each proxy row (zh)', async () => {
    const wrapper = await openSelector('zh', [0, 7])
    const rows = proxyRows(wrapper)

    expect(rows[0].get('[data-test="proxy-account-count"]').text()).toBe('0 个账号')
    expect(rows[1].get('[data-test="proxy-account-count"]').text()).toBe('7 个账号')
    expect(rows[1].get('[data-test="proxy-account-count"]').attributes('title')).toBe('当前绑定此代理的账号数')

    // The badge sits on the right: after the name/address block, before the test button.
    const children = Array.from(rows[1].element.children)
    const badgeIndex = children.findIndex((el: Element) => el.getAttribute('data-test') === 'proxy-account-count')
    const testButtonIndex = children.findIndex((el: Element) => el.classList.contains('test-btn'))
    expect(badgeIndex).toBeGreaterThan(0)
    expect(testButtonIndex).toBe(badgeIndex + 1)
  })

  it('pluralizes the count in English', async () => {
    const wrapper = await openSelector('en', [0, 1, 3])
    const texts = proxyRows(wrapper).map((row) => row.get('[data-test="proxy-account-count"]').text())

    expect(texts).toEqual(['0 accounts', '1 account', '3 accounts'])
    expect(proxyRows(wrapper)[0].get('[data-test="proxy-account-count"]').attributes('title')).toBe('Accounts currently using this proxy')
  })

  it('omits the badge when the list was loaded without counts', async () => {
    const wrapper = await openSelector('zh', [undefined, 2])
    const rows = proxyRows(wrapper)

    expect(rows[0].find('[data-test="proxy-account-count"]').exists()).toBe(false)
    expect(rows[1].get('[data-test="proxy-account-count"]').text()).toBe('2 个账号')
  })
})
