import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('Gemini Vertex API key locale keys', () => {
  it('contains Chinese labels', () => {
    expect(zh.admin.accounts.gemini.apiModeLabel).toBeTruthy()
    expect(zh.admin.accounts.gemini.apiMode.aiStudio).toBeTruthy()
    expect(zh.admin.accounts.gemini.apiMode.vertex).toBeTruthy()
    expect(zh.admin.accounts.gemini.baseUrlHintVertex).toBeTruthy()
    expect(zh.admin.accounts.gemini.tier.vertexHint).toBeTruthy()
  })

  it('contains English labels', () => {
    expect(en.admin.accounts.gemini.apiModeLabel).toBeTruthy()
    expect(en.admin.accounts.gemini.apiMode.aiStudio).toBeTruthy()
    expect(en.admin.accounts.gemini.apiMode.vertex).toBeTruthy()
    expect(en.admin.accounts.gemini.baseUrlHintVertex).toBeTruthy()
    expect(en.admin.accounts.gemini.tier.vertexHint).toBeTruthy()
  })
})
