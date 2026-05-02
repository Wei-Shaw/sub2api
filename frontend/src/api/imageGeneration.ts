export type ImageGenerationMode = 'text' | 'image'

export interface ImageGenerationRequest {
  apiKey: string
  mode: ImageGenerationMode
  prompt: string
  model: string
  size: string
  resolution: string
  imageUrls?: string[]
}

export interface GeneratedImage {
  url: string
  mimeType: string
  revisedPrompt?: string
  taskId?: string
}

interface GPTImageSubmitItem {
  status?: string
  task_id?: string
  id?: string
  url?: string
  b64_json?: string
  revised_prompt?: string
}

interface GPTImageSubmitResponse {
  code?: number
  data?: GPTImageSubmitItem[]
  error?: {
    message?: string
    code?: string
    type?: string
  }
  message?: string
}

interface GPTImageTaskImage {
  url?: string | string[]
}

interface GPTImageTaskResponse {
  code?: number
  data?: {
    id?: string
    status?: string
    progress?: number
    result?: {
      images?: GPTImageTaskImage[]
    }
    error?: {
      message?: string
    }
  }
  error?: {
    message?: string
    code?: string
    type?: string
  }
  message?: string
}

const GPT_IMAGE_BASE_PATH = '/gpt-image/v1'
const TASK_POLL_INTERVAL_MS = 5000
const TASK_TIMEOUT_MS = 180000

export async function generateImages(request: ImageGenerationRequest): Promise<GeneratedImage[]> {
  let submitPayload: GPTImageSubmitResponse
  try {
    submitPayload = await submitImageTask(request)
  } catch (err: unknown) {
    if (isRouteUnavailableError(err)) {
      return generateImagesWithOpenAICompat(request)
    }
    throw err
  }

  const immediateImages = extractImmediateImages(submitPayload)
  if (immediateImages.length > 0) return immediateImages

  const taskId = extractTaskId(submitPayload)
  if (!taskId) {
    throw new Error(extractPayloadError(submitPayload) || 'Image task was not submitted')
  }

  return pollImageTask(request.apiKey, taskId)
}

async function submitImageTask(request: ImageGenerationRequest): Promise<GPTImageSubmitResponse> {
  const body: Record<string, unknown> = {
    model: request.model || 'gpt-image-2',
    prompt: request.prompt,
    n: 1,
    size: request.size,
    resolution: request.resolution
  }

  if (request.mode === 'image' && request.imageUrls?.length) {
    body.image_urls = request.imageUrls
  }

  const response = await fetch(`${GPT_IMAGE_BASE_PATH}/images/generations`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${request.apiKey}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(body)
  })
  return parseResponse<GPTImageSubmitResponse>(response)
}

async function generateImagesWithOpenAICompat(request: ImageGenerationRequest): Promise<GeneratedImage[]> {
  const body: Record<string, unknown> = {
    model: request.model || 'gpt-image-2',
    prompt: request.prompt,
    n: 1,
    size: openAICompatSize(request.size, request.resolution),
    response_format: 'b64_json'
  }

  const endpoint = request.mode === 'image' ? '/v1/images/edits' : '/v1/images/generations'
  if (request.mode === 'image') {
    body.images = (request.imageUrls || []).map((url) => ({ image_url: url }))
  }

  const response = await fetch(endpoint, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${request.apiKey}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(body)
  })
  const payload = await parseResponse<GPTImageSubmitResponse>(response)
  const images = extractImmediateImages(payload)
  if (images.length === 0) {
    throw new Error(extractPayloadError(payload) || 'No image returned')
  }
  return images
}

async function pollImageTask(apiKey: string, taskId: string): Promise<GeneratedImage[]> {
  const startedAt = Date.now()

  while (Date.now() - startedAt < TASK_TIMEOUT_MS) {
    await sleep(TASK_POLL_INTERVAL_MS)

    const response = await fetch(`${GPT_IMAGE_BASE_PATH}/tasks/${encodeURIComponent(taskId)}`, {
      headers: {
        Authorization: `Bearer ${apiKey}`
      }
    })
    const payload = await parseResponse<GPTImageTaskResponse>(response)
    const status = (payload.data?.status || '').toLowerCase()

    if (status === 'completed') {
      const images = extractTaskImages(payload, taskId)
      if (images.length === 0) throw new Error('Image task completed without images')
      return images
    }

    if (status === 'failed' || status === 'cancelled' || status === 'canceled') {
      throw new Error(extractPayloadError(payload) || 'Image task failed')
    }
  }

  throw new Error('Image task timed out')
}

