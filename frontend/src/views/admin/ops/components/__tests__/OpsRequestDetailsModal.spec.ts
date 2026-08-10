import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'

import OpsRequestDetailsModal from '../OpsRequestDetailsModal.vue'

const { listRequestDetails, listUsage } = vi.hoisted(() => ({
  listRequestDetails: vi.fn(),
  listUsage: vi.fn()
}))

vi.mock('@/api/admin/ops', () => {
  const opsAPI = { listRequestDetails }
  return { opsAPI, default: opsAPI }
})

vi.mock('@/api/admin/usage', () => {
  const adminUsageAPI = { list: listUsage }
  return { adminUsageAPI, default: adminUsageAPI }
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
    listUsage.mockReset().mockResolvedValue({
      items: [{
        id: 101,
        user_id: 8,
        request_id: 'req-usage-test',
        model: 'gpt-test',
        first_token_ms: 1234,
        duration_ms: 4567,
        created_at: '2026-08-02T12:00:00Z',
        user: { email: 'user@example.com' }
      }],
      total: 1,
      page: 1,
      page_size: 10,
      pages: 1
    })
  })

  it('uses the full admin usage table for TTFT details and preserves TTFT ordering', async () => {
    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: '1h',
        preset: { title: 'TTFT', kind: 'success', sort: 'ttft_desc', has_ttft: true },
        platform: 'openai',
        groupId: 9
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Pagination: true,
          UsageTable: {
            props: ['data', 'columns', 'userClickable'],
            template: '<div data-testid="usage-table">{{ data.length }}:{{ data[0] && data[0].first_token_ms }}:{{ userClickable }}</div>'
          }
        }
      }
    })

    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(listUsage).toHaveBeenLastCalledWith(expect.objectContaining({
      platform: 'openai',
      group_id: 9,
      has_ttft: true,
      exact_total: true,
      sort_by: 'first_token_ms',
      sort_order: 'desc',
      start_time: expect.any(String),
      end_time: expect.any(String)
    }))
    expect(listRequestDetails).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="usage-table"]').text()).toBe('1:1234:false')

    await wrapper.get('[data-testid="ops-request-sort"]').setValue('ttft_asc')
    await flushPromises()

    expect(listUsage).toHaveBeenLastCalledWith(expect.objectContaining({
      sort_by: 'first_token_ms',
      sort_order: 'asc'
    }))
  })

  it('keeps non-TTFT drilldowns on the ops request details API', async () => {
    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: '1h',
        preset: { title: 'Slow requests', kind: 'success', sort: 'duration_desc' }
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

    expect(listRequestDetails).toHaveBeenCalledWith(expect.objectContaining({
      kind: 'success',
      sort: 'duration_desc'
    }))
    expect(listUsage).not.toHaveBeenCalled()
  })

  it('requests only SLA failures for platform availability details', async () => {
    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: '1h',
        preset: { title: 'Availability', kind: 'error', slaOnly: true }
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

    expect(listRequestDetails).toHaveBeenCalledWith(expect.objectContaining({
      kind: 'error',
      sla_only: true
    }))
  })
})
