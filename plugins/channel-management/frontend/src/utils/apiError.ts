/**
 * Plugin-local API error message extraction.
 *
 * 单点替代 5 处自实现 (ChannelMonitorView / AvailableChannelsView /
 * ChannelStatusView / MonitorFormDialog / MonitorDetailDialog) 中的
 * extractMessage / `err instanceof Error ? err.message : String(err)` 平行实现.
 *
 * 设计参考 host frontend `src/utils/apiError.ts`. plugin 不直接依赖 host npm
 * 包, 因此在 plugin 内部维护一份等价实现, 接口与 host 一致 (extractApiError
 * Message(err, fallback)).
 *
 * 处理顺序:
 *   1. host axios 拦截器转换后的 `{ status, code, message, error }` 对象
 *      (plugin SDK 经由 host 注入的 apiClient, 也走同样的拦截器);
 *   2. 原始 axios `{ response.data.detail | message }` 形态 (兜底);
 *   3. 原生 Error;
 *   4. 字符串;
 *   5. 兜底 fallback.
 */

interface ApiErrorLike {
  status?: number
  code?: number | string
  message?: string
  error?: string
  reason?: string
  response?: {
    data?: {
      detail?: string
      message?: string
      code?: number | string
    }
  }
}

/** 提取 API 错误的 code 字段 (string), 用于 i18n 映射. */
export function extractApiErrorCode(err: unknown): string | undefined {
  if (!err || typeof err !== 'object') return undefined
  const e = err as ApiErrorLike
  const code = e.code ?? e.reason ?? e.response?.data?.code
  return code != null ? String(code) : undefined
}

/**
 * 从未知错误中提取可显示的消息.
 *
 * @param err 捕获的错误 (unknown)
 * @param fallback 兜底文案 (调用方应传入 i18n key 翻译后的字符串)
 * @param i18nMap 可选的 code → 翻译文案映射
 */
export function extractApiErrorMessage(
  err: unknown,
  fallback: string,
  i18nMap?: Record<string, string>,
): string {
  if (!err) return fallback

  if (i18nMap) {
    const code = extractApiErrorCode(err)
    if (code && i18nMap[code]) return i18nMap[code]
  }

  if (typeof err === 'object' && err !== null) {
    const e = err as ApiErrorLike
    if (typeof e.message === 'string' && e.message) return e.message
    if (typeof e.error === 'string' && e.error) return e.error
    if (typeof e.response?.data?.detail === 'string' && e.response.data.detail)
      return e.response.data.detail
    if (typeof e.response?.data?.message === 'string' && e.response.data.message)
      return e.response.data.message
  }

  if (err instanceof Error) return err.message || fallback

  const str = String(err)
  return str === '[object Object]' ? fallback : str || fallback
}