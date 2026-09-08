import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useImageGeneration } from './useImageGeneration'
import type { GenerateResponse, HistoryRecord, ImageApiKey } from '../types'

const key: ImageApiKey = {
  id: 7,
  key: 'sk-image',
  name: 'Image key',
  status: 'active',
  group: { allow_image_generation: true },
}

function completedResponse(): GenerateResponse {
  return {
    job_id: 'job-1',
    status: 'succeeded',
    result: { images: [{ url: '/plugins/image-generation/api/assets/job-1/result/0', preview_url: '/plugins/image-generation/api/assets/job-1/result/0/preview', revised_prompt: 'A blue lamp' }] },
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createApi(generateResult: GenerateResponse = completedResponse()) {
  return {
    getConfig: vi.fn().mockResolvedValue({
      image_model_capabilities: {
        'gpt-image-2': {
          max_reference_images: 16, max_output_images: 10,
          sizes: { values: ['1024x1024', '1536x1024', '1024x1536'], default: '1024x1024' },
          quality: { values: ['auto', 'low', 'medium', 'high'], default: 'auto' },
          output_formats: { values: ['png', 'jpeg', 'webp'], default: 'png' },
          output_compression: { min: 0, max: 100, default: 100 },
          background: { values: ['auto', 'transparent', 'opaque'], default: 'auto' },
          input_fidelity: { values: ['low', 'high'], default: 'high' },
        },
        'gpt-image-1': {
          max_reference_images: 16, max_output_images: 10,
          sizes: { values: ['1024x1024', '1536x1024', '1024x1536'], default: '1024x1024' },
          quality: { values: ['auto', 'low', 'medium', 'high'], default: 'auto' },
          output_formats: { values: ['png', 'jpeg', 'webp'], default: 'png' },
          output_compression: { min: 0, max: 100, default: 100 },
          background: { values: ['auto', 'transparent', 'opaque'], default: 'auto' },
          input_fidelity: { values: ['low', 'high'], default: 'high' },
        },
        'gemini-2.5-flash-image': {
          max_reference_images: 10, max_output_images: 4,
          aspect_ratios: { values: ['1:1', '2:3', '3:2', '3:4', '4:3', '4:5', '5:4', '9:16', '16:9', '21:9'], default: '1:1' },
          resolutions: { values: ['1K', '2K', '4K'], default: '1K' },
        },
      },
    }),
    getImageGenerationPreference: vi.fn().mockResolvedValue({ last_api_key_id: null }),
    saveImageGenerationPreference: vi.fn().mockResolvedValue({ last_api_key_id: null }),
    uploadReference: vi.fn(),
    listPromptModels: vi.fn().mockResolvedValue({ models: ['gpt-5.1'] }),
    listConversations: vi.fn().mockResolvedValue({ items: [] }),
    listConversationMessages: vi.fn().mockResolvedValue({ items: [] }),
    optimizePrompt: vi.fn().mockResolvedValue({ prompt: 'Optimized prompt', model: 'gpt-5.1' }),
    generate: vi.fn().mockResolvedValue(generateResult),
    retryHistory: vi.fn().mockResolvedValue({ job_id: 'retry-job-1', status: 'pending' }),
    getStatus: vi.fn(),
    cancel: vi.fn(),
    deleteConversation: vi.fn(),
  }
}

describe('useImageGeneration', () => {
  it('restores a saved usable key without rewriting the preference', async () => {
    const api = createApi()
    api.getImageGenerationPreference.mockResolvedValue({ last_api_key_id: 8 })
    const savedKey = { ...key, id: 8 }
    const state = useImageGeneration({ api, loadKeys: async () => [key, savedKey] })

    await state.initialize()

    expect(state.selectedKeyId.value).toBe(8)
    expect(api.saveImageGenerationPreference).not.toHaveBeenCalled()
  })

  it('falls back to the first usable key and persists it when the saved key is unavailable', async () => {
    const api = createApi()
    api.getImageGenerationPreference.mockResolvedValue({ last_api_key_id: 99 })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    await state.initialize()

    expect(state.selectedKeyId.value).toBe(7)
    expect(api.saveImageGenerationPreference).toHaveBeenCalledWith(7)
  })

  it('falls back when the saved key is filtered out of image generation', async () => {
    const api = createApi()
    api.getImageGenerationPreference.mockResolvedValue({ last_api_key_id: 8 })
    const unavailable = { ...key, id: 8, status: 'disabled' }
    const state = useImageGeneration({ api, loadKeys: async () => [unavailable, key] })

    await state.initialize()

    expect(state.selectedKeyId.value).toBe(7)
    expect(api.saveImageGenerationPreference).toHaveBeenCalledWith(7)
  })

  it('persists a user key selection after initialization', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key, { ...key, id: 8 }] })
    await state.initialize()

    state.selectedKeyId.value = 8
    await nextTick()
    await Promise.resolve()

    expect(api.saveImageGenerationPreference).toHaveBeenCalledWith(8)
  })

  it('persists null for an empty usable key list and retains user selection on save failure', async () => {
    const api = createApi()
    api.getImageGenerationPreference.mockResolvedValue({ last_api_key_id: 99 })
    const empty = useImageGeneration({ api, loadKeys: async () => [] })
    await empty.initialize()
    expect(api.saveImageGenerationPreference).toHaveBeenCalledWith(null)

    const changing = createApi()
    changing.saveImageGenerationPreference.mockRejectedValue(new Error('sync failed'))
    const state = useImageGeneration({ api: changing, loadKeys: async () => [key, { ...key, id: 8 }] })
    await state.initialize()
    state.selectedKeyId.value = 8
    await nextTick()
    await Promise.resolve()
    expect(state.selectedKeyId.value).toBe(8)
    expect(state.errorMessage.value).toBe('sync failed')
  })
  it('appends uploaded references, ignores duplicates, and removes one by id', async () => {
    const api = createApi()
    api.uploadReference
      .mockResolvedValueOnce({
        name: 'first.png', mime_type: 'image/png', storage_key: 'uploads/first/original',
        preview_storage_key: 'uploads/first/preview', original_url: '/first/original', preview_url: '/first/preview',
      })
      .mockResolvedValueOnce({
        name: 'second.png', mime_type: 'image/png', storage_key: 'uploads/second/original',
        preview_storage_key: 'uploads/second/preview', original_url: '/second/original', preview_url: '/second/preview',
      })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()

    await state.uploadReference([
      new File(['first'], 'first.png', { type: 'image/png' }),
      new File(['second'], 'second.png', { type: 'image/png' }),
    ])
    state.setReference({
      id: 'uploads/second/original', dataUrl: '/second/preview', storageKey: 'uploads/second/original',
      fileName: 'second.png', mimeType: 'image/png',
    })

    expect(state.activeConversation.value?.referenceImages.map(item => item.fileName)).toEqual(['first.png', 'second.png'])
    state.removeReference('uploads/second/original')
    expect(state.activeConversation.value?.referenceImages.map(item => item.fileName)).toEqual(['first.png'])
    state.clearReferences()
    expect(state.activeConversation.value?.referenceImages).toEqual([])
  })

  it('clamps output count to the selected model and sends it', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()
    state.outputCount.value = 8
    state.model.value = 'gemini-2.5-flash-image'
    await nextTick()
    expect(state.maxOutputImages.value).toBe(4)
    expect(state.outputCount.value).toBe(4)
    state.prompt.value = 'four variations'
    await state.submit()
    expect(api.generate).toHaveBeenCalledWith(expect.objectContaining({ output_count: 4 }))
  })

  it('derives parameter options and defaults from the selected model', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    await state.initialize()

    expect(state.availableSizes.value).toEqual(['1024x1024', '1536x1024', '1024x1536'])
    expect(state.availableQualities.value).toEqual(['auto', 'low', 'medium', 'high'])
    expect(state.quality.value).toBe('auto')
    expect(state.outputFormat.value).toBe('png')
    expect(state.background.value).toBe('auto')
  })

  it('uses all advertised Gemini ratios and clears GPT-only settings', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()
    state.quality.value = 'high'

    state.model.value = 'gemini-2.5-flash-image'
    await nextTick()

    expect(state.availableAspectRatios.value).toEqual(['1:1', '2:3', '3:2', '3:4', '4:3', '4:5', '5:4', '9:16', '16:9', '21:9'])
    expect(state.aspectRatio.value).toBe('1:1')
    expect(state.resolution.value).toBe('1K')
    expect(state.quality.value).toBe('')
    expect(state.size.value).toBe('')
  })

  it('hides advanced settings and omits stale values for an unconfigured model', async () => {
    const customKey: ImageApiKey = {
      ...key,
      group: { allow_image_generation: true, models_list_config: { enabled: true, models: ['gpt-image-custom'] } },
    }
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [customKey] })
    await state.initialize()
    state.quality.value = 'high'
    state.model.value = 'gpt-image-custom'
    await nextTick()
    state.prompt.value = 'draw a lamp'

    await state.submit()

    expect(state.availableQualities.value).toEqual([])
    expect(state.quality.value).toBe('')
    const request = api.generate.mock.calls[0][0] as Record<string, unknown>
    expect(request).not.toHaveProperty('quality')
    expect(request).not.toHaveProperty('aspect_ratio')
  })

  it('serializes only active model parameters', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()
    state.outputFormat.value = 'webp'
    await nextTick()
    state.quality.value = 'high'
    state.outputCompression.value = 82
    state.background.value = 'transparent'
    state.prompt.value = 'draw a lamp'

    await state.submit()

    expect(api.generate).toHaveBeenCalledWith(expect.objectContaining({
      size: '1024x1024', quality: 'high', output_format: 'webp', output_compression: 82, background: 'transparent',
    }))
    expect(state.activeConversation.value?.messages[0].requestSettings?.[0].detailsLabel).toContain('画质: 高画质')
    expect(state.activeConversation.value?.messages[0].requestSettings?.[0].detailsLabel).toContain('背景: 透明背景')
    expect(state.activeConversation.value?.messages[0].requestSettings?.[0].detailsLabel).toContain('格式: webp')
    expect(state.activeConversation.value?.messages[0].requestSettings?.[0].detailsLabel).toContain('压缩: 82%')
  })

  it('submits the selected API key id without the key secret or duplicate metadata', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()
    state.prompt.value = 'draw a lamp'

    await state.submit()

    const request = api.generate.mock.calls[0][0] as Record<string, unknown>
    expect(request.api_key_id).toBe(key.id)
    expect(request).not.toHaveProperty('provider_api_key')
    expect(request.inputs).not.toHaveProperty('api_key_id')
  })

  it('uploads only files within the remaining model capacity', async () => {
    const api = createApi()
    api.getConfig.mockResolvedValue({
      image_model_capabilities: { 'gpt-image-2': { max_reference_images: 1 } },
    })
    api.uploadReference.mockResolvedValue({
      name: 'first.png', mime_type: 'image/png', storage_key: 'uploads/first/original',
      preview_storage_key: 'uploads/first/preview', original_url: '/first/original', preview_url: '/first/preview',
    })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()

    await state.uploadReference([
      new File(['first'], 'first.png', { type: 'image/png' }),
      new File(['second'], 'second.png', { type: 'image/png' }),
    ])

    expect(api.uploadReference).toHaveBeenCalledTimes(1)
    expect(state.activeConversation.value?.referenceImages).toHaveLength(1)
    expect(state.errorMessage.value).toContain('最多支持 1 张参考图')
  })

  it('keeps references and blocks submission when the selected model limit is exceeded', async () => {
    const api = createApi()
    const limitedKey: ImageApiKey = {
      ...key,
      group: { allow_image_generation: true, models_list_config: { enabled: true, models: ['gpt-image-2', 'custom-image-model'] } },
    }
    const state = useImageGeneration({ api, loadKeys: async () => [limitedKey] })
    await state.initialize()
    state.setReference({ id: 'first', dataUrl: '/first', fileName: 'first.png', mimeType: 'image/png' })
    state.setReference({ id: 'second', dataUrl: '/second', fileName: 'second.png', mimeType: 'image/png' })

    state.model.value = 'custom-image-model'
    state.prompt.value = 'Edit both images'
    await state.submit()

    expect(state.activeConversation.value?.referenceImages).toHaveLength(2)
    expect(state.maxReferenceImages.value).toBe(1)
    expect(state.referenceLimitExceeded.value).toBe(true)
    expect(api.generate).not.toHaveBeenCalled()
  })

  it('serializes every selected reference for a compatible model', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()
    state.setReference({ id: 'first', dataUrl: '/first', storageKey: 'uploads/first/original', fileName: 'first.png', mimeType: 'image/png' })
    state.setReference({ id: 'second', dataUrl: '/second', storageKey: 'uploads/second/original', fileName: 'second.png', mimeType: 'image/png' })
    state.prompt.value = 'Use both references'

    await state.submit()

    expect(api.generate).toHaveBeenCalledWith(expect.objectContaining({
      reference_images: [
        expect.objectContaining({ storage_key: 'uploads/first/original' }),
        expect.objectContaining({ storage_key: 'uploads/second/original' }),
      ],
    }))
  })

  it('uploads a reference before generation and submits storage metadata without base64', async () => {
    const api = createApi()
    api.uploadReference.mockResolvedValue({
      name: 'reference.png', mime_type: 'image/png',
      storage_key: 'image-generation/uploads/7/upload-1/original',
      preview_storage_key: 'image-generation/uploads/7/upload-1/preview',
      original_url: '/plugins/image-generation/api/references/upload-1/original',
      preview_url: '/plugins/image-generation/api/references/upload-1/preview',
    })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()

    await state.uploadReference([new File(['png-bytes'], 'reference.png', { type: 'image/png' })])
    state.prompt.value = 'Edit this image'
    await state.submit()

    expect(api.generate).toHaveBeenCalledWith(expect.objectContaining({
      input_fidelity: 'high',
      reference_images: [expect.objectContaining({
        storage_key: 'image-generation/uploads/7/upload-1/original',
        preview_storage_key: 'image-generation/uploads/7/upload-1/preview',
        data_url: undefined,
      })],
    }))
    expect(state.activeConversation.value?.messages[0].requestSettings?.[0].detailsLabel).toContain('保真度: 高保真')
  })

  it('loads summaries before the selected conversation details', async () => {
    const api = createApi()
    api.listConversations.mockResolvedValue({ items: [{ id: 'conversation-1', title: 'Lamp', preview: 'Latest', status: 'succeeded', updated_at: '2026-07-11T10:00:00Z' }] })
    api.listConversationMessages.mockResolvedValue({ items: [{
      id: 'history-1', conversation_id: 'conversation-1', user_id: 1, prompt: 'Create a lamp', status: 'succeeded',
      request: {
        model: 'gpt-image-2', size: '1024x1024', output_count: 3,
        quality: 'high', background: 'transparent', input_fidelity: 'high',
      }, result: { images: [{ url: '/original.png', preview_url: '/preview.jpg' }] },
      created_at: '2026-07-11T09:59:00Z', updated_at: '2026-07-11T10:00:00Z',
    }] })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    await state.initialize()
    expect(state.conversations.value[0].messages[0].requestSettings?.[0].countLabel).toBe('数量: 3')
    expect(state.conversations.value[0].messages[0].requestSettings?.[0].detailsLabel).toContain('画质: 高画质')
    expect(state.conversations.value[0].messages[0].requestSettings?.[0].detailsLabel).toContain('背景: 透明背景')
    expect(state.conversations.value[0].messages[0].requestSettings?.[0].detailsLabel).toContain('保真度: 高保真')

    expect(api.listConversationMessages).toHaveBeenCalledWith('conversation-1', '')
    expect(state.conversations.value[0].messages).toHaveLength(2)
    expect(state.conversations.value[0].messages[1].content).toBe('生成结果')
    expect(state.conversations.value[0].messages[1].images?.[0].src).toContain('/preview.jpg')
    expect(state.conversations.value[0].messages[1].images?.[0].revisedPrompt).toBe('Create a lamp')
  })

  it('restores variant labels for angle images from history requests', async () => {
    const api = createApi()
    api.listConversations.mockResolvedValue({ items: [{ id: 'conversation-1', title: 'Character', preview: 'Latest', status: 'succeeded', updated_at: '2026-07-11T10:00:00Z' }] })
    api.listConversationMessages.mockResolvedValue({ items: [{
      id: 'history-angles', conversation_id: 'conversation-1', user_id: 1, prompt: 'Create a character', status: 'succeeded',
      request: {
        model: 'gpt-image-2', size: '1024x1024',
        variants: [{ label: '正面', prompt: 'front' }, { label: '背面', prompt: 'back' }],
      },
      result: { images: [{ url: '/front.png' }, { url: '/back.png' }] },
      created_at: '2026-07-11T09:59:00Z', updated_at: '2026-07-11T10:00:00Z',
    }] })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    await state.initialize()

    const assistants = state.conversations.value[0].messages.filter(message => message.role === 'assistant')
    expect(assistants).toHaveLength(1)
    expect(assistants[0].images?.map(image => image.variantLabel)).toEqual(['正面', '背面'])
  })

  it('restores a canceled multi-image generation group as one conversation exchange', async () => {
    const api = createApi()
    api.listConversations.mockResolvedValue({ items: [{ id: 'conversation-1', title: 'Lamp', preview: 'Canceled', status: 'canceled', updated_at: '2026-07-16T10:00:00Z' }] })
    api.listConversationMessages.mockResolvedValue({ items: Array.from({ length: 4 }, (_, index) => ({
      id: `history-canceled-${index + 1}`, conversation_id: 'conversation-1', user_id: 1, prompt: 'Create a lamp', status: 'canceled',
      request: { model: 'gpt-image-2', size: '1024x1024', output_count: 1, generation_group_id: 'group-repeat-1' },
      error_message: '生成已取消', created_at: `2026-07-16T09:59:0${index}Z`, updated_at: '2026-07-16T10:00:00Z',
    })) })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    await state.initialize()

    expect(state.conversations.value[0].messages).toHaveLength(2)
    expect(state.conversations.value[0].messages[0].content).toBe('Create a lamp')
    expect(state.conversations.value[0].messages[0].requestSettings?.[0].countLabel).toBe('数量: 4')
    expect(state.conversations.value[0].messages[1]).toEqual(expect.objectContaining({ status: 'canceled', content: '生成已取消' }))
  })

  it('restores successful and failed slots from a partially completed history group', async () => {
    const api = createApi()
    api.listConversations.mockResolvedValue({ items: [{ id: 'conversation-1', title: 'Lamp', preview: 'Partial', status: 'failed', updated_at: '2026-07-16T10:00:00Z' }] })
    api.listConversationMessages.mockResolvedValue({ items: [
      {
        id: 'history-success', conversation_id: 'conversation-1', user_id: 1, prompt: 'Create two lamps', status: 'succeeded',
        request: { model: 'gpt-image-2', size: '1024x1024', output_count: 1, generation_group_id: 'group-1' },
        result: { images: [{ url: '/success.png' }] }, created_at: '2026-07-16T09:59:00Z', updated_at: '2026-07-16T10:00:00Z',
      },
      {
        id: 'history-failed', conversation_id: 'conversation-1', user_id: 1, prompt: 'Create two lamps', status: 'failed',
        request: { model: 'gpt-image-2', size: '1024x1024', output_count: 1, generation_group_id: 'group-1' },
        error_message: 'second image failed', created_at: '2026-07-16T09:59:01Z', updated_at: '2026-07-16T10:00:01Z',
      },
    ] })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    await state.initialize()

    const assistant = state.conversations.value[0].messages[1]
    expect(assistant.generationSlots).toHaveLength(2)
    expect(assistant.generationSlots?.[0].image?.src).toContain('/success.png')
    expect(assistant.generationSlots?.[1]).toEqual(expect.objectContaining({ status: 'failed', error: 'second image failed' }))
  })

  it('shows completed images while the rest of a history group is pending', async () => {
    const api = createApi()
    api.getStatus.mockResolvedValue({
      job_id: 'history-pending', status: 'succeeded', result: { images: [{ url: '/pending-success.png' }] },
    })
    api.listConversations.mockResolvedValue({ items: [{ id: 'conversation-1', title: 'Lamp', preview: 'Generating', status: 'pending', updated_at: '2026-07-17T10:00:00Z' }] })
    api.listConversationMessages.mockResolvedValue({ items: [
      {
        id: 'history-success', conversation_id: 'conversation-1', user_id: 1, prompt: 'Create two lamps', status: 'succeeded',
        request: { model: 'gpt-image-2', size: '1024x1024', output_count: 1, generation_group_id: 'group-1' },
        result: { images: [{ url: '/success.png' }] }, created_at: '2026-07-17T09:59:00Z', updated_at: '2026-07-17T10:00:00Z',
      },
      {
        id: 'history-pending', conversation_id: 'conversation-1', user_id: 1, prompt: 'Create two lamps', status: 'pending',
        request: { model: 'gpt-image-2', size: '1024x1024', output_count: 1, generation_group_id: 'group-1' },
        created_at: '2026-07-17T09:59:01Z', updated_at: '2026-07-17T10:00:01Z',
      },
    ] })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })

    await state.initialize()

    const assistant = state.conversations.value[0].messages[1]
    expect(assistant.status).toBe('pending')
    expect(assistant.images?.[0].src).toContain('/success.png')
    expect(assistant.generationSlots).toHaveLength(2)
    expect(assistant.generationSlots?.[0]).toEqual(expect.objectContaining({ status: 'succeeded' }))
    expect(assistant.generationSlots?.[1]).toEqual(expect.objectContaining({ status: 'pending' }))

    await vi.waitFor(() => {
      const completed = state.conversations.value[0].messages[1]
      expect(completed.generationSlots?.map(slot => slot.status)).toEqual(['succeeded', 'succeeded'])
      expect(completed.images?.[1].src).toContain('/pending-success.png')
    })
  })

  it('keeps the latest reference when older messages are loaded', async () => {
    const api = createApi()
    api.listConversations.mockResolvedValue({ items: [{ id: 'conversation-1', title: 'Lamp', preview: 'Latest', status: 'succeeded', updated_at: '2026-07-11T10:00:00Z' }] })
    api.listConversationMessages
      .mockResolvedValueOnce({
        items: [{
          id: 'history-new', conversation_id: 'conversation-1', user_id: 1, prompt: 'New prompt', status: 'succeeded',
          request: { model: 'gpt-image-2', size: '1024x1024', reference_images: [{ name: 'new-reference.png', data_url: 'data:image/png;base64,new' }] },
          result: { images: [] }, created_at: '2026-07-11T09:59:00Z', updated_at: '2026-07-11T10:00:00Z',
        }],
        next_cursor: 'older-cursor',
      })
      .mockResolvedValueOnce({
        items: [{
          id: 'history-old', conversation_id: 'conversation-1', user_id: 1, prompt: 'Old prompt', status: 'succeeded',
          request: { model: 'gpt-image-2', size: '1024x1024', reference_images: [{ name: 'old-reference.png', data_url: 'data:image/png;base64,old' }] },
          result: { images: [] }, created_at: '2026-07-10T09:59:00Z', updated_at: '2026-07-10T10:00:00Z',
        }],
        next_cursor: '',
      })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    await state.initialize()
    await state.loadOlderMessages()

    expect(api.listConversationMessages).toHaveBeenLastCalledWith('conversation-1', 'older-cursor')
    expect(state.activeConversation.value?.messages).toHaveLength(2)
    expect(state.activeConversation.value?.referenceImages[0]?.fileName).toBe('new-reference.png')
  })

  it('creates a renderable conversation before initialization requests finish', async () => {
    let resolveHistory!: (value: { items: [] }) => void
    const api = createApi()
    api.listConversations.mockReturnValue(new Promise(resolve => { resolveHistory = resolve }))
    const state = useImageGeneration({ api, loadKeys: async () => [key] })

    const initialization = state.initialize()

    expect(state.activeConversation.value).not.toBeNull()
    resolveHistory({ items: [] })
    await initialization
  })

  it('replaces the optimistic assistant message with generated images', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = 'Create a lamp'

    await state.submit()

    expect(state.activeConversation.value?.messages.map(message => message.role)).toEqual(['user', 'assistant'])
    expect(state.activeConversation.value?.messages[1].content).toBe('生成结果')
    expect(state.activeConversation.value?.messages[1].images?.[0].src).toContain('/preview')
  })

  it('falls back to the sent prompt when the result has no revised prompt', async () => {
    const api = createApi({
      job_id: 'job-1',
      status: 'succeeded',
      result: { images: [{ url: '/plugins/image-generation/api/assets/job-1/result/0', revised_prompt: '' }] },
    })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = '一只坐在客厅地毯上的小狗'

    await state.submit()

    expect(state.activeConversation.value?.messages[1].images?.[0].revisedPrompt).toBe('一只坐在客厅地毯上的小狗')
    expect(state.activeConversation.value?.messages[1].content).toBe('生成结果')
  })

  it('renders a failed message when a completed response has no image', async () => {
    const api = createApi({ job_id: 'job-1', status: 'succeeded', result: { images: [] } })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = 'Create a lamp'

    await state.submit()

    expect(state.activeConversation.value?.messages[1].status).toBe('failed')
    expect(state.activeConversation.value?.messages[1].content).toContain('图片生成未返回可显示的图片')
  })

  it('polls a pending job and can cancel it', async () => {
    const api = createApi({ job_id: 'history-1', status: 'pending' })
    api.getStatus.mockResolvedValue({ job_id: 'history-1', status: 'pending' })
    api.cancel.mockResolvedValue({ job_id: 'history-1', status: 'canceled', error_message: 'stopped' })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 60_000 })
    await state.initialize()
    state.prompt.value = 'Create a lamp'
    await state.submit()

    expect(state.generationStatus.value).toBe('polling')
    await state.cancelGeneration()
    expect(api.cancel).toHaveBeenCalledWith('history-1')
    expect(state.activeConversation.value?.messages[1].status).toBe('canceled')
    state.dispose()
  })

  it('shows images returned by a pending job before polling completes', async () => {
    const api = createApi({ job_id: 'angle-job', status: 'pending' })
    const completed = deferred<GenerateResponse>()
    api.getStatus
      .mockResolvedValueOnce({
        job_id: 'angle-job', status: 'pending',
        result: { images: [{ url: '/front.png', variant_label: '正面' }] },
      })
      .mockImplementationOnce(() => completed.promise)
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = 'Create a character'
    state.presetSelection.value = {
      styles: [], scenes: [], effects: [], angles: ['front', 'back'], separateAngleImages: true, keepAngleConsistency: false,
    }

    await state.submit()

    await vi.waitFor(() => {
      const assistant = state.activeConversation.value?.messages[1]
      expect(assistant?.status).toBe('pending')
      expect(assistant?.generationSlots?.[0].image?.src).toContain('/front.png')
      expect(assistant?.generationSlots?.[1].status).toBe('pending')
    })
    completed.resolve({
      job_id: 'angle-job', status: 'succeeded',
      result: { images: [{ url: '/front.png', variant_label: '正面' }, { url: '/back.png', variant_label: '背面' }] },
    })
    await vi.waitFor(() => {
      const assistant = state.activeConversation.value?.messages[1]
      expect(assistant?.generationSlots?.map(slot => slot.status)).toEqual(['succeeded', 'succeeded'])
      expect(assistant?.generationSlots?.[1].image?.src).toContain('/back.png')
    })
  })

  it('cancels a backend job that arrives after submission was canceled', async () => {
    const submission = deferred<GenerateResponse>()
    const api = createApi()
    api.generate.mockImplementationOnce(() => submission.promise)
    api.cancel.mockResolvedValue({ job_id: 'late-job', status: 'canceled' })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 60_000 })
    await state.initialize()
    state.prompt.value = 'Create a lamp'

    const pendingSubmit = state.submit()
    await Promise.resolve()
    await state.cancelGeneration()
    submission.resolve({ job_id: 'late-job', status: 'pending' })
    await pendingSubmit

    expect(api.cancel).toHaveBeenCalledWith('late-job')
    expect(state.activeConversation.value?.messages[1].status).toBe('canceled')
    state.dispose()
  })

  it('retries the original history task when generating an image again', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.outputCount.value = 3
    state.prompt.value = 'Create a lamp'
    await state.submit()
    const image = state.activeConversation.value!.messages[1].images![0]
    state.outputCount.value = 1

    await state.repeatFromImage(image, 'Try another')

    expect(api.generate).toHaveBeenCalledTimes(3)
    expect(api.retryHistory).toHaveBeenCalledWith('job-1', expect.objectContaining({ generation_group_id: expect.stringMatching(/^generation-/) }))
  })

  it('keeps internal generation instructions out of the editable refinement prompt', () => {
    const state = useImageGeneration({ api: createApi(), loadKeys: async () => [key], pollInterval: 1 })

    state.refineFromImage({
      id: 'image-1', src: '/image.png', createdAt: 'now',
      revisedPrompt: 'Follow the user request with highest priority.\nUser request: 蓝色玻璃台灯。只生成这个角度的一张独立图片；保持主体身份、服装、比例、材质、场景和光线与其他角度一致。',
    })

    expect(state.prompt.value).toBe('蓝色玻璃台灯')
  })

  it('adds cross-angle consistency only when the preset option is enabled', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = '红色夹克角色'
    state.presetSelection.value = {
      styles: [], scenes: [], effects: [], angles: ['front', 'back'], separateAngleImages: true, keepAngleConsistency: false,
    } as never

    await state.submit()
    expect(api.generate.mock.calls[0][0].variants[0].prompt).not.toContain('保持主体身份')

    state.prompt.value = '红色夹克角色'
    state.presetSelection.value = { ...state.presetSelection.value, keepAngleConsistency: true } as never
    await state.submit()
    expect(api.generate.mock.calls[1][0].variants[0].prompt).toContain('保持主体身份')
  })

  it('submits selected angles as one multi-view collage by default', async () => {
    const api = createApi()
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = '红色夹克角色'
    state.presetSelection.value = {
      styles: ['cinematic'],
      scenes: ['studio'],
      effects: ['dramatic_light'],
      angles: ['front', 'back'],
      separateAngleImages: false,
      keepAngleConsistency: false,
    }

    await state.submit()

    expect(api.generate).toHaveBeenCalledTimes(1)
    expect(api.generate).toHaveBeenCalledWith(expect.objectContaining({
      output_count: 1,
      prompt: expect.stringContaining('多视角拼图'),
    }))
    expect(api.generate.mock.calls[0][0]).not.toHaveProperty('variants')
    expect(state.activeConversation.value?.messages.filter(message => message.role === 'assistant')).toHaveLength(1)
  })

  it('submits separate angle images as variants in one conversation message', async () => {
    const api = createApi({
      job_id: 'angle-set', status: 'succeeded', result: {
        images: [
          { url: '/plugins/image-generation/api/assets/angle-set/result/0', variant_label: '正面' },
          { url: '/plugins/image-generation/api/assets/angle-set/result/1', variant_label: '背面' },
        ],
      },
    })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = '红色夹克角色'
    state.presetSelection.value = {
      styles: ['cinematic'], scenes: ['studio'], effects: [], angles: ['front', 'back'], separateAngleImages: true,
      keepAngleConsistency: false,
    }

    await state.submit()

    expect(api.generate).toHaveBeenCalledTimes(1)
    expect(api.generate).toHaveBeenCalledWith(expect.objectContaining({
      output_count: 2,
      variants: [
        expect.objectContaining({ label: '正面', prompt: expect.stringContaining('角度：正面') }),
        expect.objectContaining({ label: '背面', prompt: expect.stringContaining('角度：背面') }),
      ],
    }))
    const assistant = state.activeConversation.value?.messages.filter(message => message.role === 'assistant') ?? []
    expect(assistant).toHaveLength(1)
    expect(assistant[0].images?.map(image => image.variantLabel)).toEqual(['正面', '背面'])
  })

  it('keeps successful angle images and reports failed angles in the same message', async () => {
    const api = createApi({
      job_id: 'partial-angles', status: 'succeeded', result: {
        images: [{ url: '/plugins/image-generation/api/assets/partial-angles/result/0', variant_label: '正面' }],
        failed_variants: ['背面'],
      },
    })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = '红色夹克角色'
    state.presetSelection.value = {
      styles: [], scenes: [], effects: [], angles: ['front', 'back'], separateAngleImages: true,
      keepAngleConsistency: false,
    }

    await state.submit()

    const assistant = state.activeConversation.value?.messages.filter(message => message.role === 'assistant') ?? []
    expect(assistant).toHaveLength(1)
    expect(assistant[0].images?.map(image => image.variantLabel)).toEqual(['正面'])
    expect(assistant[0].content).toContain('背面')
  })

  it('fills generation slots as independent GPT images complete or fail', async () => {
    const first = deferred<GenerateResponse>()
    const second = deferred<GenerateResponse>()
    const api = createApi()
    api.generate.mockImplementationOnce(() => first.promise).mockImplementationOnce(() => second.promise)
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.outputCount.value = 2
    state.prompt.value = 'Create two lamps'

    const submission = state.submit()
    await vi.waitFor(() => expect(api.generate).toHaveBeenCalledTimes(2))
    const pending = state.activeConversation.value?.messages.filter(message => message.role === 'assistant') ?? []
    expect(pending).toHaveLength(1)
    expect(pending[0].generationSlots?.map(slot => slot.progress)).toEqual([1, 1])
    first.resolve({
      job_id: 'job-first', status: 'succeeded',
      result: { images: [{ url: '/plugins/image-generation/api/assets/job-first/result/0', preview_url: '/plugins/image-generation/api/assets/job-first/result/0/preview' }] },
    })

    await vi.waitFor(() => {
      const assistants = state.activeConversation.value?.messages.filter(message => message.role === 'assistant') ?? []
      expect(assistants).toHaveLength(1)
      expect(assistants[0].generationSlots?.[0].image?.src).toContain('job-first')
      expect(assistants[0].generationSlots?.[1].status).toBe('pending')
    })

    second.reject(new Error('second image failed'))
    await submission
    const final = state.activeConversation.value?.messages.filter(message => message.role === 'assistant') ?? []
    expect(final).toHaveLength(1)
    expect(final[0].generationSlots?.[1]).toEqual(expect.objectContaining({ status: 'failed', error: 'second image failed' }))
  })

  it('keeps a successful GPT image when a sibling task fails', async () => {
    const api = createApi()
    api.generate
      .mockResolvedValueOnce({
        job_id: 'job-success', status: 'succeeded',
        result: { images: [{ url: '/plugins/image-generation/api/assets/job-success/result/0', preview_url: '/plugins/image-generation/api/assets/job-success/result/0/preview' }] },
      })
      .mockRejectedValueOnce(new Error('second image failed'))
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.outputCount.value = 2
    state.prompt.value = 'Create two lamps'

    await state.submit()

    const assistants = state.activeConversation.value?.messages.filter(message => message.role === 'assistant') ?? []
    expect(assistants).toHaveLength(1)
    expect(assistants[0].generationSlots?.[0].image?.src).toContain('job-success')
    expect(assistants[0].generationSlots?.[1]).toEqual(expect.objectContaining({ status: 'failed', error: 'second image failed' }))
  })

  it('cancels every unfinished GPT image task and preserves completed results', async () => {
    const api = createApi({ job_id: 'unused', status: 'pending' })
    api.generate
      .mockResolvedValueOnce({ job_id: 'job-complete', status: 'succeeded', result: { images: [{ url: '/plugins/image-generation/api/assets/job-complete/result/0' }] } })
      .mockResolvedValueOnce({ job_id: 'job-pending-1', status: 'pending' })
      .mockResolvedValueOnce({ job_id: 'job-pending-2', status: 'pending' })
    api.cancel.mockImplementation(async (id: string) => ({ job_id: id, status: 'canceled' as const, error_message: 'stopped' }))
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 60_000 })
    await state.initialize()
    state.outputCount.value = 3
    state.prompt.value = 'Create three lamps'

    await state.submit()
    await state.cancelGeneration()

    expect(api.cancel).toHaveBeenCalledTimes(2)
    expect(api.cancel).toHaveBeenCalledWith('job-pending-1')
    expect(api.cancel).toHaveBeenCalledWith('job-pending-2')
    const assistants = state.activeConversation.value?.messages.filter(message => message.role === 'assistant') ?? []
    expect(assistants).toHaveLength(1)
    expect(assistants[0].generationSlots?.[0].image?.src).toContain('job-complete')
    expect(assistants[0].generationSlots?.slice(1).every(slot => slot.status === 'canceled')).toBe(true)
  })

  it('applies presets visibly to the prompt and replaces the previous preset text', async () => {
    const state = useImageGeneration({ api: createApi(), loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = '红色夹克角色'

    state.applyPresetSelection({ styles: ['cinematic'], scenes: [], effects: [], angles: ['front', 'back'], separateAngleImages: false, keepAngleConsistency: false })
    expect(state.prompt.value).toContain('风格：电影感')
    expect(state.prompt.value).toContain('角度：正面、背面')

    state.applyPresetSelection({ styles: ['anime'], scenes: ['night'], effects: [], angles: [], separateAngleImages: false, keepAngleConsistency: false })
    expect(state.prompt.value).toBe('红色夹克角色\n风格：动漫\n场景：夜景')
    expect(state.prompt.value).not.toContain('电影感')
  })

  it('copies the revised prompt back for continued refinement', async () => {
    const state = useImageGeneration({ api: createApi(), loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()

    state.refineFromImage({ id: 'image-1', src: 'data:image/png;base64,abc', revisedPrompt: '蓝色玻璃台灯', createdAt: 'now' })

    expect(state.prompt.value).toBe('蓝色玻璃台灯')
  })

  it('optimizes the current prompt with a model loaded from the selected key', async () => {
    const api = createApi()
    api.listPromptModels.mockResolvedValue({ models: ['gpt-5.1', 'claude-sonnet-4-5'] })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = 'orange cat'

    await state.optimizePrompt('gpt-5.1')

    expect(api.listPromptModels).toHaveBeenCalledWith(7)
    expect(state.promptOptimizerModels.value).toEqual(['gpt-5.1', 'claude-sonnet-4-5'])
    expect(api.optimizePrompt).toHaveBeenCalledWith(
      { prompt: 'orange cat', api_key_id: 7, model: 'gpt-5.1' },
      expect.any(AbortSignal),
    )
    expect(state.prompt.value).toBe('Optimized prompt')
  })

  it('cancels an in-flight prompt optimization without showing an error', async () => {
    const api = createApi()
    api.optimizePrompt.mockImplementation((_request, signal) => new Promise((_resolve, reject) => {
      signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = 'orange cat'

    const pending = state.optimizePrompt('gpt-5.1')
    expect(state.optimizingPrompt.value).toBe(true)
    state.cancelPromptOptimization()
    await pending

    expect(state.optimizingPrompt.value).toBe(false)
    expect(state.prompt.value).toBe('orange cat')
    expect(state.errorMessage.value).toBe('')
  })

  it('retries failed history jobs with their original stored parameters', async () => {
    const api = createApi({ job_id: 'job-1', status: 'succeeded', result: { images: [] } })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.outputCount.value = 3
    state.prompt.value = '生成一盏台灯'
    api.generate
      .mockResolvedValueOnce({ job_id: 'job-1', status: 'succeeded', result: { images: [] } })
      .mockResolvedValueOnce({ job_id: 'job-2', status: 'succeeded', result: { images: [] } })
      .mockResolvedValueOnce({ job_id: 'job-3', status: 'succeeded', result: { images: [] } })
    await state.submit()
    const failedId = state.activeConversation.value!.messages[1].id
    api.generate.mockClear()
    state.outputCount.value = 1
    state.model.value = 'gpt-image-1'
    state.size.value = '1536x1024'

    await state.retryMessage(failedId)

    expect(api.generate).not.toHaveBeenCalled()
    expect(api.retryHistory).toHaveBeenCalledTimes(3)
    expect(api.retryHistory.mock.calls.map(([historyID]) => historyID)).toEqual(['job-1', 'job-2', 'job-3'])
    const retryGroups = api.retryHistory.mock.calls.map(([, request]) => request.generation_group_id)
    expect(new Set(retryGroups).size).toBe(1)
    expect(retryGroups[0]).toMatch(/^generation-/)
  })

  it('keeps a failed exchange when retry is requested during another generation', async () => {
    const api = createApi({ job_id: 'failed-job', status: 'succeeded', result: { images: [] } })
    const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 1 })
    await state.initialize()
    state.prompt.value = 'First image'
    await state.submit()
    const failedMessages = state.activeConversation.value!.messages.slice(0, 2)
    const failedId = failedMessages[1].id

    const activeGeneration = deferred<GenerateResponse>()
    api.generate.mockImplementationOnce(() => activeGeneration.promise)
    state.prompt.value = 'Second image'
    const pendingSubmit = state.submit()

    await state.retryMessage(failedId)

    expect(state.activeConversation.value!.messages.slice(0, 2).map(message => message.id)).toEqual(
      failedMessages.map(message => message.id),
    )
    expect(api.retryHistory).not.toHaveBeenCalled()

    activeGeneration.resolve(completedResponse())
    await pendingSubmit
  })

  it('advances slot progress while a history retry is submitting', async () => {
    vi.useFakeTimers()
    try {
      const api = createApi({ job_id: 'failed-job', status: 'succeeded', result: { images: [] } })
      const state = useImageGeneration({ api, loadKeys: async () => [key], pollInterval: 60_000 })
      await state.initialize()
      state.prompt.value = 'Retry this image'
      await state.submit()
      const failedId = state.activeConversation.value!.messages[1].id
      const retry = deferred<GenerateResponse>()
      api.retryHistory.mockImplementationOnce(() => retry.promise)

      const pendingRetry = state.retryMessage(failedId)
      await Promise.resolve()
      expect(state.activeConversation.value!.messages[1].generationSlots?.[0].progress).toBe(1)

      await vi.advanceTimersByTimeAsync(700)

      expect(state.activeConversation.value!.messages[1].generationSlots?.[0].progress).toBeGreaterThan(1)
      retry.resolve(completedResponse())
      await pendingRetry
      state.dispose()
    } finally {
      vi.useRealTimers()
    }
  })

  it('allocates every output slot when retrying multiple multi-image history tasks', async () => {
    const api = createApi()
    api.listConversations.mockResolvedValue({ items: [{ id: 'conversation-1', title: 'Set', preview: 'Failed', status: 'failed', updated_at: '2026-07-16T10:00:00Z' }] })
    api.listConversationMessages.mockResolvedValue({ items: [1, 2].map(index => ({
      id: `history-${index}`, conversation_id: 'conversation-1', user_id: 1, prompt: 'Create four images', status: 'failed' as const,
      request: { model: 'gemini-2.5-flash-image', output_count: 2, generation_group_id: 'group-multi' },
      error_message: `task ${index} failed`, created_at: `2026-07-16T09:59:0${index}Z`, updated_at: '2026-07-16T10:00:00Z',
    })) })
    api.retryHistory
      .mockResolvedValueOnce({ job_id: 'retry-1', status: 'succeeded', result: { images: [{ url: '/one.png' }, { url: '/two.png' }] } })
      .mockResolvedValueOnce({ job_id: 'retry-2', status: 'succeeded', result: { images: [{ url: '/three.png' }, { url: '/four.png' }] } })
    const state = useImageGeneration({ api, loadKeys: async () => [key] })
    await state.initialize()

    await state.retryMessage(state.activeConversation.value!.messages[1].id)

    const assistant = state.activeConversation.value!.messages[1]
    expect(assistant.generationSlots).toHaveLength(4)
    expect(assistant.generationSlots?.every(slot => slot.status === 'succeeded')).toBe(true)
    expect(assistant.images?.map(image => image.originalSrc)).toEqual(['/one.png', '/two.png', '/three.png', '/four.png'])
  })
})
