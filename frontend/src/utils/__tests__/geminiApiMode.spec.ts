import { describe, expect, it } from 'vitest'

import {
  GEMINI_AI_STUDIO_BASE_URL,
  GEMINI_VERTEX_EXPRESS_BASE_URL,
  geminiAPIKeyBaseURL,
  normalizeGeminiAPIMode
} from '../geminiApiMode'

describe('Gemini API key mode', () => {
  it('keeps legacy and unknown values on AI Studio', () => {
    expect(normalizeGeminiAPIMode(undefined)).toBe('ai_studio')
    expect(normalizeGeminiAPIMode('unknown')).toBe('ai_studio')
    expect(geminiAPIKeyBaseURL('ai_studio')).toBe(GEMINI_AI_STUDIO_BASE_URL)
  })

  it('selects the Vertex Express endpoint explicitly', () => {
    expect(normalizeGeminiAPIMode('vertex')).toBe('vertex')
    expect(geminiAPIKeyBaseURL('vertex')).toBe(GEMINI_VERTEX_EXPRESS_BASE_URL)
    expect(GEMINI_VERTEX_EXPRESS_BASE_URL).toBe('https://aiplatform.googleapis.com')
  })
})
