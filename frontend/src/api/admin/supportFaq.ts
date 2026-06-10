/**
 * Admin Support Knowledge Base API client (add-support-knowledge-rag §12-§13)
 *
 * 客服知识库 admin 端 HTTP 客户端，覆盖：
 *
 *   - FAQ CRUD：list / create / update / delete / reindex
 *   - 文档索引管线：rebuild / status / purge
 *
 * 与 `api/support.ts` 拆开放：那里是通用工单 + chat 客户端，本文件聚焦 RAG。
 * 用 axios `apiClient`：所有接口都是 admin JSON，不涉及流式。
 */

import { apiClient } from '@/api/client'

// ============================================================
// FAQ
// ============================================================

/** 单条 FAQ（admin 视图）—— 与后端 dto.SupportFaqItem 对齐。 */
export interface AdminSupportFaqItem {
  id: number
  question: string
  answer: string
  tags: string[]
  enabled: boolean
  sort_order: number
  /** embedding 是否已成功写入；false → 该行不会进入向量检索。 */
  indexed: boolean
  created_at: string
  updated_at: string
}

/** Create / Update 之后后端返回 item + embedding warning。 */
export interface AdminSupportFaqMutationResponse {
  item: AdminSupportFaqItem | null
  embedding_warning?: string
}

/** POST /admin/support/faqs 入参。 */
export interface AdminSupportFaqCreateRequest {
  question: string
  answer: string
  tags?: string[]
  enabled?: boolean
  sort_order?: number
}

/** PUT /admin/support/faqs/:id 入参（部分更新；undefined 字段不修改）。 */
export interface AdminSupportFaqUpdateRequest {
  question?: string
  answer?: string
  tags?: string[]
  enabled?: boolean
  sort_order?: number
}

/** Reindex 响应 —— 与 dto.SupportFaqReindexResponse 对齐。 */
export interface AdminSupportFaqReindexResponse {
  succeeded: number
  failed: number
}

/** GET /admin/support/faqs 返回 `{ items, total }`。 */
export async function adminListFaqs(options?: {
  onlyEnabled?: boolean
  signal?: AbortSignal
}): Promise<AdminSupportFaqItem[]> {
  const params: Record<string, string> = {}
  if (options?.onlyEnabled) params.only_enabled = 'true'
  const { data } = await apiClient.get<{ items: AdminSupportFaqItem[]; total: number }>(
    '/admin/support/faqs',
    { params, signal: options?.signal }
  )
  return Array.isArray(data?.items) ? data.items : []
}

export async function adminGetFaq(id: number): Promise<AdminSupportFaqItem> {
  const { data } = await apiClient.get<AdminSupportFaqItem>(`/admin/support/faqs/${id}`)
  return data
}

export async function adminCreateFaq(
  payload: AdminSupportFaqCreateRequest
): Promise<AdminSupportFaqMutationResponse> {
  const { data } = await apiClient.post<AdminSupportFaqMutationResponse>(
    '/admin/support/faqs',
    payload
  )
  return data
}

export async function adminUpdateFaq(
  id: number,
  payload: AdminSupportFaqUpdateRequest
): Promise<AdminSupportFaqMutationResponse> {
  const { data } = await apiClient.put<AdminSupportFaqMutationResponse>(
    `/admin/support/faqs/${id}`,
    payload
  )
  return data
}

export async function adminDeleteFaq(id: number): Promise<{ id: number }> {
  const { data } = await apiClient.delete<{ id: number }>(`/admin/support/faqs/${id}`)
  return data
}

/**
 * 重新嵌入。`mode='all'` 全量重算；默认仅补 embedding=NULL 的行。
 */
export async function adminReindexFaqs(
  mode?: 'all' | 'missing'
): Promise<AdminSupportFaqReindexResponse> {
  const params: Record<string, string> = {}
  if (mode === 'all') params.mode = 'all'
  const { data } = await apiClient.post<AdminSupportFaqReindexResponse>(
    '/admin/support/faqs/reindex',
    null,
    { params }
  )
  return data
}

// ============================================================
// 文档索引管线
// ============================================================

/** 单条文档索引错误。 */
export interface AdminSupportDocIndexErrorEntry {
  url: string
  message: string
}

/** GET /admin/support/doc-index/status 响应 —— 与 dto.SupportDocIndexStatusResponse 对齐。 */
export interface AdminSupportDocIndexStatus {
  state: 'idle' | 'running' | 'completed' | 'failed' | string
  started_at: string
  last_run_at: string
  duration_seconds: number
  pages_visited: number
  pages_cap_hit: boolean
  chunks_total: number
  chunks_added: number
  chunks_removed: number
  chunks_failed_embed: number
  errors: AdminSupportDocIndexErrorEntry[]
}

/** POST /admin/support/doc-index/rebuild —— 异步触发，返回 202 + `{accepted: true}`。 */
export async function adminRebuildDocIndex(): Promise<{ accepted: boolean }> {
  const { data } = await apiClient.post<{ accepted: boolean }>(
    '/admin/support/doc-index/rebuild'
  )
  return data
}

export async function adminGetDocIndexStatus(options?: {
  signal?: AbortSignal
}): Promise<AdminSupportDocIndexStatus> {
  const { data } = await apiClient.get<AdminSupportDocIndexStatus>(
    '/admin/support/doc-index/status',
    { signal: options?.signal }
  )
  return data
}

export async function adminPurgeDocIndex(): Promise<{ deleted: number }> {
  const { data } = await apiClient.post<{ deleted: number }>(
    '/admin/support/doc-index/purge'
  )
  return data
}

// ============================================================
// Default export
// ============================================================

const adminSupportFaqApi = {
  adminListFaqs,
  adminGetFaq,
  adminCreateFaq,
  adminUpdateFaq,
  adminDeleteFaq,
  adminReindexFaqs,
  adminRebuildDocIndex,
  adminGetDocIndexStatus,
  adminPurgeDocIndex,
}

export default adminSupportFaqApi
