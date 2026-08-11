/**
 * 素材库错误提示的回归测试。
 *
 * 背景：演练台导入素材失败时曾提示 "[object Object]"。根因是各组件的 errMessage
 * 只处理 `e instanceof Error`，而 apiClient 拦截器 reject 的是**普通对象**
 * （{ status, code, message, reason }），于是 String(e) 退化成 "[object Object]"，
 * 后端明明给了 "cos image transfer is not enabled or not configured" 却完全看不到。
 *
 * 这里锁定两件事：
 *   1. 拦截器那种普通对象形状必须能取到 message，绝不能出现 "[object Object]"；
 *   2. 后端 reason 能映射到 materials.errors.<REASON> 的友好文案。
 */
import { describe, expect, it } from 'vitest'
import { extractApiErrorCode, extractI18nErrorMessage } from '@/utils/apiError'
import zhMaterials from '@/i18n/locales/zh/materials'
import enMaterials from '@/i18n/locales/en/materials'

/** 复刻 apiClient 响应拦截器 reject 的对象形状（见 src/api/client.ts）。 */
function interceptorError(code: number, message: string, reason?: string) {
  return { status: code, code, message, reason }
}

/**
 * 极简 t()：模拟 vue-i18n 的关键行为 —— 命中返回文案，未命中原样返回 key
 * （extractI18nErrorMessage 正是靠"返回值 === key"判断缺失并回落的）。
 */
function makeT(messages: Record<string, unknown>) {
  return (key: string, params?: Record<string, unknown>) => {
    const parts = key.split('.')
    let cur: unknown = { materials: messages }
    for (const p of parts) {
      if (cur && typeof cur === 'object' && p in (cur as Record<string, unknown>)) {
        cur = (cur as Record<string, unknown>)[p]
      } else {
        return key
      }
    }
    if (typeof cur !== 'string') return key
    let out = cur
    for (const [k, v] of Object.entries(params ?? {})) {
      out = out.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v))
    }
    return out
  }
}

const t = makeT(zhMaterials.materials as Record<string, unknown>)
const tEn = makeT(enMaterials.materials as Record<string, unknown>)

describe('materials error messages', () => {
  it('never renders [object Object] for the interceptor error shape', () => {
    // 这就是用户报的那个回包。
    const err = interceptorError(400, 'cos image transfer is not enabled or not configured', 'COS_NOT_CONFIGURED')
    const msg = extractI18nErrorMessage(err, t, 'materials.errors', 'fallback')
    expect(msg).not.toBe('[object Object]')
    expect(msg).not.toBe('fallback')
    expect(String(err)).toBe('[object Object]') // 证明朴素 String(e) 确实会退化
  })

  it('maps COS_NOT_CONFIGURED to an actionable hint in both locales', () => {
    const err = interceptorError(400, 'cos image transfer is not enabled or not configured', 'COS_NOT_CONFIGURED')
    // 中文提示要指向具体的配置位置，而不是把后端英文原文抛给用户。
    const zh = extractI18nErrorMessage(err, t, 'materials.errors', 'fallback')
    expect(zh).toContain('图片转存')
    expect(zh).not.toContain('cos image transfer is not enabled')

    const en = extractI18nErrorMessage(err, tEn, 'materials.errors', 'fallback')
    expect(en).toContain('Image Transfer')
  })

  it('covers every reason the material backend can return', () => {
    // 与 backend/internal/service/user_material.go + cos_transfer.go 保持同步。
    // 少一个就会让用户看到后端英文原文，因此这里逐个断言。
    const reasons = [
      'COS_NOT_CONFIGURED',
      'URL_BLOCKED',
      'URL_FETCH_FAILED',
      'EMPTY_REMOTE_FILE',
      'EMPTY_FILE',
      'FILE_TOO_LARGE',
      'UNSUPPORTED_CONTENT_TYPE',
      'UNSUPPORTED_KIND',
      'INVALID_URL',
      'MATERIAL_COUNT_QUOTA_EXCEEDED',
      'MATERIAL_SIZE_QUOTA_EXCEEDED',
    ]
    for (const reason of reasons) {
      const err = interceptorError(400, 'raw backend message', reason)
      for (const [name, translate] of [['zh', t], ['en', tEn]] as const) {
        const msg = extractI18nErrorMessage(err, translate, 'materials.errors', 'fallback')
        expect(msg, `${name}: ${reason} should be localized`).not.toBe('raw backend message')
        expect(msg, `${name}: ${reason} should be localized`).not.toBe('fallback')
      }
    }
  })

  it('falls back to the backend message for unmapped reasons', () => {
    // 未收录的 reason 不该吞掉后端信息 —— 那样用户就完全失去线索了。
    const err = interceptorError(500, 'something unexpected', 'SOME_NEW_REASON')
    expect(extractI18nErrorMessage(err, t, 'materials.errors', 'fallback')).toBe('something unexpected')
  })

  it('extracts the reason rather than the numeric http code', () => {
    // isFatalMaterialError 依赖 reason 判断"是否该提前中断批量循环"。
    const err = interceptorError(400, 'x', 'COS_NOT_CONFIGURED')
    expect(extractApiErrorCode(err)).toBe('COS_NOT_CONFIGURED')
  })

  it('still works when the backend omits reason', () => {
    const err = { status: 400, code: 400, message: 'plain failure' }
    expect(extractI18nErrorMessage(err, t, 'materials.errors', 'fallback')).toBe('plain failure')
  })
})
