import { buildGatewayUrl } from './client'
import { keysAPI } from './keys'
import {
  VISION_DEFAULT_MODEL,
  VISION_EXTRACT_PROMPT,
  parseSellingPoints,
  type SellingPoints,
} from './aiImagePrompts'
import type { ApiKey } from '@/types'

/**
 * AI生图 走平台网关的图片接口，和桌面版「熊管家AI作图」调用的是同一组端点：
 *   - 纯文生图：POST /v1/images/generations
 *   - 带参考图：POST /v1/images/edits（multipart，字段名 image）
 * 两者都用用户在平台创建的密钥做 Bearer 鉴权。
 */

export const AI_IMAGE_DEFAULT_MODEL = 'gpt-image-2'
export const AI_IMAGE_DEFAULT_SIZE = '1200x1600'

/**
 * 生图模型下拉里提供的三个模型。
 *
 * 这三个就是平台分组「模型列表」里给生图配的那三个（主人 2026-10-15 提供）。
 * 做成固定下拉而不是让用户手打，是因为模型名打错一个字符，
 * 上游只会回一句「模型不存在」，用户根本看不出是自己打错了。
 *
 * 注意：这三个是**平台侧给生图分组配的**，不是 OpenAI 官方的全部生图模型。
 * 以后分组里加了新模型，这里要跟着加，否则用户选不到。
 */
export const AI_IMAGE_MODEL_OPTIONS = [
  'gpt-image-2',
  'gpt-image-2.5-sunburst',
  'gpt-image-2.5-flare',
] as const

/** 存过的模型名如果已经不在可选项里（分组改过、或旧版本残留），回退到默认。 */
export function normalizeAIImageModel(value: string): string {
  const name = (value || '').trim()
  return (AI_IMAGE_MODEL_OPTIONS as readonly string[]).includes(name) ? name : AI_IMAGE_DEFAULT_MODEL
}

export const AI_IMAGE_SIZE_OPTIONS = [
  { label: '1200 × 1600（3:4 竖版）', value: '1200x1600' },
  { label: '1024 × 1536（2:3 竖版）', value: '1024x1536' },
  { label: '1024 × 1024（1:1 方图）', value: '1024x1024' },
  { label: '1536 × 1024（3:2 横版）', value: '1536x1024' },
]

export interface AIImageGenerationResultImage {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface AIImageGenerationResponse {
  created?: number
  data: AIImageGenerationResultImage[]
}

/** 向网关要图片的返回形式：url 便于交给客户端下载，b64_json 兼容面更广。 */
export type AIImageResponseFormat = 'url' | 'b64_json'

export interface AIImageRequest {
  apiKey: string
  prompt: string
  model?: string
  size?: string
  n?: number
  /** 参考图；有值时走 /v1/images/edits */
  referenceImage?: File | null
}

export class AIImageError extends Error {
  /** 网关返回的 error.type / error.code，用于区分「不是生图模型」等场景。 */
  readonly code: string
  readonly status: number

  constructor(message: string, code: string, status: number) {
    super(message)
    this.name = 'AIImageError'
    this.code = code
    this.status = status
  }
}

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return { Authorization: `Bearer ${apiKey}`, ...extra }
}

async function parseError(response: Response): Promise<AIImageError> {
  let message = response.statusText || `HTTP ${response.status}`
  let code = String(response.status)
  try {
    const body = await response.json()
    const err = body?.error
    if (err && typeof err === 'object') {
      message = String(err.message || message)
      code = String(err.code || err.type || code)
    } else if (typeof body?.message === 'string') {
      message = body.message
    }
  } catch {
    // 非 JSON 响应体，保留状态码文案
  }
  return new AIImageError(message, code, response.status)
}

/** 判断一个错误是否属于「这个密钥不是生图模型」。 */
export function isImagePermissionError(error: unknown): boolean {
  if (!(error instanceof AIImageError)) return false
  if (error.status === 403) return true
  const code = error.code.toLowerCase()
  const message = error.message.toLowerCase()
  return (
    code.includes('permission') ||
    code.includes('forbidden') ||
    message.includes('image generation') ||
    message.includes('not allowed') ||
    message.includes('permission')
  )
}

/** 文生图：POST /v1/images/generations */
async function generateFromText(
  request: AIImageRequest,
  responseFormat: AIImageResponseFormat,
): Promise<AIImageGenerationResponse> {
  const response = await fetch(buildGatewayUrl('/v1/images/generations'), {
    method: 'POST',
    headers: authHeaders(request.apiKey, { 'Content-Type': 'application/json' }),
    body: JSON.stringify({
      model: request.model || AI_IMAGE_DEFAULT_MODEL,
      prompt: request.prompt,
      n: request.n ?? 1,
      size: request.size || AI_IMAGE_DEFAULT_SIZE,
      response_format: responseFormat,
    }),
  })
  if (!response.ok) throw await parseError(response)
  return (await response.json()) as AIImageGenerationResponse
}

