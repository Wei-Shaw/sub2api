import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import PromptMediaReferenceInput from '@/components/video/PromptMediaReferenceInput.vue'
import { collectPromptMediaReferences } from '@/components/video/promptMediaReferences'
import { extractFieldSpecs } from '@/components/video/paramSpec'

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: {
    en: {
      videoModels: {
        playground: {
          promptReferencesImages: () => 'Images',
          promptReferencesVideos: () => 'Videos',
          promptReferencesAudio: () => 'Audio',
        },
      },
    },
  },
})

function placeCaret(editor: HTMLElement, offset: number, rect?: Partial<DOMRect>): Range {
  const textNode = editor.firstChild
  if (!textNode || textNode.nodeType !== Node.TEXT_NODE) {
    throw new Error('Expected the editor to contain a text node')
  }
  const range = document.createRange()
  range.setStart(textNode, offset)
  range.collapse(true)
  Object.defineProperty(range, 'getBoundingClientRect', {
    value: () => ({
      x: 120,
      y: 40,
      top: 40,
      right: 120,
      bottom: 60,
      left: 120,
      width: 0,
      height: 20,
      toJSON: () => ({}),
      ...rect,
    }),
  })
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(range)
  return range
}

describe('prompt media references', () => {
  it('numbers single and array media in schema and item order', () => {
    const specs = extractFieldSpecs({
      cover: { value: '', widget: 'image', extra: { 'x-order': 1 } },
      references: {
        items: { value: '', widget: 'image' },
        widget: 'ImageUrls',
        extra: { 'x-order': 2 },
      },
      clips: {
        items: { value: '' },
        widget: 'VideoUrls',
        extra: { 'x-order': 3 },
      },
      sounds: {
        items: { value: '' },
        widget: 'AudioUrls',
        extra: { 'x-order': 4 },
      },
    })
    const references = collectPromptMediaReferences(specs, {
      cover: 'https://cdn.example.com/cover.png',
      references: [
        'https://cdn.example.com/a.png',
        'https://cdn.example.com/b.png',
      ],
      clips: ['https://cdn.example.com/a.mp4'],
      sounds: ['https://cdn.example.com/a.mp3'],
    })

    expect(references.map(({ label, kind, fieldKey, itemIndex }) => ({
      label,
      kind,
      fieldKey,
      itemIndex,
    }))).toEqual([
      { label: '@IMAGE1', kind: 'image', fieldKey: 'cover', itemIndex: 0 },
      { label: '@IMAGE2', kind: 'image', fieldKey: 'references', itemIndex: 0 },
      { label: '@IMAGE3', kind: 'image', fieldKey: 'references', itemIndex: 1 },
      { label: '@VIDEO1', kind: 'video', fieldKey: 'clips', itemIndex: 0 },
      { label: '@AUDIO1', kind: 'audio', fieldKey: 'sounds', itemIndex: 0 },
    ])
  })

  it('opens grouped candidates at the caret and inserts the selected media inline', async () => {
    const imageUrl = 'https://cdn.example.com/assets/reference%20image.png'
    const wrapper = mount(PromptMediaReferenceInput, {
      attachTo: document.body,
      props: {
        modelValue: 'Use ',
        references: [
          {
            label: '@IMAGE1',
            kind: 'image',
            url: imageUrl,
            fieldKey: 'reference_images',
            itemIndex: 0,
          },
          {
            label: '@VIDEO1',
            kind: 'video',
            url: 'https://cdn.example.com/clip.mp4',
            fieldKey: 'video_urls',
            itemIndex: 0,
          },
          {
            label: '@AUDIO1',
            kind: 'audio',
            url: 'https://cdn.example.com/sound.mp3',
            fieldKey: 'audio_urls',
            itemIndex: 0,
          },
        ],
        'onUpdate:modelValue': (value: string) => wrapper.setProps({ modelValue: value }),
      },
      global: { plugins: [i18n] },
    })

    const root = wrapper.get('.prompt-media-reference-input')
    const editor = wrapper.get<HTMLElement>('.prompt-media-editor')
    Object.defineProperty(root.element, 'getBoundingClientRect', {
      value: () => ({ left: 20, top: 10, right: 620, bottom: 160, width: 600, height: 150 }),
    })
    editor.element.textContent = 'Use @'
    placeCaret(editor.element, 5)
    await editor.trigger('input')

    const menu = wrapper.get<HTMLElement>('[data-testid="prompt-media-menu"]')
    expect(menu.element.style.left).toBe('104px')
    expect(menu.element.style.top).toBe('56px')
    expect(wrapper.get('[data-testid="prompt-media-group-image"]').text()).toBe('Images')
    expect(wrapper.get('[data-testid="prompt-media-group-video"]').text()).toBe('Videos')
    expect(wrapper.get('[data-testid="prompt-media-group-audio"]').text()).toBe('Audio')

    const imageCandidate = wrapper.findAll('button').find((button) => button.text().includes('@IMAGE1'))
    expect(imageCandidate).toBeDefined()
    await imageCandidate!.trigger('mousedown')

    expect(wrapper.props('modelValue')).toBe('Use @IMAGE1 ')
    const usedReference = editor.get('.media-reference-token')
    expect(usedReference.get('img').attributes('src')).toBe(imageUrl)
    expect(usedReference.text()).toBe('@IMAGE1')
    expect(usedReference.attributes('title')).toBeUndefined()
    expect(usedReference.element.firstElementChild?.tagName).toBe('IMG')
    expect(editor.element.childNodes[1]).toBe(usedReference.element)
    expect(wrapper.find('[data-testid="prompt-media-menu"]').exists()).toBe(false)

    Object.defineProperty(usedReference.element, 'getBoundingClientRect', {
      value: () => ({ left: 100, top: 320, right: 180, bottom: 356, width: 80, height: 36 }),
    })
    await usedReference.trigger('mouseenter')
    const imagePreview = wrapper.get('[data-testid="prompt-media-image-preview"]')
    expect(imagePreview.attributes('src')).toBe(imageUrl)
    expect(imagePreview.attributes('style')).toContain('left: 100px')
    expect(imagePreview.attributes('style')).toContain('top: 24px')
    await usedReference.trigger('mouseleave')
    expect(wrapper.find('[data-testid="prompt-media-image-preview"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('renders referenced video before its alias', () => {
    const url = 'https://cdn.example.com/reference.mp4'
    const wrapper = mount(PromptMediaReferenceInput, {
      props: {
        modelValue: 'Animate @VIDEO1',
        references: [{
          label: '@VIDEO1',
          kind: 'video',
          url,
          fieldKey: 'video_url',
          itemIndex: 0,
        }],
      },
      global: { plugins: [i18n] },
    })

    const editor = wrapper.get('.prompt-media-editor')
    const usedReference = editor.get('.media-reference-token')
    expect(usedReference.get('video').attributes('src')).toBe(url)
    expect(usedReference.text()).toBe('@VIDEO1')
    expect(usedReference.element.firstElementChild?.tagName).toBe('VIDEO')
    expect(wrapper.find('.prompt-media-reference-input > .media-reference-token').exists()).toBe(false)
  })
})
