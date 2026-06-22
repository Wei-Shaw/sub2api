import { describe, it, expect } from 'vitest'
import { applyInterceptWarmup, applyRefreshTokenFallback } from '../credentialsBuilder'

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

describe('applyRefreshTokenFallback', () => {
  it('adds the original refresh token when credentials do not include one', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyRefreshTokenFallback(creds, 'rt-original')
    expect(creds.refresh_token).toBe('rt-original')
  })

  it('does not overwrite a refresh token returned by the upstream response', () => {
    const creds: Record<string, unknown> = { access_token: 'tok', refresh_token: 'rt-new' }
    applyRefreshTokenFallback(creds, 'rt-original')
    expect(creds.refresh_token).toBe('rt-new')
  })

  it('ignores empty fallback refresh tokens', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyRefreshTokenFallback(creds, '   ')
    expect('refresh_token' in creds).toBe(false)
  })
})
