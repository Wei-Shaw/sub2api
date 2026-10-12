import { apiClient } from './client'

export interface VideoPlazaTier {
  label: string
  price: number
}

export interface VideoPlazaOffer {
  group_id: number
  group_name: string
  rate_multiplier: number
  billing_mode: 'video' | 'per_request'
  tiers?: VideoPlazaTier[]
  default_price?: number
}

export interface VideoPlazaModel {
  name: string
  platform: string
  offers: VideoPlazaOffer[]
}

export interface VideoPlazaGroup {
  id: number
  name: string
  description: string
  platform: string
  subscription_type: string
  rate_multiplier: number
  user_rate_multiplier?: number
  is_exclusive: boolean
  models: VideoPlazaModel[]
}

export interface VideoModelPlazaResponse {
  groups: VideoPlazaGroup[]
}

export async function getVideoModelPlaza(options?: { signal?: AbortSignal }): Promise<VideoModelPlazaResponse> {
  const { data } = await apiClient.get<VideoModelPlazaResponse>('/video-model-plaza', {
    signal: options?.signal
  })
  return data
}

export const videoModelPlazaAPI = { getVideoModelPlaza }

export default videoModelPlazaAPI
