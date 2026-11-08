/**
 * Admin Intelligence Check API endpoints
 * 「智力检测」（鹈鹕测试）：按账号跑一次绘图题，记录作品原文；
 * 通过与否不再自动判定，由管理员人工评审（见 reviewRun）。
 */

import { apiClient } from '../client'
import type {
  CreateIntelligenceCheckRunRequest,
  IntelligenceCheckListParams,
  IntelligenceCheckRun,
  PaginatedResponse
} from '@/types'

/** 人工评审结论：pass = 通过，fail = 不通过。 */
export type IntelligenceCheckReviewVerdict = 'pass' | 'fail'

/**
 * 一次人工评审的结果。
 * verdict 是评审结论；account_status / status_synced 描述账号状态联动：
 * 联动由全局开关控制，开关关闭时 status_synced 为 false 且 account_status 为 null。
 */
export interface IntelligenceCheckReviewResult {
  id: number
  account_id: number
  verdict: string
  reviewed_by: number
  reviewed_at: string | null
  /** 联动后的账号状态（active / error），未联动时为 null。 */
  account_status: string | null
  /** 本次是否真的做了联动（取决于全局开关）。 */
  status_synced: boolean
}

/**
 * 分页查询跑测记录（按创建时间倒序）
 * @param params - 分页与过滤条件
 */
export async function listRuns(
  params: IntelligenceCheckListParams = {}
): Promise<PaginatedResponse<IntelligenceCheckRun>> {
  const { data } = await apiClient.get<PaginatedResponse<IntelligenceCheckRun>>(
    '/admin/intelligence-check/runs',
    { params }
  )
  return data
}

/**
 * 查询单条跑测记录
 * @param id - 跑测记录 ID
 */
export async function getRun(id: number): Promise<IntelligenceCheckRun> {
  const { data } = await apiClient.get<IntelligenceCheckRun>(`/admin/intelligence-check/runs/${id}`)
  return data
}

/**
 * 取作品原文。
 * 后端以 text/plain 返回且带禁止脚本的 CSP，这里必须显式声明 responseType 并禁用
 * axios 的默认 JSON 解析，否则 HTML 会被当成 JSON 解析失败。
 * @param id - 跑测记录 ID
 */
export async function getArtifact(id: number): Promise<string> {
  const { data } = await apiClient.get<string>(`/admin/intelligence-check/runs/${id}/artifact`, {
    responseType: 'text',
    transformResponse: [(value: string) => value]
  })
  return typeof data === 'string' ? data : ''
}

/**
 * 手动触发一次跑测（立即返回 queued 记录，跑测在后台执行）
 * @param payload - 触发请求
 */
export async function createRun(
  payload: CreateIntelligenceCheckRunRequest
): Promise<IntelligenceCheckRun> {
  const { data } = await apiClient.post<IntelligenceCheckRun>(
    '/admin/intelligence-check/runs',
    payload
  )
  return data
}

/**
 * 人工评审一次跑测（管理员专属，普通用户侧永不调用）。
 * 后端只接受「跑测成功且产出了作品」的记录，执行失败 / 正在跑测的记录会被拒绝。
 * @param runId - 跑测记录 ID
 * @param verdict - 评审结论
 */
export async function reviewRun(
  runId: number,
  verdict: IntelligenceCheckReviewVerdict
): Promise<IntelligenceCheckReviewResult> {
  const { data } = await apiClient.patch<IntelligenceCheckReviewResult>(
    `/admin/intelligence-check/runs/${runId}/review`,
    { verdict }
  )
  return data
}

export const intelligenceCheckAPI = {
  listRuns,
  getRun,
  getArtifact,
  createRun,
  reviewRun
}

export default intelligenceCheckAPI
