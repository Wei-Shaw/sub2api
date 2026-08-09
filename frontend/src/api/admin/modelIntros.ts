/**
 * Admin Model Intros API endpoints
 *
 * 模型介绍：为每个对外模型名（如 "gpt-4o"、"bytedance/seedance-2.5/text-to-video"）
 * 配置展示标题、纯文本描述、默认参数（JSON 对象）与封面图 URL。
 * model_key 可含 "/"，因此走后端 wildcard `*model_key`，前端只需拼在 URL 尾部即可。
 */

import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'

/**
 * VideoParamSpec：字段元信息。适用于新格式的 default_params，管理员可为
 * 每个字段声明 required / description / enum / options。
 *
 * 存储形式（前后端一致）：
 *   default_params[key] = {
 *     value:       默认值（string | number | boolean | object | null）
 *     required?:   是否必填（仅影响前端表单校验）
 *     description?: 字段描述
 *     enum?:       是否为枚举字段
 *     options?:    枚举选项列表（仅 enum=true 时有意义）
 *   }
 *
 * 旧格式（default_params[key] 直接为原始值）仍允许存在，但前端不
 * 解释为字段声明、也不展示为表单字段（退化为纯 JSON 模式）。
 */
export interface VideoParamSpec {
  value?: unknown
  required?: boolean
  description?: string
  enum?: boolean
  options?: unknown[]
}

/**
 * OutputFieldType：输出字段的渲染类型，遵循 JSON Schema 标准。
 *   - string / number / boolean：叶子标量；演练台按对应类型的文本形式渲染。
 *   - object：结构体；演练台以预格式化 JSON 展示。
 *   - array：数组；演练台以预格式化 JSON 展示。
 *
 * 主结果字段的媒体渲染（<video> / <img>）由 ModelIntro.result_type 单独指定，
 * 不再通过 OutputFieldType 表达。
 */
export type OutputFieldType = 'string' | 'number' | 'boolean' | 'object' | 'array'

/**
 * OutputFieldSpec：管理员声明的输出字段。
 *
 * key 是 fal 原生 result payload 中的字段路径，支持：
 *   - "video.url"     属性链
 *   - "images[0].url" 数组下标
 *   - "images[*]"     数组通配（返回全部元素）
 *   - "images"        直接指向数组或对象
 *   - "seed"          标量字段
 *
 * 演练台会遍历该数组按声明顺序渲染；"主结果字段"不再由 primary 控制，
 * 而是由 ModelIntro.result_field 统一指示。
 *
 * 为让"输出参数"和"输入参数"填写方式对齐，额外扩展三个可选字段：
 *   - required?：字段是否必然存在（语义/文档提示，前端只用于展示徽章）
 *   - enum?：字段是否枚举
 *   - options?：与 enum 配套的候选值列表；enum=false 时应缺省
 *
 * 为让"object / array 类型能保存嵌套子字段"，额外扩展两个可选字段：
 *   - properties?：type='object' 时使用；键为子字段名，值为一份递归 schema
 *                  （形状与 default_params 里 object schema 完全一致，可直接
 *                   走 rowToSchema/schemaToRow 递归序列化）。
 *   - items?：type='array' 时使用；值为一份递归 schema（数组同构）。
 * 其它 type 下两字段应保持缺省。
 *
 * label / default 已从编辑器中移除，仅为向后兼容旧数据保留在类型中（可选）。
 */
export interface OutputFieldSpec {
  key: string
  label?: string
  type: OutputFieldType
  description: string
  default?: string
  required?: boolean
  enum?: boolean
  options?: unknown[]
  properties?: Record<string, unknown>
  items?: unknown
}

/**
 * ResultMediaType：主结果字段的媒体渲染类型。只取 video / image 两个值，前端用于
 * 选择 <video> 还是 <img> 来大尺寸展示 result_field 指向的字段。
 */
export type ResultMediaType = 'video' | 'image'

export interface ModelIntro {
  model_key: string
  title: string
  description: string
  /**
   * description_en：模型介绍的英文版。后端新增的双文支持字段；
   * 为空时展示层会自动回落到 description，旧记录不需回填即可兼容。
   */
  description_en: string
  cover_url: string
  default_params: Record<string, unknown>
  sort_order: number
  enabled: boolean
  output_fields: OutputFieldSpec[]
  result_field: string
  result_type: ResultMediaType
  created_at: string
  updated_at: string
}

export interface ModelIntroCandidate {
  model_key: string
  account_count: number
}

export interface ModelIntroCandidateListResponse {
  items: ModelIntroCandidate[]
  total: number
}

export interface UpsertModelIntroRequest {
  model_key: string
  title: string
  description: string
  /**
   * description_en：英文版模型介绍。允许为空字符串；与 description 共同组成中英双文。
   */
  description_en: string
  cover_url: string
  default_params: Record<string, unknown>
  sort_order: number
  enabled: boolean
  output_fields: OutputFieldSpec[]
  result_field: string
  result_type: ResultMediaType
}

/**
 * 把 model_key 转成可放入 URL path 的分段：保留 "/"，其他字符转义。
 * 例："bytedance/seedance-2.5/text-to-video" -> "bytedance/seedance-2.5/text-to-video"
 * 例："model with space" -> "model%20with%20space"
 */
function encodeKeyForPath(key: string): string {
  return key
    .split('/')
    .map((seg) => encodeURIComponent(seg))
    .join('/')
}

export async function list(
  page: number = 1,
  pageSize: number = 20,
  keyword: string = '',
  options?: { signal?: AbortSignal }
): Promise<BasePaginationResponse<ModelIntro>> {
  const params: Record<string, unknown> = { page, page_size: pageSize }
  if (keyword) params.keyword = keyword
  const { data } = await apiClient.get<BasePaginationResponse<ModelIntro>>(
    '/admin/model-intros',
    { params, signal: options?.signal }
  )
  return data
}

export async function listCandidates(options?: {
  signal?: AbortSignal
}): Promise<ModelIntroCandidateListResponse> {
  const { data } = await apiClient.get<ModelIntroCandidateListResponse>(
    '/admin/model-intro-candidates',
    { signal: options?.signal }
  )
  return data
}

export async function getByKey(modelKey: string): Promise<ModelIntro> {
  const { data } = await apiClient.get<ModelIntro>(
    `/admin/model-intros/${encodeKeyForPath(modelKey)}`
  )
  return data
}

export async function create(request: UpsertModelIntroRequest): Promise<ModelIntro> {
  const { data } = await apiClient.post<ModelIntro>('/admin/model-intros', request)
  return data
}

export async function update(
  modelKey: string,
  request: UpsertModelIntroRequest
): Promise<ModelIntro> {
  const { data } = await apiClient.put<ModelIntro>(
    `/admin/model-intros/${encodeKeyForPath(modelKey)}`,
    request
  )
  return data
}

export async function deleteByKey(modelKey: string): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/model-intros/${encodeKeyForPath(modelKey)}`
  )
  return data
}

const modelIntrosAPI = {
  list,
  getByKey,
  create,
  update,
  delete: deleteByKey,
  listCandidates
}

export default modelIntrosAPI
