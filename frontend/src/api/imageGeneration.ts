import axios from 'axios'
import { apiClient } from './client'

export interface ImageGenerationModel {
  id: string
  name: string
  supports_generation: boolean
  supports_edit: boolean
}

export interface ImageGenerationRequest {
  model: string
  prompt: string
  size?: string
  quality?: string
  n?: number
}

export interface ImageEditRequest {
  model: string
  prompt: string
  image: File
  size?: string
  quality?: string
  n?: number
}

export interface OpenAIImageItem {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface OpenAIImageResponse {
  created?: number
  data?: OpenAIImageItem[]
  output?: Array<{
    type?: string
    result?: string
    image_url?: string
    url?: string
  }>
}

const gatewayClient = axios.create({
  baseURL: '',
  timeout: 180000
})

export async function listModels(apiKeyId: number): Promise<ImageGenerationModel[]> {
  const { data } = await apiClient.get<ImageGenerationModel[]>('/images/models', {
    params: { api_key_id: apiKeyId }
  })
  return data
}

export async function generateImage(apiKey: string, payload: ImageGenerationRequest): Promise<OpenAIImageResponse> {
  const { data } = await gatewayClient.post<OpenAIImageResponse>('/v1/images/generations', payload, {
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json'
    }
  })
  return data
}

export async function editImage(apiKey: string, payload: ImageEditRequest): Promise<OpenAIImageResponse> {
  const form = new FormData()
  form.append('model', payload.model)
  form.append('prompt', payload.prompt)
  form.append('image', payload.image)
  if (payload.size) form.append('size', payload.size)
  if (payload.quality) form.append('quality', payload.quality)
  if (payload.n) form.append('n', String(payload.n))

  const { data } = await gatewayClient.post<OpenAIImageResponse>('/v1/images/edits', form, {
    headers: {
      Authorization: `Bearer ${apiKey}`
    }
  })
  return data
}

export const imageGenerationAPI = {
  listModels,
  generateImage,
  editImage
}

export default imageGenerationAPI
