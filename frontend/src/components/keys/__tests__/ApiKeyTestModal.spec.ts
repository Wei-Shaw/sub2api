import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ApiKeyTestModal from '../ApiKeyTestModal.vue'
import type { ApiKey, GroupPlatform } from '@/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))

const fetchMock = vi.fn()
let wrappers: VueWrapper[] = []
const keyFor = (platform: GroupPlatform = 'openai') => ({
  id: 3, key: 'sk-user-test', name: 'User key', status: 'active',
  group: { id: 2, name: 'Test group', platform }
}) as ApiKey

function stream(events: unknown[], chunkSize = 17, newline = '\r\n') {
  const text = events.map((event) => `event: test${newline}data:${JSON.stringify(event)}${newline}${newline}`).join('')
  const bytes = new TextEncoder().encode(text)
  return new Response(new ReadableStream({
    start(controller) {
      for (let i = 0; i < bytes.length; i += chunkSize) controller.enqueue(bytes.slice(i, i + chunkSize))
      controller.close()
    }
  }), { headers: { 'Content-Type': 'text/event-stream' } })
}

function mountModal(apiKey = keyFor()) {
  const wrapper = mount(ApiKeyTestModal, {
    props: { show: true, apiKey, baseUrl: 'https://gateway.example/v1/' },
    global: { stubs: {
      BaseDialog: {
        props: ['show'], emits: ['close'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      Select: {
        props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue'],
        template: `<select :value="modelValue" :disabled="disabled" @change="$emit('update:modelValue', $event.target.value)">
          <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>`
      }, Icon: true
    } }
  })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  fetchMock.mockImplementationOnce(async () => new Response(JSON.stringify({
    data: [{ id: 'gpt-6-astra' }, { id: 'gpt-6-sol' }]
  }), { headers: { 'Content-Type': 'application/json' } }))
})

afterEach(() => {
  wrappers.forEach((wrapper) => wrapper.unmount())
  wrappers = []
  vi.unstubAllGlobals()
})

describe('ApiKeyTestModal', () => {
  it('loads key-scoped models and selects the first model', async () => {
    const wrapper = mountModal()
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith('https://gateway.example/v1/models', expect.objectContaining({
      headers: { Authorization: 'Bearer sk-user-test', Accept: 'application/json' }
    }))
    expect(wrapper.get('#key-test-model').element).toHaveProperty('value', 'gpt-6-astra')
    expect(wrapper.get('[data-test="start-key-test"]').attributes('disabled')).toBeUndefined()
  })

  it.each([
    ['default', 'Reply with OK only.', 16],
    ['pelican', 'Generate an SVG of a pelican riding a bicycle. Reply with the SVG code only.', 8192],
    ['knowledge', '不联网 你现在的知识库是什么时候的', 1024],
    ['counting', '在一个黑色的袋子里', 8192]
  ])('sends the selected model and %s template', async (mode, input, limit) => {
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('#key-test-model').setValue('gpt-6-sol')
    await wrapper.get('#key-test-mode').setValue(mode)
    fetchMock.mockResolvedValueOnce(stream([
      { type: 'response.output_text.delta', delta: 'OK' },
      { type: 'response.completed', response: { status: 'completed' } }
    ]))
    await wrapper.get('[data-test="start-key-test"]').trigger('click')
    await flushPromises()
    const [url, options] = fetchMock.mock.calls[1]
    const body = JSON.parse(options.body)
    expect(url).toBe('https://gateway.example/v1/responses')
    expect(body).toMatchObject({ model: 'gpt-6-sol', max_output_tokens: limit, stream: true })
    expect(body.input).toContain(input)
    expect(options.headers.Authorization).toBe('Bearer sk-user-test')
    expect(options.headers['X-A6API-Self-Test-Kind']).toBe(mode === 'pelican' ? 'pelican' : undefined)
    expect(body.tool_choice).toBe(mode === 'knowledge' ? 'none' : undefined)
    expect(wrapper.text()).toContain('keys.test.completed')
  })

  it('renders a sanitized pelican image and retains its source', async () => {
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('#key-test-mode').setValue('pelican')
    const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="30"/><script>alert(1)</script></svg>'
    fetchMock.mockResolvedValueOnce(stream([
      { type: 'response.output_text.delta', delta: svg.slice(0, 48) },
      { type: 'response.output_text.delta', delta: svg.slice(48) },
      { type: 'response.completed', response: { status: 'completed' } }
    ]))
    await wrapper.get('[data-test="start-key-test"]').trigger('click')
    await flushPromises()
    const url = wrapper.get('img').attributes('src')
    expect(url).toMatch(/^data:image\/svg\+xml/)
    expect(decodeURIComponent(url)).not.toContain('<script>')
    expect(wrapper.get('details pre').text()).toBe(svg)
  })

  it('accepts final-only output and validates compact output items', async () => {
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('#key-test-mode').setValue('compact')
    fetchMock.mockResolvedValueOnce(stream([
      { type: 'response.completed', response: { status: 'completed', output: [{ type: 'compaction', encrypted_content: 'data' }] } }
    ]))
    await wrapper.get('[data-test="start-key-test"]').trigger('click')
    await flushPromises()
    const request = fetchMock.mock.calls[1][1]
    expect(request.headers['OpenAI-Beta']).toBe('remote_compaction_v2')
    expect(JSON.parse(request.body).input[1]).toEqual({ type: 'compaction_trigger' })
    expect(wrapper.text()).toContain('keys.test.completed')
    fetchMock.mockResolvedValueOnce(stream([{ type: 'response.completed', response: { output: [] } }]))
    await wrapper.get('[data-test="start-key-test"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('keys.test.noCompaction')
  })

  it.each([
    [new Response(JSON.stringify({ error: { message: 'Quota exceeded' } }), { status: 429 }), 'Quota exceeded'],
    [stream([{ type: 'response.output_text.delta', delta: 'partial' }]), 'keys.test.interrupted'],
    [stream([{ type: 'response.failed', response: { error: { message: 'Upstream failed' } } }]), 'Upstream failed'],
    [stream([{ type: 'response.incomplete' }]), 'keys.test.incomplete']
  ])('shows failures without reporting success', async (response, message) => {
    const wrapper = mountModal()
    await flushPromises()
    fetchMock.mockResolvedValueOnce(response)
    await wrapper.get('[data-test="start-key-test"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain(message)
    expect(wrapper.text()).not.toContain('keys.test.completed')
  })

  it('shows model errors and allows retry', async () => {
    fetchMock.mockReset()
    fetchMock.mockRejectedValueOnce(new Error('Model service unavailable'))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.text()).toContain('Model service unavailable')
    expect(wrapper.get('[data-test="start-key-test"]').attributes('disabled')).toBeDefined()
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ data: [] })))
    await wrapper.get('button[title="common.refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('keys.test.noModels')
  })

  it('cancels requests when closed and ignores their late results', async () => {
    const wrapper = mountModal()
    await flushPromises()
    let resolve: (response: Response) => void = () => {}
    fetchMock.mockImplementationOnce(() => new Promise<Response>((done) => { resolve = done }))
    await wrapper.get('[data-test="start-key-test"]').trigger('click')
    const signal = fetchMock.mock.calls[1][1].signal as AbortSignal
    await wrapper.setProps({ show: false })
    expect(signal.aborted).toBe(true)
    resolve(stream([{ type: 'response.completed' }]))
    await flushPromises()
    await wrapper.setProps({ show: true })
    expect(wrapper.text()).not.toContain('keys.test.completed')
  })

  it('uses Gemini native streaming and preserves Chinese split across chunks', async () => {
    const wrapper = mountModal(keyFor('gemini'))
    await flushPromises()
    await wrapper.get('#key-test-mode').setValue('knowledge')
    fetchMock.mockResolvedValueOnce(stream([{ candidates: [{ content: { parts: [{ text: '知识库' }] }, finishReason: 'STOP' }] }], 1))
    await wrapper.get('[data-test="start-key-test"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls[1][0]).toContain('/v1beta/models/gpt-6-astra:streamGenerateContent?alt=sse')
    expect(wrapper.get('pre').text()).toBe('知识库')
    expect(wrapper.text()).toContain('keys.test.completed')
    expect(wrapper.get('#key-test-mode').text()).not.toContain('keys.test.compact')
  })

  it.each(['inactive', 'expired', 'quota_exhausted'] as const)('does not send requests for %s keys', async (status) => {
    const wrapper = mountModal({ ...keyFor(), status })
    await flushPromises()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('keys.test.inactive')
  })
})
