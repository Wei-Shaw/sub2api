import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import HistorySidebar from './HistorySidebar.vue'
import PromptComposer from './PromptComposer.vue'
import GeneratedImageCard from './GeneratedImageCard.vue'
import ChatThread from './ChatThread.vue'
import ImagePreviewDialog from './ImagePreviewDialog.vue'
import SidebarToggleButton from './SidebarToggleButton.vue'
import type { Conversation, ImageApiKey } from '../types'

const conversation: Conversation = {
  id: 'conversation-1',
  title: 'Lamp',
  preview: 'A blue lamp',
  lastUsedAt: 'now',
  messages: [],
  referenceImages: [],
  historyIds: ['history-1'],
}

const key: ImageApiKey = { id: 1, key: 'sk', name: 'Key', status: 'active', group: { allow_image_generation: true } }

describe('image generation components', () => {
  it('opens an original image dialog with an original download link', async () => {
    const wrapper = mount(ImagePreviewDialog, {
      props: { open: true, src: '/plugins/image-generation/api/assets/h1/result/0?token=test', alt: 'result' },
    })

    expect(wrapper.get('[role="dialog"]').attributes('aria-label')).toBe('查看原图')
    expect(wrapper.get('a[download]').attributes('href')).toContain('download=1')
    await wrapper.get('button[aria-label="关闭原图"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('moves focus into the image dialog and restores it after closing', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()
    const wrapper = mount(ImagePreviewDialog, {
      attachTo: document.body,
      props: { open: false, src: '/image.png', alt: 'result' },
    })

    await wrapper.setProps({ open: true })
    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('[data-testid="image-preview-close"]').element)
    await wrapper.setProps({ open: false })
    await nextTick()
    expect(document.activeElement).toBe(trigger)

    wrapper.unmount()
    trigger.remove()
  })

  it('emits history selection and deletion from native buttons', async () => {
    const wrapper = mount(HistorySidebar, {
      props: { conversations: [conversation], activeId: '', keys: [key], selectedKeyId: 1 },
    })

    await wrapper.get('[data-testid="history-select"]').trigger('click')
    await wrapper.get('[data-testid="history-delete-button"]').trigger('click')

    expect(wrapper.emitted('select')?.[0]).toEqual([conversation.id])
    expect(wrapper.emitted('delete')?.[0]).toEqual([conversation])
  })

  it('submits on Enter and keeps Ctrl+Enter for a newline', async () => {
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: 'lamp',
        model: 'gpt-image-2',
        size: '1024x1024',
        models: ['gpt-image-2'],
        busy: false,
      },
    })
    const textarea = wrapper.get('textarea')

    await textarea.trigger('keydown', { key: 'Enter' })
    await textarea.trigger('keydown', { key: 'Enter', ctrlKey: true })

    expect(wrapper.emitted('submit')).toHaveLength(1)
  })

  it('provides accessible names for icon-only composer controls', () => {
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: '',
        model: 'gpt-image-2',
        size: '1024x1024',
        models: ['gpt-image-2'],
        busy: false,
      },
    })

    expect(wrapper.get('[data-testid="image-send-button"]').attributes('aria-label')).toBeTruthy()
  })

  it('shows a spinner and emits cancellation while optimizing a prompt', async () => {
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: 'orange cat',
        model: 'gpt-image-2',
        size: '1024x1024',
        models: ['gpt-image-2'],
        promptOptimizerModel: 'gpt-5.1',
        promptOptimizerModels: ['gpt-5.1'],
        optimizingPrompt: true,
        busy: false,
      },
    })
    const button = wrapper.get('[data-testid="prompt-optimize-button"]')

    expect(button.attributes('aria-label')).toBe('停止优化提示词')
    expect(button.find('.magic-spinner').exists()).toBe(true)
    await button.trigger('click')

    expect(wrapper.emitted('cancelPromptOptimization')).toHaveLength(1)
  })

  it('opens image presets and emits selected styles and angles', async () => {
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: 'character',
        model: 'gpt-image-2',
        size: '1024x1024',
        models: ['gpt-image-2'],
        maxOutputImages: 4,
        busy: false,
      },
    })

    await wrapper.get('[data-testid="image-preset-button"]').trigger('click')
    expect(wrapper.get('[aria-label="关闭图片预设"]').attributes('aria-label')).toBe('关闭图片预设')
    const checkboxes = wrapper.findAll('.preset-option input')
    await checkboxes[0].setValue(true)
    await checkboxes[15].setValue(true)

    expect(wrapper.find('.preset-dialog').exists()).toBe(true)
    expect(wrapper.emitted('update:presetSelection')?.[0]?.[0]).toEqual({
      styles: ['cinematic'], scenes: [], effects: [], angles: [], separateAngleImages: false, keepAngleConsistency: false,
    })
    expect(wrapper.emitted('update:presetSelection')?.[1]?.[0]).toEqual({
      styles: [], scenes: [], effects: [], angles: ['front'], separateAngleImages: false, keepAngleConsistency: false,
    })
  })

  it('offers separate images only when multiple angles are selected', async () => {
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: 'character', model: 'gpt-image-2', size: '1024x1024', models: ['gpt-image-2'], maxOutputImages: 4, busy: false,
        presetSelection: { styles: [], scenes: [], effects: [], angles: ['front', 'back'], separateAngleImages: false, keepAngleConsistency: false } as never,
      },
    })

    await wrapper.get('[data-testid="image-preset-button"]').trigger('click')
    const separate = wrapper.get('[data-testid="separate-angle-images"]')
    expect((separate.element as HTMLInputElement).checked).toBe(false)
    await separate.setValue(true)
    expect(wrapper.emitted('update:presetSelection')?.[0]?.[0]).toEqual({
      styles: [], scenes: [], effects: [], angles: ['front', 'back'], separateAngleImages: true, keepAngleConsistency: false,
    })
    const consistency = wrapper.get('[data-testid="keep-angle-consistency"]')
    expect((consistency.element as HTMLInputElement).checked).toBe(false)
    await consistency.setValue(true)
    expect(wrapper.emitted('update:presetSelection')?.[1]?.[0]).toEqual({
      styles: [], scenes: [], effects: [], angles: ['front', 'back'], separateAngleImages: false, keepAngleConsistency: true,
    })

    const singleAngle = mount(PromptComposer, {
      props: {
        prompt: 'character', model: 'gpt-image-2', size: '1024x1024', models: ['gpt-image-2'], maxOutputImages: 4, busy: false,
        presetSelection: { styles: [], scenes: [], effects: [], angles: ['front'], separateAngleImages: false, keepAngleConsistency: false } as never,
      },
    })
    await singleAngle.get('[data-testid="image-preset-button"]').trigger('click')
    expect(singleAngle.find('[data-testid="separate-angle-images"]').exists()).toBe(false)
  })

  it('keeps native generation selects in normal document flow', async () => {
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: '',
        model: 'gpt-image-2',
        size: '1536x1024',
        sizeOptions: ['1024x1024', '1536x1024', '1024x1536'],
        outputCount: 3,
        maxOutputImages: 4,
        models: ['gpt-image-2', 'image-model-with-a-long-name'],
        busy: false,
      },
    })

    expect(wrapper.findAll('.composer-select > select')).toHaveLength(3)

    await wrapper.get('[data-testid="image-model-select"]').setValue('image-model-with-a-long-name')
    await wrapper.get('[data-testid="image-size-select"]').setValue('1024x1536')
    await wrapper.get('[data-testid="image-output-count"]').setValue('4')

    expect(wrapper.emitted('update:model')?.[0]).toEqual(['image-model-with-a-long-name'])
    expect(wrapper.emitted('update:size')?.[0]).toEqual(['1024x1536'])
    expect(wrapper.emitted('update:outputCount')?.[0]).toEqual([4])
  })

  it('renders only the parameter controls advertised by the selected model', async () => {
    const ratios = ['1:1', '2:3', '3:2', '3:4', '4:3', '4:5', '5:4', '9:16', '16:9', '21:9']
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: '', model: 'gemini-2.5-flash-image', size: '', models: ['gemini-2.5-flash-image'], busy: false,
        sizeOptions: [], aspectRatio: '1:1', aspectRatioOptions: ratios, resolution: '1K', resolutionOptions: ['1K', '2K', '4K'],
      },
    })

    expect(wrapper.findAll('[data-testid="image-aspect-ratio-select"] option')).toHaveLength(10)
    expect(wrapper.find('[data-testid="image-size-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-quality-select"]').exists()).toBe(false)
    await wrapper.get('[data-testid="image-aspect-ratio-select"]').setValue('21:9')
    expect(wrapper.emitted('update:aspectRatio')?.[0]).toEqual(['21:9'])
  })

  it('shows GPT quality, format, compression, background, and edit fidelity controls', async () => {
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: '', model: 'gpt-image-2', size: '1024x1024', models: ['gpt-image-2'], busy: false,
        sizeOptions: ['1024x1024'], quality: 'high', qualityOptions: ['auto', 'low', 'medium', 'high'],
        outputFormat: 'webp', outputFormatOptions: ['png', 'jpeg', 'webp'], outputCompression: 82,
        compressionMin: 0, compressionMax: 100, supportsOutputCompression: true,
        background: 'transparent', backgroundOptions: ['auto', 'transparent', 'opaque'],
        inputFidelity: 'high', inputFidelityOptions: ['low', 'high'],
        references: [{ id: 'ref', dataUrl: 'data:image/png;base64,cG5n', fileName: 'ref.png', mimeType: 'image/png' }],
      },
    })

    expect(wrapper.get('[data-testid="image-quality-select"]').text()).toContain('自动画质')
    expect(wrapper.get('[data-testid="image-quality-select"]').text()).toContain('标准画质')
    expect(wrapper.get('[data-testid="image-output-format-select"]').element).toBeTruthy()
    expect(wrapper.get('[data-testid="image-output-compression"]').attributes('min')).toBe('0')
    expect(wrapper.get('[data-testid="image-background-select"]').text()).toContain('透明背景')
    expect(wrapper.get('[data-testid="image-input-fidelity-select"]').text()).toContain('高保真')
    await wrapper.get('[data-testid="image-quality-select"]').setValue('medium')
    await wrapper.get('[data-testid="image-output-compression"]').setValue(75)
    expect(wrapper.emitted('update:quality')?.[0]).toEqual(['medium'])
    expect(wrapper.emitted('update:outputCompression')?.[0]).toEqual([75])
  })

  it('hides unsupported advanced controls for an unconfigured model', () => {
    const wrapper = mount(PromptComposer, {
      props: { prompt: '', model: 'gpt-image-custom', size: '1024x1024', models: ['gpt-image-custom'], busy: false },
    })

    expect(wrapper.find('[data-testid="image-quality-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-aspect-ratio-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-output-compression"]').exists()).toBe(false)
  })

  it('renders the original Chinese generation actions', () => {
    const wrapper = mount(GeneratedImageCard, {
      props: { image: { id: 'image-1', src: 'data:image/png;base64,abc', revisedPrompt: '蓝色台灯', createdAt: 'now' } },
    })

    expect(wrapper.text()).toContain('设为参考图')
    expect(wrapper.text()).toContain('优化提示词')
    expect(wrapper.text()).toContain('再次生成')
  })

  it('loads a historical image only after it enters the scroll observer range', async () => {
    let callback: IntersectionObserverCallback | undefined
    const originalObserver = window.IntersectionObserver
    class TestIntersectionObserver {
      constructor(next: IntersectionObserverCallback) { callback = next }
      observe() {}
      disconnect() {}
      unobserve() {}
      takeRecords() { return [] }
      root = null
      rootMargin = ''
      thresholds = []
    }
    window.IntersectionObserver = TestIntersectionObserver as unknown as typeof IntersectionObserver

    const scrollRoot = document.createElement('div')
    const wrapper = mount(GeneratedImageCard, {
      props: { image: { id: 'image-1', src: '/preview.jpg', revisedPrompt: '蓝色台灯', createdAt: 'now', lazy: true }, scrollRoot },
    })
    const image = wrapper.get('img')
    expect(image.attributes('src')).toBeUndefined()

		await nextTick()
		callback?.([{ isIntersecting: true, target: image.element } as unknown as IntersectionObserverEntry], {} as IntersectionObserver)
    await nextTick()
    expect(image.attributes('src')).toBe('/preview.jpg')

    wrapper.unmount()
    window.IntersectionObserver = originalObserver
  })

  it('renders Chinese sidebar controls and emits collapse from the pinned footer', async () => {
    const wrapper = mount(HistorySidebar, {
      props: { conversations: [conversation], activeId: conversation.id, keys: [key], selectedKeyId: 1 },
    })

    expect(wrapper.get('[data-testid="history-inline-collapse"]').attributes('aria-label')).toBe('收起侧边栏')
    expect(wrapper.get('[data-testid="history-inline-collapse"]').classes()).toContain('sidebar-toggle-button')
    expect(wrapper.get('[data-testid="history-inline-collapse"]').text()).toBe('收起侧边栏')
    expect(wrapper.get('[data-testid="sidebar-footer"]').get('[data-testid="history-inline-collapse"]').attributes('aria-label')).toBe('收起侧边栏')
    await wrapper.get('[data-testid="history-inline-collapse"]').trigger('click')
    expect(wrapper.emitted('collapse')).toHaveLength(1)
    expect(wrapper.get('[data-testid="history-delete-button"]').text()).toBe('删除')
    expect(wrapper.text()).toContain('新建会话')
  })

  it('reuses the main sidebar control for expansion', async () => {
    const wrapper = mount(SidebarToggleButton, { props: { direction: 'expand' } })

    expect(wrapper.get('button').attributes('aria-label')).toBe('展开侧边栏')
    expect(wrapper.get('button').text()).toBe('展开侧边栏')
    expect(wrapper.get('[data-testid="sidebar-toggle-icon"]').classes()).toContain('sidebar-toggle-icon-expand')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('click')).toHaveLength(1)
  })

  it('supports multiple references, exposes overflow validation, and removes one image', async () => {
    const references = [
      { id: 'first', dataUrl: 'data:image/png;base64,first', fileName: 'first.png', mimeType: 'image/png' },
      { id: 'second', dataUrl: 'data:image/png;base64,second', fileName: 'second.png', mimeType: 'image/png' },
    ]
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: 'Use both', model: 'custom-image-model', size: '1024x1024', models: ['custom-image-model'], busy: false,
        references, maxReferenceImages: 1, referenceLimitExceeded: true,
      },
    })

    expect(wrapper.findAll('[data-testid="reference-image-preview"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="reference-limit-error"]').text()).toContain('1')
    expect(wrapper.get('[data-testid="image-send-button"]').attributes()).toHaveProperty('disabled')
    expect(wrapper.get('[data-testid="image-prompt-input"]').attributes('aria-describedby')).toBe('reference-limit-error')

    await wrapper.get('[data-testid="reference-count-toggle"]').trigger('click')
    await wrapper.findAll('[data-testid="remove-reference-image"]')[1].trigger('click')
    expect(wrapper.emitted('removeReference')?.[0]).toEqual(['second'])

    await wrapper.setProps({ maxReferenceImages: 3, referenceLimitExceeded: false })
    const input = wrapper.get<HTMLInputElement>('[data-testid="reference-image-input"]')
    expect(input.attributes()).toHaveProperty('multiple')
    const files = [
      new File(['third'], 'third.png', { type: 'image/png' }),
      new File(['fourth'], 'fourth.png', { type: 'image/png' }),
    ]
    Object.defineProperty(input.element, 'files', { configurable: true, value: files })
    await input.trigger('change')
    expect(wrapper.emitted('referenceFiles')?.[0]).toEqual([files])
  })

  it('keeps the centered add input active and expands two images only from the count badge', async () => {
    const references = [
      { id: 'first', dataUrl: 'data:image/png;base64,first', fileName: 'first.png', mimeType: 'image/png' },
      { id: 'second', dataUrl: 'data:image/png;base64,second', fileName: 'second.png', mimeType: 'image/png' },
    ]
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: '', model: 'gpt-image-2', size: '1024x1024', models: ['gpt-image-2'], busy: false,
        references, maxReferenceImages: 16, referenceLimitExceeded: false,
      },
    })

    const countToggle = wrapper.get('[data-testid="reference-count-toggle"]')
    const input = wrapper.get<HTMLInputElement>('[data-testid="reference-image-input"]')
    expect(countToggle.text()).toBe('2/16')
    expect(wrapper.find('.reference-count').exists()).toBe(false)
    expect(countToggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.findAll('[data-testid="reference-fan-item"]')).toHaveLength(2)
    expect(input.attributes()).toHaveProperty('multiple')

    const addedFile = new File(['third'], 'third.png', { type: 'image/png' })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [addedFile] })
    await input.trigger('change')
    expect(wrapper.emitted('referenceFiles')?.[0]).toEqual([[addedFile]])
    expect(countToggle.attributes('aria-expanded')).toBe('false')

    await countToggle.trigger('click')
    expect(countToggle.attributes('aria-expanded')).toBe('true')

    const fourthFile = new File(['fourth'], 'fourth.png', { type: 'image/png' })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [fourthFile] })
    await input.trigger('change')
    expect(wrapper.emitted('referenceFiles')?.[1]).toEqual([[fourthFile]])
    expect(countToggle.attributes('aria-expanded')).toBe('true')

    await wrapper.get('[data-testid="image-chat-composer"]').trigger('keydown', { key: 'Escape' })
    expect(countToggle.attributes('aria-expanded')).toBe('false')

    await countToggle.trigger('click')
    await wrapper.setProps({ references: references.slice(0, 1) })
    expect(wrapper.find('[data-testid="reference-count-toggle"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="reference-image-input"]').attributes()).toHaveProperty('multiple')
  })

  it('stays expanded for consecutive removals and offers one clear-all action', async () => {
    const references = ['first', 'second', 'third'].map(id => ({
      id, dataUrl: `data:image/png;base64,${id}`, fileName: `${id}.png`, mimeType: 'image/png',
    }))
    const wrapper = mount(PromptComposer, {
      props: {
        prompt: '', model: 'gpt-image-2', size: '1024x1024', models: ['gpt-image-2'], busy: false,
        references, maxReferenceImages: 16, referenceLimitExceeded: false,
      },
    })

    await wrapper.get('[data-testid="reference-count-toggle"]').trigger('click')
    await wrapper.findAll('[data-testid="remove-reference-image"]')[2].trigger('click')
    await wrapper.setProps({ references: references.slice(0, 2) })
    expect(wrapper.get('[data-testid="reference-count-toggle"]').attributes('aria-expanded')).toBe('true')

    await wrapper.get('[data-testid="clear-reference-images"]').trigger('click')
    expect(wrapper.emitted('clearReferences')).toHaveLength(1)
  })

  it('offers model-limited output quantities and emits a numeric selection', async () => {
    const wrapper = mount(PromptComposer, { props: {
      prompt: '', model: 'gpt-image-2', size: '1024x1024', models: ['gpt-image-2'], busy: false,
      outputCount: 1, maxOutputImages: 4,
    } })
    const select = wrapper.get('[data-testid="image-output-count"]')
    expect(select.findAll('option').map(option => option.text())).toEqual(['1 张', '2 张', '3 张', '4 张'])
    await select.setValue('3')
    expect(wrapper.emitted('update:outputCount')?.[0]).toEqual([3])
  })

  it('renders sent user messages with reference, description, and generation parameters', async () => {
    const wrapper = mount(ChatThread, {
      props: {
        conversation: {
          ...conversation,
          messages: [{
            id: 'user-1',
            role: 'user',
            content: '生成一只小狗',
            createdAt: 'now',
            referenceImages: [{ id: 'ref-1', dataUrl: 'data:image/png;base64,abc', fileName: 'dog.png', mimeType: 'image/png' }],
            requestSettings: [{ modelLabel: 'gpt-image-2', sizeLabel: '1024x1024', countLabel: '1', detailsLabel: '画质: high | 格式: webp' }],
          }],
        },
      },
    })

		await nextTick()
    expect(wrapper.get('[data-testid="user-message-reference-image"]').attributes('src')).toContain('data:image/png')
    expect(wrapper.text()).toContain('创作描述')
    expect(wrapper.text()).toContain('生成一只小狗')
    expect(wrapper.text()).toContain('生成参数')
    expect(wrapper.text()).toContain('画质: high | 格式: webp')
    expect(wrapper.text()).toContain('Prompt')
    expect(wrapper.text()).toContain('GPT Image 2 | 1024 × 1024 | 数量: 1')
  })

  it('defers historical reference image loading until it enters the chat viewport', async () => {
    let callback: IntersectionObserverCallback | undefined
    const originalObserver = window.IntersectionObserver
    class TestIntersectionObserver {
      constructor(next: IntersectionObserverCallback) { callback = next }
      observe() {}
      disconnect() {}
      unobserve() {}
      takeRecords() { return [] }
      root = null
      rootMargin = ''
      thresholds = []
    }
    window.IntersectionObserver = TestIntersectionObserver as unknown as typeof IntersectionObserver
    const wrapper = mount(ChatThread, {
      props: { conversation: { ...conversation, messages: [{ id: 'user-1', role: 'user', content: 'dog', createdAt: 'now', referenceImages: [{ id: 'ref-1', dataUrl: '/preview.png', originalDataUrl: '/original.png', fileName: 'dog.png', mimeType: 'image/png' }] }] } },
    })
    const image = wrapper.get('[data-testid="user-message-reference-image"]')
    expect(image.attributes('src')).toBeUndefined()

    await nextTick()
    callback?.([{ isIntersecting: true, target: image.element } as unknown as IntersectionObserverEntry], {} as IntersectionObserver)
    await nextTick()
    expect(image.attributes('src')).toBe('/preview.png')

    wrapper.unmount()
    window.IntersectionObserver = originalObserver
  })

  it('renders fixed generation slots with loading progress and failures', () => {
    const wrapper = mount(ChatThread, {
      props: {
        conversation: {
          ...conversation,
          messages: [{
            id: 'assistant-1', role: 'assistant', content: '正在生成', createdAt: 'now', status: 'pending',
            generationSlots: [
              { id: 'front', label: '正面', status: 'pending', progress: 1 },
              { id: 'back', label: '背面', status: 'failed', progress: 32, error: '上游服务拒绝请求' },
            ],
          }] as never,
        },
      },
    })

    expect(wrapper.findAll('[data-testid="generation-slot"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('1%')
    expect(wrapper.text()).toContain('正面')
    expect(wrapper.text()).toContain('背面')
    expect(wrapper.text()).toContain('生成失败')
  })
  it('emits a historical reference image from the action beside its thumbnail', async () => {
    const reference = { id: 'ref-1', dataUrl: '/preview.png', originalDataUrl: '/original.png', fileName: 'dog.png', mimeType: 'image/png' }
    const wrapper = mount(ChatThread, {
      props: {
        conversation: {
          ...conversation,
          messages: [{ id: 'user-1', role: 'user', content: 'dog', createdAt: 'now', referenceImages: [reference] }],
        },
      },
    })

    await wrapper.get('[data-testid="history-reference-action"]').trigger('click')
    await wrapper.get('[data-testid="history-refine-action"]').trigger('click')

    expect(wrapper.emitted('referenceImage')?.[0]).toEqual([reference])
    expect(wrapper.emitted('refinePrompt')?.[0]).toEqual(['dog'])
    expect(wrapper.get('[data-testid="history-reference-item"]').text()).toContain('设为参考图')
    expect(wrapper.get('[data-testid="history-reference-item"]').text()).toContain('优化提示词')
  })

  it('loads older messages automatically near the top without rendering a button', async () => {
    const wrapper = mount(ChatThread, { props: { conversation, hasOlder: true, loading: false } })
    const thread = wrapper.get('[data-testid="image-chat-thread"]')

    expect(wrapper.find('.load-older').exists()).toBe(false)
    Object.defineProperty(thread.element, 'scrollTop', { value: 48, writable: true })
    Object.defineProperty(thread.element, 'scrollHeight', { value: 600, configurable: true })
    await thread.trigger('scroll')

    expect(wrapper.emitted('loadOlder')).toHaveLength(1)
  })

  it('does not load older messages outside the threshold or while unavailable', async () => {
    const wrapper = mount(ChatThread, { props: { conversation, hasOlder: true, loading: false } })
    const thread = wrapper.get('[data-testid="image-chat-thread"]')
    Object.defineProperty(thread.element, 'scrollTop', { value: 49, writable: true })
    Object.defineProperty(thread.element, 'scrollHeight', { value: 600, configurable: true })

    await thread.trigger('scroll')
    await wrapper.setProps({ loading: true })
    thread.element.scrollTop = 0
    await thread.trigger('scroll')
    await wrapper.setProps({ loading: false, hasOlder: false })
    await thread.trigger('scroll')

    expect(wrapper.emitted('loadOlder')).toBeUndefined()
  })

  it('preserves the visible message position after older messages are prepended', async () => {
    const wrapper = mount(ChatThread, { props: { conversation, hasOlder: true, loading: false } })
    const thread = wrapper.get('[data-testid="image-chat-thread"]')
    let scrollHeight = 600
    Object.defineProperty(thread.element, 'scrollTop', { value: 32, writable: true })
    Object.defineProperty(thread.element, 'scrollHeight', { get: () => scrollHeight })

    await thread.trigger('scroll')
    await wrapper.setProps({ loading: true })
    scrollHeight = 840
    await wrapper.setProps({ loading: false })
    await nextTick()

    expect(thread.element.scrollTop).toBe(272)
  })

  it('positions a newly selected conversation at its latest message after loading', async () => {
    const wrapper = mount(ChatThread, { props: { conversation, hasOlder: false, loading: false } })
    const thread = wrapper.get('[data-testid="image-chat-thread"]')
    Object.defineProperty(thread.element, 'scrollTop', { value: 24, writable: true })
    Object.defineProperty(thread.element, 'scrollHeight', { value: 900, configurable: true })
    const selectedConversation = { ...conversation, id: 'conversation-2' }

    await wrapper.setProps({ conversation: selectedConversation, loading: true })
    await wrapper.setProps({ loading: false })
    await nextTick()

    expect(thread.element.scrollTop).toBe(900)
  })

  it('does not force the current conversation to the bottom when its messages change', async () => {
    const wrapper = mount(ChatThread, { props: { conversation, hasOlder: false, loading: false } })
    const thread = wrapper.get('[data-testid="image-chat-thread"]')
    Object.defineProperty(thread.element, 'scrollTop', { value: 120, writable: true })
    Object.defineProperty(thread.element, 'scrollHeight', { value: 900, configurable: true })
    await nextTick()
    thread.element.scrollTop = 120

    await wrapper.setProps({
      conversation: {
        ...conversation,
        messages: [{ id: 'new-message', role: 'assistant', content: 'new result', createdAt: 'now' }],
      },
    })
    await nextTick()

    expect(thread.element.scrollTop).toBe(120)
  })
})
