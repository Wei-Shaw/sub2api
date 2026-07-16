/**
 * Support Ticket API endpoints
 *
 * 工单系统前端 API 客户端。覆盖 spec §8 的全部接口：
 *
 *   - 用户端：createTicket / listMyTickets / getMyTicket / appendReply / closeTicket / listCategories
 *   - admin 端：adminListTickets / adminGetTicket / adminAppendReply / adminPatchTicket
 *
 * 把用户端 + admin 端放在同一个文件而非拆 `api/admin/support.ts`：
 *   - 工单数据模型完全共享，类型定义不必重复
 *   - 前端调用时一次性 import
 *   - 与 spec §8 任务清单严格对齐
 *
 * 类型定义放在本文件而非 `types/index.ts`：
 *   - 支持票域内类型很集中，本地内聚便于后续维护
 *   - 避免在 types/index.ts 这种全局类型库里继续堆领域类型
 */

import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'

// ============================================================
// Domain Types
// ============================================================

/** 工单状态枚举（与后端 service.SupportTicketStatus* 对齐）。 */
export type TicketStatus = 'open' | 'in_progress' | 'closed'

/** 工单优先级枚举（与后端 service.SupportTicketPriority* 对齐）。 */
export type TicketPriority = 'low' | 'normal' | 'high'

/**
 * SupportTicketImage 是工单/回复携带的图片附件。
 *
 * 由 `uploadTicketAttachment()` 上传接口返回，随后作为 `images` 数组的元素回传给
 * 建单 / 追加回复接口。字段与后端 `dto.SupportTicketImage` 完全一致。
 */
export interface SupportTicketImage {
  /** 对象存储里的 object key（含前缀），主要用于运维审计，前端不直接使用。 */
  key: string
  /** 完整可访问 URL；bucket 公开可读，无需签名。 */
  url: string
  /** 图片字节数。前端用来做二次尺寸提示；≤5 MB 由后端强制。 */
  size: number
  /** 规范化后的 MIME 类型，白名单：image/png、image/jpeg。 */
  mime: string
}

/** 单张图片体积上限（5 MB，与后端 `SupportTicketImageMaxBytes` 保持一致）。 */
export const SUPPORT_TICKET_IMAGE_MAX_BYTES = 5 * 1024 * 1024
/** 每条主帖 / 回复允许携带的最大图片数（与后端一致）。 */
export const SUPPORT_TICKET_IMAGES_MAX_COUNT = 5
/** 允许上传的 MIME 白名单（后端会用 magic bytes 兜底校验）。 */
export const SUPPORT_TICKET_ALLOWED_IMAGE_MIMES = ['image/png', 'image/jpeg'] as const

/**
 * SupportTicket 是工单列表场景的元素。
 *
 * 编译期不含 `chat_context` 字段——后端 DTO `SupportTicketListItem` 也已经
 * 把它从 schema 里完全移除（spec D1.A 二级保险），列表路径不会泄露大字段。
 */
export interface SupportTicket {
  id: number
  user_id: number
  title: string
  content: string
  category: string
  status: TicketStatus
  priority: TicketPriority
  closed_at?: string | null
  created_at: string
  updated_at: string
}

/** SupportTicketReply 是单条工单回复。 */
export interface SupportTicketReply {
  id: number
  ticket_id: number
  /** ON DELETE SET NULL 之后作者用户被删除时为空。 */
  author_id?: number | null
  is_admin: boolean
  content: string
  created_at: string
  /**
   * 附件图片列表；后端保证永远是数组（NOT NULL DEFAULT '[]'）。
   * 兼容旧记录：读取时用 `reply.images ?? []`。
   */
  images?: SupportTicketImage[]
}

/**
 * SupportTicketWithReplies 是 GET 详情接口的完整投影。
 *
 * 比 SupportTicket 多两个字段：
 *   - `chat_context`：用户在浮窗带过来的对话上下文快照（只在详情场景返回）
 *   - `replies`：按 created_at 升序的完整回复时间线
 */
