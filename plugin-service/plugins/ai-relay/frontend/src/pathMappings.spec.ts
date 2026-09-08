import { describe, expect, it } from 'vitest'
import { canonicalPath, mappingRecordFromRows, mappingRowsFromRecord } from './pathMappings'

describe('path mapping helpers', () => {
  it('normalizes optional v1 path prefixes', () => {
    expect(canonicalPath('/v1/responses/compact/')).toBe('responses/compact')
  })

  it('preserves the selected source prefix and omits blank rows', () => {
    expect(mappingRecordFromRows([
      { id: 1, source: '/v1/responses/compact/', target: '/chat/completions/' },
      { id: 2, source: '', target: '' },
    ])).toEqual({ 'v1/responses/compact': 'chat/completions' })
  })

  it('converts records into editable rows', () => {
    expect(mappingRowsFromRecord({ models: 'models' }, () => 7)[0]).toEqual({
      id: 7,
      source: 'models',
      target: 'models',
    })
  })
})
