import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountTestModal from '../AccountTestModal.vue'

const { getAvailableModels, syncUpstreamModels, copyToClipboard } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  syncUpstreamModels: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels,
      syncUpstreamModels
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
    'admin.accounts.imagePromptDefault': 'Generate a cute orange cat astronaut sticker on a clean pastel background.',
    'admin.accounts.videoPromptDefault': 'A tiny blue square slowly moving left to right on a white background.'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.accounts.imageReceived' && params?.count) {
          return `received-${params.count}`
        }
        if (key === 'admin.accounts.videoReceived' && params?.count) {
          return `video-received-${params.count}`
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

function mountModal(accountOverride: Record<string, unknown> = {}) {
  return mount(AccountTestModal, {
    props: {
      show: false,
      account: {
        id: 42,
        name: 'Gemini Image Test',
        platform: 'gemini',
        type: 'apikey',
        status: 'active',
        ...accountOverride
      }
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
    getAvailableModels.mockReset()
    getAvailableModels.mockResolvedValue([
      { id: 'gemini-2.0-flash', display_name: 'Gemini 2.0 Flash' },
      { id: 'gemini-2.5-flash-image', display_name: 'Gemini 2.5 Flash Image' },
      { id: 'gemini-3.1-flash-image', display_name: 'Gemini 3.1 Flash Image' }
    ])
    syncUpstreamModels.mockReset()
    syncUpstreamModels.mockResolvedValue({
      models: ['gemini-3.1-flash-image', 'gemini-2.5-flash']
    })
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

  it('xAI 测试弹窗优先同步云端模型列表', async () => {
    syncUpstreamModels.mockResolvedValueOnce({
      models: ['grok-4.3', 'grok-build-0.1']
    })

    const wrapper = mountModal({
      name: 'xAI OAuth',
      platform: 'xai',
      type: 'oauth',
      credentials: {}
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(syncUpstreamModels).toHaveBeenCalledWith(42)
    expect(getAvailableModels).not.toHaveBeenCalled()
    expect((wrapper.vm as any).availableModels.map((m: any) => m.id)).toEqual([
      'grok-4.3',
      'grok-build-0.1'
    ])
    expect((wrapper.vm as any).selectedModelId).toBe('grok-4.3')
  })

  it('xAI 测试弹窗保留视频模型并优先选择文本模型', async () => {
    syncUpstreamModels.mockResolvedValueOnce({
      models: ['grok-imagine-video', 'grok-imagine-image', 'grok-4.3']
    })

    const wrapper = mountModal({
      name: 'xAI API Key',
      platform: 'xai',
      type: 'apikey',
      credentials: {}
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect((wrapper.vm as any).availableModels.map((m: any) => m.id)).toEqual([
      'grok-4.3',
      'grok-imagine-image',
      'grok-imagine-video'
    ])
    expect((wrapper.vm as any).selectedModelId).toBe('grok-4.3')
  })

  it('xAI 图片模型测试会携带提示词', async () => {
    syncUpstreamModels.mockResolvedValueOnce({
      models: ['grok-imagine-image']
    })

    const wrapper = mountModal({
      name: 'xAI Image',
      platform: 'xai',
      type: 'apikey',
      credentials: {}
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('draw a small robot holding a camera')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'grok-imagine-image',
      prompt: 'draw a small robot holding a camera'
    })
  })

  it('SSE 错误事件会渲染到输出区', async () => {
    syncUpstreamModels.mockResolvedValueOnce({
      models: ['grok-imagine-image']
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"grok-imagine-image"}\n',
        'data: {"type":"error","error":"xAI 图片生成权限不足或额度用尽"}\n'
      ])
    ) as any

    const wrapper = mountModal({
      name: 'xAI Image',
      platform: 'xai',
      type: 'apikey',
      credentials: {}
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(wrapper.text()).toContain('Error: xAI 图片生成权限不足或额度用尽')
  })

  it('xAI 视频模型测试会携带提示词并渲染视频预览', async () => {
    syncUpstreamModels.mockResolvedValueOnce({
      models: ['grok-imagine-video']
    })
    global.fetch = vi.fn().mockResolvedValue(
      createStreamResponse([
        'data: {"type":"test_start","model":"grok-imagine-video"}\n',
        'data: {"type":"status","text":"xAI video task created: video-task-1"}\n',
        'data: {"type":"video","video_url":"https://cdn.example.test/video-task-1.mp4","mime_type":"video/mp4"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ])
    ) as any

    const wrapper = mountModal({
      name: 'xAI Video',
      platform: 'xai',
      type: 'apikey',
      credentials: {}
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    const promptInput = wrapper.find('textarea.textarea-stub')
    expect(promptInput.exists()).toBe(true)
    await promptInput.setValue('make a tiny blue square move across the frame')

    const buttons = wrapper.findAll('button')
    const startButton = buttons.find((button) => button.text().includes('admin.accounts.startTest'))
    expect(startButton).toBeTruthy()

    await startButton!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(global.fetch).toHaveBeenCalledTimes(1)
    const [, request] = (global.fetch as any).mock.calls[0]
    expect(JSON.parse(request.body)).toEqual({
      model_id: 'grok-imagine-video',
      prompt: 'make a tiny blue square move across the frame'
    })

    const preview = wrapper.find('video[aria-label="test-video-1"]')
    expect(preview.exists()).toBe(true)
    expect(preview.attributes('src')).toBe('https://cdn.example.test/video-task-1.mp4')
  })

  it('其他支持上游同步的平台也会优先同步远端模型列表', async () => {
    syncUpstreamModels.mockResolvedValueOnce({
      models: ['gpt-5.4', 'gpt-image-2']
    })

    const wrapper = mountModal({
      name: 'OpenAI API Key',
      platform: 'openai',
      type: 'apikey',
      credentials: {}
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(syncUpstreamModels).toHaveBeenCalledWith(42)
    expect(getAvailableModels).not.toHaveBeenCalled()
    expect((wrapper.vm as any).availableModels.map((m: any) => m.id)).toEqual([
      'gpt-5.4',
      'gpt-image-2'
    ])
    expect((wrapper.vm as any).selectedModelId).toBe('gpt-5.4')
  })

  it('xAI 测试弹窗保留账号模型映射优先级', async () => {
    const wrapper = mountModal({
      name: 'xAI API Key',
      platform: 'xai',
      type: 'apikey',
      credentials: {
        model_mapping: {
          'grok-alias': 'grok-4.3'
        }
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(syncUpstreamModels).not.toHaveBeenCalled()
    expect(getAvailableModels).toHaveBeenCalledWith(42)
  })
})
