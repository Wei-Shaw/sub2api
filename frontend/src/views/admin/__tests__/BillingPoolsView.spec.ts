import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminGroup, BillingPool } from '@/types'
import BillingPoolsView from '../BillingPoolsView.vue'

const {
  listBillingPools,
  getBillingPoolById,
  createBillingPool,
  updateBillingPool,
  deleteBillingPool,
  replaceMembers,
  getAllGroups,
  showError,
  showSuccess,
  showWarning
} = vi.hoisted(() => ({
  listBillingPools: vi.fn(),
  getBillingPoolById: vi.fn(),
  createBillingPool: vi.fn(),
  updateBillingPool: vi.fn(),
  deleteBillingPool: vi.fn(),
  replaceMembers: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn()
}))

const messages: Record<string, string> = {
  'admin.billingPools.createPool': 'Create pool',
  'admin.billingPools.allowUserReorderShort': 'User reorder',
  'admin.billingPools.requirePrimaryShort': 'Requires primary',
  'admin.billingPools.balanceFallbackShort': 'Balance fallback',
  'admin.billingPools.platformScopes.same_platform': 'Same platform',
  'admin.billingPools.platformScopes.mixed_platform': 'Mixed platform',
  'admin.billingPools.columns.name': 'Name',
  'admin.billingPools.columns.status': 'Status',
  'admin.billingPools.columns.platformScope': 'Platform scope',
  'admin.billingPools.columns.options': 'Options',
  'admin.billingPools.columns.groupCount': 'Groups',
  'admin.billingPools.columns.updatedAt': 'Updated',
  'admin.billingPools.columns.actions': 'Actions',
  'common.active': 'Active',
  'common.inactive': 'Inactive',
  'common.refresh': 'Refresh'
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    billingPools: {
      list: listBillingPools,
      getById: getBillingPoolById,
      create: createBillingPool,
      update: updateBillingPool,
      delete: deleteBillingPool,
      replaceMembers
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.billingPools.groupCount') return `${params?.count} groups`
        return messages[key] ?? key
      }
    })
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="actions" /><slot name="table" /><slot name="pagination" /></div>'
}
const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map((column) => column.key).join(',') }}</div>
      <div v-for="row in data" :key="row.id" data-test="pool-row">
        <slot name="cell-name" :row="row" :value="row.name" />
        <slot name="cell-status" :row="row" :value="row.status" />
        <slot name="cell-platform_scope" :row="row" :value="row.platform_scope" />
        <slot name="cell-options" :row="row" />
        <slot name="cell-group_count" :row="row" :value="row.group_count" />
        <slot name="cell-updated_at" :row="row" :value="row.updated_at" />
      </div>
    </div>
  `
}
const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\', $event.target.value)"><option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option></select>'
}

const wrappers: ReturnType<typeof mount>[] = []

function makeBillingPool(overrides: Partial<BillingPool> = {}): BillingPool {
  return {
    id: 10,
    name: 'Pooled Subscriptions',
    code: 'sub_pool',
    description: 'Subscription fallback pool',
    status: 'active',
    platform_scope: 'mixed_platform',
    allow_user_reorder: true,
    require_primary_subscription: true,
    allow_balance_fallback: true,
    group_count: 2,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-02T00:00:00Z',
    ...overrides
  }
}

function createAdminGroup(overrides: Partial<AdminGroup> = {}): AdminGroup {
  return {
    id: 1,
    name: 'Primary Subscription',
    description: null,
    platform: 'anthropic',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'subscription',
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    allow_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    image_price_1k: null,
    image_price_2k: null,
    image_price_4k: null,
    claude_code_only: false,
    fallback_group_id: null,
    fallback_group_id_on_invalid_request: null,
    require_oauth_only: false,
    require_privacy_set: false,
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    model_routing: null,
    model_routing_enabled: false,
    mcp_xml_inject: false,
    sort_order: 1,
    ...overrides
  }
}

describe('admin BillingPoolsView', () => {
  beforeEach(() => {
    localStorage.clear()

    listBillingPools.mockReset()
    getBillingPoolById.mockReset()
    createBillingPool.mockReset()
    updateBillingPool.mockReset()
    deleteBillingPool.mockReset()
    replaceMembers.mockReset()
    getAllGroups.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()

    listBillingPools.mockResolvedValue({
      items: [makeBillingPool()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getAllGroups.mockResolvedValue([createAdminGroup()])
  })

  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  })

  it('loads billing pools on entry and renders pool fallback metadata', async () => {
    const wrapper = mount(BillingPoolsView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          DataTable: DataTableStub,
          Select: SelectStub,
          Pagination: true,
          BaseDialog: true,
          ConfirmDialog: true,
          EmptyState: true,
          Icon: true,
          PlatformIcon: true
        }
      }
    })
    wrappers.push(wrapper)

    await flushPromises()

    expect(listBillingPools).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        status: undefined,
        platform_scope: undefined,
        search: undefined,
        sort_by: 'updated_at',
        sort_order: 'desc'
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(getAllGroups).toHaveBeenCalledTimes(1)

    expect(wrapper.get('[data-test="columns"]').text().split(',')).toEqual([
      'name',
      'status',
      'platform_scope',
      'options',
      'group_count',
      'updated_at',
      'actions'
    ])
    expect(wrapper.text()).toContain('Pooled Subscriptions')
    expect(wrapper.text()).toContain('sub_pool')
    expect(wrapper.text()).toContain('Active')
    expect(wrapper.text()).toContain('Mixed platform')
    expect(wrapper.text()).toContain('User reorder')
    expect(wrapper.text()).toContain('Requires primary')
    expect(wrapper.text()).toContain('Balance fallback')
    expect(wrapper.text()).toContain('2 groups')
  })
})
