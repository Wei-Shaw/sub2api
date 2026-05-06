import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const cachedPublicSettings = vi.hoisted(() => ({ value: { image_generation_enabled: false } }))
const {
  generateImage,
  editImage,
  listKeys,
  getAvailableChannels,
} = vi.hoisted(() => ({
  generateImage: vi.fn(),
  editImage: vi.fn(),
  listKeys: vi.fn(),
  getAvailableChannels: vi.fn(),
}))

vi.hoisted(() => {
  Object.defineProperty(URL, 'createObjectURL', {
    value: vi.fn(() => 'blob:reference-image'),
    configurable: true,
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    value: vi.fn(),
    configurable: true,
  })
})

vi.mock('@/api/images', () => ({
  generateImage,
  editImage,
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
  beforeEach(() => {
    cachedPublicSettings.value = { image_generation_enabled: false }
    generateImage.mockReset()
    editImage.mockReset()
    listKeys.mockReset()
    getAvailableChannels.mockReset()
    listKeys.mockResolvedValue({
      items: [
        { id: 1, key: 'sk-active', name: '主 Key', status: 'active', expires_at: null },
        { id: 2, key: 'sk-inactive', name: '停用 Key', status: 'inactive', expires_at: null },
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

  it('keeps the prompt composer in the left settings column and leaves the workspace for results', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    const settingsPanel = wrapper.get('[data-testid="image-generation-settings-panel"]')
    const workspace = wrapper.get('[data-testid="image-generation-workspace"]')
    const composer = settingsPanel.get('[data-testid="image-generation-composer"]')
    expect(workspace.classes()).toContain('flex')
    expect(composer.classes()).toContain('space-y-3')
    expect(composer.classes().join(' ')).not.toContain('bottom-')
    expect(composer.find('[data-testid="image-generation-prompt"]').exists()).toBe(true)
    expect(composer.find('[data-testid="image-generation-submit"]').classes()).toContain('w-full')
    expect(workspace.find('[data-testid="image-generation-composer"]').exists()).toBe(false)
  })

  it('uses selected API key and generation endpoint when no reference image is uploaded', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    generateImage.mockResolvedValue({ data: [{ b64_json: 'abc' }] })

    const wrapper = mount(ImageGenerationView)
    await flushPromises()

    await wrapper.get('[data-testid="image-generation-prompt"]').setValue('draw a cat')
    await wrapper.get('[data-testid="image-generation-submit"]').trigger('submit')
    await flushPromises()

    expect(generateImage).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'gpt-image-2', prompt: 'draw a cat' }),
      'sk-active',
    )
    expect(editImage).not.toHaveBeenCalled()
  })

  it('uses image edit endpoint when reference images are uploaded', async () => {
    cachedPublicSettings.value = { image_generation_enabled: true }
    editImage.mockResolvedValue({ data: [{ url: 'https://example.com/image.png' }] })

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

    expect(editImage).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'gpt-image-2', prompt: 'make it cinematic', images: [file] }),
      'sk-active',
    )
  })
})
