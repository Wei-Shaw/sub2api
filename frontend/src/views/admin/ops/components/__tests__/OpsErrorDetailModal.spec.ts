import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'
import type { OpsErrorDetail } from '@/api/admin/ops'

const { getRequestErrorDetail, getUpstreamErrorDetail, listRequestErrorUpstreamErrors, showError } = vi.hoisted(() => ({
  getRequestErrorDetail: vi.fn(),
  getUpstreamErrorDetail: vi.fn(),
  listRequestErrorUpstreamErrors: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRequestErrorDetail,
    getUpstreamErrorDetail,
    listRequestErrorUpstreamErrors
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.ops.errorDetail.title': 'Error Detail',
    'admin.ops.errorDetail.titleWithId': 'Error #{id}',
    'admin.ops.errorDetail.loading': 'Loading',
    'admin.ops.errorDetail.noErrorSelected': 'No error selected',
    'admin.ops.errorDetail.account': 'Account',
    'admin.ops.errorDetail.user': 'User',
    'admin.ops.errorDetail.platform': 'Platform',
    'admin.ops.errorDetail.group': 'Group',
    'admin.ops.errorDetail.model': 'Model',
    'admin.ops.errorDetail.status': 'Status',
    'admin.ops.errorDetail.message': 'Message',
    'admin.ops.errorDetail.requestId': 'Request ID',
    'admin.ops.errorDetail.time': 'Time',
    'admin.ops.errorDetail.responseBody': 'Response Body',
    'admin.ops.errorDetails.upstreamErrors': 'Upstream Errors',
    'admin.ops.errorDetail.upstreamEvent.account': 'Account',
    'admin.ops.errorDetail.upstreamEvent.status': 'Status',
    'admin.ops.errorDetail.upstreamEvent.requestId': 'Request ID',
    'admin.ops.errorDetail.responsePreview.expand': 'Expand',
    'admin.ops.errorDetail.responsePreview.collapse': 'Collapse',
    'common.noData': 'No Data',
    'common.loading': 'Loading'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.ops.errorDetail.titleWithId') {
          return `Error #${params?.id ?? ''}`
        }
        return messages[key] ?? key
      }
    })
  }
})

function createDetail(overrides: Partial<OpsErrorDetail> = {}): OpsErrorDetail {
  return {
    id: 890,
    created_at: '2026-03-23T00:48:39Z',
    phase: 'request',
    type: 'request_error',
    error_owner: 'client',
    error_source: 'client_request',
    severity: 'error' as any,
    status_code: 429,
    platform: 'openai',
    model: 'gpt-5.4',
    is_retryable: false,
    retry_count: 0,
    resolved: false,
    resolved_at: null,
    resolved_by_user_id: null,
    resolved_retry_id: null,
    client_request_id: 'client-req-1',
    request_id: 'req-1',
    message: 'The usage limit has been reached',
    user_id: 101,
    user_email: 'zqysl123@gmail.com',
    api_key_id: null,
    account_id: 88,
    account_name: 'kasdgfl132@outlook.com',
    group_id: 5,
    group_name: 'codex',
    client_ip: null,
    request_path: '/v1/chat/completions',
    stream: false,
    error_body: '{"error":{"message":"The usage limit has been reached"}}',
    user_agent: 'Vitest',
    request_body: '{}',
    upstream_status_code: 429,
    upstream_error_message: '',
    upstream_error_detail: '',
    upstream_errors: '',
    auth_latency_ms: null,
    routing_latency_ms: null,
    upstream_latency_ms: null,
    response_latency_ms: null,
    time_to_first_token_ms: null,
    ...overrides
  }
}

describe('OpsErrorDetailModal', () => {
  beforeEach(() => {
    getRequestErrorDetail.mockReset()
    getUpstreamErrorDetail.mockReset()
    listRequestErrorUpstreamErrors.mockReset()
    showError.mockReset()
  })

  it('在请求错误详情中显示用户并补充账号名，且关联上游错误显示账号名', async () => {
    getRequestErrorDetail.mockResolvedValue(createDetail())
    listRequestErrorUpstreamErrors.mockResolvedValue({
      items: [
        createDetail({
          id: 891,
          phase: 'upstream',
          error_owner: 'provider',
          request_id: 'upstream-req-1',
          client_request_id: '',
          message: 'The usage limit has been reached'
        })
      ],
      total: 1,
      page: 1,
      page_size: 100
    })

    const wrapper = mount(OpsErrorDetailModal, {
      props: {
        show: true,
        errorId: 890,
        errorType: 'request'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /></div>'
          },
          Icon: true
        }
      }
    })

    await flushPromises()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('zqysl123@gmail.com')
    expect(text).toContain('Account: kasdgfl132@outlook.com')
    expect(text).toContain('upstream-req-1')
    expect(text).toContain('kasdgfl132@outlook.com')
  })
})
