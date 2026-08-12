import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import VideoPlaygroundSchemaField from '@/components/video/VideoPlaygroundSchemaField.vue'
import {
  extractFieldSpecs,
  fieldSpecToDefaultValue,
} from '@/components/video/paramSpec'

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
  missingWarn: false,
  fallbackWarn: false,
})

function mountField(params: Record<string, unknown>) {
  const spec = extractFieldSpecs(params)[0]
  return mount(VideoPlaygroundSchemaField, {
    props: {
      spec,
      modelValue: fieldSpecToDefaultValue(spec),
    },
    global: {
      plugins: [i18n, createPinia()],
      stubs: {
        MaterialPickerModal: true,
      },
    },
  })
}

describe('VideoPlaygroundSchemaField media arrays', () => {
  it('renders only the select control for an enum input field', () => {
    const wrapper = mountField({
      mode: { value: 'fast', widget: 'input', enum: true, options: ['fast', 'quality'] },
    })

    expect(wrapper.findComponent({ name: 'Select' }).exists()).toBe(true)
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.find('textarea').exists()).toBe(false)
  })

  it('renders ImageUrls defaults as a compact gallery with max-items guidance', () => {
    const urls = [
      'https://cdn.example.com/reference-a.png',
      'https://cdn.example.com/reference-b.png',
    ]
    const wrapper = mountField({
      reference_images: {
        items: { value: '', widget: 'image' },
        widget: 'ImageUrls',
        maxItems: 4,
        value: urls,
      },
    })

    expect(wrapper.findAll('.img-cell img').map((img) => img.attributes('src'))).toEqual(urls)
    expect(wrapper.find('.array-max-items-hint').exists()).toBe(true)
    expect(wrapper.props('spec').maxItems).toBe(4)
    expect(wrapper.find('.img-cell').classes()).toEqual(expect.arrayContaining(['h-28', 'w-28']))

    const galleryPosition = wrapper.html().indexOf('class="img-cell')
    const actionsPosition = wrapper.html().indexOf('data-testid="media-group-actions"')
    expect(actionsPosition).toBeGreaterThan(galleryPosition)
    expect(wrapper.find('.img-cell a').exists()).toBe(false)
    expect(wrapper.find('.img-cell .image-remove-button').exists()).toBe(true)
    expect(wrapper.find('.add-image-tile').classes()).toEqual(
      expect.arrayContaining(['h-28', 'w-28']),
    )
    expect(wrapper.find('.image-urls-field > div:first-child button').text()).toContain(
      'materials.clearAll',
    )
    expect(wrapper.find('[data-testid="media-group-actions"] .text-red-500').exists()).toBe(false)
  })

  it('renders arrays whose item widget is image as an image gallery', () => {
    const urls = [
      'https://cdn.example.com/start.png',
      'https://cdn.example.com/end.png',
    ]
    const wrapper = mountField({
      reference_images: {
        items: { value: '', widget: 'image' },
        value: urls,
      },
    })

    expect(wrapper.findAll('.img-cell img').map((img) => img.attributes('src'))).toEqual(urls)
  })

  it('opens a large preview when an image thumbnail is clicked', async () => {
    const url = 'https://cdn.example.com/preview.png'
    const wrapper = mountField({
      reference_images: {
        items: { value: '', widget: 'image' },
        widget: 'ImageUrls',
        value: [url],
      },
    })

    await wrapper.find('.img-cell .img-drag').trigger('click')

    const preview = document.body.querySelector('[data-testid="image-preview"]')
    expect(preview).not.toBeNull()
    expect(preview?.querySelector('img')?.getAttribute('src')).toBe(url)
    wrapper.unmount()
  })

  it('renders an ordinary textarea for a prompt field unless the schema selects PromptTextArea', () => {
    const spec = extractFieldSpecs({ prompt: { value: '', widget: 'textarea' } })[0]
    const wrapper = mount(VideoPlaygroundSchemaField, {
      props: {
        spec,
        modelValue: 'Use @IMAGE1',
        mediaReferences: [{
          label: '@IMAGE1',
          kind: 'image',
          url: 'https://cdn.example.com/reference.png',
          fieldKey: 'image_url',
          itemIndex: 0,
        }],
      },
      global: {
        plugins: [i18n, createPinia()],
      },
    })

    expect(wrapper.findComponent({ name: 'PromptMediaReferenceInput' }).exists()).toBe(false)
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.find('.prompt-reference-hint').exists()).toBe(false)
  })

  it('renders PromptTextArea for any string key and filters configured references', () => {
    const spec = extractFieldSpecs({
      instruction: {
        value: '', widget: 'PromptTextArea', rows: 5,
        reference_fields: ['reference_images'],
      },
    })[0]
    const wrapper = mount(VideoPlaygroundSchemaField, {
      props: {
        spec,
        modelValue: 'Animate @IMAGE1',
        mediaReferences: [
          { label: '@IMAGE1', kind: 'image', url: 'https://cdn.example.com/a.png', fieldKey: 'reference_images', itemIndex: 0 },
          { label: '@VIDEO1', kind: 'video', url: 'https://cdn.example.com/a.mp4', fieldKey: 'video_urls', itemIndex: 0 },
        ],
      },
      global: { plugins: [i18n, createPinia()] },
    })

    const editor = wrapper.getComponent({ name: 'PromptMediaReferenceInput' })
    expect(editor.props('references')).toHaveLength(1)
    expect(editor.props('references')[0].fieldKey).toBe('reference_images')
    expect(wrapper.find('.prompt-reference-hint').exists()).toBe(true)
  })
})
