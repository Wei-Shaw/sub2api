import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

import VideoTaskDetailView from '../VideoTaskDetailView.vue'

const { getTask, cancelTask, loadUsdCnyRate } = vi.hoisted(() => ({
  getTask: vi.fn(),
  cancelTask: vi.fn(),
  loadUsdCnyRate: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/api/admin/video', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/admin/video')>()
  return {
    ...actual,
    videoTaskAPI: {
      ...actual.videoTaskAPI,
      get: (...args: unknown[]) => getTask(...args),
      cancel: (...args: unknown[]) => cancelTask(...args),
      getLocalAssetBlob: vi.fn(),
    },
  }
})

vi.mock('@/composables/useAdminDisplayCurrencyRate', () => ({
  useAdminDisplayCurrencyRate: () => ({ usdCnyRate: { value: 7.2 }, loadUsdCnyRate }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showInfo: vi.fn(), showSuccess: vi.fn() }),
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAdmin: true }) }))

vi.mock('@/utils/productMode', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/productMode')>()
  return { ...actual, isVideoGatewayDemoMode: false }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => ({ params: { id: '42' } }),
    useRouter: () => ({ push: vi.fn() }),
    RouterLink: { template: '<a><slot /></a>' },
  }
})

const task = (delivery_status: 'archiving' | 'deliverable') => ({
  id: 42,
  provider_account_id: 1,
  provider_account_name: 'demo-account',
  provider: 'mock',
  model: 'mock-video-v1',
  task_type: 'text_to_video',
  prompt: 'test prompt',
  status: 'succeeded',
  dispatch_state: 'accepted',
  settlement_status: 'settled',
  archive_status: delivery_status === 'archiving' ? 'pending' : 'succeeded',
  capture_status: 'succeeded',
  delivery_status,
  next_action: delivery_status === 'archiving' ? 'archive' : 'download',
  result_url: delivery_status === 'archiving' ? 'https://example.invalid/result.mp4' : '',
  local_asset_available: delivery_status === 'deliverable',
  error_message: '',
  cost_estimate: 0,
  created_by: 1,
  created_by_email: 'admin@example.com',
  created_by_name: 'Admin',
  created_by_label: 'Admin',
  created_at: '2026-07-08T00:00:00Z',
  updated_at: '2026-07-08T00:00:00Z',
  completed_at: '2026-07-08T00:01:00Z',
  negative_prompt: '',
  reference_image_url: '',
  reference_video_url: '',
  aspect_ratio: '16:9',
  duration: 5,
  resolution: '1080p',
  events: [],
})

describe('VideoTaskDetailView delivery lifecycle', () => {
  let wrapper: VueWrapper | null = null

  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    getTask.mockResolvedValueOnce(task('archiving')).mockResolvedValueOnce(task('deliverable'))
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    vi.useRealTimers()
  })

  it('renders archive progress, then stops when local asset is deliverable', async () => {
    wrapper = mount(VideoTaskDetailView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    expect(getTask).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('生成完成，归档中')

    await vi.advanceTimersByTimeAsync(15_000)
    await flushPromises()
    expect(getTask).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('本地资产可下载')

    await vi.advanceTimersByTimeAsync(60_000)
    expect(getTask).toHaveBeenCalledTimes(2)
  })

  it('shows the real initial fetch failure before a task exists', async () => {
    getTask.mockReset().mockRejectedValue(new Error('task fetch unavailable'))
    wrapper = mount(VideoTaskDetailView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('task fetch unavailable')
  })

  it('labels a remote deliverable without claiming that a local asset exists', async () => {
    getTask.mockReset().mockResolvedValue({
      ...task('deliverable'),
      local_asset_available: false,
      result_url: 'https://example.invalid/result.mp4',
    })
    wrapper = mount(VideoTaskDetailView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('结果可下载')
    expect(wrapper.text()).not.toContain('本地资产可下载')
  })
})
