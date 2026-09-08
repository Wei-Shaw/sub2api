import { describe, expect, it } from 'vitest'
import { messages } from './index'

describe('image generation locales', () => {
  it('keeps English and Chinese message keys aligned', () => {
    expect(Object.keys(messages.zh.imageGeneration).sort()).toEqual(Object.keys(messages.en.imageGeneration).sort())
  })

  it('includes task and failure messages in both locales', () => {
    for (const locale of ['en', 'zh'] as const) {
      expect(messages[locale].imageGeneration).toEqual(expect.objectContaining({
        repeatGeneration: expect.any(String),
        emptyGenerationResult: expect.any(String),
        cancelGeneration: expect.any(String),
        deleteHistory: expect.any(String),
      }))
    }
  })
})
