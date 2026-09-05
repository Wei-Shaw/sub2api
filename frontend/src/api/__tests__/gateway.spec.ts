import { afterEach, describe, expect, it, vi } from 'vitest'
import { createChatCompletion, extractAssistantContent } from '@/api/gateway'

describe('gateway API', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('sends a non-streaming chat request with the selected API key', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ choices: [{ message: { content: '真实回复' } }] }), { status: 200 })
    )

    const response = await createChatCompletion('sk-live-test', 'deepseek-chat', '你好')

    expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/v1\/chat\/completions$/), expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ Authorization: 'Bearer sk-live-test' }),
      body: JSON.stringify({
        model: 'deepseek-chat',
        messages: [{ role: 'user', content: '你好' }],
        stream: false
      })
    }))
    expect(extractAssistantContent(response)).toBe('真实回复')
  })

  it('rejects non-success responses instead of returning a fake result', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: { message: '余额不足' } }), { status: 402 })
    )

    await expect(createChatCompletion('sk-live-test', 'deepseek-chat', '你好')).rejects.toMatchObject({
      message: '余额不足',
      status: 402
    })
  })
})
