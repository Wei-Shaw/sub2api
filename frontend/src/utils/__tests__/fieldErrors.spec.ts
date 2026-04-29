import { describe, it, expect } from 'vitest'
import { parseFieldErrors, fieldErrorI18nKeys, type FieldError } from '@/utils/fieldErrors'

// 后端实际 envelope（pkgerrors.Metadata 是 map[string]string，字段错误数组
// 必须 JSON 序列化进字符串值）：
//   { reason, metadata: { fields: '[...]', count: 'N' } }
// axios 拦截器（client.ts:97-103）扁平化为 { status, code:HTTP, reason, message, metadata }
function envelope(reason: string, fields: FieldError[] | string): Record<string, unknown> {
  return {
    status: 400,
    code: 400,
    reason,
    message: 'validation failed',
    metadata: {
      count: String(Array.isArray(fields) ? fields.length : 0),
      fields: typeof fields === 'string' ? fields : JSON.stringify(fields),
    },
  }
}

describe('parseFieldErrors', () => {
  it('合法 envelope 反序列化 metadata.fields 字符串', () => {
    const err = envelope('SERVICE_QUOTA_VALIDATION_ERROR', [
      { path: 'limiters[0].limit_value', code: 'MUST_BE_POSITIVE' },
      { path: 'paths[1].platform', code: 'PLATFORM_REQUIRED' },
    ])
    expect(parseFieldErrors(err)).toEqual([
      { path: 'limiters[0].limit_value', code: 'MUST_BE_POSITIVE' },
      { path: 'paths[1].platform', code: 'PLATFORM_REQUIRED' },
    ])
  })

  it('expectedReason 匹配时返回字段错误', () => {
    const err = envelope('SERVICE_QUOTA_VALIDATION_ERROR', [{ path: 'a', code: 'X' }])
    expect(parseFieldErrors(err, 'SERVICE_QUOTA_VALIDATION_ERROR')).toEqual([{ path: 'a', code: 'X' }])
  })

  it('expectedReason 不匹配返回 null（避免误吞别的模块错误）', () => {
    const err = envelope('PAYMENT_VALIDATION_ERROR', [{ path: 'a', code: 'X' }])
    expect(parseFieldErrors(err, 'SERVICE_QUOTA_VALIDATION_ERROR')).toBe(null)
  })

  it('reason 缺失返回 null', () => {
    expect(parseFieldErrors({ metadata: { fields: '[]' } })).toBe(null)
    expect(parseFieldErrors({ reason: '', metadata: { fields: '[]' } })).toBe(null)
    // reason 类型必须是 string
    expect(parseFieldErrors({ reason: 123, metadata: { fields: '[]' } })).toBe(null)
  })

  it('metadata 缺失或非对象返回 null', () => {
    expect(parseFieldErrors({ reason: 'X' })).toBe(null)
    expect(parseFieldErrors({ reason: 'X', metadata: 'oops' })).toBe(null)
    expect(parseFieldErrors({ reason: 'X', metadata: null })).toBe(null)
  })

  it('metadata.fields 缺失或非字符串返回 null', () => {
    expect(parseFieldErrors({ reason: 'X', metadata: {} })).toBe(null)
    expect(parseFieldErrors({ reason: 'X', metadata: { fields: 123 } })).toBe(null)
    expect(parseFieldErrors({ reason: 'X', metadata: { fields: ['a'] } })).toBe(null)
  })

  it('metadata.fields JSON 解析失败返回 null（破损报文兜底）', () => {
    expect(parseFieldErrors(envelope('X', 'not-a-json{'))).toBe(null)
    expect(parseFieldErrors(envelope('X', ''))).toBe(null)
  })

  it('metadata.fields 解析后非数组返回 null', () => {
    expect(parseFieldErrors(envelope('X', '{"path":"a","code":"X"}'))).toBe(null)
    expect(parseFieldErrors(envelope('X', 'null'))).toBe(null)
    expect(parseFieldErrors(envelope('X', '"string"'))).toBe(null)
  })

  it('null / undefined / 字符串 / 数字 等非对象输入返回 null', () => {
    expect(parseFieldErrors(null)).toBe(null)
    expect(parseFieldErrors(undefined)).toBe(null)
    expect(parseFieldErrors('error')).toBe(null)
    expect(parseFieldErrors(42)).toBe(null)
    expect(parseFieldErrors(new Error('boom'))).toBe(null) // Error 实例无 reason 字段
  })

  it('单条 field 缺 path 或 code 时跳过；保留合法的', () => {
    const err = {
      reason: 'X',
      metadata: {
        fields: JSON.stringify([
          { path: 'a', code: 'OK' },
          { path: 'b' }, // 缺 code
          { code: 'Y' }, // 缺 path
          null, // 非对象
          { path: 1, code: 'X' }, // path 类型错
          { path: 'c', code: 2 }, // code 类型错
          { path: 'valid', code: 'GOOD' },
        ]),
      },
    }
    expect(parseFieldErrors(err)).toEqual([
      { path: 'a', code: 'OK' },
      { path: 'valid', code: 'GOOD' },
    ])
  })

  it('空数组合法返回空数组（不是 null）', () => {
    expect(parseFieldErrors(envelope('X', []))).toEqual([])
  })
})

describe('fieldErrorI18nKeys', () => {
  it('未传 moduleNamespace 时只返回通用 namespace key', () => {
    expect(fieldErrorI18nKeys('REQUIRED')).toEqual(['validation.fieldErrors.REQUIRED'])
  })

  it('传 moduleNamespace 时模块优先 + 通用兜底，按顺序返回', () => {
    expect(fieldErrorI18nKeys('REQUIRED', 'admin.serviceQuota.errors')).toEqual([
      'admin.serviceQuota.errors.REQUIRED',
      'validation.fieldErrors.REQUIRED',
    ])
  })

  it('保持 code 原样（不大小写转换）', () => {
    expect(fieldErrorI18nKeys('mustBePositive', 'ns')).toEqual([
      'ns.mustBePositive',
      'validation.fieldErrors.mustBePositive',
    ])
  })
})
