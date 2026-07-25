import type { UsageRequestType } from '@/types'

export interface UsageRequestTypeLike {
  request_type?: string | null
  stream?: boolean | null
  openai_ws_mode?: boolean | null
}

const VALID_REQUEST_TYPES = new Set<UsageRequestType>(['unknown', 'sync', 'stream', 'ws_v2', 'cyber'])

export const isUsageRequestType = (value: unknown): value is UsageRequestType => {
  return typeof value === 'string' && VALID_REQUEST_TYPES.has(value as UsageRequestType)
}

export const resolveUsageRequestType = (value: UsageRequestTypeLike): UsageRequestType => {
  // cyber 与 transport 正交，始终优先保留。
  if (value.request_type === 'cyber') {
    return 'cyber'
  }
  // 上游 Responses WSv2 标记优先于下游 stream/sync 展示。
  if (value.openai_ws_mode) {
    return 'ws_v2'
  }
  if (isUsageRequestType(value.request_type)) {
    return value.request_type
  }
  return value.stream ? 'stream' : 'sync'
}

export const requestTypeToLegacyStream = (requestType?: UsageRequestType | null): boolean | null | undefined => {
  // cyber 与 stream 正交（cyber 可发生在 stream 或非 stream 请求），不映射到 legacy stream 维度。
  if (!requestType || requestType === 'unknown' || requestType === 'cyber') {
    return null
  }
  if (requestType === 'sync') {
    return false
  }
  return true
}
