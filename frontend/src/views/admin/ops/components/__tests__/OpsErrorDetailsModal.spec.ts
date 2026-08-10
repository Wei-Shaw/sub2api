import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpsErrorDetailsModal from '../OpsErrorDetailsModal.vue'

const { listRequestErrors, listUpstreamErrors } = vi.hoisted(() => ({
  listRequestErrors: vi.fn(),
  listUpstreamErrors: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listRequestErrors,
    listUpstreamErrors
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('OpsErrorDetailsModal failure scopes', () => {
  beforeEach(() => {
    listRequestErrors.mockReset().mockResolvedValue({ items: [], total: 0 })
    listUpstreamErrors.mockReset().mockResolvedValue({ items: [], total: 0 })
  })

  it('uses the exact platform-failure view for the platform card', async () => {
    const wrapper = shallowMount(OpsErrorDetailsModal, {
      props: {
        show: false,
        timeRange: '1h',
        errorType: 'request',
        failureScope: 'platform'
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(listRequestErrors).toHaveBeenCalled()
    expect(listRequestErrors.mock.calls.at(-1)?.[0]).toMatchObject({
      view: 'platform_failures'
    })
    expect(listUpstreamErrors).not.toHaveBeenCalled()
  })

  it('keeps all provider phases in the provider card drill-down', async () => {
    const wrapper = shallowMount(OpsErrorDetailsModal, {
      props: {
        show: false,
        timeRange: '1h',
        errorType: 'upstream',
        failureScope: 'provider'
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(listUpstreamErrors).toHaveBeenCalled()
    expect(listUpstreamErrors.mock.calls.at(-1)?.[0]).toMatchObject({
      view: 'provider_failures'
    })
    expect(listUpstreamErrors.mock.calls.at(-1)?.[0]).not.toHaveProperty('phase')
    expect(listRequestErrors).not.toHaveBeenCalled()
  })
})
