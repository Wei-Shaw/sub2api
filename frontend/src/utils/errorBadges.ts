/**
 * 错误请求/用量明细共享的徽章配色与列映射。
 * 配色统一 bg-X-100/text-X-800 体系,与 UsageTable 一致。
 */

import type { UsageRequestType } from '@/types'

export type UsageRequestKind = UsageRequestType

/** 状态码徽章:≥500 红、429 紫、≥400 琥珀、其余灰 */
export function statusCodeBadgeClass(code: number): string {
  if (code >= 500) return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
  if (code === 429) return 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200'
  if (code >= 400) return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'
  return 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-200'
}

/** 请求类型徽章配色(cyber 红、ws 紫、stream 蓝、sync 灰、未知琥珀) */
export function requestTypeBadgeClass(kind: UsageRequestKind): string {
  if (kind === 'cyber') return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
  if (kind === 'ws_v2') return 'bg-violet-100 text-violet-800 dark:bg-violet-900 dark:text-violet-200'
  if (kind === 'stream') return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
  if (kind === 'sync') return 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-gray-200'
  return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'
}

/** 请求类型 i18n 键(展示方自行 t()) */
export function requestTypeLabelKey(kind: UsageRequestKind): string {
  if (kind === 'cyber') return 'usage.cyber'
  if (kind === 'ws_v2') return 'usage.ws'
  if (kind === 'stream') return 'usage.stream'
  if (kind === 'sync') return 'usage.sync'
  return 'usage.unknown'
}

/**
 * 数字 request_type(1 同步/2 流式/3 WS)→ kind;
 * 缺失时按 stream 布尔回退,两者都缺返回 null(展示为 -)。
 */
export function numericRequestTypeKind(
  requestType?: number | null,
  stream?: boolean | null
): UsageRequestKind | null {
  const rt = requestType ?? (stream == null ? 0 : stream ? 2 : 1)
  if (rt === 3) return 'ws_v2'
  if (rt === 2) return 'stream'
  if (rt === 1) return 'sync'
  return null
}

/** 错误表列 key → 后端 sort_by(status 列实际按 status_code 排序) */
export function mapErrorSortKey(key: string): string {
  return key === 'status' ? 'status_code' : key
}

/**
 * 错误请求筛选的常用状态码固定候选(管理端 + 用户端共用)。
 * 用固定列表而非「当前页出现过的码」派生,避免目标状态码只在后续页/筛选外时无法选中
 * ——后端 status_code 过滤对全量数据生效,选项不应被当前页数据限制。
 */
export const COMMON_ERROR_STATUS_CODES = [400, 401, 403, 404, 408, 413, 429, 499, 500, 502, 503, 504, 529]
