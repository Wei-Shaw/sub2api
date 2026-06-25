import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModels, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.imagePromptDefault': 'Generate a cute orange cat astronaut sticker on a clean pastel background.'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.imageReceived' && params?.count) {
          return `received-${params.count}`
        }
        return messages[key] || key
      }
    })
  }
})

function createStreamResponse(lines: string[]) {
  const encoder = new TextEncoder()
  const chunks = lines.map((line) => encoder.encode(line))
  let index = 0

  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < chunks.length) {
            return { done: false, value: chunks[index++] }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

function mountModal() {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account: {
        id: 42,
        name: 'Gemini Image Test',
        platform: 'gemini',
        type: 'apikey',
        status: 'active'
      }
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: {
          props: ['modelValue', 'options', 'valueKey'],
          emits: ['update:modelValue'],
          template: `
            <select
              class="select-stub"
              :value="modelValue"
              @change="$emit('update:modelValue', $event.target.value)"
            >
              <option v-for="option in options" :key="option[valueKey]" :value="option[valueKey]">
                {{ option.display_name }}
              </option>
            </select>
          `
        },
        TextArea: {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<textarea class="textarea-stub" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
        },
        Icon: true
      }
    }
  })
}

function findStartButton(wrapper: ReturnType<typeof mountModal>) {
  return wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.startTest'))
}

describe('AccountTestModal', () => {
  beforeEach(() => {
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.0-flash', display_name: 'Gemini 2.0 Flash' },
      { id: 'gemini-2.5-flash-image', display_name: 'Gemini 2.5 Flash Image' },
      { id: 'gemini-3.1-flash-image', display_name: 'Gemini 3.1 Flash Image' }
    ])
    copyToClipboard.mockReset()
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: vi.fn((key: string) => (key === 'auth_token' ? 'test-token' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
        clear: vi.fn()
      },
      configurable: true
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gemini-2.5-flash-image"}\n',
        'data: {"type":"image","image_url":"data:image/png;base64,QUJD","mime_type":"image/png"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('gemini 图片模型测试会携带提示词并渲染图片预览', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('draw a tiny orange cat astronaut')

    const startButton = findStartButton(wrapper)
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gemini-3.1-flash-image',
      message: 'hi',
      prompt: 'draw a tiny orange cat astronaut'
    })

    const preview = wrapper.find('img[alt="test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')
  })

  it('fills the client input from a preset without starting the test', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="account-test-client-preset-toggle"]').trigger('click')
    const searchInput = wrapper.get('[data-testid="account-test-client-preset-search"]')
    await searchInput.setValue('codex')
    const presetOption = wrapper.get('[data-testid="account-test-client-preset-option-0"]')
    await presetOption.trigger('click')
    expect(global.fetch).not.toHaveBeenCalled()

    const clientInput = wrapper.get('[data-testid="account-test-client"]').element as HTMLInputElement
    expect(clientInput.value).toBe('codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color')

    const startButton = findStartButton(wrapper)
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      model_id: 'gemini-3.1-flash-image',
      message: 'hi',
      client: 'codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color'
    })
  })

  it('allows the client input to be edited after preset fill', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.get('[data-testid="account-test-client-preset-toggle"]').trigger('click')
    await wrapper.get('[data-testid="account-test-client-preset-option-1"]').trigger('click')

    const clientInput = wrapper.get('[data-testid="account-test-client"]')
    await clientInput.setValue('MyCustomClient/1.2.3')

    const startButton = findStartButton(wrapper)
    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      client: 'MyCustomClient/1.2.3'
    })
  })

  it('submits the configured test message', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const messageInput = wrapper.find('[data-testid="account-test-message"]')
    expect(messageInput.exists()).toBe(true)
    await messageInput.setValue('check upstream client gate')

    const startButton = findStartButton(wrapper)
    expect(startButton).toBeTruthy()
    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      model_id: 'gemini-3.1-flash-image',
      message: 'check upstream client gate'
    })
  })

  it('does not start a test when the test message is blank', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const messageInput = wrapper.find('[data-testid="account-test-message"]')
    expect(messageInput.exists()).toBe(true)
    await messageInput.setValue('   ')

    const startButton = findStartButton(wrapper)
    expect(startButton).toBeTruthy()
    expect(startButton!.attributes('disabled')).toBeDefined()

    await startButton!.trigger('click')
    await flushPromises()

    expect(global.fetch).not.toHaveBeenCalled()
  })
})
