import { describe, expect, it } from 'vitest'
import { hasWebImageConfiguration, isFreeWebImageAccount, isOpenAIImageTestModel, readWebImageSettings } from '../webImageBridge'
import type { Account } from '@/types'

const free = { platform: 'openai', type: 'oauth', parent_account_id: null, credentials: { plan_type: ' Free ' } } as Account

describe('Free web image configuration', () => {
  it('only accepts known Free OAuth parent accounts', () => {
    expect(isFreeWebImageAccount(free)).toBe(true)
    for (const change of [{ type: 'apikey' }, { platform: 'anthropic' }, { parent_account_id: 1 }, { credentials: { plan_type: 'pro' } }, { credentials: {} }]) {
      expect(isFreeWebImageAccount({ ...free, ...change } as Account)).toBe(false)
    }
  })

  it('distinguishes explicit boolean configuration from a missing or malformed flag', () => {
    expect(hasWebImageConfiguration()).toBe(false)
    expect(hasWebImageConfiguration({})).toBe(false)
    expect(hasWebImageConfiguration({ chatgpt_web_image_enabled: 'true' })).toBe(false)
    expect(hasWebImageConfiguration({ chatgpt_web_image_enabled: true })).toBe(true)
    expect(hasWebImageConfiguration({ chatgpt_web_image_enabled: false })).toBe(true)
  })

  it('keeps absent settings disabled and ignores the retired default image selection', () => {
    expect(readWebImageSettings()).toEqual({ enabled: false, regularChat: false, markdownPreview: true })
    expect(readWebImageSettings({ chatgpt_web_image_enabled: false, chatgpt_web_image_regular_chat: true, chatgpt_web_image_markdown_preview: false, chatgpt_web_image_default_model: ' custom-image ' })).toEqual({ enabled: false, regularChat: true, markdownPreview: false })
  })

  it('treats every Free image name as a web entry regardless of model authorization or legacy mappings', () => {
    for (const mapping of [undefined, {}, { 'gpt-6-luna': 'gpt-6-luna' }, { 'gpt-image-*': 'gpt-6-luna' }]) {
      for (const image of ['gpt-image-1', 'gpt-image-1.5', 'gpt-image-2', 'gpt-image-2.5-flare', 'gpt-image-2.5-sunburst', 'gpt-image-future']) {
        expect(isOpenAIImageTestModel(mapping, image, true)).toBe(true)
      }
      expect(isOpenAIImageTestModel(mapping, 'gpt-6-luna', true)).toBe(false)
    }
    expect(isOpenAIImageTestModel({ illustration: 'gpt-image-2' }, 'illustration', true)).toBe(true)
  })

  it('uses the mapped target when choosing image versus text account tests', () => {
    expect(isOpenAIImageTestModel({}, 'gpt-image-2')).toBe(true)
    expect(isOpenAIImageTestModel({ picture: 'gpt-image-2' }, 'picture')).toBe(true)
    expect(isOpenAIImageTestModel({ 'gpt-image-custom': 'gpt-5.1' }, 'gpt-image-custom')).toBe(false)
    expect(isOpenAIImageTestModel({ 'gpt-image-*': 'gpt-5.1' }, 'gpt-image-custom')).toBe(false)
    expect(isOpenAIImageTestModel({ '*': 'gpt-image-2', 'gpt-image-custom': 'gpt-5.1' }, 'gpt-image-custom')).toBe(false)
  })

  it('resolves aliases using exact mappings before the longest trailing wildcard', () => {
    const mapping = { '*': 'gpt-image-1', 'photo-*': 'gpt-6-luna', 'photo-portrait': 'gpt-image-2' }
    expect(isOpenAIImageTestModel(mapping, 'photo-landscape', true)).toBe(false)
    expect(isOpenAIImageTestModel(mapping, 'photo-portrait', true)).toBe(true)
    expect(isOpenAIImageTestModel(mapping, 'illustration', true)).toBe(true)
  })
})
