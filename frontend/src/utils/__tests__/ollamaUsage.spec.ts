import { describe, expect, it } from 'vitest'
import { formatOllamaResetText } from '../ollamaUsage'

describe('Ollama reset hints', () => {
  const zh = (_: string, params: Record<string, string>) => `约${params.time}`
  const en = (_: string, params: Record<string, string>) => `${params.time} (approx.)`
  it('localizes an upstream week hint without converting it to an absolute date', () => {
    expect(formatOllamaResetText('Refills in 1 week', 'zh', zh)).toBe('约1周后')
    expect(formatOllamaResetText('Refills in 2 days.', 'zh', zh)).toBe('约2天后')
    expect(formatOllamaResetText('Refills in 1 week', 'en', en)).toBe('in 1 week (approx.)')
  })
  it('preserves unknown text and does not invent missing data', () => {
    expect(formatOllamaResetText(undefined, 'en', en)).toBeUndefined()
    expect(formatOllamaResetText('Next billing date unknown', 'en', en)).toBe('Next billing date unknown')
  })
})