export interface SupportTicketWithReplies extends SupportTicket {
  chat_context?: string | null
  /** 主帖附带的图片附件（可选，兼容旧数据使用 `?? []`）。 */
  images?: SupportTicketImage[]
  replies: SupportTicketReply[]
}

/** GET /api/v1/support/categories 返回结构。 */
export interface SupportTicketCategoriesResponse {
  categories: string[]
  default_priority: TicketPriority
}

/** POST /api/v1/support/tickets 入参。 */
export interface CreateTicketRequest {
  title: string
  content: string
  category: string
  /** 缺省走后端 settings.default_priority。 */
  priority?: TicketPriority
  /** 浮窗带过来的对话快照；服务端不解析，仅原样存储。 */
  chat_context?: string
  /**
   * 图片附件（≤5 张）。由 `uploadTicketAttachment()` 预先获取，然后随 payload 一起提交。
   * 缺省或空数组表示无附件。
   */
  images?: SupportTicketImage[]
}

/** POST /api/v1/support/tickets/:id/replies 与 admin 版共用的入参。 */
export interface AppendReplyRequest {
  content: string
  /** 与 CreateTicketRequest.images 同规则。 */
  images?: SupportTicketImage[]
}

/** PATCH /api/v1/admin/support/tickets/:id 入参。 */
export interface AdminTicketPatch {
  status?: TicketStatus
  priority?: TicketPriority
  category?: string
}

/** GET /api/v1/admin/support/tickets 过滤参数。 */
export interface AdminTicketFilter {
  status?: TicketStatus
  priority?: TicketPriority
  category?: string
  user_id?: number
  /** 关键词，后端 ILIKE on (title, content)，前端无需 trim/截断。 */
  q?: string
}

// ============================================================
// 用户端接口
// ============================================================

/** 创建工单。返回完整详情（含 chat_context，replies 为空数组）。 */
export async function createTicket(req: CreateTicketRequest): Promise<SupportTicketWithReplies> {
  const { data } = await apiClient.post<SupportTicketWithReplies>('/support/tickets', req)
  return data
}

/** 分页拉取自己的工单列表（不含 chat_context）。 */
export async function listMyTickets(
  page: number = 1,
  pageSize: number = 20,
  options?: { signal?: AbortSignal }
): Promise<BasePaginationResponse<SupportTicket>> {
  const { data } = await apiClient.get<BasePaginationResponse<SupportTicket>>('/support/tickets', {
    params: { page, page_size: pageSize },
    signal: options?.signal,
  })
  return data
}

/** 拉取自己的工单详情（含 chat_context + 回复时间线）。 */
export async function getMyTicket(id: number): Promise<SupportTicketWithReplies> {
  const { data } = await apiClient.get<SupportTicketWithReplies>(`/support/tickets/${id}`)
  return data
}

/**
 * 用户给工单追加回复。已关闭工单返回 409。
 *
 * 兼容两种调用：
 *   - `appendReply(id, content)`：老调用形态，纯文本。
 *   - `appendReply(id, { content, images })`：新形态，可携带图片附件。
 *
 * 保留 overload 避免调用方一次性全部改造，同时给未来新增字段留出扩展空间。
 */
export async function appendReply(
  id: number,
  reqOrContent: string | AppendReplyRequest
): Promise<SupportTicketReply> {
  const body: AppendReplyRequest =
    typeof reqOrContent === 'string' ? { content: reqOrContent } : reqOrContent
  const { data } = await apiClient.post<SupportTicketReply>(
    `/support/tickets/${id}/replies`,
    body
  )
  return data
}

/** 关闭工单。已关闭工单再调返回 409。 */
export async function closeTicket(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`/support/tickets/${id}/close`)
  return data
}

/** 拉新建页用的分类下拉与默认优先级。feature_disabled 时返回 404。 */
export async function listCategories(): Promise<SupportTicketCategoriesResponse> {
  const { data } = await apiClient.get<SupportTicketCategoriesResponse>('/support/categories')
  return data
}

