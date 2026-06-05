/**
 * fieldErrors — 通用字段级校验错误解析器。
 *
 * 后端约定：当请求触发字段级校验失败时，返回 ApplicationError 形如
 *   { reason: "<MODULE>_VALIDATION_ERROR", metadata: { fields: '[{path, code}, ...]', count: 'N' } }
 * 由于 pkgerrors.Metadata 是 map[string]string，字段错误数组必须 JSON 序列化进字符串值。
 *
 * axios 拦截器（frontend/src/api/client.ts:97-103）把响应体扁平化为 {status, code, reason, message, metadata}。
 *
 * 本模块负责把这层 envelope 还原为 FieldError[]。所有调用方（service quota /
 * 未来其他模块）共用同一份解析逻辑，避免每个模块抄一遍 JSON.parse + 数组校验。
 */

/** 单个字段错误：path 是 JSON 路径（如 'limiters[0].limit_value'），code 是机器可读的错误码常量 */
export interface FieldError {
  path: string
  code: string
}

/**
 * 解析 axios 拦截器抛出的 API 错误对象，提取 metadata.fields 字段错误数组。
 *
 * @param err            - 任意捕获到的错误（unknown，不假设形状）
 * @param expectedReason - 可选 reason 过滤。传入时仅当 err.reason === expectedReason 才解析；
 *                         不传时接受任何 reason，让通用 dialog 也能用
 *
 * 返回 null 的所有路径都让调用方走 fallback toast：
 *   - err 不是对象 / null
 *   - expectedReason 不匹配
 *   - metadata 缺失或非对象
 *   - metadata.fields 不是字符串
 *   - JSON.parse 抛错
 *   - 解析后非数组
 *
 * 单条 field 缺 path 或 code（或类型不对）时被静默跳过——避免一条破损 field
 * 让整个错误响应都不可解析，影响其它字段红字渲染。
 */
export function parseFieldErrors(err: unknown, expectedReason?: string): FieldError[] | null {
  if (!err || typeof err !== 'object') return null
  const e = err as Record<string, unknown>

  if (expectedReason !== undefined && e.reason !== expectedReason) return null
  if (typeof e.reason !== 'string' || e.reason === '') return null

  const metadata = e.metadata
  if (!metadata || typeof metadata !== 'object') return null

  const raw = (metadata as Record<string, unknown>).fields
  if (typeof raw !== 'string') return null

  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!Array.isArray(parsed)) return null

  const out: FieldError[] = []
  for (const f of parsed) {
    if (!f || typeof f !== 'object') continue
    const obj = f as Record<string, unknown>
    if (typeof obj.path !== 'string' || typeof obj.code !== 'string') continue
    out.push({ path: obj.path, code: obj.code })
  }
  return out
}

/**
 * 把 FieldError 解析为 i18n key 序列：先在模块专属命名空间查（如
 * 'admin.serviceQuota.errors'），未命中回退到通用 'validation.fieldErrors'。
 *
 * 调用方负责实际 t(key) 调用——本函数只返回候选 key 列表，不耦合 vue-i18n。
 *
 * @example
 *   const keys = fieldErrorI18nKeys('REQUIRED', 'admin.serviceQuota.errors')
 *   // → ['admin.serviceQuota.errors.REQUIRED', 'validation.fieldErrors.REQUIRED']
 *   for (const key of keys) {
 *     const v = t(key)
 *     if (v !== key) return v
 *   }
 */
export function fieldErrorI18nKeys(code: string, moduleNamespace?: string): string[] {
  const out: string[] = []
  if (moduleNamespace) out.push(`${moduleNamespace}.${code}`)
  out.push(`validation.fieldErrors.${code}`)
  return out
}
