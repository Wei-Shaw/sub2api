import { keysAPI } from './keys'
import type { ApiKey } from '@/types'

export interface ImageGenerationKeyOption {
  id: number
  name: string
  key: string
  maskedKey: string
  groupName: string
  groupId: number | null
}

export interface GenerateImageRequest {
  apiKey: string
  prompt: string
  size: string
  quality: 'auto' | 'low' | 'medium' | 'high'
  count: number
  referenceImage?: File | null
}

export interface GeneratedImage {
  url: string
  revisedPrompt?: string
}

export interface ImageGenerationResponse {
  created?: number
  model?: string
  size?: string
  quality?: string
  data?: Array<{
    b64_json?: string
    url?: string
    revised_prompt?: string
  }>
}

function isAvailableApiKey(key: ApiKey): boolean {
  return (
    key.status === 'active' &&
    typeof key.key === 'string' &&
    key.key.trim() !== ''
  )
}

function maskApiKey(key: string): string {
  const normalized = key.trim()
  if (normalized.length <= 12) return normalized
  return `${normalized.slice(0, 7)}...${normalized.slice(-4)}`
}

export async function listImageGenerationKeys(): Promise<ImageGenerationKeyOption[]> {
  const result = await keysAPI.list(1, 100, {
    status: 'active',
    sort_by: 'created_at',
    sort_order: 'desc'
  })

  return result.items
    .filter(isAvailableApiKey)
    .map((item) => ({
      id: item.id,
      name: item.name,
      key: item.key,
      maskedKey: maskApiKey(item.key),
      groupName: item.group?.name || '',
      groupId: item.group_id
    }))
}

export async function generateImage(request: GenerateImageRequest): Promise<GeneratedImage[]> {
  const referenceImage = request.referenceImage ?? null
  const endpoint = referenceImage ? '/v1/images/edits' : '/v1/images/generations'

  const init: RequestInit = {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${request.apiKey}`
    }
  }

  if (referenceImage) {
    const form = new FormData()
    form.append('model', 'gpt-image-2')
    form.append('prompt', request.prompt)
    form.append('size', request.size)
    form.append('quality', request.quality)
    form.append('n', String(request.count))
    form.append('response_format', 'b64_json')
    form.append('image', referenceImage, referenceImage.name || 'reference-image.png')
    init.body = form
  } else {
    init.headers = {
      ...init.headers,
      'Content-Type': 'application/json'
    }
    init.body = JSON.stringify({
      model: 'gpt-image-2',
      prompt: request.prompt,
      size: request.size,
      quality: request.quality,
      n: request.count,
      response_format: 'b64_json'
    })
  }

  const response = await fetch(endpoint, init)
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(extractImagesErrorMessage(payload, response.statusText))
  }

  const body = payload as ImageGenerationResponse
  const images = (body.data || [])
    .map((item): GeneratedImage | null => {
      const url = item.b64_json
        ? `data:image/png;base64,${item.b64_json}`
        : item.url || ''
      if (!url) return null
      return {
        url,
        revisedPrompt: item.revised_prompt
      }
    })
    .filter((item): item is GeneratedImage => item !== null)

  if (images.length === 0) {
    throw new Error('No images returned from API')
  }

  return images
}

async function readJSON(response: Response): Promise<unknown> {
  const text = await response.text()
  if (!text) return {}
  try {
    return JSON.parse(text)
  } catch {
    return { message: text }
  }
}

function extractImagesErrorMessage(payload: unknown, fallback: string): string {
  if (payload && typeof payload === 'object') {
    const data = payload as Record<string, any>
    const nested = data.error
    if (nested && typeof nested === 'object') {
      if (typeof nested.message === 'string') return nested.message
      if (typeof nested.error === 'string') return nested.error
    }
    if (typeof data.message === 'string') return data.message
    if (typeof data.error === 'string') return data.error
    if (typeof data.detail === 'string') return data.detail
  }
  return fallback || 'Image generation failed'
}

export const imageGenerationAPI = {
  listImageGenerationKeys,
  generateImage
}

export default imageGenerationAPI
