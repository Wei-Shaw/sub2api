import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import ProxyPoolsView from '../ProxyPoolsView.vue'

const {
  poolList,
  poolCreate,
  poolUpdate,
  poolRemove,
  poolListProxies,
  poolListAccounts,
  poolAssign,
  poolRemoveProxies,
  poolRebind,
  poolRebindLogs,
  proxyGetAll,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  poolList: vi.fn(),
  poolCreate: vi.fn(),
  poolUpdate: vi.fn(),
  poolRemove: vi.fn(),
  poolListProxies: vi.fn(),
  poolListAccounts: vi.fn(),
  poolAssign: vi.fn(),
  poolRemoveProxies: vi.fn(),
  poolRebind: vi.fn(),
  poolRebindLogs: vi.fn(),
  proxyGetAll: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    proxyPools: {
      list: poolList,
      create: poolCreate,
      update: poolUpdate,
      remove: poolRemove,
      listProxies: poolListProxies,
      listAccounts: poolListAccounts,
      assignProxies: poolAssign,
      removeProxies: poolRemoveProxies,
      rebind: poolRebind,
      rebindLogs: poolRebindLogs,
    },
    proxies: {
      getAll: proxyGetAll,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: defineComponent({
    name: 'AppLayoutStub',
    setup(_, { slots }) {
      return () => h('div', { class: 'app-layout-stub' }, slots.default?.())
    },
  }),
}))

vi.mock('@/components/layout/TablePageLayout.vue', () => ({
  default: defineComponent({
    name: 'TablePageLayoutStub',
    setup(_, { slots }) {
      return () => h('div', { class: 'table-page-layout-stub' }, [
        slots.filters?.(),
        slots.table?.(),
      ])
    },
  }),
}))

vi.mock('@/components/common/ConfirmDialog.vue', () => ({
  default: defineComponent({
    name: 'ConfirmDialogStub',
    props: {
      show: Boolean,
      title: String,
      message: String,
      confirmText: String,
      cancelText: String,
    },
    emits: ['confirm', 'cancel'],
    setup(props, { emit, slots }) {
      return () =>
        props.show
          ? h('div', { class: 'confirm-dialog-stub' }, [
              h('h4', props.title),
              h('p', props.message),
              slots.default?.(),
              h('button', { type: 'button', onClick: () => emit('cancel') }, props.cancelText),
              h('button', { type: 'button', onClick: () => emit('confirm') }, props.confirmText),
            ])
          : null
    },
  }),
}))

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: defineComponent({
    name: 'BaseDialogStub',
    props: {
      show: Boolean,
      title: String,
      width: String,
    },
    emits: ['close'],
    setup(props, { slots }) {
      return () =>
        props.show
          ? h('section', { class: 'base-dialog-stub' }, [
              h('h3', props.title),
              slots.default?.({
                close: () => props.show,
              }),
              slots.footer?.({}),
            ])
          : null
    },
  }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'error',
}))