async function parseResponse<T extends { error?: { message?: string }; message?: string }>(response: Response): Promise<T> {
  const text = await response.text()
  let payload: T | null = null
  if (text.trim()) {
    try {
      payload = JSON.parse(text) as T
    } catch {
      payload = null
    }
  }

  if (!response.ok) {
    throw createHTTPError(response.status, extractPayloadError(payload) || text || `HTTP ${response.status}`)
  }

  if (!payload) {
    throw new Error('Empty image API response')
  }

  const payloadCode = (payload as { code?: unknown }).code
  const code = typeof payloadCode === 'number' ? payloadCode : 200
  if (code >= 400) {
    throw new Error(extractPayloadError(payload) || `Image API error ${code}`)
  }

  return payload
}

function createHTTPError(status: number, message: string): Error & { status?: number } {
  const error = new Error(message) as Error & { status?: number }
  error.status = status
  return error
}

function isRouteUnavailableError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  const status = (err as { status?: number }).status
  return status === 404 || status === 405
}

function extractTaskId(payload: GPTImageSubmitResponse): string {
  const first = payload.data?.[0]
  return first?.task_id || first?.id || ''
}

function extractImmediateImages(payload: GPTImageSubmitResponse): GeneratedImage[] {
  return (payload.data || [])
    .map((item): GeneratedImage | null => {
      if (item.url) {
        return {
          url: item.url,
          mimeType: mimeTypeFromImageURL(item.url),
          revisedPrompt: item.revised_prompt
        }
      }
      if (item.b64_json) {
        return {
          url: `data:image/png;base64,${item.b64_json}`,
          mimeType: 'image/png',
          revisedPrompt: item.revised_prompt
        }
      }
      return null
    })
    .filter((item): item is GeneratedImage => item !== null)
}

function extractTaskImages(payload: GPTImageTaskResponse, taskId: string): GeneratedImage[] {
  const images = payload.data?.result?.images || []
  const urls = images.flatMap((image) => {
    if (Array.isArray(image.url)) return image.url
    if (typeof image.url === 'string') return [image.url]
    return []
  })

  return urls
    .filter((url) => url.trim() !== '')
    .map((url) => ({
      url,
      mimeType: mimeTypeFromImageURL(url),
      taskId
    }))
}

function extractPayloadError(payload: unknown): string {
  if (!payload || typeof payload !== 'object') return ''
  const source = payload as {
    error?: { message?: string }
    message?: string
    data?: { error?: { message?: string } }
  }
  return source.error?.message || source.data?.error?.message || source.message || ''
}

function openAICompatSize(size: string, resolution: string): string {
  const normalizedSize = size.trim().toLowerCase()
  if (!normalizedSize || normalizedSize === 'auto') return 'auto'

  const widerSize = resolution === '1k' ? '1536x1024' : '1792x1024'
  const tallerSize = resolution === '1k' ? '1024x1536' : '1024x1792'

  switch (normalizedSize) {
    case '1:1':
      return '1024x1024'
    case '3:2':
    case '4:3':
    case '5:4':
      return '1536x1024'
    case '2:3':
    case '3:4':
    case '4:5':
      return '1024x1536'
    case '16:9':
    case '2:1':
    case '21:9':
      return widerSize
    case '9:16':
    case '1:2':
    case '9:21':
      return tallerSize
    default:
      return '1024x1024'
  }
}

function mimeTypeFromImageURL(value: string): string {
  const dataURLMatch = /^data:([^;,]+)[;,]/i.exec(value)
  if (dataURLMatch?.[1]) return dataURLMatch[1]

  const pathname = safeURLPathname(value).toLowerCase()
  if (pathname.endsWith('.jpg') || pathname.endsWith('.jpeg')) return 'image/jpeg'
  if (pathname.endsWith('.webp')) return 'image/webp'
  if (pathname.endsWith('.gif')) return 'image/gif'
  return 'image/png'
}

function safeURLPathname(value: string): string {
  try {
    return new URL(value).pathname
  } catch {
    return value.split('?')[0] || ''
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

export const imageGenerationAPI = {
  generateImages
}
