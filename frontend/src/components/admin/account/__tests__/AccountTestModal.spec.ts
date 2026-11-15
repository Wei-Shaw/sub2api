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
      prompt: 'draw a tiny orange cat astronaut'
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
  })

  function freeAccount(extra: Record<string, unknown> = {}) {
    return { id: 42, name: 'Free images', platform: 'openai', type: 'oauth', status: 'active', credentials: { plan_type: 'free', model_mapping: { illustration: 'gpt-image-2.5-flare', 'gpt-image-2': 'gpt-image-2' } }, extra: { chatgpt_web_image_enabled: true, chatgpt_web_image_regular_chat: true, ...extra } }
  }

  it('Free 图片别名走网页并标明兼容入口，每次点击测试使用新的 operation_id', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'illustration', display_name: 'illustration' }])
    global.fetch = vi.fn().mockImplementation(async () => createStreamResponse([
      'data: {"type":"test_start","provider":"chatgpt_web","model":"illustration","capability_only":true}\n',
      'data: {"type":"image","provider":"chatgpt_web","image_url":"data:image/png;base64,QUJD","mime_type":"image/png"}\n',
      'data: {"type":"test_complete","success":true}\n'
    ]))
    const wrapper = mountModal(freeAccount())
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.get('[data-testid="free-web-image-test-path"]').text()).toContain('admin.accounts.openai.webImageTestHint')
    expect((wrapper.vm as any).modelOptionsForMode[0].display_name).toContain('admin.accounts.openai.webImageEntryBadge')
    ;(wrapper.vm as any).testMode = 'compact'
    await wrapper.get('textarea.textarea-stub').setValue('draw an orange circle')
    await (wrapper.vm as any).startTest()
    await flushPromises()
    const first = JSON.parse((global.fetch as any).mock.calls[0][1].body)
    expect(first).toMatchObject({ model_id: 'illustration', mode: 'default', prompt: 'draw an orange circle' })
    expect(first.operation_id).toEqual(expect.any(String))
    expect(first.operation_id.length).toBeGreaterThan(10)
    expect(wrapper.text()).toContain('admin.accounts.openai.webImageCompatibleEntry')
    expect(wrapper.text()).toContain('admin.accounts.openai.webImageTestPath')
    expect(wrapper.text()).not.toContain('admin.accounts.usingModel')
    expect(wrapper.find('img[alt="test-image-1"]').attributes('src')).toBe('data:image/png;base64,QUJD')
    await (wrapper.vm as any).startTest()
    const second = JSON.parse((global.fetch as any).mock.calls[1][1].body)
    expect(second.operation_id).not.toBe(first.operation_id)
    wrapper.unmount()
  })

  it('未配置桥接的 Free 账号禁用图片测试，不回退原生图片', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-image-2', display_name: 'GPT Image 2' }])
    const account = freeAccount()
    delete (account.extra as Record<string, unknown>).chatgpt_web_image_enabled
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.find('[data-testid="free-web-image-test-path"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="free-web-image-test-disabled"]').exists()).toBe(true)
    expect((wrapper.vm as any).canStartTest).toBe(false)
    await (wrapper.vm as any).startTest()
    expect(global.fetch).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('Free 图片入口优先于旧文本映射，普通聊天测试仍保持原语义', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-image-custom', display_name: 'Legacy alias' }, { id: 'gpt-6-luna', display_name: 'GPT 6 Luna' }])
    global.fetch = vi.fn().mockImplementation(async () => createStreamResponse(['data: {"type":"test_complete","success":true}\n']))
    const account = freeAccount()
    account.credentials.model_mapping = { 'gpt-image-custom': 'gpt-5.1', 'gpt-6-luna': 'gpt-6-luna' } as any
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.find('[data-testid="free-web-image-test-path"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="free-web-image-test-disabled"]').exists()).toBe(false)
    expect((wrapper.vm as any).supportsOpenAIImageTest).toBe(true)
    expect((wrapper.vm as any).modelOptionsForMode[0].display_name).toContain('admin.accounts.openai.webImageEntryBadge')
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    expect(JSON.parse((global.fetch as any).mock.calls[0][1].body)).toMatchObject({ model_id: 'gpt-image-custom', mode: 'default' })
    ;(wrapper.vm as any).selectedModelId = 'gpt-6-luna'
    await flushPromises()
    expect(wrapper.find('[data-testid="free-web-image-test-path"]').exists()).toBe(false)
    expect((wrapper.vm as any).supportsOpenAIImageTest).toBe(false)
    expect((wrapper.vm as any).modelOptionsForMode[1].display_name).toBe('GPT 6 Luna')
    await (wrapper.vm as any).startTest()
    expect(JSON.parse((global.fetch as any).mock.calls[1][1].body)).toMatchObject({ model_id: 'gpt-6-luna', mode: 'compact', prompt: '' })
    wrapper.unmount()
  })

  it('非Free账号图片名称映射到文本时保持普通测试', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-image-custom', display_name: 'Text alias' }])
    global.fetch = vi.fn().mockResolvedValue(createStreamResponse(['data: {"type":"test_complete","success":true}\n']))
    const account = freeAccount()
    account.credentials = { ...account.credentials, plan_type: 'plus', model_mapping: { 'gpt-image-custom': 'gpt-5.1' } as any }
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect((wrapper.vm as any).supportsOpenAIImageTest).toBe(false)
    expect(wrapper.find('[data-testid="free-web-image-test-path"]').exists()).toBe(false)
    ;(wrapper.vm as any).testMode = 'compact'
    await (wrapper.vm as any).startTest()
    expect(JSON.parse((global.fetch as any).mock.calls[0][1].body)).toMatchObject({ model_id: 'gpt-image-custom', mode: 'compact' })
    wrapper.unmount()
  })

  it.each([{ chatgpt_web_image_enabled: false }, { chatgpt_web_image_regular_chat: false }])('Free 图片缺少开关或普通会话许可时禁用测试：%o', async extra => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-image-2', display_name: 'gpt-image-2' }])
    const wrapper = mountModal(freeAccount(extra))
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.find('[data-testid="free-web-image-test-disabled"]').exists()).toBe(true)
    expect((wrapper.vm as any).canStartTest).toBe(false)
    await (wrapper.vm as any).startTest()
    expect(global.fetch).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([{}, { 'gpt-6-luna': 'gpt-6-luna' }])('Free所有图片入口无需账号图片模型授权：%o', async mapping => {
    const images = ['gpt-image-1', 'gpt-image-1.5', 'gpt-image-2', 'gpt-image-2.5-flare', 'gpt-image-2.5-sunburst']
    getAvailableModels.mockResolvedValue(images.map(id => ({ id, display_name: id })))
    global.fetch = vi.fn().mockImplementation(async () => createStreamResponse(['data: {"type":"test_complete","success":true}\n']))
    const account = freeAccount()
    account.credentials.model_mapping = mapping as any
    const wrapper = mountModal(account)
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect((wrapper.vm as any).modelOptionsForMode.map((model: { id: string }) => model.id)).toEqual(images)
    for (const image of images) {
      (wrapper.vm as any).selectedModelId = image
      await flushPromises()
      expect(wrapper.find('[data-testid="free-web-image-test-disabled"]').exists()).toBe(false)
      expect((wrapper.vm as any).canStartTest).toBe(true)
      await (wrapper.vm as any).startTest()
      expect(JSON.parse((global.fetch as any).mock.calls.at(-1)[1].body)).toMatchObject({ model_id: image, mode: 'default' })
    }
    expect(global.fetch).toHaveBeenCalledTimes(images.length)
    wrapper.unmount()
  })

})
