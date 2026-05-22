import { beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())

vi.mock('../client', () => ({
  apiClient: {
    post,
  },
}))

describe('chat API', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('sends non-streaming chat completions with the selected API key', async () => {
    post.mockResolvedValueOnce({ data: { choices: [{ message: { content: 'ok' } }] } })
    const { createChatCompletion } = await import('../chat')

    await createChatCompletion({
      apiKey: 'sk-user',
      model: 'gpt-5.4',
      messages: [{ role: 'user', content: 'hello' }],
    })

    expect(post).toHaveBeenCalledWith('/chat/completions', {
      model: 'gpt-5.4',
      messages: [{ role: 'user', content: 'hello' }],
      stream: false,
    }, {
      headers: { Authorization: 'Bearer sk-user' },
    })
  })

  it('streams chat completion deltas from SSE chunks', async () => {
    const chunks = [
      'data: {"choices":[{"delta":{"content":"he"}}]}\n\n',
      'data: {"choices":[{"delta":{"content":"llo"}}]}\n\n',
      'data: [DONE]\n\n',
    ]
    const encoder = new TextEncoder()
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const onDelta = vi.fn()
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'gpt-5.4',
      messages: [{ role: 'user', content: 'hello' }],
      promptCacheKey: 'chat-session-100',
      onDelta,
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/chat/completions', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({
        Authorization: 'Bearer sk-user',
        'Content-Type': 'application/json',
      }),
      body: JSON.stringify({
        model: 'gpt-5.4',
        messages: [{ role: 'user', content: 'hello' }],
        stream: true,
        prompt_cache_key: 'chat-session-100',
      }),
    }))
    expect(onDelta).toHaveBeenNthCalledWith(1, 'he')
    expect(onDelta).toHaveBeenNthCalledWith(2, 'llo')
  })

  it('streams OpenAI platform chats through the Responses endpoint', async () => {
    const chunks = [
      'data: {"type":"response.output_text.delta","delta":"he"}\n\n',
      'data: {"type":"response.output_text.delta","delta":"llo"}\n\n',
      'data: [DONE]\n\n',
    ]
    const encoder = new TextEncoder()
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const onDelta = vi.fn()
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'gpt-5.4',
      platform: 'openai',
      messages: [
        { role: 'user', content: 'hello' },
        { role: 'assistant', content: 'hi' },
      ],
      promptCacheKey: 'chat-session-100',
      onDelta,
    })

    expect(fetchMock).toHaveBeenCalledWith('/v1/responses', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        model: 'gpt-5.4',
        input: [
          { role: 'user', content: 'hello' },
          { role: 'assistant', content: 'hi' },
        ],
        stream: true,
        prompt_cache_key: 'chat-session-100',
      }),
    }))
    expect(onDelta).toHaveBeenNthCalledWith(1, 'he')
    expect(onDelta).toHaveBeenNthCalledWith(2, 'llo')
  })

  it('maps image parts for OpenAI Responses requests', async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'gpt-5.5',
      platform: 'openai',
      messages: [{
        role: 'user',
        content: [
          { type: 'text', text: 'describe this' },
          {
            type: 'image',
            imageUrl: 'data:image/png;base64,ZmFrZQ==',
            mimeType: 'image/png',
            name: 'shot.png',
            size: 4,
          },
        ],
      }],
      onDelta: vi.fn(),
    })

    expect(fetchMock).toHaveBeenCalledWith('/v1/responses', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        model: 'gpt-5.5',
        input: [{
          role: 'user',
          content: [
            { type: 'input_text', text: 'describe this' },
            { type: 'input_image', image_url: 'data:image/png;base64,ZmFrZQ==' },
          ],
        }],
        stream: true,
      }),
    }))
  })

  it('maps image parts for default chat completions requests', async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'gpt-5.5',
      messages: [{
        role: 'user',
        content: [
          { type: 'text', text: 'describe this' },
          {
            type: 'image',
            imageUrl: 'data:image/png;base64,ZmFrZQ==',
            mimeType: 'image/png',
          },
        ],
      }],
      onDelta: vi.fn(),
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/chat/completions', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        model: 'gpt-5.5',
        messages: [{
          role: 'user',
          content: [
            { type: 'text', text: 'describe this' },
            { type: 'image_url', image_url: { url: 'data:image/png;base64,ZmFrZQ==' } },
          ],
        }],
        stream: true,
      }),
    }))
  })

  it('streams Anthropic platform chats through the Messages endpoint', async () => {
    const chunks = [
      'event: content_block_delta\n',
      'data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}\n\n',
      'event: message_stop\n',
      'data: {"type":"message_stop"}\n\n',
    ]
    const encoder = new TextEncoder()
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const onDelta = vi.fn()
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'claude-sonnet-4-5',
      platform: 'anthropic',
      messages: [
        { role: 'system', content: 'be concise' },
        { role: 'user', content: 'hello' },
      ],
      onDelta,
    })

    expect(fetchMock).toHaveBeenCalledWith('/v1/messages', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        model: 'claude-sonnet-4-5',
        max_tokens: 4096,
        system: 'be concise',
        messages: [{ role: 'user', content: 'hello' }],
        stream: true,
      }),
    }))
    expect(onDelta).toHaveBeenCalledWith('ok')
  })

  it('maps image parts for Anthropic platform messages', async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'claude-sonnet-4-5',
      platform: 'anthropic',
      messages: [{
        role: 'user',
        content: [
          { type: 'text', text: 'describe this' },
          {
            type: 'image',
            imageUrl: 'data:image/jpeg;base64,ZmFrZQ==',
            mimeType: 'image/jpeg',
          },
        ],
      }],
      onDelta: vi.fn(),
    })

    expect(fetchMock).toHaveBeenCalledWith('/v1/messages', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        model: 'claude-sonnet-4-5',
        max_tokens: 4096,
        messages: [{
          role: 'user',
          content: [
            { type: 'text', text: 'describe this' },
            {
              type: 'image',
              source: {
                type: 'base64',
                media_type: 'image/jpeg',
                data: 'ZmFrZQ==',
              },
            },
          ],
        }],
        stream: true,
      }),
    }))
  })

  it('streams Gemini platform chats through the native Gemini endpoint', async () => {
    const chunks = [
      'data: {"candidates":[{"content":{"parts":[{"text":"he"}]}}]}\n\n',
      'data: {"candidates":[{"content":{"parts":[{"text":"llo"}]}}]}\n\n',
    ]
    const encoder = new TextEncoder()
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const onDelta = vi.fn()
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'gemini-2.5-pro',
      platform: 'gemini',
      messages: [
        { role: 'user', content: 'hello' },
        { role: 'assistant', content: 'hi' },
      ],
      onDelta,
    })

    expect(fetchMock).toHaveBeenCalledWith('/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        contents: [
          { role: 'user', parts: [{ text: 'hello' }] },
          { role: 'model', parts: [{ text: 'hi' }] },
        ],
      }),
    }))
    expect(onDelta).toHaveBeenNthCalledWith(1, 'he')
    expect(onDelta).toHaveBeenNthCalledWith(2, 'llo')
  })

  it('maps image parts for Gemini platform contents', async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.close()
      },
    })
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: stream,
      headers: new Headers({ 'content-type': 'text/event-stream' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const { streamChatCompletion } = await import('../chat')

    await streamChatCompletion({
      apiKey: 'sk-user',
      model: 'gemini-2.5-pro',
      platform: 'gemini',
      messages: [{
        role: 'user',
        content: [
          { type: 'text', text: 'describe this' },
          {
            type: 'image',
            imageUrl: 'data:image/webp;base64,ZmFrZQ==',
            mimeType: 'image/webp',
          },
        ],
      }],
      onDelta: vi.fn(),
    })

    expect(fetchMock).toHaveBeenCalledWith('/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        contents: [{
          role: 'user',
          parts: [
            { text: 'describe this' },
            {
              inline_data: {
                mime_type: 'image/webp',
                data: 'ZmFrZQ==',
              },
            },
          ],
        }],
      }),
    }))
  })

})
