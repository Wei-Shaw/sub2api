import { describe, expect, it } from 'vitest'
import { formatBytes } from '../format'
import { formatByteRate } from '@/views/admin/ops/utils/opsFormatters'

describe('fractional byte rates', () => {
  it.each([[0.5, '0.5 Bytes'], [0.01, '0.01 Bytes'], [0, '0 Bytes'], [1024, '1 KB']])('formats %s bytes', (value, expected) => {
    expect(formatBytes(value as number)).toBe(expected)
  })

  it('keeps the byte unit when an observation averages less than one byte per second', () => {
    expect(formatByteRate(30, 1)).toBe('0.5 Bytes/s')
  })
})
