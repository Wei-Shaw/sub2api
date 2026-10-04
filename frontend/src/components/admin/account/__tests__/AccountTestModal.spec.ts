import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'
import Select from '@/components/common/Select.vue'

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
        transition: true,
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: { props: ['modelValue', 'options'], emits: ['update:modelValue'], template: '<div class="select-stub"></div>' },
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

  it.each(['pelican', 'knowledge', 'counting'])('OpenAI %s 测试使用当前选中的模型并显示流式回答', async (mode) => {
    getAvailableModels.mockResolvedValue([
      { id: 'gpt-6-astra', display_name: 'GPT-6 Astra' },
      { id: 'gpt-6-sol', display_name: 'GPT-6 Sol' }
    ])
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"gpt-6-sol"}\n',
        'data: {"type":"content","text":"模型回答\\n完整分析"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      id: 42,
      name: 'OpenAI API Key',
      platform: 'openai',
      type: 'apikey',
      status: 'active'
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const [modelSelect, modeSelect] = wrapper.findAllComponents(Select)
    expect(modelSelect.props('modelValue')).toBe('gpt-6-astra')
    expect(modeSelect.props('options')).toEqual(expect.arrayContaining([
      expect.objectContaining({ value: 'knowledge' }),
      expect.objectContaining({ value: 'counting' })
    ]))
    modelSelect.vm.$emit('update:modelValue', 'gpt-6-sol')
    modeSelect.vm.$emit('update:modelValue', mode)
    await flushPromises()
    const startButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()
    await startButton!.trigger('click')
    await flushPromises()

    const request = vi.mocked(fetch).mock.calls[0][1]
    expect(JSON.parse(String(request?.body))).toMatchObject({
      model_id: 'gpt-6-sol',
      prompt: '',
      mode
    })
    expect(wrapper.text()).toContain('模型回答\n完整分析')
    expect(wrapper.text()).toContain('admin.accounts.testCompleted')
  })

  it('鹈鹕测试把分段 SVG 回答显示为图片，保留代码并支持放大和重试清理', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-6-astra', display_name: 'GPT-6 Astra' }])
    const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 480"><circle cx="100" cy="100" r="40" fill="red"/></svg>'
    const output = `\`\`\`svg\n${svg}\n\`\`\``
    vi.mocked(fetch).mockResolvedValue(createStreamResponse([
      `data: ${JSON.stringify({ type: 'content', text: output.slice(0, 40) })}\n`,
      `data: ${JSON.stringify({ type: 'content', text: output.slice(40) })}\n`,
      'data: {"type":"test_complete","success":true}\n'
    ]))
    const wrapper = mountModal({ id: 42, name: 'OpenAI', platform: 'openai', type: 'apikey', status: 'active' })
    await wrapper.setProps({ show: true })
    await flushPromises()
    wrapper.findAllComponents(Select)[1].vm.$emit('update:modelValue', 'pelican')
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.startTest'))!.trigger('click')
    await flushPromises()

    const preview = wrapper.find('img[alt="test-image-1"]')
    expect(preview.exists()).toBe(true)
    expect(decodeURIComponent(preview.attributes('src').split(',')[1])).toContain('viewBox="0 0 640 480"')
    expect(wrapper.text()).toContain(output)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    await preview.trigger('click')
    expect(document.body.querySelector('img[alt="admin.accounts.imageLightboxAlt"]')?.getAttribute('src')).toBe(preview.attributes('src'))

    vi.mocked(fetch).mockResolvedValue(createStreamResponse([
      'data: {"type":"content","text":"<svg>incomplete"}\n',
      'data: {"type":"test_complete","success":true}\n'
    ]))
    await wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.retry'))!.trigger('click')
    await flushPromises()
    expect(wrapper.find('img[alt="test-image-1"]').exists()).toBe(false)
    expect(document.body.querySelector('img[alt="admin.accounts.imageLightboxAlt"]')).toBeNull()
    expect(wrapper.find('[role="alert"]').text()).toBe('admin.accounts.openai.svgPreviewFailed')
    expect(wrapper.text()).toContain('<svg>incomplete')
    wrapper.unmount()
  })

  it('SVG 图片加载失败显示提示，其他模式的 SVG 回答保持文本输出', async () => {
    getAvailableModels.mockResolvedValue([{ id: 'gpt-6-astra', display_name: 'GPT-6 Astra' }])
    const output = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><rect width="20" height="20"/></svg>'
    vi.mocked(fetch).mockImplementation(async () => createStreamResponse([
      `data: ${JSON.stringify({ type: 'content', text: output })}\n`,
      'data: {"type":"test_complete","success":true}\n'
    ]))
    const wrapper = mountModal({ id: 42, name: 'OpenAI', platform: 'openai', type: 'apikey', status: 'active' })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const modeSelect = wrapper.findAllComponents(Select)[1]
    modeSelect.vm.$emit('update:modelValue', 'pelican')
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.startTest'))!.trigger('click')
    await flushPromises()
    await wrapper.find('img[alt="test-image-1"]').trigger('error')
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.find('img[alt="test-image-1"]').exists()).toBe(false)

    modeSelect.vm.$emit('update:modelValue', 'knowledge')
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.retry'))!.trigger('click')
    await flushPromises()
    expect(wrapper.find('img[alt="test-image-1"]').exists()).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain(output)
    wrapper.unmount()
  })
})
