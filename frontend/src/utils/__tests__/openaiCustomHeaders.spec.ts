import { describe, expect, it } from 'vitest'
import {
  buildOpenAICustomHeadersObject,
  getOpenAICustomHeaderRowError,
  readOpenAICustomHeaders
} from '../openaiCustomHeaders'

describe('openaiCustomHeaders', () => {
  it('reads object and row formats', () => {
    expect(readOpenAICustomHeaders({
      'X-Trace-Id': 'trace-1',
      Authorization: 'Bearer ignored-by-build'
    })).toEqual([
      { name: 'X-Trace-Id', value: 'trace-1' },
      { name: 'Authorization', value: 'Bearer ignored-by-build' }
    ])

    expect(readOpenAICustomHeaders([
      { name: 'X-Name', value: 'name-value' },
      { key: 'X-Key', value: 'key-value' },
      { header: 'X-Header', value: 'header-value' }
    ])).toEqual([
      { name: 'X-Name', value: 'name-value' },
      { name: 'X-Key', value: 'key-value' },
      { name: 'X-Header', value: 'header-value' }
    ])
  })

  it('builds only complete allowed unique headers', () => {
    const rows = [
      { name: ' X-Trace-Id ', value: ' trace-1 ' },
      { name: 'Authorization', value: 'Bearer blocked' },
      { name: 'X-Dupe', value: 'one' },
      { name: 'x-dupe', value: 'two' },
      { name: 'Bad Header', value: 'bad' },
      { name: 'X-Blank', value: '' }
    ]

    expect(buildOpenAICustomHeadersObject(rows)).toEqual({
      'X-Trace-Id': 'trace-1',
      Authorization: 'Bearer blocked'
    })
    expect(getOpenAICustomHeaderRowError(rows[1], rows)).toBeNull()
    expect(getOpenAICustomHeaderRowError(rows[2], rows)).toBe('duplicate')
    expect(getOpenAICustomHeaderRowError(rows[3], rows)).toBe('duplicate')
    expect(getOpenAICustomHeaderRowError(rows[4], rows)).toBe('invalid')
    expect(getOpenAICustomHeaderRowError(rows[5], rows)).toBe('incomplete')
  })
})
