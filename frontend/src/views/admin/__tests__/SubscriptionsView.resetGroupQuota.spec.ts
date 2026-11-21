import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SubscriptionsView from '@/views/admin/SubscriptionsView.vue'

const { listSubscriptions, getAllGroups, resetGroupQuota, showSuccess, showError } = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  getAllGroups: vi.fn(),
  resetGroupQuota: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list: listSubscriptions,
      resetGroupQuota,
      assign: vi.fn(),
      extend: vi.fn(),
      revoke: vi.fn(),
      restore: vi.fn(),
      resetQuota: vi.fn()
    },
    groups: {
      getAll: getAllGroups
    },
    usage: {
      searchUsers: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const AppLayoutStub = defineComponent({
  template: '<main><slot /></main>'
})

const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>'
})

const ConfirmDialogStub = defineComponent({
  props: {
    show: { type: Boolean, default: false }
  },
  emits: ['confirm', 'cancel'],
  template: '<button v-if="show" data-testid="confirm-dialog" @click="$emit(\'confirm\')">confirm</button>'
})

function mountView() {
  return mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: true,
        Pagination: true,
        BaseDialog: true,
        ConfirmDialog: ConfirmDialogStub,
        EmptyState: true,
        Select: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Icon: true,
        RouterLink: true
      }
    }
  })
}

describe('SubscriptionsView group quota reset', () => {
  beforeEach(() => {
    localStorage.clear()
    listSubscriptions.mockReset()
    getAllGroups.mockReset()
    resetGroupQuota.mockReset()
    showSuccess.mockReset()
    showError.mockReset()

    listSubscriptions.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
    getAllGroups.mockResolvedValue([
      { id: 42, name: 'Team Plan', subscription_type: 'subscription', status: 'active' }
    ])
    resetGroupQuota.mockResolvedValue({
      total: 2,
      success: 2,
      failed: 0,
      failed_subscription_ids: [],
      errors: []
    })
  })

  it('shows the action for a selected group and resets all quota windows', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="reset-group-quota"]').exists()).toBe(false)
    ;(wrapper.vm as any).filters.group_id = '42'
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="reset-group-quota"]').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog"]').trigger('click')
    await flushPromises()

    expect(resetGroupQuota).toHaveBeenCalledWith(42, {
      daily: true,
      weekly: true,
      monthly: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.subscriptions.groupQuotaResetSuccess')
    expect(listSubscriptions).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
