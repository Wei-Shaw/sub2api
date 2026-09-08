import { describe, expect, it, vi } from 'vitest'
import { createRelayApi, RelayApiError } from './api'

function response(body: unknown, init: ResponseInit = {}) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

describe('relay API client', () => {
  it('loads the relay runtime base URL', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ base_url: 'http://plugin-server:8091' }))

    await expect(createRelayApi('/plugins/ai-relay/api', fetcher).getRuntime()).resolves.toEqual({
      base_url: 'http://plugin-server:8091',
    })
    expect(fetcher).toHaveBeenCalledWith(
      '/plugins/ai-relay/api/runtime',
      expect.objectContaining({ credentials: 'same-origin' }),
    )
  })

  it('lists routes with encoded filters', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ items: [], pagination: { page: 1, page_size: 20, total: 0, total_pages: 1 } }))
    await createRelayApi('/plugins/ai-relay/api', fetcher).listRoutes({ page: 2, page_size: 10, platform: 'open ai', search: 'compact route' })

    expect(fetcher).toHaveBeenCalledWith(
      '/plugins/ai-relay/api/routes?page=2&page_size=10&platform=open+ai&search=compact+route',
      expect.objectContaining({ credentials: 'same-origin' }),
    )
  })

  it('creates, updates, and batch deletes routes with JSON payloads', async () => {
    const fetcher = vi.fn().mockImplementation(() => Promise.resolve(response({})))
    const api = createRelayApi('/plugins/ai-relay/api', fetcher)
    const route = { platform: 'agnes', slug: 'zhipu', name: 'Zhipu', base_url: 'https://open.bigmodel.cn/v1', path_mappings: {} }
    await api.createRoute(route)
    await api.updateRoute('agnes', 'zhipu', route)
    await api.deleteRoutes([{ platform: 'agnes', slug: 'zhipu' }])

    expect(fetcher.mock.calls[0][0]).toBe('/plugins/ai-relay/api/routes')
    expect(fetcher.mock.calls[0][1]).toEqual(expect.objectContaining({ method: 'POST', body: JSON.stringify(route) }))
    expect(fetcher.mock.calls[1][0]).toBe('/plugins/ai-relay/api/routes/agnes/zhipu')
    expect(fetcher.mock.calls[1][1]).toEqual(expect.objectContaining({ method: 'PUT', body: JSON.stringify(route) }))
    expect(fetcher.mock.calls[2][1]).toEqual(expect.objectContaining({ method: 'DELETE', body: JSON.stringify({ items: [{ platform: 'agnes', slug: 'zhipu' }] }) }))
  })

  it('returns undefined for 204 and exposes status for forbidden responses', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(response({ error: 'administrator access is required' }, { status: 403 }))
    const api = createRelayApi('/plugins/ai-relay/api', fetcher)

    await expect(api.deleteRoutes([])).resolves.toBeUndefined()
    await expect(api.listPlatforms()).rejects.toEqual(expect.objectContaining({
      name: 'RelayApiError', status: 403, message: 'administrator access is required',
    }))
    expect(fetcher.mock.results[1]).toBeDefined()
    expect(fetcher.mock.results[1].value).toBeInstanceOf(Promise)
  })
})

it('exports a typed API error', () => {
  expect(new RelayApiError('bad', 400)).toBeInstanceOf(Error)
})
