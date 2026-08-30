import { describe, expect, it } from 'vitest'
import { inferModelProvider } from '../modelProvider'

describe('inferModelProvider', () => {
  it.each([
    ['claude-sonnet-5', 'openai', 'anthropic'],
    ['gpt-5.6-sol', 'openai', 'openai'],
    ['gemini-3.7-flash', 'openai', 'gemini'],
    ['grok-4.6', 'openai', 'grok'],
    ['deepseek-v4-pro-0813', 'openai', 'deepseek'],
    ['kimi-k2.7-code', 'openai', 'kimi'],
    ['glm-5.3', 'openai', 'zhipu'],
    ['MiniMax-M3', 'openai', 'minimax'],
    ['qwen3.8-max', 'openai', 'qwen'],
    ['mimo-v2.5-pro', 'openai', 'mimo'],
    ['hy3', 'openai', 'hunyuan'],
    ['Auto-Model', 'openai', 'auto']
  ])('maps %s to its real vendor', (model, platform, expected) => {
    expect(inferModelProvider(model, platform)).toBe(expected)
  })

  it('keeps the backend platform for unknown model names', () => {
    expect(inferModelProvider('custom-company-model', 'gemini')).toBe('gemini')
  })
})