/**
 * uploadTicketAttachment 上传一张工单图片附件。
 *
 * 调用约定：
 *   - 单次上传单张（form field 名固定 `file`），前端遇到多张时并行/串行多次调用即可。
 *   - 后端会用 magic bytes 校验图片格式（不信任 Content-Type 头），前端只做体积
 *     快速校验（≤ 5 MB），避免上传后被后端拒绝浪费流量。
 *   - 未启用工单功能 / COS 未配置 → 404 / 400，由 `extractI18nErrorMessage` 展示错误码。
 *
 * @param file 用户选择的 File 对象；应事先做 5 MB / MIME 前端校验。
 * @param onProgress 可选的进度回调，接收 0..100 的百分比。
 * @returns 返回结构化附件记录，直接放进 `CreateTicketRequest.images` / `AppendReplyRequest.images`。
 */
export async function uploadTicketAttachment(
  file: File,
  onProgress?: (percent: number) => void
): Promise<SupportTicketImage> {
  const form = new FormData()
  form.append('file', file)

  const { data } = await apiClient.post<SupportTicketImage>(
    '/support/tickets/attachments',
    form,
    {
      // 传 FormData 时把默认的 application/json 覆盖掉，让 axios/浏览器
      // 自动带上正确的 multipart boundary。
      headers: { 'Content-Type': 'multipart/form-data' },
      // 大图片可能需要比默认 30s 更长；60s 足够 5 MB 在弱网下完成。
      timeout: 60_000,
      onUploadProgress: onProgress
        ? (evt) => {
            if (evt.total) {
              onProgress(Math.round((evt.loaded / evt.total) * 100))
            }
          }
        : undefined,
    }
  )
  return data
}

// ============================================================
// admin 端接口
// ============================================================

/**
 * admin 分页 + 多维过滤拉工单列表。
 *
 * 排序由后端强制：priority CASE-DESC, created_at DESC, id DESC。
 *
 * 过滤参数中：
 *   - 空字符串 / undefined 字段会被 axios 自动丢弃（默认 paramsSerializer 行为），
 *     与后端 strings.TrimSpace 的语义对齐。
 *   - q 关键词后端会截断到 200 字符（前端不做截断，便于展示原始输入）。
 */
export async function adminListTickets(
  filter: AdminTicketFilter = {},
  page: number = 1,
  pageSize: number = 20,
  options?: { signal?: AbortSignal }
): Promise<BasePaginationResponse<SupportTicket>> {
  const params: Record<string, string | number> = { page, page_size: pageSize }
  if (filter.status) params.status = filter.status
  if (filter.priority) params.priority = filter.priority
  if (filter.category) params.category = filter.category
  if (typeof filter.user_id === 'number' && filter.user_id > 0) params.user_id = filter.user_id
  if (filter.q && filter.q.trim() !== '') params.q = filter.q.trim()

  const { data } = await apiClient.get<BasePaginationResponse<SupportTicket>>(
    '/admin/support/tickets',
    {
      params,
      signal: options?.signal,
    }
  )
  return data
}

/** admin 取工单详情（含 chat_context + 回复）。 */
export async function adminGetTicket(id: number): Promise<SupportTicketWithReplies> {
  const { data } = await apiClient.get<SupportTicketWithReplies>(`/admin/support/tickets/${id}`)
  return data
}

/**
 * admin 追加回复。
 *
 * 后端语义：
 *   - 自动触发 open → in_progress（用户视角能直接看到状态变化）
 *   - 不卡 feature_enabled：即使关掉了工单总开关，admin 仍可处理存量
 *   - 已关闭工单返回 409
 */
export async function adminAppendReply(
  id: number,
  reqOrContent: string | AppendReplyRequest
): Promise<SupportTicketReply> {
  const body: AppendReplyRequest =
    typeof reqOrContent === 'string' ? { content: reqOrContent } : reqOrContent
  const { data } = await apiClient.post<SupportTicketReply>(
    `/admin/support/tickets/${id}/replies`,
    body
  )
  return data
}

