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
        if (key === 'admin.accounts.imagePreviewAlt' && params?.index) {
          return `test-image-${params.index}`
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

function mountModal(account: Record<string, unknown> = {
  id: 42,
  name: 'Gemini Image Test',
  platform: 'gemini',
  type: 'apikey',
  status: 'active'
}) {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: { template: '<div class="select-stub"></div>' },
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

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'gemini-3.1-flash-image',
      prompt: 'draw a tiny orange cat astronaut',
      compress_request_body: false
    })

    const preview = wrapper.find('img[alt="test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('data:image/png;base64,QUJD')
  })

  it('grok 账号测试默认选择 Grok 模型', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'grok-4.3', display_name: 'Grok 4.3' },
      { id: 'grok-build-0.1', display_name: 'Grok Build 0.1' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"grok-4.3"}\n',
        'data: {"type":"content","text":"ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 13,
      name: 'Grok Account',
      platform: 'grok',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'grok-4.3',
      prompt: '',
      mode: 'text'
    })
  })

  it('OpenAI Compact 探测会携带 compact 测试模式', async () => {
    getAvailableModels.mockResolvedValue([
      { id: 'gpt-5.4', display_name: 'GPT-5.4' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 42,
      name: 'OpenAI OAuth',
      platform: 'openai',
      type: 'oauth',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    ;(wrapper.vm as any).selectedModelId = 'gpt-5.4'
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toMatchObject({
      model_id: 'gpt-5.4',
      prompt: '',
      mode: 'compact'
    })
    // OAuth 账号没有「压缩请求体」选项，也不会带这个字段。
    expect(wrapper.find('[data-testid="account-test-compress-request-body"]').exists()).toBe(false)
    expect(JSON.parse(request.body)).not.toHaveProperty('compress_request_body')
  })

  describe('压缩请求体测试', () => {
    const apiKeyAccount = {
      id: 7,
      name: 'Example relay',
      platform: 'deepseek',
      type: 'apikey',
      status: 'active'
    }

    async function runCompressedTest(lines: string[]) {
      getAvailableModels.mockResolvedValue([{ id: 'deepseek-v4.1-flash', display_name: 'DeepSeek V4.1 Flash' }])
      global.fetch = vi.fn().mockResolvedValue(createStreamResponse(lines)) as any
      const wrapper = mountModal(apiKeyAccount)
      await wrapper.setProps({ show: true })
      await flushPromises()
      await wrapper.find('[data-testid="account-test-compress-request-body"]').setValue(true)
      await (wrapper.vm as any).startTest()
      await flushPromises()
      return wrapper
    }

    it('勾选后请求带 compress_request_body，通过时提示可以打开开关', async () => {
      const wrapper = await runCompressedTest([
        'data: {"type":"test_start","model":"deepseek-v4.1-flash"}\n',
        'data: {"type":"test_complete","success":true,"request_compression":{"account_enabled":false,"attempts":[{"host":"relay.example","raw_bytes":812,"wire_bytes":640}]}}\n'
      ])

      const [, request] = (global.fetch as any).mock.calls[0]
      expect(JSON.parse(request.body)).toMatchObject({ compress_request_body: true })
      const text = wrapper.text()
      expect(text).toContain('admin.accounts.requestZstd.testSending')
      expect(text).toContain('admin.accounts.requestZstd.attempt')
      expect(text).toContain('admin.accounts.requestZstd.supported')
      expect(text).not.toContain('admin.accounts.requestZstd.failed')
    })

    it('压缩后失败、且账号已打开开关时提示去关掉', async () => {
      const wrapper = await runCompressedTest([
        'data: {"type":"error","error":"API returned 400: invalid character","request_compression":{"account_enabled":true,"attempts":[{"host":"api.example.com","raw_bytes":812,"wire_bytes":640}]}}\n'
      ])

      const text = wrapper.text()
      expect(text).toContain('admin.accounts.requestZstd.failedEnabled')
      expect(text).not.toContain('admin.accounts.requestZstd.supported')
    })

    it('一个请求体都没压到时提示无法判断', async () => {
      const wrapper = await runCompressedTest([
        'data: {"type":"test_complete","success":true,"request_compression":{"account_enabled":false,"attempts":[]}}\n'
      ])

      const text = wrapper.text()
      expect(text).toContain('admin.accounts.requestZstd.noAttempt')
      expect(text).not.toContain('admin.accounts.requestZstd.supported')
    })

    it('不勾选时显式测试原始请求体', async () => {
      getAvailableModels.mockResolvedValue([{ id: 'deepseek-v4.1-flash', display_name: 'DeepSeek V4.1 Flash' }])
      global.fetch = vi.fn().mockResolvedValue(
        createStreamResponse(['data: {"type":"test_complete","success":true}\n'])
      ) as any
      const wrapper = mountModal(apiKeyAccount)
      await wrapper.setProps({ show: true })
      await flushPromises()
      expect(wrapper.find('[data-testid="account-test-compress-request-body"]').exists()).toBe(true)
      await (wrapper.vm as any).startTest()
      await flushPromises()

      const [, request] = (global.fetch as any).mock.calls[0]
      expect(JSON.parse(request.body)).toHaveProperty('compress_request_body', false)
    })
  })
})
