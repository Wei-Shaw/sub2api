import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

import OpsAlertEventsCard from '../OpsAlertEventsCard.vue'
import { ref } from 'vue'
const { listAlertEvents, showError } = vi.hoisted(() => ({ listAlertEvents: vi.fn(), showError: vi.fn() }))
vi.mock('@vueuse/core', () => ({ useMediaQuery: () => ref(true) }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { listAlertEvents } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError }) }))
const rows = (first: number, count = 10) => Array.from({ length: count }, (_, i) => ({
  id: first + i, severity: 'P1', status: 'firing', created_at: '2026-10-03T00:00:00Z',
}))
type View = { events: ReturnType<typeof rows>; loading: boolean; loadingMore: boolean; hasMore: boolean; loadMore: () => Promise<void> }
function mountCard() {
  const wrapper = shallowMount(OpsAlertEventsCard)
  return { wrapper, vm: wrapper.vm as unknown as View }
}

describe('alert events filtered pagination', () => {
  beforeEach(() => { vi.resetAllMocks() })

  it.each(['resolve', 'reject'] as const)('ignores a superseded first page that %ss', async outcome => {
    const old = deferred<ReturnType<typeof rows>>()
    listAlertEvents.mockReturnValueOnce(old.promise).mockResolvedValueOnce(rows(100))
    const { wrapper, vm } = mountCard()
    wrapper.findAllComponents({ name: 'Select' })[1].vm.$emit('change', 'P2')
    await flushPromises()
    if (outcome === 'resolve') old.resolve(rows(1))
    else old.reject(new Error('obsolete request'))
    await flushPromises()
    expect(vm.events.map(row => row.id)).toEqual(rows(100).map(row => row.id))
    expect(vm.hasMore).toBe(true)
    expect(showError).not.toHaveBeenCalled()
  })

  it.each(['resolve', 'reject'] as const)('ignores an old next page that %ss after changing filters', async outcome => {
    const old = deferred<ReturnType<typeof rows>>()
    const current = deferred<ReturnType<typeof rows>>()
    listAlertEvents.mockResolvedValueOnce(rows(1)).mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce(rows(100)).mockReturnValueOnce(current.promise)
    const { wrapper, vm } = mountCard()
    await flushPromises()
    const oldLoad = vm.loadMore()
    wrapper.findAllComponents({ name: 'Select' })[1].vm.$emit('change', 'P2')
    await flushPromises()
    expect(vm.loadingMore).toBe(false)
    const nextLoad = vm.loadMore()
    if (outcome === 'resolve') old.resolve(rows(11, 1))
    else old.reject(new Error('obsolete request'))
    await oldLoad
    expect(vm.loadingMore).toBe(true)
    expect(vm.events.map(row => row.id)).toEqual(rows(100).map(row => row.id))
    expect(vm.hasMore).toBe(true)
    current.resolve(rows(110, 1))
    await nextLoad
    expect(vm.events.map(row => row.id)).toEqual([...rows(100), ...rows(110, 1)].map(row => row.id))
    expect(vm.hasMore).toBe(false)
  })
})
