import { afterEach, describe, expect, it, vi } from 'vitest'

describe('Support Chat API', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('normalizes a relative API base when fetching FAQs', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'api/v1')
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: { items: [] } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    )

    const { fetchFaqs } = await import('@/api/supportChat')
    await fetchFaqs()

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/support/chat/faqs',
      expect.objectContaining({ method: 'GET' })
    )
  })

  it('uses the normalized API route for streaming chat', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'api/v1')
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('data: [DONE]\n\n', {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      })
    )

    const { streamChat } = await import('@/api/supportChat')
    const handle = streamChat(
      { messages: [{ role: 'user', content: 'hello' }] },
      {
        onChunk: vi.fn(),
        onError: vi.fn(),
        onDone: vi.fn(),
      }
    )
    await handle.done

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/support/chat',
      expect.objectContaining({ method: 'POST' })
    )
  })
})
