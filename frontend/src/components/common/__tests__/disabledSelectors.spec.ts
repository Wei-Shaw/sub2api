import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import Select from '../Select.vue'
import ProxySelector from '../ProxySelector.vue'
import type { Proxy } from '@/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy: vi.fn() } } }))
enableAutoUnmount(afterEach)

describe('selectors disabled while open', () => {
  it('closes an open Select and can reopen when enabled again', async () => {
    const wrapper = mount(Select, {
      props: { modelValue: 1, options: [{ value: 1, label: 'One' }, { value: 2, label: 'Two' }] },
      global: { stubs: { Teleport: true, Transition: true } },
    })
    await wrapper.get('.select-trigger').trigger('click')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)
    await wrapper.setProps({ disabled: true })
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.setProps({ disabled: false })
    await wrapper.get('.select-trigger').trigger('click')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(true)
  })

  it('closes an open proxy picker when the account form becomes disabled', async () => {
    const wrapper = mount(ProxySelector, {
      props: { modelValue: null, proxies: [{ id: 1, name: 'Proxy', host: 'localhost', port: 8080, protocol: 'http' } as Proxy] },
      global: { stubs: { Transition: true } },
    })
    await wrapper.get('.select-trigger').trigger('click')
    expect(wrapper.find('.select-dropdown').exists()).toBe(true)
    await wrapper.setProps({ disabled: true })
    expect(wrapper.find('.select-dropdown').exists()).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
