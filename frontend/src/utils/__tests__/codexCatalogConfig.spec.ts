import { describe, expect, it } from 'vitest'
import {
  findCodexCatalogModel,
  formatCodexReasoningEffortTomlLine,
  parseCodexCatalogModels,
  selectCodexConfigReasoningEffort
} from '@/utils/codexCatalogConfig'

describe('codexCatalogConfig', () => {
  it('parses catalog slugs and finds a model by id', () => {
    const content = JSON.stringify({
      models: [
        { slug: 'glm-5.3', default_reasoning_level: 'none', supported_reasoning_levels: [{ effort: 'none' }] },
        { slug: '  ', supported_reasoning_levels: [] }
      ]
    })
    expect(parseCodexCatalogModels(content).map((model) => model.slug)).toEqual(['glm-5.3'])
    expect(findCodexCatalogModel(content, 'glm-5.3')?.slug).toBe('glm-5.3')
    expect(findCodexCatalogModel(content, 'missing')).toBeUndefined()
  })

  it('omits effort when the descriptor only advertises none', () => {
    expect(selectCodexConfigReasoningEffort({
      slug: 'glm-5.3',
      default_reasoning_level: 'none',
      supported_reasoning_levels: [{ effort: 'none' }]
    })).toBeNull()
    expect(formatCodexReasoningEffortTomlLine(null)).toBe('')
  })

  it('does not emit an effort absent from supported_reasoning_levels', () => {
    expect(selectCodexConfigReasoningEffort({
      slug: 'glm-5.3',
      default_reasoning_level: 'xhigh',
      supported_reasoning_levels: [{ effort: 'none' }]
    })).toBeNull()
  })

  it('uses the catalog default when it is a supported non-none effort', () => {
    expect(selectCodexConfigReasoningEffort({
      slug: 'gpt-5.5',
      default_reasoning_level: 'medium',
      supported_reasoning_levels: [
        { effort: 'low' },
        { effort: 'medium' },
        { effort: 'high' },
        { effort: 'xhigh' }
      ]
    })).toBe('medium')
    expect(formatCodexReasoningEffortTomlLine('medium')).toBe('model_reasoning_effort = "medium"\n')
  })

  it('falls back to the first usable supported effort when default is missing', () => {
    expect(selectCodexConfigReasoningEffort({
      slug: 'custom',
      supported_reasoning_levels: [{ effort: 'none' }, { effort: 'high' }]
    })).toBe('high')
  })
})
