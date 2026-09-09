import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import { cpaApi } from '../cpa'

/**
 * 回归测试：apiClient 的响应拦截器已把 { code, message, data } 解包成 data。
 * 这里用真实拦截器 + mock adapter 覆盖，防止再次出现「多解一层」导致
 * `Cannot read properties of undefined (reading 'files')`。
 */
const originalAdapter = apiClient.defaults.adapter

function mockEnvelope(payload: unknown, status = 200) {
  const adapter = vi.fn().mockResolvedValue({
    status,
    data: { code: 0, message: 'success', data: payload },
    headers: {},
    config: {},
    statusText: 'OK',
  })
  apiClient.defaults.adapter = adapter
  return adapter
}

afterEach(() => {
  vi.restoreAllMocks()
  apiClient.defaults.adapter = originalAdapter
})

describe('cpa api unwrapping', () => {
  it('listAuthFiles returns the payload directly (not a second .data hop)', async () => {
    mockEnvelope({ files: [{ name: 'codex-pro.json', auth_index: 'idx-1', success: 3, failed: 1 }] })

    const result = await cpaApi.listAuthFiles()

    expect(result).toBeDefined()
    expect(result.files).toHaveLength(1)
    expect(result.files[0].name).toBe('codex-pro.json')
  })

  it('getConfig returns the payload', async () => {
    mockEnvelope({ enabled: true, base_url: 'http://cpa:8317', management_key_configured: true })

    const result = await cpaApi.getConfig()

    expect(result.enabled).toBe(true)
    expect(result.base_url).toBe('http://cpa:8317')
  })

  it('getAuthFileQuota returns the quota payload', async () => {
    mockEnvelope({ plan_type: 'pro', rate_limit: { primary_window: { used_percent: 14 } } })

    const result = await cpaApi.getAuthFileQuota('idx-1')

    expect(result.plan_type).toBe('pro')
    expect(result.rate_limit?.primary_window?.used_percent).toBe(14)
  })

  it('listErrorLogs returns the file list', async () => {
    mockEnvelope({ files: [{ name: 'error-1.log', size: 10, modified: 1 }] })

    const result = await cpaApi.listErrorLogs()

    expect(result.files).toHaveLength(1)
  })

  it('getApiKeyUsage returns the provider map', async () => {
    mockEnvelope({ openai: { 'http://up|sk-x': { success: 1, failed: 0 } } })

    const result = await cpaApi.getApiKeyUsage()

    expect(Object.keys(result)).toContain('openai')
  })

  it('sends auth_index as a query param and encodes log names', async () => {
    const quotaAdapter = mockEnvelope({ plan_type: 'pro' })
    await cpaApi.getAuthFileQuota('idx-1')
    expect(quotaAdapter.mock.calls[0][0].url).toBe('/admin/cpa/auth-files/quota')
    // 客户端拦截器会给 GET 注入 timezone，这里只断言我们自己的参数
    expect(quotaAdapter.mock.calls[0][0].params).toEqual(expect.objectContaining({ auth_index: 'idx-1' }))

    const logAdapter = mockEnvelope({ name: 'a b.log', content: 'x' })
    await cpaApi.getErrorLogContent('a b.log')
    expect(logAdapter.mock.calls[0][0].url).toBe('/admin/cpa/error-logs/a%20b.log')
  })

  it('updateConfig posts the full body', async () => {
    const adapter = mockEnvelope({ enabled: true, base_url: 'http://cpa:8317', management_key_configured: true })

    await cpaApi.updateConfig({ enabled: true, base_url: 'http://cpa:8317', management_key: 'secret' })

    const config = adapter.mock.calls[0][0]
    expect(config.method).toBe('put')
    expect(config.url).toBe('/admin/cpa/config')
    expect(JSON.parse(config.data)).toEqual({
      enabled: true,
      base_url: 'http://cpa:8317',
      management_key: 'secret',
    })
  })
})
