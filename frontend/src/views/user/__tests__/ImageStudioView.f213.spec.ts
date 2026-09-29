import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import ImageStudioView from '../ImageStudioView.vue'
import type { ImageStudioJob } from '@/api/imageStudio'

const { f213Api } = vi.hoisted(() => ({
  f213Api: {
    listJobs: vi.fn(),
    createJob: vi.fn(),
    deleteJob: vi.fn(),
    listAssets: vi.fn(),
    deleteAsset: vi.fn(),
    fetchAssetBlob: vi.fn()
  }
}))

vi.mock('@/api/imageStudio', () => ({ imageStudioAPI: f213Api }))
vi.mock('@/api/keys', () => ({ keysAPI: { list: vi.fn().mockResolvedValue({ items: [] }) } }))
vi.mock('@/api/batchImage', () => ({ saveBlob: vi.fn() }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key, locale: ref('en') })
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

const f213RunningJob: ImageStudioJob = {
  id: 1,
  api_key_id: 1,
  status: 'running',
  kind: 'generate',
  model: 'gpt-image-2',
  prompt: 'cat',
  params: {},
  image_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  assets: []
}

function f213Page() {
  return { items: [f213RunningJob], total: 1, page: 1, page_size: 20, pages: 1 }
}

function f213Deferred() {
  let resolve!: (value: ReturnType<typeof f213Page>) => void
  const promise = new Promise<ReturnType<typeof f213Page>>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

let f213Wrapper: ReturnType<typeof shallowMount> | undefined

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  f213Wrapper?.unmount()
  f213Wrapper = undefined
  vi.clearAllMocks()
  vi.useRealTimers()
})

describe('ImageStudioView polling after unmount (F2-13)', () => {
  it('stops polling when unmounted while a poll request is in flight', async () => {
    const pending = f213Deferred()
    f213Api.listJobs.mockResolvedValueOnce(f213Page()).mockReturnValueOnce(pending.promise).mockResolvedValue(f213Page())

    f213Wrapper = shallowMount(ImageStudioView, { global: { stubs: { 'router-link': true } } })
    await flushPromises()
    expect(f213Api.listJobs).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(2500)
    expect(f213Api.listJobs).toHaveBeenCalledTimes(2)

    f213Wrapper.unmount()
    f213Wrapper = undefined
    pending.resolve(f213Page())
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)

    expect(f213Api.listJobs).toHaveBeenCalledTimes(2)
  })

  it('does not start polling when unmounted before the initial load finishes', async () => {
    const pending = f213Deferred()
    f213Api.listJobs.mockReturnValueOnce(pending.promise).mockResolvedValue(f213Page())

    f213Wrapper = shallowMount(ImageStudioView, { global: { stubs: { 'router-link': true } } })
    f213Wrapper.unmount()
    f213Wrapper = undefined
    pending.resolve(f213Page())
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)

    expect(f213Api.listJobs).toHaveBeenCalledTimes(1)
  })
})
