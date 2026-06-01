import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OpsErrorLogTable from '../OpsErrorLogTable.vue'
import type { OpsErrorLog } from '@/api/admin/ops'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.ops.errorLog.time': 'Time',
    'admin.ops.errorLog.type': 'Type',
    'admin.ops.errorLog.platform': 'Platform',
    'admin.ops.errorLog.model': 'Model',
    'admin.ops.errorLog.group': 'Group',
    'admin.ops.errorLog.user': 'User',
    'admin.ops.errorLog.status': 'Status',
    'admin.ops.errorLog.message': 'Message',
    'admin.ops.errorLog.action': 'Action',
    'admin.ops.errorLog.details': 'Details',
    'admin.ops.errorLog.accountId': 'Account ID',
    'admin.ops.errorLog.userId': 'User ID',
    'admin.ops.errorLog.acc': 'ACC:',
    'admin.ops.errorLog.typeRequest': 'Request',
    'common.unknown': 'Unknown'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

function createLog(overrides: Partial<OpsErrorLog> = {}): OpsErrorLog {
  return {
    id: 1,
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
    client_request_id: 'client-req-1',
    request_id: 'req-1',
    message: 'The usage limit has been reached',
    user_id: 101,
    user_email: 'user@example.com',
    api_key_id: null,
    account_id: 88,
    account_name: 'kasdgfl132@outlook.com',
    group_id: 5,
    group_name: 'codex',
    client_ip: null,
    request_path: '/v1/chat/completions',
    stream: false,
    ...overrides
  }
}

function mountTable(rows: OpsErrorLog[]) {
  return mount(OpsErrorLogTable, {
    props: {
      rows,
      total: rows.length,
      loading: false,
      page: 1,
      pageSize: 10
    },
    global: {
      stubs: {
        Pagination: true,
        'el-tooltip': {
          template: '<div><slot /></div>'
        }
      }
    }
  })
}

describe('OpsErrorLogTable', () => {
  it('在请求错误中保留用户信息并补充账号名', () => {
    const wrapper = mountTable([createLog()])
    const text = wrapper.text()

    expect(text).toContain('user@example.com')
    expect(text).toContain('ACC: kasdgfl132@outlook.com')
  })

  it('在上游错误中优先显示账号名', () => {
    const wrapper = mountTable([
      createLog({
        phase: 'upstream',
        error_owner: 'provider',
        user_email: 'user@example.com'
      })
    ])
    const text = wrapper.text()

    expect(text).toContain('kasdgfl132@outlook.com')
    expect(text).not.toContain('ACC: kasdgfl132@outlook.com')
  })
})
