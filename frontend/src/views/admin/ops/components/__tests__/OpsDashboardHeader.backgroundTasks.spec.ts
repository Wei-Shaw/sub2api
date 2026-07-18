import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { defineComponent } from 'vue'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'
import { backgroundTasksAPI, type BackgroundTask } from '@/api/admin/backgroundTasks'

vi.mock('@/api', () => ({
  adminAPI: {
    groups: { getAll: vi.fn().mockResolvedValue([]) },
    settings: { getSettings: vi.fn().mockResolvedValue({}) },
    payment: { getConfig: vi.fn().mockResolvedValue({ data: { enabled: false } }) },
  },
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRealtimeTrafficSummary: vi.fn().mockResolvedValue({ enabled: true, summary: null }),
  },
}))

vi.mock('@/api/admin/backgroundTasks', () => ({
  backgroundTasksAPI: {
    list: vi.fn(),
    cancel: vi.fn(),
    retry: vi.fn(),
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /></div>',
})

function makeTask(overrides: Partial<BackgroundTask> = {}): BackgroundTask {
  return {
    id: 1,
    task_type: 'openai_quota_reset',
    resource_type: 'openai_account',
    resource_id: '42',
    account_id: 42,
    account_name: 'codex-account',
    credit_expires_at: '2030-01-01T02:00:00Z',
    run_at: '2030-01-01T01:00:00Z',
    status: 'pending',
    attempt_count: 1,
    dispatch_count: 0,
    can_cancel: true,
    can_retry: false,
    created_at: '2030-01-01T00:00:00Z',
    updated_at: '2030-01-01T00:00:00Z',
    ...overrides,
  }
}

function mountHeader() {
  return mount(OpsDashboardHeader, {
    props: {
      platform: '',
      groupId: null,
      timeRange: '1h',
      queryMode: 'auto',
      loading: false,
      lastUpdated: null,
      overview: {},
    },
    global: {
      plugins: [createPinia()],
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: true,
        HelpTooltip: true,
        Icon: true,
      },
    },
  })
}

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
})

describe('OpsDashboardHeader background task instances', () => {
  it('lists task attempts and exposes only valid cancel or retry actions', async () => {
    const pending = makeTask({ id: 11, attempt_count: 2, dispatch_count: 1 })
    const indeterminate = makeTask({
      id: 12,
      account_name: 'uncertain-account',
      status: 'indeterminate',
      attempt_count: 5,
      dispatch_count: 5,
      can_cancel: false,
      can_retry: true,
      last_error_message: 'timeout',
    })
    vi.mocked(backgroundTasksAPI.list).mockResolvedValue({
      items: [pending, indeterminate], total: 2, page: 1, page_size: 10, pages: 1,
    })
    vi.mocked(backgroundTasksAPI.cancel).mockResolvedValue(makeTask({ id: 11, status: 'canceled', can_cancel: false }))
    vi.mocked(backgroundTasksAPI.retry).mockResolvedValue(makeTask({ id: 12, status: 'pending', can_cancel: false }))

    const wrapper = mountHeader()
    await flushPromises()
    await wrapper.get('[data-testid="ops-jobs-details"]').trigger('click')
    await wrapper.get('[data-testid="ops-background-tasks-tab"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('codex-account')
    expect(wrapper.text()).toContain('uncertain-account')
    expect(wrapper.text()).toContain('2 / 1')
    expect(wrapper.text()).toContain('5 / 5')
    expect(wrapper.find('[data-testid="cancel-background-task-12"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="retry-background-task-11"]').exists()).toBe(false)

    await wrapper.get('[data-testid="cancel-background-task-11"]').trigger('click')
    await flushPromises()
    expect(backgroundTasksAPI.cancel).toHaveBeenCalledWith(11)

    await wrapper.get('[data-testid="retry-background-task-12"]').trigger('click')
    await flushPromises()
    expect(backgroundTasksAPI.retry).toHaveBeenCalledWith(12)
    wrapper.unmount()
  })

  it('requests the next task page through the generic list API', async () => {
    vi.mocked(backgroundTasksAPI.list).mockImplementation(async (params = {}) => ({
      items: [makeTask({ id: params.page === 2 ? 22 : 21, account_name: params.page === 2 ? 'page-two' : 'page-one' })],
      total: 11,
      page: params.page ?? 1,
      page_size: 10,
      pages: 2,
    }))
    const wrapper = mountHeader()
    await flushPromises()
    await wrapper.get('[data-testid="ops-jobs-details"]').trigger('click')
    await wrapper.get('[data-testid="ops-background-tasks-tab"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('page-one')

    await wrapper.get('[data-testid="background-task-page-next"]').trigger('click')
    await flushPromises()
    expect(backgroundTasksAPI.list).toHaveBeenLastCalledWith({ page: 2, page_size: 10 })
    expect(wrapper.text()).toContain('page-two')
    wrapper.unmount()
  })

  it('refreshes task state after a cancel conflict', async () => {
    const pending = makeTask({ id: 31, status: 'pending', can_cancel: true })
    const running = makeTask({ id: 31, status: 'running', can_cancel: false, dispatch_count: 1 })
    vi.mocked(backgroundTasksAPI.list)
      .mockResolvedValueOnce({ items: [pending], total: 1, page: 1, page_size: 10, pages: 1 })
      .mockResolvedValueOnce({ items: [pending], total: 1, page: 1, page_size: 10, pages: 1 })
      .mockResolvedValueOnce({ items: [running], total: 1, page: 1, page_size: 10, pages: 1 })
    vi.mocked(backgroundTasksAPI.cancel).mockRejectedValue(new Error('dispatch already started'))
    const wrapper = mountHeader()
    await flushPromises()
    await wrapper.get('[data-testid="ops-jobs-details"]').trigger('click')
    await wrapper.get('[data-testid="ops-background-tasks-tab"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="cancel-background-task-31"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('dispatch already started')
    expect(wrapper.find('[data-testid="cancel-background-task-31"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.ops.backgroundTasks.statuses.running')
    wrapper.unmount()
  })
})