/** 图生图 / 改图：POST /v1/images/edits（multipart） */
async function generateFromImage(
  request: AIImageRequest,
  responseFormat: AIImageResponseFormat,
): Promise<AIImageGenerationResponse> {
  const form = new FormData()
  form.append('model', request.model || AI_IMAGE_DEFAULT_MODEL)
  form.append('prompt', request.prompt)
  form.append('n', String(request.n ?? 1))
  form.append('size', request.size || AI_IMAGE_DEFAULT_SIZE)
  form.append('response_format', responseFormat)
  form.append('image', request.referenceImage as File, (request.referenceImage as File).name || 'image.png')

  const response = await fetch(buildGatewayUrl('/v1/images/edits'), {
    method: 'POST',
    headers: authHeaders(request.apiKey),
    body: form,
  })
  if (!response.ok) throw await parseError(response)
  return (await response.json()) as AIImageGenerationResponse
}

async function generateOnce(
  request: AIImageRequest,
  responseFormat: AIImageResponseFormat,
): Promise<AIImageGenerationResponse> {
  return request.referenceImage
    ? generateFromImage(request, responseFormat)
    : generateFromText(request, responseFormat)
}

/**
 * 要 URL 还是 base64 这件事，在网关里会影响选路：
 * 显式要 `url` 时，网关不做「url 转 base64 回填」，返回的地址可以被客户端直接下载——
 * 这正是「让客户端识别到图片」需要的东西。
 * 但代价是请求会被要求走 images-native 能力，如果这个分组的账号不具备该能力，
 * 网关会以「没有可用账号」拒绝。这种情况退一步要 base64，至少先把图生成出来。
 *
 * 注意（不要改）：这里刻意**不使用** `/v1/images/generations/async` 那套异步任务接口。
 * 异步任务会把结果转存到平台的对象存储，图片就落到我们服务器上了；
 * 而主人明确要求「图片不能在服务器那里，会增多成本」。
 * 所以图片一律由上游直接给地址、客户端自己去下载，平台不落地、不保存。
 */
function shouldFallbackToBase64(error: unknown): boolean {
  if (!(error instanceof AIImageError)) return false
  if (error.status !== 400 && error.status !== 404 && error.status !== 503) return false
  const text = `${error.code} ${error.message}`.toLowerCase()
  return (
    text.includes('no available') ||
    text.includes('no_available') ||
    text.includes('available account') ||
    text.includes('capabilit') ||
    text.includes('native')
  )
}

export async function generateAIImage(request: AIImageRequest): Promise<AIImageGenerationResponse> {
  try {
    return await generateOnce(request, 'url')
  } catch (error) {
    if (!shouldFallbackToBase64(error)) throw error
    return generateOnce(request, 'b64_json')
  }
}

/**
 * AI 识别卖点：把产品图丢给视觉模型，让它吐出产品名/人群/三个俄语卖点/痛点/差异化。
 * 走的是平台网关的 /v1/chat/completions，和桌面程序用的是同一个接口。
 */
export async function extractSellingPoints(request: {
  apiKey: string
  image: File
  model?: string
  /** 视觉接口地址，留空用当前站点的网关 */
  apiBase?: string
  /** 已经填过的产品名等资料，一并交给模型参考 */
  hints?: Record<string, string>
}): Promise<SellingPoints> {
  const base64 = await fileToBase64(request.image)
  const hintText = Object.entries(request.hints || {})
    .filter(([, value]) => String(value || '').trim())
    .map(([key, value]) => `${key}: ${value}`)
    .join('\n')

  const content: Array<Record<string, unknown>> = [
    { type: 'text', text: hintText ? `${VISION_EXTRACT_PROMPT}\n\n补充资料：\n${hintText}` : VISION_EXTRACT_PROMPT },
    { type: 'image_url', image_url: { url: base64 } },
  ]

  const url = request.apiBase?.trim()
    ? request.apiBase.trim()
    : buildGatewayUrl('/v1/chat/completions')

  const response = await fetch(url, {
    method: 'POST',
    headers: authHeaders(request.apiKey, { 'Content-Type': 'application/json' }),
    body: JSON.stringify({
      model: request.model || VISION_DEFAULT_MODEL,
      temperature: 0.3,
      messages: [{ role: 'user', content }],
    }),
  })
  if (!response.ok) throw await parseError(response)

  const body = (await response.json()) as {
    choices?: Array<{ message?: { content?: string } }>
  }
  const text = body?.choices?.[0]?.message?.content || ''
  const parsed = parseSellingPoints(text)
  if (!parsed) throw new AIImageError('AI 返回的内容无法解析', 'parse_error', 200)
  return parsed
}

