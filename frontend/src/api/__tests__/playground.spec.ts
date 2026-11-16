import { afterEach, describe, expect, it, vi } from 'vitest'
import { parsePlaygroundStream, streamAccountChat } from '../admin/playground'

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
})

function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  return new ReadableStream({
    start(controller) {
      chunks.forEach((chunk) => controller.enqueue(encoder.encode(chunk)))
      controller.close()
    }
  })
}

describe('parsePlaygroundStream', () => {
  it('returns complete events and keeps the unfinished tail', () => {
    const { events, rest } = parsePlaygroundStream(
      'data: {"type":"content","text":"a"}\n\ndata: {"type":"cont'
    )
    expect(events).toEqual([{ type: 'content', text: 'a' }])
    expect(rest).toBe('data: {"type":"cont')
  })

  it('skips comments, malformed JSON and events without a type', () => {
    const { events } = parsePlaygroundStream(
      ': keepalive\ndata: {broken\ndata: {"text":"no type"}\ndata: {"type":"test_complete","success":true}\n'
    )
    expect(events).toEqual([{ type: 'test_complete', success: true }])
  })
})

describe('streamAccountChat', () => {
  it('posts to the account test endpoint and reports events split across chunks', async () => {
    localStorage.setItem('auth_token', 'admin-token')
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: streamOf([
        'data: {"type":"content","te',
        'xt":"你"}\n\ndata: {"type":"content","text":"好"}\n\n',
        'data: {"type":"test_complete","success":true}'
      ])
    }))
    vi.stubGlobal('fetch', fetchMock)
    const events: unknown[] = []
    const request = { model_id: 'm', messages: [{ role: 'user' as const, content: 'hi' }] }

    await streamAccountChat(42, request, (event) => events.push(event))

    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toMatch(/\/admin\/accounts\/42\/test$/)
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer admin-token')
    expect(JSON.parse(init.body as string)).toEqual(request)
    expect(events).toEqual([
      { type: 'content', text: '你' },
      { type: 'content', text: '好' },
      { type: 'test_complete', success: true }
    ])
  })

  it('rejects on an HTTP error status', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 403, body: null })))

    await expect(
      streamAccountChat(1, { model_id: 'm', messages: [] }, () => {})
    ).rejects.toThrow('HTTP 403')
  })
})
