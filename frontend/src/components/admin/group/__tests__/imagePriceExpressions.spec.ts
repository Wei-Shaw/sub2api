import { describe, expect, it } from 'vitest'
import {
  imagePriceExpressionsToRows,
  serializeImagePriceExpressions,
  supportsImagePriceExpressions,
  validateImagePriceExpressionRows
} from '../imagePriceExpressions'

describe('image price expression forms', () => {
  it('preserves unknown models, case and source without mutating the saved map', () => {
    const saved = { 'Future-Image': 'fixed(0.123)', 'future-image': 'fixed(0.456)' }
    const rows = imagePriceExpressionsToRows(saved)
    expect(serializeImagePriceExpressions(rows)).toEqual(saved)
    rows[0].expression = 'fixed(0.5)'
    expect(saved['Future-Image']).toBe('fixed(0.123)')
  })

  it('sends an explicit empty object to clear all saved rules', () => {
    expect(serializeImagePriceExpressions([])).toEqual({})
    expect(serializeImagePriceExpressions([{ model: '', expression: '' }])).toEqual({})
    expect(imagePriceExpressionsToRows(null)).toEqual([])
  })

  it.each([
    [{ model: 'image', expression: '' }, 'incompleteRow'],
    [{ model: '', expression: 'fixed(0.04)' }, 'incompleteRow'],
    [{ model: ' image ', expression: 'fixed(0.04)' }, 'modelWhitespace']
  ] as const)('rejects an invalid row before serialization', (row, error) => {
    expect(validateImagePriceExpressionRows([row])).toBe(error)
    expect(() => serializeImagePriceExpressions([row])).toThrow(error)
  })

  it('rejects duplicate exact model IDs rather than overwriting a price', () => {
    const rows = [
      { model: 'image', expression: 'fixed(0.04)' },
      { model: 'image', expression: 'fixed(0.08)' }
    ]
    expect(validateImagePriceExpressionRows(rows)).toBe('duplicateModel')
    expect(() => serializeImagePriceExpressions(rows)).toThrow('duplicateModel')
  })

  it('does not evaluate source or automatically multiply the submitted total', () => {
    const source = 'not-valid-server-syntax(0.08)'
    expect(serializeImagePriceExpressions([{ model: 'image', expression: source }])).toEqual({ image: source })
  })

  it('preserves arbitrary model IDs without mutating the object prototype', () => {
    const saved = JSON.parse('{"__proto__":"fixed(0.08)"}')
    expect(serializeImagePriceExpressions(imagePriceExpressionsToRows(saved))).toEqual(saved)
  })

  it.each([
    ['grok', true], ['openai', true], ['composite', true],
    ['gemini', false], ['antigravity', false], ['anthropic', false]
  ])('only offers expressions for supported platform %s', (platform, supported) => {
    expect(supportsImagePriceExpressions(platform as string)).toBe(supported)
  })
})
