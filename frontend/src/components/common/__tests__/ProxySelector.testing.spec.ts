import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ProxySelector from '../ProxySelector.vue'
import type { Proxy } from '@/types'

const testProxy = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.clearAllMocks() })

async function openSelector(accountCounts?: number[]) {
  const wrapper = mount(ProxySelector, {
    props: { modelValue: null, proxies: [1, 2].map(id => ({
      id, name: `Proxy ${id}`, host: 'localhost', port: 8080, protocol: 'http',
      account_count: accountCounts?.[id - 1]
    } as Proxy)) },
    global: { stubs: { Icon: true } }
  })
  await wrapper.get('.select-trigger').trigger('click')
  return wrapper
}

describe('proxy account counts', () => {
  it('displays zero and nonzero counts in dropdown options', async () => {
    const wrapper = await openSelector([0, 7])
    const options = wrapper.findAll('.select-option').slice(1)

    expect(options[0].get('.bg-gray-100').text()).toBe('0')
    expect(options[1].get('.bg-gray-100').text()).toBe('7')
  })

  it('does not display a count when it is unavailable', async () => {
    const wrapper = await openSelector()

    expect(wrapper.findAll('.select-option .bg-gray-100')).toHaveLength(0)
  })
})

describe('proxy connection tests', () => {
  it('does not restart an individual test when a batch is started', async () => {
    let finish!: (result: object) => void
    testProxy.mockImplementation((id: number) => id === 1
      ? new Promise(resolve => { finish = resolve })
      : Promise.resolve({ success: true, country: 'GB' }))
    const wrapper = await openSelector()
    await wrapper.findAll('.test-btn')[0].trigger('click')
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy.mock.calls.map(([id]) => id)).toEqual([1, 2])
    expect(wrapper.findAll('.test-btn')[0].attributes('disabled')).toBeDefined()
    finish({ success: true, country: 'US' })
    await flushPromises()
    expect(wrapper.text()).toContain('US')
    expect(wrapper.findAll('.test-btn')[0].attributes('disabled')).toBeUndefined()
  })

  it('shows per-proxy outcomes and allows another batch after a failure', async () => {
    testProxy.mockImplementation((id: number) => id === 1
      ? Promise.reject(new Error('offline'))
      : Promise.resolve({ success: true, country: 'GB' }))
    const wrapper = await openSelector()
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('admin.proxies.testFailed')
    expect(wrapper.text()).toContain('GB')
    expect(wrapper.get('.batch-test-btn').attributes('disabled')).toBeUndefined()
    await wrapper.get('.batch-test-btn').trigger('click')
    await flushPromises()
    expect(testProxy).toHaveBeenCalledTimes(4)
  })
})
