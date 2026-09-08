import type { Pagination, Platform, RelayRoute, RelayRuntime, RoutePage } from './types'

export class RelayApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message)
    this.name = 'RelayApiError'
  }
}

export interface RelayApi {
  getRuntime(): Promise<RelayRuntime>
  listPlatforms(): Promise<Platform[]>
  listRoutes(query: Partial<Pagination> & { platform?: string; search?: string }): Promise<RoutePage>
  createRoute(route: Omit<RelayRoute, 'key'>): Promise<RelayRoute>
  updateRoute(platform: string, slug: string, route: Omit<RelayRoute, 'key'>): Promise<RelayRoute>
  deleteRoutes(routes: Array<Pick<RelayRoute, 'platform' | 'slug'>>): Promise<void>
}

async function readResponse<T>(response: Response): Promise<T> {
  const contentType = response.headers.get('content-type') || ''
  const body = response.status === 204
    ? undefined
    : contentType.includes('application/json')
      ? await response.json()
      : await response.text()
  if (!response.ok) {
    const message = typeof body === 'object' && body !== null
      ? String((body as Record<string, unknown>).error || (body as Record<string, unknown>).message || response.statusText || response.status)
      : String(body || response.statusText || response.status)
    throw new RelayApiError(message, response.status)
  }
  return body as T
}

export function createRelayApi(base: string, fetcher: typeof fetch = window.fetch.bind(window)): RelayApi {
  const apiBase = base.replace(/\/+$/, '')
  const request = async <T>(path: string, init: RequestInit = {}): Promise<T> => {
    const response = await fetcher(`${apiBase}${path}`, {
      credentials: 'same-origin',
      ...init,
      headers: { 'Content-Type': 'application/json', ...init.headers },
    })
    return readResponse<T>(response)
  }
  const json = (method: string, body: unknown): RequestInit => ({ method, body: JSON.stringify(body) })

  return {
    getRuntime: () => request<RelayRuntime>('/runtime'),
    listPlatforms: () => request<{ items: Platform[] }>('/platforms').then(payload => payload.items || []),
    listRoutes: query => {
      const params = new URLSearchParams()
      params.set('page', String(query.page || 1))
      params.set('page_size', String(query.page_size || 20))
      if (query.platform) params.set('platform', query.platform)
      if (query.search) params.set('search', query.search)
      return request<RoutePage>(`/routes?${params.toString()}`)
    },
    createRoute: route => request<RelayRoute>('/routes', json('POST', route)),
    updateRoute: (platform, slug, route) => request<RelayRoute>(`/routes/${encodeURIComponent(platform)}/${encodeURIComponent(slug)}`, json('PUT', route)),
    deleteRoutes: routes => request<void>('/routes', json('DELETE', { items: routes })),
  }
}
