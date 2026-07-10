import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'

import VideoCreateTaskView from '../VideoCreateTaskView.vue'
import VideoTaskDetailView from '../VideoTaskDetailView.vue'

const { listProviders, createTask, getTask, forbiddenNetwork } = vi.hoisted(() => ({
  listProviders: vi.fn(),
  createTask: vi.fn(),
  getTask: vi.fn(),
  forbiddenNetwork: vi.fn(() => {
    throw new Error('external network is forbidden in VideoReliabilityFlow')
  }),
}))

vi.mock('@/api/admin/video', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/admin/video')>()
  return {
    ...actual,
    videoTaskAPI: {
      ...actual.videoTaskAPI,
      listProviders: (...args: unknown[]) => listProviders(...args),
      create: (...args: unknown[]) => createTask(...args),
      get: (...args: unknown[]) => getTask(...args),
      cancel: vi.fn(),
      getLocalAssetBlob: vi.fn(),
    },
  }
})

vi.mock('@/utils/productMode', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/productMode')>()
  return { ...actual, isVideoGatewayDemoMode: false }
})

const provider = {
  id: 1,
  provider: 'mock' as const,
  display_name: 'Local mock provider',
  enabled: true,
  api_key_configured: false,
  masked_key: '',
  base_url: 'mock://video',
  default_model: 'mock-video-v1',
  rate_limit_per_minute: 60,
  metadata_json: {},
  key_status: 'normal',
  health_status: 'healthy',
  diagnostic_type: '',
  suggested_action: '',
  priority: 1,
  current_inflight: 0,
  today_tasks: 0,
  today_failures: 0,
  last_error: '',
  last_test_at: '',
  route_available: true,
  route_skip_reason: '',
  created_at: '2026-07-10T00:00:00Z',
  updated_at: '2026-07-10T00:00:00Z',
}

function task(overrides: Record<string, unknown> = {}) {
  return {
    id: 88,
    provider_account_id: 1,
    provider_account_name: 'Local mock provider',
    provider: 'mock' as const,
    model: 'mock-video-v1',
    task_type: 'text_to_video' as const,
    prompt: 'local reliability proof',
    status: 'submitted' as const,
    dispatch_state: 'accepted',
    settlement_status: 'not_required',
    archive_status: 'pending',
    capture_status: 'pending',
    delivery_status: 'processing' as const,
    next_action: 'poll',
    result_url: '',
    local_asset_available: false,
    error_message: '',
    cost_estimate: 0,
    created_by: 7,
    created_by_email: 'local@example.invalid',
    created_by_name: 'Local proof',
    created_by_label: 'Local proof',
    created_at: '2026-07-10T00:00:00Z',
    updated_at: '2026-07-10T00:00:01Z',
    completed_at: null,
    negative_prompt: '',
    reference_image_url: '',
    reference_video_url: '',
    aspect_ratio: '16:9',
    duration: 5,
    resolution: '720p',
    upstream_task_id: 'mock-video-88',
    routing_strategy: 'mock-only',
    routing_reason: 'local test adapter',
    events: [],
    ...overrides,
  }
}

async function mountFlow(initialPath: string): Promise<{ wrapper: VueWrapper; router: ReturnType<typeof createRouter> }> {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/video/create', component: VideoCreateTaskView },
      { path: '/admin/video/tasks', component: { template: '<div>tasks</div>' } },
      { path: '/admin/video/tasks/:id', component: VideoTaskDetailView },
    ],
  })
  await router.push(initialPath)
  await router.isReady()
  const wrapper = mount(RouterView, {
    global: {
      plugins: [pinia, router],
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: true,
      },
    },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('VideoReliabilityFlow local API proof', () => {
  let wrapper: VueWrapper | null = null

  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    localStorage.clear()
    vi.stubGlobal('fetch', forbiddenNetwork)
    listProviders.mockResolvedValue({ items: [provider] })
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('creates, polls, renders archiving, then stops on a local deliverable', async () => {
    createTask.mockResolvedValue(task({ status: 'queued', dispatch_state: 'pending' }))
    getTask
      .mockResolvedValueOnce(task())
      .mockResolvedValueOnce(task({ status: 'running' }))
      .mockResolvedValueOnce(task({
        status: 'succeeded',
        delivery_status: 'archiving',
        next_action: 'archive',
        result_url: '/api/v1/video/mock-assets/88.svg',
        completed_at: '2026-07-10T00:00:03Z',
      }))
      .mockResolvedValueOnce(task({
        status: 'succeeded',
        archive_status: 'succeeded',
        capture_status: 'succeeded',
        delivery_status: 'deliverable',
        next_action: 'download',
        local_asset_available: true,
        result_url: '',
        completed_at: '2026-07-10T00:00:03Z',
      }))

    const mounted = await mountFlow('/admin/video/create')
    wrapper = mounted.wrapper
    expect(listProviders).toHaveBeenCalledTimes(1)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(createTask).toHaveBeenCalledTimes(1)
    expect(mounted.router.currentRoute.value.fullPath).toBe('/admin/video/tasks/88')
    expect(getTask).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(2_000)
    await flushPromises()
    expect(getTask).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(2_000)
    await flushPromises()
    expect(getTask).toHaveBeenCalledTimes(3)
    expect(wrapper.text()).toContain('生成完成，归档中')

    await vi.advanceTimersByTimeAsync(15_000)
    await flushPromises()
    expect(getTask).toHaveBeenCalledTimes(4)
    expect(wrapper.text()).toContain('本地资产可下载')

    await vi.advanceTimersByTimeAsync(60_000)
    expect(getTask).toHaveBeenCalledTimes(4)
    expect(forbiddenNetwork).not.toHaveBeenCalled()
  })

  it('renders the durable failure reason and does not retry a dead delivery', async () => {
    getTask.mockResolvedValue(task({
      id: 89,
      status: 'succeeded',
      archive_status: 'failed',
      capture_status: 'succeeded',
      delivery_status: 'delivery_failed',
      next_action: 'review_delivery',
      error_message: '本地归档重试已耗尽，暂无可交付资产。',
      completed_at: '2026-07-10T00:00:03Z',
    }))

    const mounted = await mountFlow('/admin/video/tasks/89')
    wrapper = mounted.wrapper
    expect(getTask).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('交付失败，请查看原因')
    expect(wrapper.text()).toContain('本地归档重试已耗尽，暂无可交付资产。')

    await vi.advanceTimersByTimeAsync(60_000)
    expect(getTask).toHaveBeenCalledTimes(1)
    expect(forbiddenNetwork).not.toHaveBeenCalled()
  })
})
