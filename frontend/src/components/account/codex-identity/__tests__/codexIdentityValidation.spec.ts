import { describe, expect, it } from 'vitest'
import type { CodexIdentityPolicy } from '@/types/codexIdentity'
import {
  availableCodexIdentityProxyIDs,
  createDefaultCodexIdentityPolicy,
  createDefaultCodexOSProfile,
  serializeCodexIdentityPolicy,
  validateCodexIdentityPolicy,
} from '@/utils/codexIdentityValidation'

describe('Codex identity policy contract', () => {
  it('defaults to a backward-compatible off policy using backend constants', () => {
    const policy = createDefaultCodexIdentityPolicy()

    expect(policy).toEqual({
      mode: 'off',
      binding_scope: 'api_key_os',
      session_policy: { mode: 'conversation_isolated' },
      affinity_ttl_seconds: 3600,
      unsupported_policy: 'reject',
      profiles: [],
    })
    expect(validateCodexIdentityPolicy(policy).valid).toBe(true)
  })

  it('accepts Linux Desktop and Generic third-party profiles', () => {
    const linux = createDefaultCodexOSProfile('linux')
    linux.canonical_surface = 'desktop'
    linux.slot_count = 3
    linux.proxy_id = 7
    linux.slots = [{ index: 1, proxy_id: 8 }]
    const generic = createDefaultCodexOSProfile('generic')
    generic.canonical_surface = 'third_party'

    const policy: CodexIdentityPolicy = {
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      profiles: [linux, generic],
    }

    expect(validateCodexIdentityPolicy(policy, {
      availableProxyIDs: new Set([7, 8]),
    })).toEqual({ valid: true, errors: [], warnings: [] })
  })

  it('matches the backend TTL lower bound of 60 seconds', () => {
    const profile = createDefaultCodexOSProfile('windows')
    const policy: CodexIdentityPolicy = {
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      affinity_ttl_seconds: 60,
      profiles: [profile],
    }

    expect(validateCodexIdentityPolicy(policy).valid).toBe(true)
    policy.affinity_ttl_seconds = 59
    expect(validateCodexIdentityPolicy(policy).errors.map((issue) => issue.code))
      .toContain('AFFINITY_TTL_OUT_OF_RANGE')
  })

  it('rejects invalid profile, proxy, slot and session-pool combinations', () => {
    const generic = createDefaultCodexOSProfile('generic')
    generic.architecture = 'x86_64'
    generic.slot_count = 2
    generic.proxy_id = 999
    generic.slots = [{ index: 2, proxy_id: 1 }]
    const policy: CodexIdentityPolicy = {
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      profiles: [generic],
      session_policy: { mode: 'session_pool', sessions_per_device: 4 },
    }

    const codes = validateCodexIdentityPolicy(policy, {
      availableProxyIDs: new Set([1]),
    }).errors.map((issue) => issue.code)

    expect(codes).toEqual(expect.arrayContaining([
      'ARCHITECTURE_NOT_ALLOWED',
      'PROXY_NOT_FOUND',
      'SLOT_OVERRIDE_OUT_OF_RANGE',
      'SESSION_SLOT_COUNT_OUT_OF_RANGE',
    ]))
  })

  it('strips service-managed policy version and profile epoch from payloads', () => {
    const raw = {
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      version: 12,
      profiles: [{
        ...createDefaultCodexOSProfile('windows'),
        epoch: 9,
      }],
    } as CodexIdentityPolicy

    const serialized = serializeCodexIdentityPolicy(raw)
    expect(serialized).not.toHaveProperty('version')
    expect(serialized.profiles?.[0]).not.toHaveProperty('epoch')
    expect(serialized.profiles?.[0]).toMatchObject({
      os_class: 'windows',
      canonical_surface: 'desktop',
      architecture: 'x86_64',
      slot_count: 1,
    })
  })

  it('warns without invalidating explicit shared-session modes', () => {
    const policy: CodexIdentityPolicy = {
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      profiles: [createDefaultCodexOSProfile('macos')],
      session_policy: {
        mode: 'device_shared',
        max_active_conversations_per_slot: 1,
        disable_cross_key_continuation: true,
      },
    }
    const result = validateCodexIdentityPolicy(policy)

    expect(result.valid).toBe(true)
    expect(result.warnings[0]?.code).toBe('DEVICE_SHARED_SESSION_RISK')

    policy.session_policy = { mode: 'session_pool', sessions_per_device: 2 }
    const pooled = validateCodexIdentityPolicy(policy)
    expect(pooled.valid).toBe(true)
    expect(pooled.warnings[0]?.code).toBe('SESSION_POOL_SHARED_SESSION_RISK')
  })

  it('uses only active proxies while preserving legacy entries without a status', () => {
    expect([...availableCodexIdentityProxyIDs([
      { id: 1, name: 'active', status: 'active' },
      { id: 2, name: 'inactive', status: 'inactive' },
      { id: 3, name: 'expired', status: 'expired' },
      { id: 4, name: 'legacy' },
    ])]).toEqual([1, 4])
  })

  it('rejects device sharing without its mandatory isolation guards', () => {
    const policy = {
      ...createDefaultCodexIdentityPolicy(),
      mode: 'os_profile_device_pool',
      profiles: [createDefaultCodexOSProfile('linux')],
      session_policy: { mode: 'device_shared' },
    } as CodexIdentityPolicy

    expect(validateCodexIdentityPolicy(policy).errors.map((issue) => issue.code))
      .toContain('DEVICE_SHARED_RESTRICTIONS_INVALID')
  })
})
