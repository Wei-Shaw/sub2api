import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import type { Proxy, ProxyGroup } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    proxies: {
      testProxy: vi.fn(),
    },
  },
}))

const proxies: Proxy[] = [
  {
    id: 12,
    name: 'HK-1',
    protocol: 'http',
    host: '1.1.1.1',
    port: 8080,
    status: 'active',
  } as Proxy,
]

const groups: ProxyGroup[] = [
  {
    id: 5,
    name: 'grok-pool',
    strategy: 'sticky',
    sticky_by_account: true,
    status: 'active',
    proxy_count: 3,
    created_at: '',
    updated_at: '',
  },
]

describe('ProxySelector', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('selects a proxy and emits numeric id', async () => {
    const wrapper = mount(ProxySelector, {
      props: {
        modelValue: null,
        proxies,
        mode: 'proxy',
      },
      global: { stubs: { Icon: true, Transition: false } },
    })

    await wrapper.get('[data-testid="proxy-selector"]').trigger('click')
    await nextTick()
    await wrapper.get('[data-testid="proxy-option-12"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([12])
  })

  it('treats string modelValue as equal to numeric proxy id (loose equality)', async () => {
    const wrapper = mount(ProxySelector, {
      props: {
        modelValue: '12' as unknown as number,
        proxies,
        mode: 'proxy',
      },
      global: { stubs: { Icon: true, Transition: false } },
    })

    // selected label should resolve the proxy even when modelValue is a string
    expect(wrapper.text()).toContain('HK-1')

    await wrapper.get('[data-testid="proxy-selector"]').trigger('click')
    await nextTick()
    const option = wrapper.get('[data-testid="proxy-option-12"]')
    expect(option.classes().join(' ')).toContain('select-option-selected')
  })

  it('supports group mode selection and sticky label', async () => {
    const wrapper = mount(ProxySelector, {
      props: {
        modelValue: null,
        groups,
        mode: 'group',
      },
      global: { stubs: { Icon: true, Transition: false } },
    })

    expect(wrapper.text()).toContain('admin.accounts.noProxyGroup')

    await wrapper.get('[data-testid="proxy-group-selector"]').trigger('click')
    await nextTick()
    await wrapper.get('[data-testid="proxy-group-option-5"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([5])
  })

  it('shows selected group label with sticky marker', async () => {
    const wrapper = mount(ProxySelector, {
      props: {
        modelValue: 5,
        groups,
        mode: 'group',
      },
      global: { stubs: { Icon: true, Transition: false } },
    })

    expect(wrapper.text()).toContain('grok-pool')
    expect(wrapper.text()).toContain('sticky')
  })
})