/**
 * 拆解爆款版式：把同行的爆款主图丢给视觉模型，让它吐出 9 个版式维度。
 *
 * 和 extractSellingPoints 走同一个网关接口（/v1/chat/completions），
 * 只是提示词和温度不同（桌面程序里拆解用 0.2、识别卖点用 0.3）。
 * 返回**原始文本**，解析交给 aiImageHot.extractJsonObject，
 * 因为视觉模型经常在 JSON 前后夹带解释文字。
 */
export async function analyzeHotStyle(request: {
  apiKey: string
  image: File
  model?: string
  prompt: string
  temperature: number
  timeoutMs?: number
}): Promise<string> {
  const base64 = await fileToBase64(request.image)
  const url = buildGatewayUrl('/v1/chat/completions')

  // 浏览器 fetch 默认没有超时，视觉模型又慢，必须自己掐表，
  // 否则请求会一直挂着、用户以为页面卡死了。
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), request.timeoutMs ?? 90000)

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: authHeaders(request.apiKey, { 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        model: request.model || VISION_DEFAULT_MODEL,
        temperature: request.temperature,
        messages: [
          {
            role: 'user',
            content: [
              { type: 'text', text: request.prompt },
              { type: 'image_url', image_url: { url: base64 } },
            ],
          },
        ],
      }),
      signal: controller.signal,
    })
    if (!response.ok) throw await parseError(response)

    const body = (await response.json()) as {
      choices?: Array<{ message?: { content?: string } }>
    }
    return body?.choices?.[0]?.message?.content || ''
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new AIImageError('拆解超时（超过 90 秒）', 'timeout', 0)
    }
    throw error
  } finally {
    clearTimeout(timer)
  }
}

/** 把本地图片读成 data URL，供视觉接口使用。 */
export function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(new AIImageError('读取图片失败', 'read_error', 0))
    reader.readAsDataURL(file)
  })
}

/** 把返回项转成可直接放进 <img src> 的地址。 */
export function toDisplayUrl(image: AIImageGenerationResultImage): string {
  if (image.b64_json) return `data:image/png;base64,${image.b64_json}`
  return image.url || ''
}

/**
 * 判断一条图片地址是否值得写进 localStorage。
 * base64 的 data URL 动辄几 MB，写进 localStorage 会立刻撑爆配额，
 * 因此这类图片只保留在当前会话内存里。
 */
export function isPersistableImageUrl(value: string): boolean {
  return Boolean(value) && !value.startsWith('data:')
}

/** 把返回项转成可下载的 Blob。 */
export async function toBlob(image: AIImageGenerationResultImage): Promise<Blob> {
  if (image.b64_json) {
    const binary = atob(image.b64_json)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
    return new Blob([bytes], { type: 'image/png' })
  }
  const response = await fetch(image.url as string)
  if (!response.ok) throw new AIImageError('download failed', String(response.status), response.status)
  return response.blob()
}

export interface KeyValidationResult {
  ok: boolean
  /** 命中的密钥所属分组名，用于提示文案 */
  groupName?: string
  reason?: 'not-owned' | 'invalid' | 'not-image-model' | 'load-failed'
}

/**
 * 校验用户填入的密钥。
 * 平台自己的密钥列表接口会返回完整密钥与所属分组，因此这里不需要额外的后端接口：
 *   1. 在「我的密钥」里找到这条密钥 → 找不到说明不是本账号创建的，或密钥无效；
 *   2. 看所属分组是否开启生图能力 → 没开就提示「这个密钥不是生图模型」。
 */
export async function validateAIImageKey(rawKey: string): Promise<KeyValidationResult> {
  const key = rawKey.trim()
  if (!key) return { ok: false, reason: 'invalid' }

  let items: ApiKey[] = []
  try {
    const pageSize = 100
    let page = 1
    while (true) {
      const response = await keysAPI.list(page, pageSize)
      items = items.concat(response.items || [])
      if (page >= (response.pages || 1) || (response.items || []).length === 0) break
      page += 1
      if (page > 20) break
    }
  } catch {
    return { ok: false, reason: 'load-failed' }
  }

  const matched = items.find((item) => item.key === key)
  if (!matched) return { ok: false, reason: 'not-owned' }
  if (matched.status && matched.status !== 'active') {
    return { ok: false, reason: 'invalid', groupName: matched.group?.name }
  }
  if (!matched.group?.allow_image_generation) {
    return { ok: false, reason: 'not-image-model', groupName: matched.group?.name }
  }
  return { ok: true, groupName: matched.group?.name }
}
