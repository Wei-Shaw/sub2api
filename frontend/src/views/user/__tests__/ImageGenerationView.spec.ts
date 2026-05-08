import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const cachedPublicSettings = vi.hoisted(() => ({ value: { image_generation_enabled: false } }))
const {
  createImageGenerationTask,
  createImageEditTask,
  getImageTask,
  downloadImageTask,
  listKeys,
  getAvailableChannels,
} = vi.hoisted(() => ({
  createImageGenerationTask: vi.fn(),
  createImageEditTask: vi.fn(),
  getImageTask: vi.fn(),
  downloadImageTask: vi.fn(),
  listKeys: vi.fn(),
  getAvailableChannels: vi.fn(),
}))

vi.hoisted(() => {
  let objectURLIndex = 0
  Object.defineProperty(URL, 'createObjectURL', {
    value: vi.fn(() => `blob:mock-${objectURLIndex++}`),
    configurable: true,
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    value: vi.fn(),
    configurable: true,
  })
})

vi.mock('@/api/images', () => ({
  createImageGenerationTask,
  createImageEditTask,
  getImageTask,
  downloadImageTask,
}))

vi.mock('@/api/keys', () => ({
  keysAPI: {
    list: listKeys,
  },
}))

vi.mock('@/api/channels', () => ({
  userChannelsAPI: {
    getAvailable: getAvailableChannels,
  },
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: cachedPublicSettings.value,
    showError: vi.fn(),
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

import ImageGenerationView from '../ImageGenerationView.vue'

describe('ImageGenerationView', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  beforeEach(() => {
    cachedPublicSettings.value = { image_generation_enabled: false }
    createImageGenerationTask.mockReset()
    createImageEditTask.mockReset()
    getImageTask.mockReset()
    downloadImageTask.mockReset()
    getImageTask.mockResolvedValue({
      task_id: 'img_test',
      status: 'succeeded',
      expires_at: '2026-05-08T13:00:00Z',
      download_url: 'data:image/png;base64,aGVsbG8=',
      mime_type: 'image/png',
    })
    downloadImageTask.mockResolvedValue(new Blob(['image'], { type: 'image/png' }))
    listKeys.mockReset()
    getAvailableChannels.mockReset()
    listKeys.mockResolvedValue({
      items: [
        { id: 1, key: 'sk-active', name: '主 Key', status: 'active', expires_at: null, group: { platform: 'openai' } },
        { id: 2, key: 'sk-inactive', name: '停用 Key', status: 'inactive', expires_at: null, group: { platform: 'openai' } },
      ],
    })
    getAvailableChannels.mockResolvedValue([
      {
        name: 'OpenAI',
        platforms: [
          {
            platform: 'openai',
            groups: [],
            supported_models: [
              {
                name: 'gpt-image-2',
                platform: 'openai',
                pricing: { billing_mode: 'image', per_request_price: 0.08, intervals: [] },
              },
              {
                name: 'gpt-5.4',
                platform: 'openai',
                pricing: { billing_mode: 'token', per_request_price: null, intervals: [] },
              },
            ],
          },
        ],
      },
    ])
  })

  it('shows disabled state when the feature flag is off', () => {
    cachedPublicSettings.value = { image_generation_enabled: false }

    const wrapper = mount(ImageGenerationView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
        },
      },
    })

    expect(wrapper.text()).toContain('imageGeneration.disabled')
    expect(wrapper.find('[data-testid="image-generation-form"]').exists()).toBe(false)
  })

  it('renders the generator form when the feature flag is on', () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
        },
      },
    })

    expect(wrapper.find('[data-testid="image-generation-form"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-prompt"]').exists()).toBe(true)
  })

  it('does not render a redundant page header above the image tool', () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)

    expect(wrapper.find('[data-testid="image-generation-page-header"]').exists()).toBe(false)
  })

  it('loads active API keys and image models into the left settings column', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    expect(wrapper.find('[data-testid="image-generation-settings-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-api-key"]').text()).toContain('主 Key')
    expect(wrapper.find('[data-testid="image-generation-api-key"]').text()).not.toContain('停用 Key')
    expect(wrapper.find('[data-testid="image-generation-model"]').text()).toContain('gpt-image-2')
    expect(wrapper.find('[data-testid="image-generation-model"]').text()).not.toContain('gpt-5.4')
  })

  it('only offers OpenAI API keys for image generation', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    listKeys.mockResolvedValue({
      items: [
        { id: 1, key: 'sk-claude', name: 'Claude Key', status: 'active', expires_at: null, group: { platform: 'anthropic' } },
        { id: 2, key: 'sk-openai', name: 'OpenAI Key', status: 'active', expires_at: null, group: { platform: 'openai' } },
      ],
    })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    expect(wrapper.find('[data-testid="image-generation-api-key"]').text()).toContain('OpenAI Key')
    expect(wrapper.find('[data-testid="image-generation-api-key"]').text()).not.toContain('Claude Key')
    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(createImageGenerationTask).toHaveBeenCalledWith(expect.any(Object), 'sk-openai')
  })

  it('keeps the prompt composer in the left settings column and leaves the workspace for results', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    const settingsPanel = wrapper.get('[data-testid="image-generation-settings-panel"]')
    const settingsScroll = wrapper.get('[data-testid="image-generation-settings-scroll"]')
    const workspace = wrapper.get('[data-testid="image-generation-workspace"]')
    const composer = settingsPanel.get('[data-testid="image-generation-composer"]')
    expect(workspace.classes()).toContain('flex')
    expect(settingsPanel.classes()).toEqual(expect.arrayContaining(['h-[calc(100vh-11rem)]', 'min-h-[560px]']))
    expect(settingsPanel.classes()).toContain('overflow-hidden')
    expect(settingsScroll.classes()).toEqual(expect.arrayContaining(['shrink-0', 'space-y-4']))
    expect(settingsScroll.classes().join(' ')).not.toContain('overflow-y-auto')
    expect(composer.classes()).toContain('shrink-0')
    expect(composer.classes()).toContain('space-y-3')
    expect(composer.classes().join(' ')).not.toContain('bottom-')
    expect(composer.find('[data-testid="image-generation-prompt"]').exists()).toBe(true)
    const actions = composer.get('[data-testid="image-generation-actions"]')
    expect(actions.classes()).toEqual(expect.arrayContaining(['grid', 'grid-cols-2', 'gap-2']))
    const submit = composer.find('[data-testid="image-generation-submit"]')
    expect(submit.classes()).not.toContain('w-full')
    expect(submit.classes()).toEqual(expect.arrayContaining(['h-9', 'min-w-0', 'whitespace-nowrap']))
    expect(composer.find('[data-testid="image-generation-options-trigger"]').exists()).toBe(true)
    expect(composer.find('[data-testid="image-generation-options-trigger"]').classes()).toEqual(expect.arrayContaining(['h-9', 'min-w-0']))
    expect(workspace.find('[data-testid="image-generation-composer"]').exists()).toBe(false)
  })

  it('uses a larger fixed reference image area inside the left settings panel', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    const referenceArea = wrapper.get('[data-testid="image-generation-reference-area"]')
    expect(referenceArea.classes()).toEqual(expect.arrayContaining(['min-h-[150px]', 'max-h-[150px]']))
    expect(referenceArea.text()).toContain('imageGeneration.uploadReference')

    const file = new File(['image'], 'ref.png', { type: 'image/png' })
    Object.defineProperty(wrapper.get('[data-testid="image-generation-reference-input"]').element, 'files', {
      value: [file],
      configurable: true,
    })
    await wrapper.get('[data-testid="image-generation-reference-input"]').trigger('change')

    expect(wrapper.get('[data-testid="image-generation-reference-area"]').classes()).toEqual(
      expect.arrayContaining(['min-h-[150px]', 'max-h-[150px]']),
    )
    expect(wrapper.findAll('[data-testid^="image-generation-reference-thumb-"]')).toHaveLength(1)
  })

  it('moves aspect ratio, resolution, and quality controls into the feature options dialog', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    expect(wrapper.find('[data-testid="image-generation-size"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-generation-quality"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-generation-options-dialog"]').exists()).toBe(false)

    await wrapper.get('[data-testid="image-generation-options-trigger"]').trigger('click')

    expect(wrapper.find('[data-testid="image-generation-options-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="image-generation-options-close"]').text()).toBe('✓')
    expect(wrapper.get('[data-testid="image-generation-options-close"]').attributes('aria-label')).toBe('imageGeneration.optionsDone')
    expect(wrapper.get('[data-testid="image-generation-options-close"]').classes()).toEqual(expect.arrayContaining(['h-8', 'w-8', 'rounded-full', 'bg-emerald-500', 'text-white']))
    expect(wrapper.find('[data-testid="image-generation-aspect-1:1"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-aspect-2:3"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-aspect-3:2"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-resolution-1k"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-resolution-2k"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-resolution-4k"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-generation-option-quality"]').exists()).toBe(true)
  })

  it('shows a policy and copyright guidance hint in the empty workspace', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    const composer = wrapper.get('[data-testid="image-generation-composer"]')
    expect(composer.find('[data-testid="image-generation-policy-hint"]').exists()).toBe(false)
    const hint = wrapper.get('[data-testid="image-generation-policy-hint"]')
    expect(hint.text()).toContain('imageGeneration.policyHint')
    expect(wrapper.get('[data-testid="image-generation-workspace"]').text()).toContain('imageGeneration.policyHint')
  })

  it('uses aspect ratio and resolution selections for image generation while keeping defaults when unchanged', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw default')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(createImageGenerationTask).toHaveBeenLastCalledWith(
      expect.objectContaining({ size: '1024x1024', quality: 'auto' }),
      'sk-active',
    )

    await wrapper.get('[data-testid="image-generation-options-trigger"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-aspect-3:2"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-resolution-4k"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-option-quality"]').setValue('high')
    await wrapper.get('[data-testid="image-generation-options-close"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw custom')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(createImageGenerationTask).toHaveBeenLastCalledWith(
      expect.objectContaining({ size: '3072x2048', quality: 'high' }),
      'sk-active',
    )
  })

  it('shows a compact icon-only options trigger with the current aspect, resolution, and quality summary', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    expect(wrapper.get('[data-testid="image-generation-options-trigger"]').text()).not.toContain('imageGeneration.options')
    expect(wrapper.get('[data-testid="image-generation-options-summary"]').text()).toBe('1:1 · 1K · auto')

    await wrapper.get('[data-testid="image-generation-options-trigger"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-aspect-3:2"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-resolution-4k"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-option-quality"]').setValue('high')

    expect(wrapper.get('[data-testid="image-generation-options-summary"]').text()).toBe('3:2 · 4K · high')
  })

  it('uses selected API key and generation endpoint when no reference image is uploaded', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(createImageGenerationTask).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'gpt-image-2', prompt: 'draw a cat' }),
      'sk-active',
    )
    expect(createImageEditTask).not.toHaveBeenCalled()
  })

  it('uses image edit endpoint when reference images are uploaded', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageEditTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    const file = new File(['image'], 'ref.png', { type: 'image/png' })
    Object.defineProperty(wrapper.get('[data-testid="image-generation-reference-input"]').element, 'files', {
      value: [file],
      configurable: true,
    })
    await wrapper.get('[data-testid="image-generation-reference-input"]').trigger('change')
    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('make it cinematic')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(createImageEditTask).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'gpt-image-2', prompt: 'make it cinematic', images: [file] }),
      'sk-active',
    )
  })

  it('shows a friendly policy error while preserving the upstream detail', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
    getImageTask.mockResolvedValue({
      task_id: 'img_test',
      status: 'failed',
      expires_at: '2026-05-08T13:00:00Z',
      error_message: 'Prompt violates content policy',
    })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw protected character')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('imageGeneration.policyRejected')
    expect(wrapper.text()).toContain('Prompt violates content policy')
  })

  it('submits the prompt with Enter and keeps Shift Enter for new lines', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    const prompt = wrapper.get('[data-testid="image-generation-prompt"]')
    await prompt.setValue('draw a cat')
    await prompt.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(createImageGenerationTask).toHaveBeenCalledWith(
      expect.objectContaining({ prompt: 'draw a cat' }),
      'sk-active',
    )

    createImageGenerationTask.mockClear()
    await prompt.setValue('line one')
    await prompt.trigger('keydown', { key: 'Enter', shiftKey: true })
    await flushPromises()

    expect(createImageGenerationTask).not.toHaveBeenCalled()
  })

  it('limits uploaded reference images to two and disables multiple file selection', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    expect(wrapper.get('[data-testid="image-generation-reference-input"]').attributes('multiple')).toBeUndefined()

    const first = new File(['first'], 'first.png', { type: 'image/png' })
    const second = new File(['second'], 'second.png', { type: 'image/png' })
    const third = new File(['third'], 'third.png', { type: 'image/png' })
    Object.defineProperty(wrapper.get('[data-testid="image-generation-reference-input"]').element, 'files', {
      value: [first, second, third],
      configurable: true,
    })
    await wrapper.get('[data-testid="image-generation-reference-input"]').trigger('change')

    expect(wrapper.findAll('[data-testid^="image-generation-reference-thumb-"]')).toHaveLength(2)
  })

  it('downloads generated images from the history card', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 4, 8, 12, 34, 56))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const appendChild = vi.spyOn(document.body, 'appendChild')

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()
    await wrapper.get('[data-testid="image-generation-session-download-0"]').trigger('click')

    expect(click).toHaveBeenCalled()
    const link = appendChild.mock.calls.at(-1)?.[0] as HTMLAnchorElement
    expect(link.download).toBe('image-20260508-123456.png')
    vi.useRealTimers()
  })

  it('shows generation duration in history without rendering revised prompt text', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    vi.useFakeTimers()
    vi.setSystemTime(1_000)
    createImageGenerationTask.mockImplementation(async () => {
      vi.setSystemTime(3_345)
      return { task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" }
    })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    const historyRail = wrapper.get('[data-testid="image-generation-history-rail"]')
    const resultStage = wrapper.get('[data-testid="image-generation-result-stage"]')
    expect(historyRail.text()).toContain('imageGeneration.duration')
    expect(historyRail.text()).toContain('2.3s')
    expect(resultStage.text()).not.toContain('imageGeneration.duration')
    expect(resultStage.text()).not.toContain('2.3s')
    expect(wrapper.text()).not.toContain('internal revised prompt')

    vi.useRealTimers()
  })

  it('uses the previous generated image as a reference for the next generation', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
    createImageEditTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()
    await wrapper.get('[data-testid="image-generation-session-use-reference-0"]').trigger('click')
    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('make it cinematic')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(createImageEditTask).toHaveBeenCalledWith(
      expect.objectContaining({
        prompt: 'make it cinematic',
        images: [expect.any(File)],
      }),
      'sk-active',
    )
  })

  it('reuses the generated image URL for the generated reference preview', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    const generatedSrc = wrapper.get('[data-testid="image-generation-result-image-0"]').attributes('src')
    await wrapper.get('[data-testid="image-generation-session-use-reference-0"]').trigger('click')
    await flushPromises()

    const referenceSrc = wrapper.get('[data-testid="image-generation-reference-thumb-0"] img').attributes('src')
    expect(referenceSrc).toBe(generatedSrc)

    await wrapper.get('[data-testid="image-generation-reference-thumb-0"] button').trigger('click')
    expect(URL.revokeObjectURL).not.toHaveBeenCalledWith(generatedSrc)
  })

  it('clears the prompt and replaces existing references when continuing from a generated image', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    const file = new File(['old reference'], 'old.png', { type: 'image/png' })
    Object.defineProperty(wrapper.get('[data-testid="image-generation-reference-input"]').element, 'files', {
      value: [file],
      configurable: true,
    })
    await wrapper.get('[data-testid="image-generation-reference-input"]').trigger('change')

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('make it cinematic')
    await wrapper.get('[data-testid="image-generation-session-use-reference-0"]').trigger('click')
    await flushPromises()

    expect((wrapper.get('[data-testid="image-generation-prompt"]').element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.findAll('[data-testid^="image-generation-reference-thumb-"]')).toHaveLength(2)
  })

  it('keeps every generated image in a right history rail that can be downloaded and used as a reference', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask
      .mockResolvedValueOnce({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
      .mockResolvedValueOnce({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
    getImageTask
      .mockResolvedValueOnce({ task_id: 'img_test', status: 'succeeded', expires_at: '2026-05-08T13:00:00Z', download_url: 'data:image/png;base64,Zmlyc3Q=' })
      .mockResolvedValueOnce({ task_id: 'img_test', status: 'succeeded', expires_at: '2026-05-08T13:00:00Z', download_url: 'data:image/png;base64,c2Vjb25k' })
    downloadImageTask
      .mockResolvedValueOnce(new Blob(['first'], { type: 'image/png' }))
      .mockResolvedValueOnce(new Blob(['second'], { type: 'image/png' }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()
    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a dog')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    const historyRail = wrapper.get('[data-testid="image-generation-history-rail"]')
    expect(historyRail.text()).toContain('imageGeneration.sessionImages')
    expect(historyRail.classes().join(' ')).toContain('h-[calc(100vh-11rem)]')
    expect(historyRail.classes().join(' ')).toContain('min-h-[560px]')
    expect(historyRail.classes().join(' ')).not.toContain('overflow-y-auto')
    expect(wrapper.get('[data-testid="image-generation-history-list"]').classes().join(' ')).toContain('overflow-y-auto')
    expect(wrapper.findAll('[data-testid^="image-generation-session-image-"]')).toHaveLength(2)

    await wrapper.get('[data-testid="image-generation-session-download-0"]').trigger('click')
    expect(click).toHaveBeenCalled()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('turn first into a sticker')
    await wrapper.get('[data-testid="image-generation-session-use-reference-0"]').trigger('click')
    await flushPromises()

    expect((wrapper.get('[data-testid="image-generation-prompt"]').element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.findAll('[data-testid^="image-generation-reference-thumb-"]')).toHaveLength(1)
  })

  it('clicks history thumbnails to preview that image and highlights the active preview', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask
      .mockResolvedValueOnce({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
      .mockResolvedValueOnce({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })
    getImageTask
      .mockResolvedValueOnce({ task_id: 'img_test', status: 'succeeded', expires_at: '2026-05-08T13:00:00Z', download_url: 'data:image/png;base64,Zmlyc3Q=' })
      .mockResolvedValueOnce({ task_id: 'img_test', status: 'succeeded', expires_at: '2026-05-08T13:00:00Z', download_url: 'data:image/png;base64,c2Vjb25k' })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('first')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()
    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('second')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[data-testid="image-generation-result-image-0"]').attributes('src')).toContain('blob:')
    expect(wrapper.get('[data-testid="image-generation-session-image-1"]').classes().join(' ')).toContain('border-primary-500')

    await wrapper.get('[data-testid="image-generation-session-preview-0"]').trigger('click')

    expect(wrapper.get('[data-testid="image-generation-result-image-0"]').attributes('src')).toContain('blob:')
    expect(wrapper.get('[data-testid="image-generation-session-image-0"]').classes().join(' ')).toContain('border-primary-500')
  })

  it('centers the latest generated image and lets it fill the result workspace', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    createImageGenerationTask.mockResolvedValue({ task_id: "img_test", status: "pending", expires_at: "2026-05-08T13:00:00Z" })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    const resultStage = wrapper.get('[data-testid="image-generation-result-stage"]')
    const image = wrapper.get('[data-testid="image-generation-result-image-0"]')

    expect(resultStage.classes()).toEqual(expect.arrayContaining(['flex', 'items-center', 'justify-center']))
    expect(resultStage.classes().join(' ')).not.toContain('grid')
    expect(resultStage.find('[data-testid="image-generation-download-0"]').exists()).toBe(false)
    expect(resultStage.find('[data-testid="image-generation-use-reference-0"]').exists()).toBe(false)
    expect(resultStage.text()).not.toContain('imageGeneration.duration')
    expect(image.classes()).toEqual(expect.arrayContaining(['max-h-full', 'max-w-full', 'object-contain']))
  })

  it('always shows the right history rail even before any image is generated', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    const workspace = wrapper.get('[data-testid="image-generation-workspace"]')
    const historyRail = wrapper.get('[data-testid="image-generation-history-rail"]')
    expect(workspace.classes()).toEqual(expect.arrayContaining(['h-[calc(100vh-11rem)]', 'min-h-[560px]']))
    expect(historyRail.classes()).toEqual(expect.arrayContaining(['h-[calc(100vh-11rem)]', 'min-h-[560px]']))
    expect(historyRail.text()).toContain('imageGeneration.sessionImages')
    expect(historyRail.text()).toContain('imageGeneration.sessionImagesEmpty')
    expect(wrapper.findAll('[data-testid^="image-generation-session-image-"]')).toHaveLength(0)
  })

  it('shows elapsed generation time while loading', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    vi.useFakeTimers()
    vi.setSystemTime(0)
    let resolveGenerate: (value: { task_id: string; status: string; expires_at: string }) => void = () => {}
    createImageGenerationTask.mockReturnValue(new Promise((resolve) => {
      resolveGenerate = resolve
    }))

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('slow image')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(12_300)

    expect(wrapper.get('[data-testid="image-generation-loading-elapsed"]').text()).toContain('12.3s')

    resolveGenerate({ task_id: 'img_test', status: 'pending', expires_at: '2026-05-08T13:00:00Z' })
    await flushPromises()
    vi.useRealTimers()
  })
})
