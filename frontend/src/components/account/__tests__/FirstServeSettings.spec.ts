import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Proxy, ProxyGroup } from '@/types'
import FirstServeSettings from '../FirstServeSettings.vue'
import { readFirstServeConfig, type FirstServeConfig } from '@/utils/firstServe'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${Object.values(params).join(',')}` : key }) }))

const groups = [{ id: 10, name: 'Group A', status: 'active', proxy_ids: [1, 2], member_count: 2, available_member_count: 2 },
  { id: 20, name: 'Group B', status: 'active', proxy_ids: [3], member_count: 1, available_member_count: 1 }] as ProxyGroup[]
const proxies = [1, 2, 3].map(id => ({ id, name: `Proxy ${id}`, host: `proxy-${id}`, port: 8080, ip_address: `203.0.113.${id}`, status: 'active', username: 'secret-user', password: 'secret-password' })) as Proxy[]

function render(config = readFirstServeConfig(), props: { enabled?: boolean; proxyGroupId?: number | null; proxyGroups?: ProxyGroup[] } = {}) {
  const wrapper = mount(FirstServeSettings, {
    props: { enabled: true, modelValue: config, proxyGroupId: 10, accountName: 'Account A', proxyGroups: groups, proxies,
      ...props,
      'onUpdate:enabled': (value: boolean) => { void wrapper.setProps({ enabled: value }) },
      'onUpdate:modelValue': (value: FirstServeConfig) => { void wrapper.setProps({ modelValue: value }) },
      'onUpdate:proxyGroupId': (value: number | null) => { void wrapper.setProps({ proxyGroupId: value }) }
    }
  })
  return wrapper
}

describe('FirstServeSettings', () => {
  it('defaults to an active group with two available members only when enabled and preserves manual choices', async () => {
    const wrapper = render(readFirstServeConfig(), { enabled: false, proxyGroupId: null,
      proxyGroups: [{ ...groups[0]!, id: 30, status: 'inactive' }, groups[1]!, groups[0]!] })
    expect(wrapper.emitted('update:proxyGroupId')).toBeUndefined()
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.props('proxyGroupId')).toBe(10)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="first-serve-config"]').element as HTMLDetailsElement).open).toBe(false)
    await wrapper.get('[data-testid="first-serve-group"]').setValue('20')
    await wrapper.get('[role="switch"]').trigger('click')
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.props('proxyGroupId')).toBe(20)
    await wrapper.get('[data-testid="first-serve-group"]').setValue('')
    expect(wrapper.props('proxyGroupId')).toBeNull()
    expect(wrapper.vm.validate()).toBe(true)
    wrapper.unmount()
  })

  it('selects a default after proxy groups finish loading', async () => {
    const wrapper = render(readFirstServeConfig(), { enabled: false, proxyGroupId: null, proxyGroups: [] })
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.props('proxyGroupId')).toBeNull()
    await wrapper.setProps({ proxyGroups: groups })
    expect(wrapper.props('proxyGroupId')).toBe(10)
    expect(wrapper.vm.validate()).toBe(true)
    wrapper.unmount()
  })

  it('keeps configuration usable when imports have no proxy group yet', async () => {
    const wrapper = render(readFirstServeConfig(), { enabled: false, proxyGroupId: null, proxyGroups: [groups[1]!] })
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.emitted('update:proxyGroupId')).toBeUndefined()
    expect(wrapper.vm.validate()).toBe(true)
    expect((wrapper.get('[data-testid="first-serve-config"]').element as HTMLDetailsElement).open).toBe(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('Account A')
    wrapper.unmount()
  })

  it('only defaults to a group containing all explicitly allowed proxies', async () => {
    const config = { ...readFirstServeConfig(), proxy_mode: 'selected' as const, proxy_ids: [1, 2] }
    const wrapper = render(config, { enabled: false, proxyGroupId: null,
      proxyGroups: [{ ...groups[0]!, id: 30, proxy_ids: [3, 4] }, groups[0]!] })
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.props('proxyGroupId')).toBe(10)
    expect(wrapper.props('modelValue')).toEqual(config)
    expect(wrapper.vm.validate()).toBe(true)
    wrapper.unmount()
  })

  it('defaults to account sharing while preserving an explicitly saved conversation scope', () => {
    expect(readFirstServeConfig().reuse_scope).toBe('account')
    expect(readFirstServeConfig({ openai_first_serve: { ttl_minutes: 12 } }).reuse_scope).toBe('account')
    expect(readFirstServeConfig({ openai_first_serve: { reuse_scope: 'session' } }).reuse_scope).toBe('session')
  })

  it('defaults to a 240 second IP rotation interval', () => {
    expect(readFirstServeConfig().rotate_seconds).toBe(240)
    expect(readFirstServeConfig({ openai_first_serve: { rotate_seconds: 600 } }).rotate_seconds).toBe(600)
  })

  it('keeps configuration collapsed and preserves it when the switch is turned off and back on', async () => {
    const wrapper = render()
    expect(wrapper.get('[role="switch"]').attributes('aria-checked')).toBe('true')
    expect((wrapper.get('[data-testid="first-serve-config"]').element as HTMLDetailsElement).open).toBe(false)
    await wrapper.get('[data-testid="first-serve-rotate_seconds"]').setValue('600')
    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.find('[data-testid="first-serve-config"]').exists()).toBe(false)
    expect(wrapper.vm.validate()).toBe(true)
    await wrapper.get('[role="switch"]').trigger('click')
    expect((wrapper.get('[data-testid="first-serve-config"]').element as HTMLDetailsElement).open).toBe(false)
    expect(wrapper.props('modelValue').rotate_seconds).toBe(600)
    wrapper.unmount()
  })

  it('opens collapsed fields for validation failures and native invalid events', async () => {
    const wrapper = render({ ...readFirstServeConfig(), rotate_seconds: 0 })
    expect(wrapper.vm.validate()).toBe(false)
    expect((wrapper.get('[data-testid="first-serve-config"]').element as HTMLDetailsElement).open).toBe(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('Account A')
    await wrapper.setProps({ enabled: false })
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    await wrapper.setProps({ enabled: true })
    await wrapper.get('[data-testid="first-serve-rotate_seconds"]').trigger('invalid')
    expect((wrapper.get('[data-testid="first-serve-config"]').element as HTMLDetailsElement).open).toBe(true)
    wrapper.unmount()
  })

  it('limits proxy choices to the bound group, displays exit IPs without credentials, and requires explicit selection', async () => {
    const wrapper = render()
    await wrapper.get('[data-testid="first-serve-proxy-mode"]').setValue('selected')
    expect(wrapper.get('[role="alert"]').text()).toContain('Account A')
    expect(wrapper.text()).toContain('203.0.113.1')
    expect(wrapper.text()).not.toContain('secret-user')
    expect(wrapper.text()).not.toContain('secret-password')
    expect(wrapper.find('[data-testid="first-serve-proxy-3"]').exists()).toBe(false)
    await wrapper.get('[data-testid="first-serve-proxy-1"]').setValue(true)
    await wrapper.get('[data-testid="first-serve-proxy-2"]').setValue(true)
    expect(wrapper.vm.validate()).toBe(true)
    await wrapper.get('[data-testid="first-serve-group"]').setValue('20')
    expect(wrapper.vm.validate()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('#1, #2')
    expect(wrapper.props('modelValue').proxy_ids).toEqual([1, 2])
    expect(wrapper.props('modelValue').proxy_mode).toBe('selected')
    wrapper.unmount()
  })

  it('validates custom timing values and never treats an empty selected list as all proxies', async () => {
    const wrapper = render()
    await wrapper.get('[data-testid="first-serve-rotate_seconds"]').setValue('600')
    expect(wrapper.props('modelValue').rotate_seconds).toBe(600)
    expect(wrapper.vm.validate()).toBe(true)
    await wrapper.get('[data-testid="first-serve-rotate_seconds"]').setValue('0')
    expect(wrapper.vm.validate()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('rotate_seconds')
    await wrapper.get('[data-testid="first-serve-rotate_seconds"]').setValue('10')
    await wrapper.get('[data-testid="first-serve-proxy-mode"]').setValue('selected')
    expect(wrapper.vm.validate()).toBe(false)
    await wrapper.get('[data-testid="first-serve-proxy-mode"]').setValue('all')
    expect(wrapper.vm.validate()).toBe(true)
    wrapper.unmount()
  })
})
