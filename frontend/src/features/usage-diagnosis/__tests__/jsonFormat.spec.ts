import { describe, expect, it } from 'vitest'
import { tryParseJsonBody, unwrapJson, formatHeaders } from '../utils/jsonFormat'

describe('jsonFormat', () => {
  it('unwraps nested json strings up to 3 layers', () => {
    const nested = JSON.stringify(JSON.stringify({ a: 1 }))
    expect(unwrapJson(nested)).toEqual({ a: 1 })
  })

  it('handles truncated json without crashing', () => {
    const raw = '{"a":1,"b":2'
    const parsed = tryParseJsonBody(raw)
    expect(parsed.empty).toBe(false)
    if (parsed.truncated) {
      expect(parsed.value).toBeTruthy()
    } else {
      expect(typeof parsed.value === 'string' || typeof parsed.value === 'object').toBe(true)
    }
  })

  it('formats headers as name: value lines', () => {
    expect(formatHeaders({ A: '1', B: '2' })).toContain('A: 1')
  })
})
