import { describe, it, expect } from 'vitest'
import {
  applyInterceptWarmup,
  buildTempUnschedRules,
  hasInvalidTempUnschedResetAtTime,
  isValidTempUnschedResetAtTime
} from '../credentialsBuilder'

describe('applyInterceptWarmup', () => {
  it('create + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, true, 'create')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('create + enabled=false: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, false, 'create')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, true, 'edit')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('edit + enabled=false + field exists: should delete the field', () => {
    const creds: Record<string, unknown> = { api_key: 'sk', intercept_warmup_requests: true }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=false + field absent: should not throw', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('should not affect other fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      base_url: 'url',
      intercept_warmup_requests: true
    }
    applyInterceptWarmup(creds, false, 'edit')
    expect(creds.api_key).toBe('sk')
    expect(creds.base_url).toBe('url')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })
})

describe('temp unschedulable rule builder', () => {
  it('accepts reset_at_time without duration when the time is in range', () => {
    const rules = buildTempUnschedRules([
      {
        error_code: 503,
        keywords: 'overloaded',
        duration_minutes: 0,
        description: 'daily reset',
        reset_at_time: '00:00'
      }
    ])

    expect(rules).toEqual([
      {
        error_code: 503,
        keywords: ['overloaded'],
        duration_minutes: 0,
        description: 'daily reset',
        reset_at_time: '00:00'
      }
    ])
  })

  it('rejects out-of-range reset_at_time values even with a duration fallback', () => {
    expect(isValidTempUnschedResetAtTime('23:59')).toBe(true)
    expect(isValidTempUnschedResetAtTime('24:00')).toBe(false)
    expect(isValidTempUnschedResetAtTime('99:99')).toBe(false)
    expect(
      hasInvalidTempUnschedResetAtTime([
        {
          error_code: 503,
          keywords: 'overloaded',
          duration_minutes: 30,
          description: '',
          reset_at_time: '24:00'
        }
      ])
    ).toBe(true)

    const rules = buildTempUnschedRules([
      {
        error_code: 503,
        keywords: 'overloaded',
        duration_minutes: 30,
        description: '',
        reset_at_time: '24:00'
      },
      {
        error_code: 429,
        keywords: 'rate limit',
        duration_minutes: 10,
        description: '',
        reset_at_time: '99:99'
      }
    ])

    expect(rules).toEqual([])
  })
})
