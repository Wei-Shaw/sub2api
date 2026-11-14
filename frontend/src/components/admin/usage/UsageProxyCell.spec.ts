import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UsageProxyCell from './UsageProxyCell.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => {
    const messages: Record<string, string> = {
      'admin.usage.proxyNotRecorded': '未记录',
      'admin.usage.proxyDirect': '直连',
    }
    return messages[key] || key
  } }),
}))

describe('UsageProxyCell', () => {
  it('shows only the proxy identity even for historical records with exit IP data', () => {
    const proxy = { id: 42, name: '代理 A', exit_ip: '203.0.113.7', exit_ip_checked_at: '2026-10-24T00:00:00Z' }
    const wrapper = mount(UsageProxyCell, { props: { proxy } })
    expect(wrapper.text()).toBe('代理 A')
    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('distinguishes historical records, direct requests, and unnamed proxies', () => {
    for (const [proxy, text] of [[null, '未记录'], [{}, '直连'], [{ id: 1 }, '#1']] as const) {
      const wrapper = mount(UsageProxyCell, { props: { proxy } })
      expect(wrapper.text()).toBe(text)
    }
  })
})
