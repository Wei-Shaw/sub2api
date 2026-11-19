import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RechargeCenterView from '../RechargeCenterView.vue'

const { getConfig } = vi.hoisted(() => ({ getConfig: vi.fn() }))
vi.mock('@/api/payment', () => ({ paymentAPI: { getConfig } }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({
    t: (key: string, params?: { name: string }) => key === 'payment.rechargeCenter.openStore'
      ? `前往 ${params?.name} 店铺`
      : key,
  }),
}))

const urls = {
  recharge_primary_url: 'https://primary.example/shop',
  recharge_backup_url: 'https://backup.example/shop',
  recharge_primary_order_url: 'https://primary.example/#/orders',
  recharge_backup_order_url: 'https://backup.example/orders',
}
const mountView = () => mount(RechargeCenterView, {
  global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
})

describe('RechargeCenterView', () => {
  beforeEach(() => {
    getConfig.mockReset()
  })

  it('maps each store and order lookup to its own external URL when online payment is disabled', async () => {
    getConfig.mockResolvedValue({ data: { ...urls, payment_enabled: false } })
    const wrapper = mountView()
    await flushPromises()
    const links = wrapper.findAll('a')
    expect(links.map(link => link.attributes('href'))).toEqual(Object.values(urls))
    for (const link of links) {
      expect(link.attributes('target')).toBe('_blank')
      expect(link.attributes('rel')).toBe('noopener noreferrer')
    }
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it.each([
    ['https://catfk.com/shop/123', 'catfk.com'],
    ['https://www.wzyp.cn/shop/456', 'wzyp.cn'],
    ['https://shop.example:8443/product?name=FakeStore', 'shop.example'],
  ])('uses the hostname from %s in the store button', async (url, name) => {
    getConfig.mockResolvedValue({ data: { recharge_primary_url: url } })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('a').text()).toBe(`前往 ${name} 店铺`)
    expect(wrapper.get('a').attributes('href')).toBe(url)
  })

  it('disables absent and unsafe links and sanitizes the guide', async () => {
    getConfig.mockResolvedValue({ data: {
      recharge_primary_url: 'javascript:alert(1)',
      recharge_backup_url: 'https://user:password@backup.example',
      recharge_primary_order_url: '',
      help_text: '<script>alert(1)</script><img src=x onerror="alert(1)"> **Guide**',
    } })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findAll('a')).toHaveLength(0)
    expect(wrapper.findAll('script')).toHaveLength(0)
    expect(wrapper.find('.markdown-body').html()).not.toContain('onerror')
    expect(wrapper.text()).toContain('Guide')
  })

  it('shows a loading state and a clear configuration failure', async () => {
    let reject!: (reason?: unknown) => void
    getConfig.mockReturnValue(new Promise((_resolve, no) => { reject = no }))
    const wrapper = mountView()
    expect(wrapper.get('[role="status"]').text()).toBe('common.loading')
    reject(new Error('unavailable'))
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('payment.rechargeCenter.configError')
    expect(wrapper.findAll('a')).toHaveLength(0)
  })
})
