import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DOMPurify from 'dompurify'
import PlaygroundView from '../PlaygroundView.vue'
import { readChatDeltas, responseError } from '../playgroundStream'

vi.mock('@/api/keys', () => ({
  keysAPI: {
    list: vi.fn().mockResolvedValue({ items: [{ id: 1, name: 'key', key: 'sk-test', group: { name: 'g' } }] })
  }
}))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

function streamOf(...parts: string[]) {
  const encoder = new TextEncoder()
  return new ReadableStream<Uint8Array>({
    start(controller) {
      parts.forEach((part) => controller.enqueue(encoder.encode(part)))
      controller.close()
    }
  })
}

async function collect(body: ReadableStream<Uint8Array>) {
  let out = ''
  for await (const delta of readChatDeltas(body)) out += delta
  return out
}

describe('playgroundStream', () => {
  it('joins deltas split across chunks and stops at [DONE]', async () => {
    const body = streamOf(
      'data: {"choices":[{"delta":{"role":"assistant"}}]}\n\n',
      'data: {"choices":[{"delta":{"content":"Hel"}}]}\n\ndata: {"choi',
      'ces":[{"delta":{"content":"lo"}}]}\n\n: keep-alive\n\ndata: [DONE]\n\n'
    )
    expect(await collect(body)).toBe('Hello')
  })

  it('throws on an in-stream error event', async () => {
    await expect(collect(streamOf('data: {"error":{"message":"quota"}}\n\n'))).rejects.toThrow('quota')
  })

  it('reads the gateway error message', async () => {
    expect(await responseError(new Response('{"error":{"message":"bad key"}}', { status: 401 }))).toBe('bad key')
    expect(await responseError(new Response('', { status: 502 }))).toBe('HTTP 502')
  })
})

describe('PlaygroundView streaming render', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('re-renders only the streaming reply, at most once per animation frame', async () => {
    const frames = new Map<number, FrameRequestCallback>()
    let lastFrame = 0
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      frames.set(++lastFrame, cb)
      return lastFrame
    })
    vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
    const runFrame = () => {
      const callbacks = [...frames.values()]
      frames.clear()
      callbacks.forEach((cb) => cb(0))
    }
    Element.prototype.scrollTo ??= () => {}

    const encoder = new TextEncoder()
    let stream!: ReadableStreamDefaultController<Uint8Array>
    vi.stubGlobal('fetch', vi.fn(async (path: string) => {
      if (path === '/v1/models') return new Response(JSON.stringify({ data: [{ id: 'model-a' }] }))
      return new Response(new ReadableStream<Uint8Array>({ start: (controller) => { stream = controller } }))
    }))

    const wrapper = shallowMount(PlaygroundView, { global: { renderStubDefaultSlot: true, stubs: { RouterLink: true } } })
    await flushPromises()
    const vm = wrapper.vm as unknown as { messages: Array<{ role: string; content: string }>; draft: string; send: () => Promise<void> }

    // 20-message conversation: 10 finished assistant turns rendered as markdown.
    vm.messages = Array.from({ length: 20 }, (_, i) =>
      i % 2 ? { role: 'assistant', content: `**answer ${i}**` } : { role: 'user', content: `question ${i}` }
    )
    await nextTick()
    const sanitize = vi.spyOn(DOMPurify, 'sanitize')

    vm.draft = 'next question'
    const sending = vm.send()
    await flushPromises()
    for (let i = 0; i < 500; i++) {
      stream.enqueue(encoder.encode(`data: {"choices":[{"delta":{"content":"w${i} "}}]}\n\n`))
      await flushPromises()
      if (i % 5 === 4) {
        runFrame()
        await nextTick()
      }
    }
    stream.close()
    await sending
    await flushPromises()

    // 500 deltas, 5 per frame: one sanitize per frame, finished turns never re-rendered.
    expect(sanitize).toHaveBeenCalledTimes(100)
    expect(sanitize.mock.calls.some(([html]) => String(html).includes('answer'))).toBe(false)
    expect(wrapper.html()).toContain('w499')
    wrapper.unmount()
  })
})