/**
 * admin PATCH 工单字段。
 *
 * 任意一个字段都为 undefined 时返回 400（service ErrSupportTicketNoFieldsToUpdate）；
 * status: closed → 非 closed 转移返回 409；status → closed 时后端自动同步设置
 * closed_at。
 */
export async function adminPatchTicket(id: number, patch: AdminTicketPatch): Promise<SupportTicket> {
  const { data } = await apiClient.patch<SupportTicket>(`/admin/support/tickets/${id}`, patch)
  return data
}

// ============================================================
// admin 端：客服对话记录（add-support-chat-transcript-log，只读）
// ============================================================

/** 客服对话回包状态完整分类（与后端 ChatLogStatus* 对齐）。 */
export type ChatLogStatus =
  | 'success'
  | 'upstream_auth'
  | 'upstream_error'
  | 'interrupted'
  | 'rate_limited'
  | 'config_error'

/** 会话列表项（不含消息正文）。 */
export interface ChatConversationListItem {
  id: number
  session_id: string
  user_id: number | null
  /** 后端 LEFT JOIN users 后回填；匿名或用户彻底删除时缺失。 */
  user_email?: string | null
  client_ip: string
  turn_count: number
  last_status: ChatLogStatus | string
  first_at: string | null
  last_at: string | null
  created_at: string
}

/** 会话详情里的单条消息。 */
export interface ChatConversationMessage {
  id: number
  role: 'user' | 'assistant' | string
  content: string
  status?: ChatLogStatus | string
  error_message?: string
  model?: string
  latency_ms?: number
  created_at: string
}

/** 会话详情：会话头 + 消息时间线。 */
export interface ChatConversationDetail extends ChatConversationListItem {
  messages: ChatConversationMessage[]
}

/** GET /api/v1/admin/support/chat/conversations 过滤参数。 */
export interface AdminChatLogFilter {
  status?: string
  user_id?: number
  ip?: string
  q?: string
  from?: string
  to?: string
}

/** admin 分页 + 多维过滤拉客服对话记录列表（不含消息正文）。 */
export async function adminListChatConversations(
  filter: AdminChatLogFilter = {},
  page: number = 1,
  pageSize: number = 20,
  options?: { signal?: AbortSignal }
): Promise<BasePaginationResponse<ChatConversationListItem>> {
  const params: Record<string, string | number> = { page, page_size: pageSize }
  if (filter.status) params.status = filter.status
  if (typeof filter.user_id === 'number' && filter.user_id > 0) params.user_id = filter.user_id
  if (filter.ip && filter.ip.trim() !== '') params.ip = filter.ip.trim()
  if (filter.q && filter.q.trim() !== '') params.q = filter.q.trim()
  if (filter.from) params.from = filter.from
  if (filter.to) params.to = filter.to

  const { data } = await apiClient.get<BasePaginationResponse<ChatConversationListItem>>(
    '/admin/support/chat/conversations',
    {
      params,
      signal: options?.signal,
    }
  )
  return data
}

/** admin 取客服对话详情（含整段消息时间线）。 */
export async function adminGetChatConversation(id: number): Promise<ChatConversationDetail> {
  const { data } = await apiClient.get<ChatConversationDetail>(
    `/admin/support/chat/conversations/${id}`
  )
  return data
}

// ============================================================
// 默认导出
// ============================================================

const supportAPI = {
  // 用户端
  createTicket,
  listMyTickets,
  getMyTicket,
  appendReply,
  closeTicket,
  listCategories,
  uploadTicketAttachment,
  // admin 端
  adminListTickets,
  adminGetTicket,
  adminAppendReply,
  adminPatchTicket,
  // admin 端：客服对话记录
  adminListChatConversations,
  adminGetChatConversation,
}

export default supportAPI
