import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { GroupsStatusResponse } from '@/api/groupsStatus'

const { fetchPublicSettings, getGroupsStatus } = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn(),
  getGroupsStatus: vi.fn()
}))

vi.mock('@/api/groupsStatus', () => ({ getGroupsStatus }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ fetchPublicSettings })
}))

import GroupsStatusView from '../GroupsStatusView.vue'

const GroupsStatusContentStub = defineComponent({
  name: 'GroupsStatusContent',
  props: {
    response: { type: Object, default: null },
    loading: { type: Boolean, required: true },
    error: { type: Boolean, default: false },
    lastUpdatedAt: { type: Date, default: null }
  },
  emits: ['retry'],
  template: `
    <section data-testid="groups-status-content">
      <span data-testid="loading-state">{{ String(loading) }}</span>
      <span data-testid="error-state">{{ String(error) }}</span>
      <span data-testid="group-count">{{ response ? response.summary.group_count : 'none' }}</span>
      <span data-testid="updated-state">{{ lastUpdatedAt ? 'set' : 'none' }}</span>
      <button data-testid="retry-status" type="button" @click="$emit('retry')">Retry</button>
    </section>
  `
})

function responseWithGroupCount(groupCount: number): GroupsStatusResponse {
  return {
    groups: [],
    summary: {
      group_count: groupCount,
      available_group_count: groupCount,
      account_count: groupCount,
      available_account_count: groupCount,
      rate_limited_account_count: 0
    }
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function mountView() {
  return mount(GroupsStatusView, {
    global: {
      stubs: {
        PlazaNavBar: { template: '<nav />' },
        GroupsStatusContent: GroupsStatusContentStub
      }
    }
  })
}

describe('GroupsStatusView request lifecycle', () => {
  beforeEach(() => {
    fetchPublicSettings.mockReset()
    getGroupsStatus.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads the initial public status successfully', async () => {
    getGroupsStatus.mockResolvedValueOnce(responseWithGroupCount(4))

    const wrapper = mountView()
    await flushPromises()

    expect(fetchPublicSettings).toHaveBeenCalledTimes(1)
    expect(getGroupsStatus).toHaveBeenCalledTimes(1)
    expect(getGroupsStatus.mock.calls[0][0].signal).toBeInstanceOf(AbortSignal)
    expect(wrapper.get('[data-testid="group-count"]').text()).toBe('4')
    expect(wrapper.get('[data-testid="loading-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="error-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="updated-state"]').text()).toBe('set')

    wrapper.unmount()
  })

  it('recovers from a failed request when retry succeeds', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    getGroupsStatus
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockResolvedValueOnce(responseWithGroupCount(2))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="group-count"]').text()).toBe('none')
    expect(wrapper.get('[data-testid="loading-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="error-state"]').text()).toBe('true')

    await wrapper.get('[data-testid="retry-status"]').trigger('click')
    await flushPromises()

    expect(getGroupsStatus).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="group-count"]').text()).toBe('2')
    expect(wrapper.get('[data-testid="loading-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="error-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="updated-state"]').text()).toBe('set')

    wrapper.unmount()
  })

  it('ignores a late failure from an aborted request after a newer request succeeds', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const firstRequest = deferred<GroupsStatusResponse>()
    const secondRequest = deferred<GroupsStatusResponse>()
    let firstSignal: AbortSignal | undefined
    getGroupsStatus
      .mockImplementationOnce((options?: { signal?: AbortSignal }) => {
        firstSignal = options?.signal
        return firstRequest.promise
      })
      .mockImplementationOnce(() => secondRequest.promise)

    const wrapper = mountView()
    await wrapper.get('[data-testid="retry-status"]').trigger('click')

    expect(firstSignal?.aborted).toBe(true)
    secondRequest.resolve(responseWithGroupCount(7))
    await flushPromises()

    expect(wrapper.get('[data-testid="group-count"]').text()).toBe('7')
    expect(wrapper.get('[data-testid="loading-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="error-state"]').text()).toBe('false')

    firstRequest.reject(new Error('late abort failure'))
    await flushPromises()

    expect(wrapper.get('[data-testid="group-count"]').text()).toBe('7')
    expect(wrapper.get('[data-testid="loading-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="error-state"]').text()).toBe('false')
    expect(consoleError).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('ignores a stale success when the aborted transport does not honor cancellation', async () => {
    const firstRequest = deferred<GroupsStatusResponse>()
    const secondRequest = deferred<GroupsStatusResponse>()
    getGroupsStatus
      .mockImplementationOnce(() => firstRequest.promise)
      .mockImplementationOnce(() => secondRequest.promise)

    const wrapper = mountView()
    await wrapper.get('[data-testid="retry-status"]').trigger('click')

    secondRequest.resolve(responseWithGroupCount(8))
    await flushPromises()
    firstRequest.resolve(responseWithGroupCount(1))
    await flushPromises()

    expect(wrapper.get('[data-testid="group-count"]').text()).toBe('8')
    expect(wrapper.get('[data-testid="loading-state"]').text()).toBe('false')
    expect(wrapper.get('[data-testid="error-state"]').text()).toBe('false')

    wrapper.unmount()
  })

  it('aborts the active request when the view unmounts', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const pendingRequest = deferred<GroupsStatusResponse>()
    let requestSignal: AbortSignal | undefined
    getGroupsStatus.mockImplementationOnce((options?: { signal?: AbortSignal }) => {
      requestSignal = options?.signal
      return pendingRequest.promise
    })

    const wrapper = mountView()
    expect(requestSignal?.aborted).toBe(false)

    wrapper.unmount()
    expect(requestSignal?.aborted).toBe(true)

    pendingRequest.reject(new Error('request canceled'))
    await flushPromises()
    expect(consoleError).not.toHaveBeenCalled()
  })
})
