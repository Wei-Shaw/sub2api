import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProxyBindingSelector from '../ProxyBindingSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const proxies = [
  { id: 11, name: '代理 IP 1', protocol: 'http', host: '127.0.0.1', port: 8080 }
] as any

const proxyGroups = [
  { id: 7, name: '代理组 1', available_member_count: 2, member_count: 3, proxy_ids: [] }
] as any

describe('ProxyBindingSelector', () => {
  it('在同一个下拉列表中展示无代理、代理组和代理 IP', () => {
    const wrapper = mount(ProxyBindingSelector, {
      props: { proxyId: null, proxyGroupId: null, proxies, proxyGroups }
    })

    expect(wrapper.findAll('select')).toHaveLength(1)
    expect(wrapper.findAll('option').map((option) => option.attributes('value'))).toEqual([
      'none',
      'group:7',
      'proxy:11'
    ])
  })

  it('选择代理组或无代理时只更新对应绑定字段', async () => {
    const wrapper = mount(ProxyBindingSelector, {
      props: { proxyId: 11, proxyGroupId: null, proxies, proxyGroups }
    })
    const select = wrapper.find('select')

    await select.setValue('group:7')
    expect(wrapper.emitted('update:proxyId')).toEqual([[null]])
    expect(wrapper.emitted('update:proxyGroupId')).toEqual([[7]])

    await select.setValue('none')
    expect(wrapper.emitted('update:proxyId')).toEqual([[null], [null]])
    expect(wrapper.emitted('update:proxyGroupId')).toEqual([[7], [null]])
  })
})
