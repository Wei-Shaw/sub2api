import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import BindProxyPoolModal from '../BindProxyPoolModal.vue'

const { bindAccounts, showError, showSuccess } = vi.hoisted(() => ({
  bindAccounts: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    proxyPools: { bindAccounts }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const pools = [
  {
    id: 3,
    name: 'healthy-pool',
    status: 'active',
    health_interval_seconds: 300,
    failure_threshold: 2,
    auto_rebind: true,
    proxy_count: 2,
    healthy_proxy_count: 2,
    unhealthy_proxy_count: 0,
    bound_account_count: 7,
    created_at: '2026-08-04T00:00:00Z',
    updated_at: '2026-08-04T00:00:00Z'
  }
]

function mountModal() {
  return mount(BindProxyPoolModal, {
    props: {
      show: false,
      accountIds: [101, 102],
      pools
    } as any,
    global: {
      stubs: {
        BaseDialog: {
          props: ['show', 'title'],
          template: '<section v-if="show"><slot /><slot name="footer" /></section>'
        },
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          template: '<select class="pool-select" :value="modelValue" @change="$emit(\'update:modelValue\', Number($event.target.value))"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>'
        },
        Icon: true
      }
    }
  })
}

describe('BindProxyPoolModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    bindAccounts.mockResolvedValue({ assigned: 2, failed: 0, results: [] })
  })

  it('binds every selected account to the active pool', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const bindButton = wrapper.findAll('button').find((button) => button.text().includes('admin.proxyPools.bindAccounts'))
    expect(bindButton).toBeDefined()
    await bindButton!.trigger('click')
    await flushPromises()

    expect(bindAccounts).toHaveBeenCalledWith(3, [101, 102])
    expect(showSuccess).toHaveBeenCalled()
    expect(wrapper.emitted('bound')).toHaveLength(1)
  })

  it('reports a partial batch result', async () => {
    bindAccounts.mockResolvedValue({ assigned: 1, failed: 1, results: [] })
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const bindButton = wrapper.findAll('button').find((button) => button.text().includes('admin.proxyPools.bindAccounts'))
    await bindButton!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
  })
})
