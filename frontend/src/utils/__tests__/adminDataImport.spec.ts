import { describe, expect, it } from 'vitest'
import { mergeAccountImportPayloads, normalizeAccountImportPayload } from '@/utils/adminDataImport'

describe('normalizeAccountImportPayload', () => {
  it('兼容纯账号数组批量导入', () => {
    const payload = normalizeAccountImportPayload(
      [
        {
          name: 'acc-1',
          platform: 'openai',
          type: 'oauth',
          credentials: { access_token: 'token-1' },
          concurrency: 2,
          priority: 10
        },
        {
          name: 'acc-2',
          platform: 'gemini',
          type: 'apikey',
          credentials: { api_key: 'token-2' },
          concurrency: 3,
          priority: 20
        }
      ],
      '2026-04-04T00:00:00.000Z'
    )

    expect(payload.type).toBe('sub2api-data')
    expect(payload.version).toBe(1)
    expect(payload.exported_at).toBe('2026-04-04T00:00:00.000Z')
    expect(payload.proxies).toEqual([])
    expect(payload.accounts).toHaveLength(2)
    expect(payload.accounts[0].name).toBe('acc-1')
    expect(payload.accounts[1].type).toBe('apikey')
  })

  it('兼容 data 包装格式并提取内联代理', () => {
    const payload = normalizeAccountImportPayload(
      {
        data: {
          accounts: [
            {
              name: 'acc-1',
              platform: 'openai',
              type: 'oauth',
              credentials: { access_token: 'token-1' },
              proxy: {
                name: 'proxy-a',
                protocol: 'http',
                host: '127.0.0.1',
                port: 8080,
                username: 'user',
                password: 'pass',
                status: 'active'
              },
              concurrency: 1,
              priority: 5
            }
          ]
        }
      },
      '2026-04-04T00:00:00.000Z'
    )

    expect(payload.accounts).toHaveLength(1)
    expect(payload.accounts[0].proxy_key).toBe('http|127.0.0.1|8080|user|pass')
    expect(payload.proxies).toEqual([
      {
        proxy_key: 'http|127.0.0.1|8080|user|pass',
        name: 'proxy-a',
        protocol: 'http',
        host: '127.0.0.1',
        port: 8080,
        username: 'user',
        password: 'pass',
        status: 'active'
      }
    ])
  })

  it('不支持的格式会报错', () => {
    expect(() => normalizeAccountImportPayload('invalid')).toThrow('Unsupported import payload')
  })

  it('支持合并多个导入 payload', () => {
    const payloadA = normalizeAccountImportPayload([
      {
        name: 'acc-1',
        platform: 'openai',
        type: 'oauth',
        credentials: { access_token: 'token-1' },
        concurrency: 1,
        priority: 10
      }
    ])
    const payloadB = normalizeAccountImportPayload({
      accounts: [
        {
          name: 'acc-2',
          platform: 'gemini',
          type: 'apikey',
          credentials: { api_key: 'token-2' },
          concurrency: 2,
          priority: 20
        }
      ],
      proxies: [
        {
          proxy_key: 'http|127.0.0.1|8080||',
          name: 'proxy-a',
          protocol: 'http',
          host: '127.0.0.1',
          port: 8080,
          status: 'active'
        }
      ]
    })

    const merged = mergeAccountImportPayloads([payloadA, payloadB], '2026-04-04T00:00:00.000Z')

    expect(merged.exported_at).toBe('2026-04-04T00:00:00.000Z')
    expect(merged.accounts).toHaveLength(2)
    expect(merged.proxies).toHaveLength(1)
  })
})
