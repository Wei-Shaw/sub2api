/**
 * inbox.ts —— 通用信箱（general-inbox）REST 客户端。
 *
 * 对应后端 backend/internal/inbox/http_handler.go：
 *   - GET  /inbox/catchup?since=<seq>  冷启动 / 断线补齐
 *   - POST /inbox/ack       { seq }     累积 ack 水位推进
 *   - GET  /inbox/unread-count          未读数（局部展示用，权威水位仍以 catchup 为准）
 *
 * WebSocket（实时推送）不在此文件，见 stores/inbox.ts 内的连接逻辑。
 * 返回体经 apiClient 拦截器解包，这里拿到的已是 data 负载。
 */
import { apiClient } from './client'

// ============================================================
// Domain Types
// ============================================================

/** 消息投递范围：单播 / 广播。 */
export type InboxScope = 'direct' | 'broadcast'

/**
 * InboxMessage 是 catchup / WS push 下发的统一消息体。
 * payload 是业务方自定义 JSON object（如工单事件），前端按 namespace 渲染。
 */
export interface InboxMessage {
  seq: number
  scope: InboxScope
  namespace: string
  payload: Record<string, unknown>
  created_at: string
}

/** GET /inbox/catchup 响应。 */
export interface InboxCatchupResponse {
  messages: InboxMessage[]
  /** 服务端记录的已 ack 水位；客户端持久化为 local_ack_seq 的下界。 */
  acked_seq: number
  /** 候选未耗尽，客户端可继续用新的 since 再 catchup 一次。 */
  has_more: boolean
}

/** POST /inbox/ack 响应。 */
export interface InboxAckResponse {
  acked_seq: number
}

/** GET /inbox/unread-count 响应。 */
export interface InboxUnreadCountResponse {
  count: number
  /** 未读集合被服务端 LIMIT 截断（离线堆积过多），count 为下界。 */
  truncated: boolean
}

// ============================================================
// API
// ============================================================

/**
 * getInboxCatchup 拉取 since 之后的消息。
 * since 传客户端持久化的 local_ack_seq；服务端首次调用会懒初始化水位到 now。
 */
export async function getInboxCatchup(
  since: number,
  options?: { signal?: AbortSignal }
): Promise<InboxCatchupResponse> {
  const { data } = await apiClient.get<InboxCatchupResponse>('/inbox/catchup', {
    params: { since },
    signal: options?.signal,
  })
  return data
}

/** postInboxAck 推进服务端累积 ack 水位到 seq（幂等，服务端只前进不回退）。 */
export async function postInboxAck(seq: number): Promise<InboxAckResponse> {
  const { data } = await apiClient.post<InboxAckResponse>('/inbox/ack', { seq })
  return data
}

/** getInboxUnreadCount 拉未读数（badge 展示用）。 */
export async function getInboxUnreadCount(
  options?: { signal?: AbortSignal }
): Promise<InboxUnreadCountResponse> {
  const { data } = await apiClient.get<InboxUnreadCountResponse>('/inbox/unread-count', {
    signal: options?.signal,
  })
  return data
}
