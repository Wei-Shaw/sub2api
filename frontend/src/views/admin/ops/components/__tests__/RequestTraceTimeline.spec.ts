import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RequestTraceTimeline from '../trace/RequestTraceTimeline.vue'
import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'

const mocks = vi.hoisted(() => ({
  getRequestTrace: vi.fn(),
  copyToClipboard: vi.fn(),
  getRequestErrorDetail: vi.fn()
}))

vi.mock('@/api/admin/opsTrace', () => ({
  getRequestTrace: mocks.getRequestTrace
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRequestErrorDetail: mocks.getRequestErrorDetail,
    getUpstreamErrorDetail: vi.fn(),
    listRequestErrorUpstreamErrors: vi.fn().mockResolvedValue({ items: [] })
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: mocks.copyToClipboard, copied: { value: false } })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}(${Object.entries(params).map(([k, v]) => `${k}=${v}`).join(',')})` : key
    })
  }
})

const globalOpts = { stubs: { Icon: true } }

function failoverTrace() {
  return {
    request_id: 'req-1',
    attempts: [
      {
        attempt_no: 2,
        account_id: 102,
        retry_from: 101,
        platform: 'anthropic',
        upstream_status: 429,
        failure_kind: 'rate_limit',
        error_code: 'rate_limit_error',
        error_msg: 'x'.repeat(200),
        credential_fp: 'bbbbbbbbbbbbbbbb',
        proxy_id: 7,
        created_at: '2026-11-10T10:00:01.500Z'
      },
      {
        attempt_no: 1,
        account_id: 101,
        platform: 'anthropic',
        upstream_status: 401,
        failure_kind: 'auth',
        error_code: 'authentication_error',
        error_msg: 'invalid x-api-key',
        credential_fp: 'aaaaaaaaaaaaaaaa',
        created_at: '2026-11-10T10:00:00.300Z'
      }
    ],
    status_events: [
      {
        account_id: 101,
        from_state: 'active',
        to_state: 'invalid',
        reason: 'upstream 401',
        source: 'classify',
        request_id: 'req-1',
        created_at: '2026-11-10T10:00:00.400Z'
      }
    ],
    error: {
      id: 9,
      error_phase: 'upstream',
      error_type: 'upstream_error',
      status_code: 429,
      upstream_status_code: 429,
      error_message: 'All available accounts exhausted',
      created_at: '2026-11-10T10:00:02Z'
    }
  }
}

describe('RequestTraceTimeline', () => {
  beforeEach(() => {
    mocks.getRequestTrace.mockReset()
    mocks.copyToClipboard.mockReset()
  })

  it('renders a 2-hop failover chain with a switch marker, kind colors and status event', async () => {
    mocks.getRequestTrace.mockResolvedValue(failoverTrace())
    const wrapper = mount(RequestTraceTimeline, { props: { requestId: 'req-1' }, global: globalOpts })
    await flushPromises()

    expect(mocks.getRequestTrace).toHaveBeenCalledWith('req-1')
    const hops = wrapper.findAll('[data-testid="trace-attempt"]')
    expect(hops).toHaveLength(2)
    // sorted by attempt_no
    expect(hops[0].text()).toContain('admin.traceRequest.attemptNo(n=1)')
    expect(hops[1].text()).toContain('admin.traceRequest.attemptNo(n=2)')

    const markers = wrapper.findAll('[data-testid="trace-switch-marker"]')
    expect(markers).toHaveLength(1)
    expect(markers[0].text()).toContain('admin.traceRequest.switchFromTo(from=101,to=102)')
    expect(wrapper.get('[data-testid="trace-switch-count"]').text()).toContain('n=1')

    const dots = wrapper.findAll('[data-kind]')
    expect(dots[0].attributes('data-kind')).toBe('auth')
    expect(dots[0].classes()).toContain('bg-red-500')
    expect(dots[1].attributes('data-kind')).toBe('rate_limit')
    expect(dots[1].classes()).toContain('bg-orange-500')

    expect(hops[0].text()).toContain('HTTP 401')
    expect(hops[0].text()).toContain('authentication_error')
    expect(hops[0].text()).toContain('aaaaaaaaaaaaaaaa')
    expect(hops[1].text()).toContain('#7')
    expect(hops[1].text()).toContain('1.20s')

    // status event attached under hop 1 (account 101)
    const events = hops[0].find('[data-testid="trace-status-events"]')
    expect(events.exists()).toBe(true)
    expect(events.text()).toContain('admin.traceRequest.statusEvent.invalid(id=101)')
    expect(hops[1].find('[data-testid="trace-status-events"]').exists()).toBe(false)

    // long error message folds and expands
    const msg = hops[1].get('[data-testid="trace-error-msg"]')
    expect(msg.classes()).toContain('line-clamp-2')
    await hops[1].get('[data-testid="trace-error-msg-toggle"]').trigger('click')
    expect(hops[1].get('[data-testid="trace-error-msg"]').classes()).not.toContain('line-clamp-2')

    // account id copy
    await hops[0].get('[data-testid="trace-account-id"]').trigger('click')
    expect(mocks.copyToClipboard).toHaveBeenCalledWith('101', expect.any(String))

    // final failure summary
    const failure = wrapper.get('[data-testid="trace-final-failure"]')
    expect(failure.text()).toContain('All available accounts exhausted')
    expect(wrapper.find('[data-testid="trace-final-success"]').exists()).toBe(false)
  })

  it('renders the success summary from usage', async () => {
    mocks.getRequestTrace.mockResolvedValue({
      request_id: 'req-ok',
      attempts: [],
      status_events: [],
      usage: {
        user_id: 1,
        api_key_id: 2,
        account_id: 300,
        model: 'claude-sonnet-5-5',
        input_tokens: 1200,
        output_tokens: 345,
        total_cost: 0.0123,
        duration_ms: 2345,
        first_token_ms: 410,
        created_at: '2026-11-10T10:00:00Z'
      }
    })
    const wrapper = mount(RequestTraceTimeline, { props: { requestId: 'req-ok' }, global: globalOpts })
    await flushPromises()

    const success = wrapper.get('[data-testid="trace-final-success"]')
    expect(success.text()).toContain('#300')
    expect(success.text()).toContain('claude-sonnet-5-5')
    expect(success.text()).toContain('1200 / 345')
    expect(success.text()).toContain('2.35s')
    expect(success.text()).toContain('410ms')
    expect(success.text()).toContain('$0.0123')
    expect(wrapper.find('[data-testid="trace-final-failure"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="trace-no-attempts"]').text()).toContain('admin.traceRequest.noAttemptsSuccess')
  })

  it('shows the empty state when the API returns 404 TRACE_NOT_FOUND', async () => {
    mocks.getRequestTrace.mockRejectedValue({ status: 404, code: 404, reason: 'TRACE_NOT_FOUND', message: 'request trace not found' })
    const wrapper = mount(RequestTraceTimeline, { props: { requestId: 'old-req' }, global: globalOpts })
    await flushPromises()

    expect(wrapper.get('[data-testid="trace-empty"]').text()).toContain('admin.traceRequest.empty')
    expect(wrapper.find('[data-testid="trace-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="trace-attempts"]').exists()).toBe(false)
  })

  it('shows an error state with retry when loading fails', async () => {
    mocks.getRequestTrace.mockRejectedValueOnce({ status: 500, message: 'boom' })
    const wrapper = mount(RequestTraceTimeline, { props: { requestId: 'req-x' }, global: globalOpts })
    await flushPromises()

    const err = wrapper.get('[data-testid="trace-error"]')
    expect(err.text()).toContain('admin.traceRequest.loadFailed')
    expect(err.text()).toContain('boom')

    mocks.getRequestTrace.mockResolvedValueOnce(failoverTrace())
    await wrapper.get('[data-testid="trace-retry"]').trigger('click')
    await flushPromises()
    expect(mocks.getRequestTrace).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('[data-testid="trace-attempt"]')).toHaveLength(2)
  })

  it('shows loading state while the request is pending', async () => {
    mocks.getRequestTrace.mockReturnValue(new Promise(() => {}))
    const wrapper = mount(RequestTraceTimeline, { props: { requestId: 'req-slow' }, global: globalOpts })
    await flushPromises()
    expect(wrapper.find('[data-testid="trace-loading"]').exists()).toBe(true)
  })

  it('does not call the API without a request id', async () => {
    const wrapper = mount(RequestTraceTimeline, { props: { requestId: '' }, global: globalOpts })
    await flushPromises()
    expect(mocks.getRequestTrace).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="trace-no-request-id"]').exists()).toBe(true)
  })
})

describe('OpsErrorDetailModal trace tab', () => {
  const baseDetail = {
    id: 1,
    created_at: '2026-11-10T10:00:00Z',
    phase: 'request',
    type: 'upstream_error',
    error_owner: 'provider',
    severity: 'P1',
    status_code: 502,
    platform: 'anthropic',
    model: 'claude',
    resolved: false,
    message: 'failed',
    is_business_limited: false
  }

  beforeEach(() => {
    mocks.getRequestTrace.mockReset()
    mocks.getRequestErrorDetail.mockReset()
  })

  function mountModal() {
    return mount(OpsErrorDetailModal, {
      props: { show: true, errorId: 1, errorType: 'request' },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } }
    })
  }

  it('switches to the trace tab and loads the trace by request_id', async () => {
    mocks.getRequestErrorDetail.mockResolvedValue({ ...baseDetail, request_id: 'req-1' })
    mocks.getRequestTrace.mockResolvedValue(failoverTrace())
    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.find('[data-testid="request-trace-timeline"]').exists()).toBe(false)
    await wrapper.get('[data-testid="error-detail-tab-trace"]').trigger('click')
    await flushPromises()

    expect(mocks.getRequestTrace).toHaveBeenCalledWith('req-1')
    expect(wrapper.find('[data-testid="request-trace-timeline"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="trace-switch-marker"]')).toHaveLength(1)
  })

  it('disables the trace tab when the error has no request_id', async () => {
    mocks.getRequestErrorDetail.mockResolvedValue({ ...baseDetail, request_id: '' })
    const wrapper = mountModal()
    await flushPromises()

    const tab = wrapper.get('[data-testid="error-detail-tab-trace"]')
    expect(tab.attributes('disabled')).toBeDefined()
    expect(tab.attributes('title')).toBe('admin.traceRequest.tabTraceDisabled')
  })
})
