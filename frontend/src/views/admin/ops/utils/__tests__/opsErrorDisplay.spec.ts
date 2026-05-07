import { describe, expect, it } from 'vitest'
import {
  hasUpstreamCallContext,
  isAuthFailureBeforeScheduling,
  isLocalMetadataEndpoint,
  normalizeOpsPath
} from '../opsErrorDisplay'

describe('opsErrorDisplay', () => {
  it('normalizes query strings and fragments from paths', () => {
    expect(normalizeOpsPath('/v1/models?foo=bar#section')).toBe('/v1/models')
    expect(normalizeOpsPath(' /antigravity/v1/usage?page=1 ')).toBe('/antigravity/v1/usage')
  })

  it('detects local metadata endpoints from request and inbound paths', () => {
    expect(isLocalMetadataEndpoint({
      inbound_endpoint: '/v1/models'
    })).toBe(true)

    expect(isLocalMetadataEndpoint({
      request_path: '/antigravity/v1/models?view=full'
    })).toBe(true)

    expect(isLocalMetadataEndpoint({
      inbound_endpoint: '/v1/chat/completions',
      request_path: '/v1/chat/completions'
    })).toBe(false)
  })

  it('treats upstream placeholders as no upstream call context', () => {
    expect(hasUpstreamCallContext({
      upstream_errors: '[]',
      upstream_error_detail: 'null',
      upstream_error_message: ''
    })).toBe(false)

    expect(hasUpstreamCallContext({
      upstream_error_message: 'Service temporarily unavailable'
    })).toBe(true)
  })

  it('marks auth failures on local metadata endpoints as not entering scheduling', () => {
    expect(isAuthFailureBeforeScheduling({
      phase: 'auth',
      request_path: '/v1/models'
    })).toBe(true)

    expect(isAuthFailureBeforeScheduling({
      phase: 'internal',
      status_code: 401,
      message: 'API key is required in Authorization header (Bearer scheme)',
      request_path: '/v1/models'
    })).toBe(true)

    expect(isAuthFailureBeforeScheduling({
      phase: 'auth',
      request_path: '/v1/models',
      upstream_error_message: 'provider rejected request'
    })).toBe(false)

    expect(isAuthFailureBeforeScheduling({
      phase: 'routing',
      request_path: '/v1/models'
    })).toBe(false)
  })
})
