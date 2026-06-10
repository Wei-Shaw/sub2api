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

/** 用户给工单追加回复。已关闭工单返回 409。 */
export async function appendReply(id: number, content: string): Promise<SupportTicketReply> {
  const { data } = await apiClient.post<SupportTicketReply>(`/support/tickets/${id}/replies`, { content })
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
export async function adminAppendReply(id: number, content: string): Promise<SupportTicketReply> {
  const { data } = await apiClient.post<SupportTicketReply>(
    `/admin/support/tickets/${id}/replies`,
    { content }
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
  // admin 端
  adminListTickets,
  adminGetTicket,
  adminAppendReply,
  adminPatchTicket,
}

export default supportAPI
