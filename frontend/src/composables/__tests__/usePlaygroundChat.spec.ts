import { describe, expect, it, vi } from 'vitest'
import {
  buildPlaygroundHistory,
  usePlaygroundChat,
  type PlaygroundMessage
} from '../usePlaygroundChat'
import type { PlaygroundRequest, PlaygroundStreamEvent } from '@/api/admin/playground'

type Emit = (event: PlaygroundStreamEvent) => void

/** A stream stub that replays the given events, then resolves. */
function replay(events: PlaygroundStreamEvent[]) {
  return vi.fn(async (_id: number, _request: PlaygroundRequest, onEvent: Emit) => {
    events.forEach(onEvent)
  })
}

const options = { accountId: 7, modelId: 'claude-sonnet-5-5' }

function message(role: 'user' | 'assistant', content: string, id = 0): PlaygroundMessage {
  return { id, role, content, status: 'done' }
}

describe('buildPlaygroundHistory', () => {
  it('sends completed exchanges followed by the new user message', () => {
    const history = buildPlaygroundHistory(
      [message('user', 'q1'), message('assistant', 'a1')],
      'q2'
    )
    expect(history).toEqual([
      { role: 'user', content: 'q1' },
      { role: 'assistant', content: 'a1' },
      { role: 'user', content: 'q2' }
    ])
  })

  it('drops an exchange whose reply is empty so roles keep alternating', () => {
    const history = buildPlaygroundHistory(
      [
        message('user', 'q1'),
        message('assistant', 'a1'),
        message('user', 'failed question'),
        { ...message('assistant', ''), status: 'error', error: 'boom' }
      ],
      'q3'
    )
    expect(history.map((entry) => entry.content)).toEqual(['q1', 'a1', 'q3'])
  })
})

describe('usePlaygroundChat', () => {
  it('streams a reply into the assistant message', async () => {
    const stream = replay([
      { type: 'test_start', model: 'claude-sonnet-5-5' },
      { type: 'content', text: 'Hel' },
      { type: 'content', text: 'lo' },
      { type: 'test_complete', success: true }
    ])
    const chat = usePlaygroundChat(stream)

    await chat.send('  hi  ', options)

    expect(chat.messages.value).toHaveLength(2)
    const [user, reply] = chat.messages.value
    expect(user).toMatchObject({ role: 'user', content: 'hi', status: 'done' })
    expect(reply).toMatchObject({
      role: 'assistant',
      content: 'Hello',
      status: 'done',
      model: 'claude-sonnet-5-5'
    })
    expect(reply.firstTokenMs).toBeTypeOf('number')
    expect(reply.totalMs).toBeTypeOf('number')
    expect(chat.sending.value).toBe(false)
  })

  it('sends the history and only the parameters that were set', async () => {
    const stream = replay([{ type: 'content', text: 'a1' }, { type: 'test_complete', success: true }])
    const chat = usePlaygroundChat(stream)

    await chat.send('q1', options)
    await chat.send('q2', { ...options, system: '  be brief ', maxTokens: 512, temperature: 0 })

    expect(stream.mock.calls[0][0]).toBe(7)
    expect(stream.mock.calls[0][1]).toEqual({
      model_id: 'claude-sonnet-5-5',
      messages: [{ role: 'user', content: 'q1' }]
    })
    expect(stream.mock.calls[1][1]).toEqual({
      model_id: 'claude-sonnet-5-5',
      messages: [
        { role: 'user', content: 'q1' },
        { role: 'assistant', content: 'a1' },
        { role: 'user', content: 'q2' }
      ],
      system: 'be brief',
      max_tokens: 512,
      temperature: 0
    })
  })

  it('records an upstream error and keeps any partial text', async () => {
    const chat = usePlaygroundChat(
      replay([
        { type: 'content', text: 'partial' },
        { type: 'error', error: 'API returned 429' }
      ])
    )

    await chat.send('hi', options)

    expect(chat.messages.value[1]).toMatchObject({
      content: 'partial',
      status: 'error',
      error: 'API returned 429'
    })
  })

  it('records a network failure', async () => {
    const chat = usePlaygroundChat(
      vi.fn(async () => {
        throw new Error('HTTP 502')
      })
    )

    await chat.send('hi', options)

    expect(chat.messages.value[1]).toMatchObject({ status: 'error', error: 'HTTP 502' })
    expect(chat.sending.value).toBe(false)
  })

  it('flags a reply whose conversation was not applied', async () => {
    const chat = usePlaygroundChat(
      replay([
        { type: 'content', text: 'ok' },
        { type: 'status', code: 'conversation_not_applied', text: 'notice' },
        { type: 'test_complete', success: true }
      ])
    )

    await chat.send('hi', options)

    expect(chat.messages.value[1].conversationNotApplied).toBe(true)
  })

  it('stop aborts the request and marks the reply as stopped', async () => {
    const stream = vi.fn(
      (_id: number, _request: PlaygroundRequest, onEvent: Emit, signal?: AbortSignal) =>
        new Promise<void>((_resolve, reject) => {
          onEvent({ type: 'content', text: 'par' })
          signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
        })
    )
    const chat = usePlaygroundChat(stream)

    const pending = chat.send('hi', options)
    expect(chat.sending.value).toBe(true)
    chat.stop()
    await pending

    expect(chat.messages.value[1]).toMatchObject({ content: 'par', status: 'stopped' })
    expect(chat.messages.value[1].error).toBeUndefined()
    expect(chat.sending.value).toBe(false)
  })

  it('ignores blank input and a second send while one is in flight', async () => {
    let release: () => void = () => {}
    const stream = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          release = resolve
        })
    )
    const chat = usePlaygroundChat(stream)

    await chat.send('   ', options)
    expect(stream).not.toHaveBeenCalled()

    const first = chat.send('one', options)
    await chat.send('two', options)
    release()
    await first

    expect(stream).toHaveBeenCalledTimes(1)
    expect(chat.messages.value).toHaveLength(2)
  })

  it('clear empties the conversation', async () => {
    const chat = usePlaygroundChat(replay([{ type: 'test_complete', success: true }]))
    await chat.send('hi', options)

    chat.clear()

    expect(chat.messages.value).toEqual([])
    expect(chat.isEmpty.value).toBe(true)
  })
})