vi.mock('@/utils/format', () => ({
  formatDateTime: (value: string) => `formatted:${value}`,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function makePool(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: 'pool-a',
    description: null,
    status: 'active',
    health_interval_seconds: 300,
    failure_threshold: 2,
    auto_rebind: true,
    proxy_count: 2,
    healthy_count: 1,
    unhealthy_count: 1,
    unknown_count: 0,
    bound_account_sum: 3,
    unassigned_account_count: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ProxyPoolsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    poolList.mockResolvedValue([makePool()])
    poolListProxies.mockResolvedValue([
      {
        id: 11,
        name: 'proxy-1',
        host: '1.2.3.4',
        port: 8080,
        status: 'active',
        pool_health: 'healthy',
        pool_failures: 0,
        pool_checked_at: '2026-01-01T00:00:00Z',
        latency_ms: 120,
        account_count: 3,
      },
    ])
    poolListAccounts.mockImplementation((_poolId: number, page: number) => Promise.resolve({
      items: [{
        id: page === 1 ? 21 : 31,
        name: page === 1 ? 'account-21' : 'account-31',
        platform: 'openai',
        type: 'oauth',
        status: 'active',
        proxy_id: 11,
        proxy_name: 'proxy-1',
      }],
      total: 11,
      page,
      page_size: 10,
      pages: 2,
    }))
    poolRebindLogs.mockResolvedValue([
      {
        id: 1,
        pool_id: 1,
        from_proxy_id: 11,
        from_proxy_name: 'proxy-1',
        to_proxy_id: 12,
        to_proxy_name: 'proxy-2',
        account_count: 3,
        reason: 'unhealthy',
        created_at: '2026-01-01T00:00:00Z',
      },
    ])
    poolRebind.mockResolvedValue({
      reboundAccounts: 3,
      partialFailure: false,
      failedProxies: 0,
    })
    poolAssign.mockResolvedValue(1)
    poolRemoveProxies.mockResolvedValue(1)
    poolRemove.mockResolvedValue(undefined)
    proxyGetAll.mockResolvedValue([
      { id: 11, name: 'proxy-1', host: '1.2.3.4', port: 8080, status: 'active' },
      { id: 12, name: 'proxy-2', host: '5.6.7.8', port: 8081, status: 'active', pool_id: 2 },
    ])
  })

  it('renders the pool list with health stats', async () => {
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()

    expect(poolList).toHaveBeenCalled()
    expect(wrapper.text()).toContain('pool-a')
    expect(wrapper.text()).toContain('formatted:2026-01-01T00:00:00Z')
    expect(wrapper.text()).toContain('admin.proxyPools.unassignedAccountBadge')

    const statusSelect = wrapper
      .findAllComponents({ name: 'Select' })
      .find((candidate) => candidate.props('options')?.some?.((option: { value: string }) => option.value === 'active'))
    expect(statusSelect?.props('options')?.[0]).toEqual({
      label: 'admin.proxyPools.allStatus',
      value: '',
    })
  })

  it('debounces pool search filtering', async () => {
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()
    vi.useFakeTimers()
    try {
      const input = wrapper.get('input[placeholder="admin.proxyPools.searchPools"]')
      await input.setValue('no-match')

      expect(wrapper.text()).toContain('pool-a')
      await vi.advanceTimersByTimeAsync(249)
      await wrapper.vm.$nextTick()
      expect(wrapper.text()).toContain('pool-a')

      await vi.advanceTimersByTimeAsync(1)
      await wrapper.vm.$nextTick()
      expect(wrapper.text()).not.toContain('pool-a')
    } finally {
      vi.useRealTimers()
      wrapper.unmount()
    }
  })

  it('creates a pool through the form dialog', async () => {
    poolCreate.mockResolvedValue(makePool())
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()

    // 打开创建弹窗
    const createButton = wrapper.findAll('button').find((b) => b.text().includes('admin.proxyPools.createPool'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')

    const inputs = wrapper.findAll('input')
    const nameInput = inputs.find((i) => i.attributes('placeholder') === 'admin.proxyPools.poolNamePlaceholder')
    expect(nameInput).toBeTruthy()
    await nameInput!.setValue('pool-new')

    // footer 保存按钮
    const footerSave = wrapper.findAll('button').find((b) => b.text() === 'common.save')
    await footerSave!.trigger('click')
    await flushPromises()

    expect(poolCreate).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'pool-new', auto_rebind: true })
    )
    expect(showSuccess).toHaveBeenCalled()
  })

  it('opens detail and shows proxies, rebind logs; manual rebind works', async () => {
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()

    const detailButton = wrapper.findAll('button').find((b) => b.text() === 'admin.proxyPools.detail')
    expect(detailButton).toBeTruthy()
    await detailButton!.trigger('click')
    await flushPromises()

    expect(poolListProxies).toHaveBeenCalledWith(1)
    expect(poolRebindLogs).toHaveBeenCalledWith(1, 50)
    expect(poolListAccounts).toHaveBeenCalledWith(1, 1, 10)
    expect(wrapper.text()).toContain('proxy-1')
    expect(wrapper.text()).toContain('account-21')
    expect(wrapper.text()).toContain('proxy-2')

    const rebindButton = wrapper.findAll('button').find((b) => b.text().includes('admin.proxyPools.rebindNow'))
    expect(rebindButton).toBeTruthy()
    await rebindButton!.trigger('click')
    await flushPromises()

    expect(poolRebind).toHaveBeenCalledWith(1)
    expect(showSuccess).toHaveBeenCalled()
  })

  it('loads another page of assigned accounts', async () => {
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()
    const detailButton = wrapper.findAll('button').find((b) => b.text() === 'admin.proxyPools.detail')
    await detailButton!.trigger('click')
    await flushPromises()

    const nextButton = wrapper
      .findAll('button')
      .find((button) => button.attributes('aria-label') === 'pagination.next')
    expect(nextButton).toBeTruthy()
    await nextButton!.trigger('click')
    await flushPromises()

    expect(poolListAccounts).toHaveBeenLastCalledWith(1, 2, 10)
    expect(wrapper.text()).toContain('account-31')
  })

  it('shows the source pool before reassigning a proxy', async () => {
    poolList.mockResolvedValue([
      makePool(),
      makePool({ id: 2, name: 'source-pool' }),
    ])
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()

    const detailButton = wrapper.findAll('button').find((button) => button.text() === 'admin.proxyPools.detail')
    await detailButton!.trigger('click')
    await flushPromises()
    const assignButton = wrapper.findAll('button').find((button) => button.text() === 'admin.proxyPools.assignProxy')
    await assignButton!.trigger('click')
    await flushPromises()

    const proxyRow = wrapper.findAll('label').find((label) => label.text().includes('proxy-2'))
    expect(proxyRow).toBeTruthy()
    await proxyRow!.get('input[type="checkbox"]').trigger('change')
    const assignConfirm = wrapper.findAll('button').find((button) => button.text() === 'admin.proxyPools.assignConfirm')
    await assignConfirm!.trigger('click')
    await flushPromises()

    expect(poolAssign).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="proxy-reassignment-sources"]').text()).toContain('source-pool')
    expect(wrapper.get('[data-testid="proxy-reassignment-sources"]').text()).toContain('proxy-2')

    const reassignConfirm = wrapper.findAll('button').find((button) => button.text() === 'admin.proxyPools.reassignProxyAction')
    await reassignConfirm!.trigger('click')
    await flushPromises()

    expect(poolAssign).toHaveBeenCalledWith(1, [12])
  })

  it('shows a partial failure and refreshes the open detail from the list source', async () => {
    poolList
      .mockResolvedValueOnce([makePool()])
      .mockResolvedValueOnce([makePool({ name: 'pool-updated', bound_account_sum: 7 })])
    poolRebind.mockResolvedValue({
      reboundAccounts: 2,
      partialFailure: true,
      failedProxies: 1,
    })
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()

    const detailButton = wrapper.findAll('button').find((b) => b.text() === 'admin.proxyPools.detail')
    await detailButton!.trigger('click')
    await flushPromises()

    const rebindButton = wrapper.findAll('button').find((b) => b.text().includes('admin.proxyPools.rebindNow'))
    await rebindButton!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.proxyPools.rebindPartial')
    expect(wrapper.text()).toContain('pool-updated')
  })

  it('deletes a pool after confirmation', async () => {
    const wrapper = mount(ProxyPoolsView)
    await flushPromises()

    const deleteButton = wrapper
      .findAll('button')
      .find((b) => b.attributes('title') === 'admin.proxyPools.deletePool')
    expect(deleteButton).toBeTruthy()
    await deleteButton!.trigger('click')
    await flushPromises()

    // ConfirmDialog 的确认按钮
    const confirmButton = wrapper.findAll('button').find((b) => b.text() === 'common.delete')
    await confirmButton!.trigger('click')
    await flushPromises()

    expect(poolRemove).toHaveBeenCalledWith(1)
    expect(showSuccess).toHaveBeenCalled()
  })
})
