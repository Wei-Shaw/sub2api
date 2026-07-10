import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

import VideoTasksView from '../VideoTasksView.vue'

const { listTasks, loadUsdCnyRate } = vi.hoisted(() => ({
  listTasks: vi.fn(),
  loadUsdCnyRate: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/api/admin/video', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/admin/video')>()
  return {
    ...actual,
    videoTaskAPI: {
      ...actual.videoTaskAPI,
      list: (...args: unknown[]) => listTasks(...args),
    },
  }
})

vi.mock('@/composables/useAdminDisplayCurrencyRate', () => ({
  useAdminDisplayCurrencyRate: () => ({
    usdCnyRate: { value: 7.2 },
    loadUsdCnyRate,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isAdmin: true,
  }),
}))

vi.mock('@/utils/productMode', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/productMode')>()
  return {
    ...actual,
    isVideoGatewayDemoMode: false,
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({ push: vi.fn() }),
    RouterLink: { template: '<a><slot /></a>' },
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

function runningTaskFixture() {
  return {
    id: 42,
    provider_account_id: 1,
    provider_account_name: 'demo-account',
    provider: 'mock',
    model: 'mock-video-v1',
    task_type: 'text_to_video',
    prompt: 'test prompt',
    status: 'running',
    result_url: '',
    error_message: '',
    cost_estimate: 0,
    created_by: 1,
    created_by_email: 'admin@example.com',
    created_by_name: 'Admin',
    created_by_label: 'Admin',
    created_at: '2026-07-08T00:00:00Z',
    updated_at: '2026-07-08T00:00:00Z',
    completed_at: null,
    negative_prompt: '',
    reference_image_url: '',
    reference_video_url: '',
    aspect_ratio: '16:9',
    duration: 5,
    resolution: '1080p',
  }
}

describe('VideoTasksView polling', () => {
  let wrapper: VueWrapper | null = null

  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    listTasks.mockResolvedValue({
      items: [runningTaskFixture()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    vi.useRealTimers()
  })

  it('polls list API while non-terminal tasks are visible', async () => {
    wrapper = mount(VideoTasksView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          Icon: true,
        },
      },
    })

    await flushPromises()
    expect(listTasks).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()

    expect(listTasks.mock.calls.length).toBeGreaterThan(1)
  })

  it('stops polling once every visible task is deliverable', async () => {
    listTasks
      .mockResolvedValueOnce({ items: [{ ...runningTaskFixture(), status: 'succeeded', delivery_status: 'deliverable', local_asset_available: true }], total: 1, page: 1, page_size: 20, pages: 1 })
    wrapper = mount(VideoTasksView, {
      global: { stubs: { AppLayout: AppLayoutStub, Icon: true } },
    })
    await flushPromises()
    expect(listTasks).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(20_000)
    expect(listTasks).toHaveBeenCalledTimes(1)
  })

  it('labels a remote deliverable without claiming it is locally archived', async () => {
    listTasks.mockResolvedValueOnce({
      items: [{ ...runningTaskFixture(), status: 'succeeded', delivery_status: 'deliverable', next_action: 'download', local_asset_available: false, result_url: 'https://example.invalid/result.mp4' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    wrapper = mount(VideoTasksView, {
      global: { stubs: { AppLayout: AppLayoutStub, Icon: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('结果可下载')
    expect(wrapper.text()).not.toContain('本地资产可下载')
  })
})
