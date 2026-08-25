import { describe, expect, it } from 'vitest'
import { aggregatePlazaModels, modelPlazaModelKey, sortAggregatedPlazaModels, type ModelPlazaGroup } from '@/api/modelPlaza'

function group(id: number, platform: string, models: Array<{ name: string; platform?: string }>): ModelPlazaGroup {
  return {
    id,
    name: `group-${id}`,
    description: '',
    platform,
    subscription_type: 'standard',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    models: models.map((model) => ({
      name: model.name,
      platform: model.platform ?? platform,
      pricing: null,
      official_pricing: null
    }))
  }
}

describe('Model Plaza aggregation', () => {
  it('keeps same model names separate across platforms and groups same platform offers', () => {
    const result = aggregatePlazaModels([
      group(1, 'openai', [{ name: 'shared-model' }]),
      group(2, 'openai', [{ name: 'shared-model' }]),
      group(3, 'anthropic', [{ name: 'shared-model' }])
    ])

    expect(modelPlazaModelKey('openai', 'shared-model')).toBe('openai:shared-model')
    expect(result).toHaveLength(2)
    expect(result.find((model) => model.key === 'openai:shared-model')?.offers).toHaveLength(2)
    expect(result.find((model) => model.key === 'anthropic:shared-model')?.offers).toHaveLength(1)
  })

  it('sorts aggregated models without changing exact platform:model identity', () => {
    const result = aggregatePlazaModels([
      { ...group(1, 'openai', [{ name: 'z-model' }, { name: 'a-model' }]), rate_multiplier: 2 },
      { ...group(2, 'openai', [{ name: 'z-model' }]), rate_multiplier: 1 }
    ])

    expect(sortAggregatedPlazaModels(result, 'name').map((model) => model.key)).toEqual(['openai:a-model', 'openai:z-model'])
    expect(sortAggregatedPlazaModels(result, 'platform').map((model) => model.platform)).toEqual(['openai', 'openai'])
    expect(sortAggregatedPlazaModels(result, 'offers')[0]?.key).toBe('openai:z-model')
  })
})
