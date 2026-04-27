import { describe, it, expect } from 'vitest'
import {
  validateServiceQuotaRule,
  parseServiceQuotaApiErrors,
  ServiceQuotaErrorCode,
} from '@/utils/validateServiceQuota'
import type { ServiceQuotaRuleInput } from '@/api/admin/serviceQuota'

function blankForm(overrides: Partial<ServiceQuotaRuleInput> = {}): ServiceQuotaRuleInput {
  return {
    enabled: true,
    name: null,
    counter_mode: 'per_user',
    is_fallback: false,
    target_user_ids: null,
    limiters: [{ limiter_type: 'rpm', window_mode: 'fixed', limit_value: 60 }],
    paths: [{ platform: 'anthropic', channel_id: null, group_id: null, account_id: null, model_pattern: null }],
    ...overrides,
  }
}

describe('validateServiceQuotaRule', () => {
  it('合规表单返回 ok=true，errors 空', () => {
    const r = validateServiceQuotaRule(blankForm())
    expect(r.ok).toBe(true)
    expect(r.errors).toEqual([])
  })

  it('limit_value=null 报 REQUIRED', () => {
    const r = validateServiceQuotaRule(
      blankForm({ limiters: [{ limiter_type: 'rpm', window_mode: 'fixed', limit_value: null as unknown as number }] }),
    )
    expect(r.ok).toBe(false)
    expect(r.errors).toContainEqual({ path: 'limiters[0].limit_value', code: ServiceQuotaErrorCode.Required })
  })

  it('limit_value=0 报 MUST_BE_POSITIVE', () => {
    const r = validateServiceQuotaRule(
      blankForm({ limiters: [{ limiter_type: 'rpm', window_mode: 'fixed', limit_value: 0 }] }),
    )
    expect(r.ok).toBe(false)
    expect(r.errors).toContainEqual({ path: 'limiters[0].limit_value', code: ServiceQuotaErrorCode.MustBePositive })
  })

  it('limit_value=-1 报 MUST_BE_POSITIVE', () => {
    const r = validateServiceQuotaRule(
      blankForm({ limiters: [{ limiter_type: 'rpm', window_mode: 'fixed', limit_value: -1 }] }),
    )
    expect(r.errors).toContainEqual({ path: 'limiters[0].limit_value', code: ServiceQuotaErrorCode.MustBePositive })
  })

  it('TPM 缺 token_components 报 TOKEN_COMPONENTS_REQUIRED', () => {
    const r = validateServiceQuotaRule(
      blankForm({
        limiters: [
          { limiter_type: 'tpm', window_mode: 'fixed', limit_value: 1000, token_components: [] },
        ],
      }),
    )
    expect(r.errors).toContainEqual({
      path: 'limiters[0].token_components',
      code: ServiceQuotaErrorCode.TokenComponentsRequired,
    })
  })

  it('counter_mode=user 但 target_user_ids 空报 TARGET_USERS_REQUIRED', () => {
    const r = validateServiceQuotaRule(blankForm({ counter_mode: 'user', target_user_ids: [] }))
    expect(r.errors).toContainEqual({ path: 'target_user_ids', code: ServiceQuotaErrorCode.TargetUsersRequired })
  })

  it('path.platform 缺失报 PLATFORM_REQUIRED', () => {
    const r = validateServiceQuotaRule(
      blankForm({
        paths: [{ platform: null, channel_id: null, group_id: null, account_id: null, model_pattern: null }],
      }),
    )
    expect(r.errors).toContainEqual({ path: 'paths[0].platform', code: ServiceQuotaErrorCode.PlatformRequired })
  })

  it('path 其他字段全空但 platform 有值时合规', () => {
    const r = validateServiceQuotaRule(
      blankForm({
        paths: [{ platform: 'openai', channel_id: null, group_id: null, account_id: null, model_pattern: null }],
      }),
    )
    expect(r.ok).toBe(true)
  })

  it('limiters / paths 数组为空报 REQUIRED', () => {
    const r = validateServiceQuotaRule(blankForm({ limiters: [], paths: [] }))
    expect(r.errors).toContainEqual({ path: 'limiters', code: ServiceQuotaErrorCode.Required })
    expect(r.errors).toContainEqual({ path: 'paths', code: ServiceQuotaErrorCode.Required })
  })
})

describe('parseServiceQuotaApiErrors', () => {
  it('合法 SERVICE_QUOTA_VALIDATION_ERROR 提取 fields', () => {
    const err = {
      code: 'SERVICE_QUOTA_VALIDATION_ERROR',
      details: {
        fields: [
          { path: 'limiters[0].limit_value', code: 'MUST_BE_POSITIVE' },
          { path: 'paths[1].platform', code: 'PLATFORM_REQUIRED' },
        ],
      },
    }
    expect(parseServiceQuotaApiErrors(err)).toEqual([
      { path: 'limiters[0].limit_value', code: 'MUST_BE_POSITIVE' },
      { path: 'paths[1].platform', code: 'PLATFORM_REQUIRED' },
    ])
  })

  it('其他错误返回 null', () => {
    expect(parseServiceQuotaApiErrors({ code: 'UNAUTHORIZED' })).toBe(null)
    expect(parseServiceQuotaApiErrors(new Error('network'))).toBe(null)
    expect(parseServiceQuotaApiErrors(null)).toBe(null)
    expect(parseServiceQuotaApiErrors('string error')).toBe(null)
  })

  it('details.fields 缺失或非数组返回 null', () => {
    expect(parseServiceQuotaApiErrors({ code: 'SERVICE_QUOTA_VALIDATION_ERROR' })).toBe(null)
    expect(parseServiceQuotaApiErrors({ code: 'SERVICE_QUOTA_VALIDATION_ERROR', details: {} })).toBe(null)
    expect(
      parseServiceQuotaApiErrors({ code: 'SERVICE_QUOTA_VALIDATION_ERROR', details: { fields: 'oops' } }),
    ).toBe(null)
  })

  it('单个 field 缺 path/code 跳过', () => {
    const err = {
      code: 'SERVICE_QUOTA_VALIDATION_ERROR',
      details: {
        fields: [
          { path: 'a', code: 'X' },
          { path: 'b' }, // 缺 code
          { code: 'Y' }, // 缺 path
          null,
        ],
      },
    }
    expect(parseServiceQuotaApiErrors(err)).toEqual([{ path: 'a', code: 'X' }])
  })
})
