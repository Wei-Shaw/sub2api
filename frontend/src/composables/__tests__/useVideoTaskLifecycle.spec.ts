import { effectScope } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { preferredVideoTaskAsset, useVideoTaskLifecycle } from '../useVideoTaskLifecycle'
import type { VideoTask } from '@/api/admin/video'

const processingTask = { id: 1, status: 'running', delivery_status: 'processing' } as VideoTask
const archivingTask = { id: 1, status: 'succeeded', delivery_status: 'archiving' } as VideoTask
const deliverableTask = {
  id: 1,
  status: 'succeeded',
  delivery_status: 'deliverable',
  local_asset_available: true,
  result_url: 'https://example.invalid/remote.mp4',
} as VideoTask

async function flushAsync() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('useVideoTaskLifecycle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('uses a slower terminal poll while archiving and stops when deliverable', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(archivingTask)
      .mockResolvedValueOnce(deliverableTask)
    const scope = effectScope()
    const lifecycle = scope.run(() => useVideoTaskLifecycle<VideoTask>({
      fetch,
      autoStart: false,
      baseDelayMs: 10,
      terminalDelayMs: 50,
      maxDelayMs: 100,
    }))!

    await lifecycle.start()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(lifecycle.isPolling.value).toBe(true)
    await vi.advanceTimersByTimeAsync(49)
    expect(fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    await flushAsync()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(lifecycle.isPolling.value).toBe(false)
    scope.stop()
  })

  it('keeps one request in flight and coalesces concurrent refreshes', async () => {
    let resolveFirst!: (task: VideoTask) => void
    const first = new Promise<VideoTask>((resolve) => { resolveFirst = resolve })
    const fetch = vi.fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce(deliverableTask)
    const scope = effectScope()
    const lifecycle = scope.run(() => useVideoTaskLifecycle<VideoTask>({ fetch, autoStart: false }))!

    const starting = lifecycle.start()
    void lifecycle.refresh()
    void lifecycle.refresh()
    expect(fetch).toHaveBeenCalledTimes(1)
    resolveFirst(processingTask)
    await starting
    await flushAsync()
    expect(fetch).toHaveBeenCalledTimes(2)
    scope.stop()
  })

  it('backs off retries exponentially and caps the delay', async () => {
    const fetch = vi.fn()
      .mockRejectedValueOnce(new Error('one'))
      .mockRejectedValueOnce(new Error('two'))
      .mockRejectedValueOnce(new Error('three'))
      .mockResolvedValueOnce(deliverableTask)
    const scope = effectScope()
    const lifecycle = scope.run(() => useVideoTaskLifecycle<VideoTask>({
      fetch,
      autoStart: false,
      baseDelayMs: 10,
      maxDelayMs: 25,
    }))!

    await lifecycle.start()
    expect(fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(10)
    expect(fetch).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(20)
    expect(fetch).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(24)
    expect(fetch).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetch).toHaveBeenCalledTimes(4)
    scope.stop()
  })

  it('pauses while hidden and refreshes when visible again', async () => {
    let visibility: DocumentVisibilityState = 'hidden'
    vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visibility)
    const fetch = vi.fn().mockResolvedValue(deliverableTask)
    const scope = effectScope()
    const lifecycle = scope.run(() => useVideoTaskLifecycle<VideoTask>({ fetch, autoStart: false }))!

    await lifecycle.start()
    expect(fetch).not.toHaveBeenCalled()
    visibility = 'visible'
    document.dispatchEvent(new Event('visibilitychange'))
    await flushAsync()
    expect(fetch).toHaveBeenCalledTimes(1)
    scope.stop()
  })

  it('aborts an in-flight request on scope disposal', async () => {
    let observedSignal: AbortSignal | undefined
    const fetch = vi.fn((signal: AbortSignal) => {
      observedSignal = signal
      return new Promise<VideoTask>((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
      })
    })
    const scope = effectScope()
    scope.run(() => useVideoTaskLifecycle<VideoTask>({ fetch }))
    await flushAsync()
    expect(observedSignal?.aborted).toBe(false)
    scope.stop()
    expect(observedSignal?.aborted).toBe(true)
  })

  it('prefers a local asset and never falls back after delivery failure', () => {
    expect(preferredVideoTaskAsset(deliverableTask)).toEqual({ kind: 'local', url: '/video/tasks/1/local-asset' })
    expect(preferredVideoTaskAsset({
      ...deliverableTask,
      local_asset_available: false,
      delivery_status: 'delivery_failed',
    })).toEqual({ kind: 'unavailable', url: '' })
  })
})
