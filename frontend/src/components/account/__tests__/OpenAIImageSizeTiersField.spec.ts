import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpenAIImageSizeTiersField from '../OpenAIImageSizeTiersField.vue'
import {
  applyOpenAIImageSizeTiersToExtra,
  openAIImageSizeTiersPayload,
  readOpenAIImageSizeTiers
} from '../openaiImageSizeTiers'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('openaiImageSizeTiers helpers', () => {
  it('reads unrestricted extra as all tiers', () => {
    expect(readOpenAIImageSizeTiers(undefined)).toEqual(['1K', '2K', '4K'])
    expect(readOpenAIImageSizeTiers({ openai_image_size_tiers: [] })).toEqual(['1K', '2K', '4K'])
    expect(readOpenAIImageSizeTiers({ openai_image_size_tiers: ['8K'] })).toEqual(['1K', '2K', '4K'])
  })

  it('reads restricted tiers in canonical order', () => {
    expect(readOpenAIImageSizeTiers({ openai_image_size_tiers: ['4k', ' 1K'] })).toEqual(['1K', '4K'])
  })

  it('omits the key when every tier is allowed', () => {
    expect(openAIImageSizeTiersPayload(['4K', '1K', '2K'])).toBeNull()
    expect(openAIImageSizeTiersPayload(['2K', '1K'])).toEqual(['1K', '2K'])

    const extra: Record<string, unknown> = { openai_image_size_tiers: ['4K'] }
    applyOpenAIImageSizeTiersToExtra(extra, ['1K', '2K', '4K'])
    expect(extra).not.toHaveProperty('openai_image_size_tiers')
    applyOpenAIImageSizeTiersToExtra(extra, ['4K'])
    expect(extra.openai_image_size_tiers).toEqual(['4K'])
  })
})

describe('OpenAIImageSizeTiersField', () => {
  it('toggles tiers and keeps at least one selected', async () => {
    const wrapper = mount(OpenAIImageSizeTiersField, {
      props: {
        modelValue: ['1K', '2K', '4K'],
        'onUpdate:modelValue': (value: string[]) => wrapper.setProps({ modelValue: value as never })
      }
    })

    await wrapper.get('[data-testid="openai-image-size-tier-4K"]').setValue(false)
    expect(wrapper.props('modelValue')).toEqual(['1K', '2K'])
    await wrapper.get('[data-testid="openai-image-size-tier-1K"]').setValue(false)
    expect(wrapper.props('modelValue')).toEqual(['2K'])

    const last = wrapper.get('[data-testid="openai-image-size-tier-2K"]')
    await last.setValue(false)
    expect((last.element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.props('modelValue')).toEqual(['2K'])

    await wrapper.get('[data-testid="openai-image-size-tier-4K"]').setValue(true)
    expect(wrapper.props('modelValue')).toEqual(['2K', '4K'])
  })
})
