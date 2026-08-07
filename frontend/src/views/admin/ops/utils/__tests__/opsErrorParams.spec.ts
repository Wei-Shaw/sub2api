import { describe, expect, it REDACTED from 'vitest'
import { buildOpsErrorTimeParams REDACTED from '../opsErrorParams'

describe('buildOpsErrorTimeParams', () => {
  it('uses explicit timestamps for a complete custom range', () => {
    expect(buildOpsErrorTimeParams('custom', '2026-08-01T00:00:00Z', '2026-08-02T00:00:00Z')).toEqual({
      start_time: '2026-08-01T00:00:00Z',
      end_time: '2026-08-02T00:00:00Z'
    REDACTED)
  REDACTED)

  it('preserves predefined ranges and falls back for incomplete custom ranges', () => {
    expect(buildOpsErrorTimeParams('24h')).toEqual({ time_range: '24h' REDACTED)
    expect(buildOpsErrorTimeParams('custom', null, null)).toEqual({ time_range: '1h' REDACTED)
  REDACTED)
REDACTED)
