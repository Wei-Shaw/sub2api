import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'

import OpsRequestDetailsModal from '../OpsRequestDetailsModal.vue'

const { listRequestDetails } = vi.hoisted(() => ({
  listRequestDetails: vi.fn()
}))

vi.mock('@/api/admin/ops', () => {
  const opsAPI = { listRequestDetails }
  return { opsAPI, default: opsAPI }
})

vi.mock('@vueuse/core', () => ({
  useMediaQuery: () => ({ value: true })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('OpsRequestDetailsModal', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    listRequestDetails.mockReset().mockResolvedValue({
      items: [{
        kind: 'success',
        created_at: '2026-08-02T12:00:00Z',
        request_id: 'req-test',
        platform: 'openai',
        model: 'gpt-test',
        first_token_ms: 1234,
        duration_ms: 4567,
        stream: true
      }],
      total: 1,
      page: 1,
      page_size: 10
    })
  })

  it('shows per-request TTFT and total duration in seconds and changes TTFT ordering', async () => {
    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: '1h',
        preset: { title: 'TTFT', kind: 'success', sort: 'ttft_desc', has_ttft: true }
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Pagination: true
        }
      }
    })

    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(listRequestDetails).toHaveBeenLastCalledWith(expect.objectContaining({
      kind: 'success',
      sort: 'ttft_desc',
      has_ttft: true
    }))
    expect(wrapper.text()).toContain('1.23 s')
    expect(wrapper.text()).toContain('4.57 s')

    await wrapper.get('[data-testid="ops-request-sort"]').setValue('ttft_asc')
    await flushPromises()

    expect(listRequestDetails).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'ttft_asc' }))
  })
})
