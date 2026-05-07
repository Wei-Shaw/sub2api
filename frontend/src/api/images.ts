import { apiClient } from './client'

export interface ImageGenerationRequest {
  model: string
  prompt: string
  size: string
  quality: string
  n?: number
  response_format?: 'url' | 'b64_json'
}

export interface ImageEditRequest extends ImageGenerationRequest {
  images: File[]
}

export interface GeneratedImage {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface ImageGenerationResponse {
  created?: number
  data: GeneratedImage[]
}

const IMAGE_REQUEST_TIMEOUT_MS = 0

function authHeaders(apiKey: string) {
  return apiKey ? { Authorization: `Bearer ${apiKey}` } : undefined
}

export async function generateImage(payload: ImageGenerationRequest, apiKey = ''): Promise<ImageGenerationResponse> {
  const { data } = await apiClient.post<ImageGenerationResponse>('/images/generations', payload, {
    headers: authHeaders(apiKey),
    timeout: IMAGE_REQUEST_TIMEOUT_MS,
  })
  return data
}

export async function editImage(payload: ImageEditRequest, apiKey = ''): Promise<ImageGenerationResponse> {
  const form = new FormData()
  form.append('model', payload.model)
  form.append('prompt', payload.prompt)
  form.append('size', payload.size)
  form.append('quality', payload.quality)
  form.append('response_format', payload.response_format || 'b64_json')
  form.append('n', String(payload.n ?? 1))
  for (const image of payload.images) {
    form.append('image', image)
  }

  const { data } = await apiClient.post<ImageGenerationResponse>('/images/edits', form, {
    headers: authHeaders(apiKey),
    timeout: IMAGE_REQUEST_TIMEOUT_MS,
  })
  return data
}
