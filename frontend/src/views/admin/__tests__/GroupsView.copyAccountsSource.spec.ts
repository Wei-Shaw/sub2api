import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

const {
  listGroups,
  getAllIncludingInactive,
  getModelsListCandidates,
  getUsageSummary,
  getCapacitySummary,
  getLiveCapability
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getAllIncludingInactive: vi.fn(),
  getModelsListCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      getAll: vi.fn(),
      getAllIncludingInactive,
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      getLiveCapability,
      create: vi.fn(),
      update: vi.fn(),
      duplicate: vi.fn(),
      delete: vi.fn(),
      updateSortOrder: vi.fn()
    },
    accounts: {
      list: vi.fn(),
      getById: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const makeGroup = (overrides: Partial<AdminGroup>): AdminGroup =>
  ({
    id: 1,
    name: 'Group',
    description: null,
    platform: 'anthropic',
    rate_multiplier: 1,
    rpm_limit: 0,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
    created_at: '2026-07-16T00:00:00Z',
    updated_at: '2026-07-16T00:00:00Z',
    model_routing: null,
    model_routing_enabled: false,
    supported_model_scopes: [],
    account_count: 3,
    active_account_count: 3,
    rate_limited_account_count: 0,
    sort_order: 10,
    ...overrides
  }) as AdminGroup

// 当页只有这一个分组：修复前复制账号的候选源就只能看到它（且被"排除自身"过滤掉，结果为空）
const currentPageGroup = makeGroup({ id: 1, name: 'Page One Group' })
// 其他页 / 被筛选掉的分组：修复后必须出现在候选源里
const offPageGroup = makeGroup({ id: 2, name: 'Off Page Group' })
const offPageInactiveGroup = makeGroup({
  id: 3,
  name: 'Off Page Inactive Group',
  status: 'inactive'
})
const emptyGroup = makeGroup({
  id: 4,
  name: 'Off Page Empty Group',
  account_count: 0,
  active_account_count: 0
})
const otherPlatformGroup = makeGroup({
  id: 5,
  name: 'Off Page OpenAI Group',
  platform: 'openai'
})

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })

const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>'
})

const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false }
  },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>'
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const SelectStub = defineComponent({
  props: {
    modelValue: { type: null, default: null },
    options: { type: Array, default: () => [] },
    placeholder: { type: String, default: '' },
    multiple: { type: Boolean, default: false }
  },
  template: '<div class="select-stub" :data-placeholder="placeholder"></div>'
})

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        PlatformIcon: true,
        Icon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: true
      }
    }
  })
}

const copyAccountsOptionLabels = (wrapper: ReturnType<typeof mountView>) => {
  const select = wrapper
    .findAllComponents(SelectStub)
    .find(
      (candidate) =>
        candidate.props('placeholder') === 'admin.groups.copyAccounts.selectPlaceholder'
    )
  expect(select).toBeDefined()
  return (select!.props('options') as { value: number; label: string }[]).map(
    (option) => option.label
  )
}

describe('GroupsView copy-accounts source groups', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    listGroups.mockResolvedValue({
      items: [currentPageGroup],
      total: 5,
      page: 1,
      page_size: 1,
      pages: 5
    })
    getAllIncludingInactive.mockResolvedValue([
      currentPageGroup,
      offPageGroup,
      offPageInactiveGroup,
      emptyGroup,
      otherPlatformGroup
    ])
    getModelsListCandidates.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
    getLiveCapability.mockResolvedValue({ supported: false })
  })

  it('编辑分组时列出全部分组，而不只是列表当页的分组', async () => {
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('common.edit'))
    expect(editButton).toBeDefined()
    await editButton!.trigger('click')
    await flushPromises()

    const labels = copyAccountsOptionLabels(wrapper)
    expect(labels.some((label) => label.includes('Off Page Group'))).toBe(true)
    expect(labels.some((label) => label.includes('Off Page Inactive Group'))).toBe(true)
    // 仍然排除自身、无账号的分组，以及平台不匹配的分组
    expect(labels.some((label) => label.includes('Page One Group'))).toBe(false)
    expect(labels.some((label) => label.includes('Off Page Empty Group'))).toBe(false)
    expect(labels.some((label) => label.includes('Off Page OpenAI Group'))).toBe(false)

    wrapper.unmount()
  })
})
