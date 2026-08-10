import { apiClient } from './client'

export type ImagePlaygroundTaskStatus = 'processing' | 'completed' | 'failed'

export interface ImagePlaygroundSizeConstraints {
  max_edge: number
  multiple_of: number
  max_aspect_ratio: number
  min_pixels: number
  max_pixels: number
}

export interface ImagePlaygroundModelOption {
  id: string
  sizes: string[]
  custom_size_constraints?: ImagePlaygroundSizeConstraints
  qualities: string[]
  max_images: number
  output_formats: string[]
  backgrounds: string[]
  output_compression: boolean
  supports_image_input: boolean
  max_input_images?: number
  max_input_image_bytes?: number
  input_image_formats?: string[]
  experimental_above_pixels?: number
}

export interface ImagePlaygroundGroupOption {
  id: number
  name: string
  platform: string
  subscription_type: string
  available: boolean
  unavailable_reason?: string
  models: ImagePlaygroundModelOption[]
}

export interface ImagePlaygroundOptions {
  enabled: boolean
  retention_hours: number
  groups: ImagePlaygroundGroupOption[]
}

export interface ImagePlaygroundSubmitRequest {
  group_id: number
  model: string
  prompt: string
  size?: string
  quality?: string
  n?: number
  output_format?: string
  background?: string
}

export interface ImagePlaygroundImage {
	id?: string
	index: number
  url: string
  download_url: string
}

export interface ImagePlaygroundTask {
  id: string
  object: string
  status: ImagePlaygroundTaskStatus
  group_id: number
  platform: string
  model: string
  prompt_preview?: string
  input_image_count?: number
  images: ImagePlaygroundImage[]
  error?: unknown
  created_at: number
  completed_at?: number
  expires_at?: number
  poll_url: string
}

export interface AdminImagePlaygroundTask {
  task: ImagePlaygroundTask
  user_id: number
  api_key_id: number
  user_email?: string
  username?: string
  storage_bytes: number
  image_sizes: number[]
}

export interface AdminImagePlaygroundPage {
  tasks: AdminImagePlaygroundTask[]
  page: number
  page_size: number
  total: number
  total_images: number
  storage_bytes: number
}

export async function getImagePlaygroundOptions(): Promise<ImagePlaygroundOptions> {
  const response = await apiClient.get<ImagePlaygroundOptions>('/image-playground/options')
	return response.data
}

export async function submitImagePlaygroundTask(
  payload: ImagePlaygroundSubmitRequest,
  referenceImages: File[] = [],
): Promise<ImagePlaygroundTask> {
  if (referenceImages.length === 0) {
    const response = await apiClient.post<ImagePlaygroundTask>('/image-playground/tasks', payload)
    return response.data
  }

  const form = new FormData()
  Object.entries(payload).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      form.append(key, String(value))
    }
  })
  referenceImages.forEach((file) => form.append('images', file, file.name))
  const response = await apiClient.post<ImagePlaygroundTask>('/image-playground/tasks', form, {
    timeout: 60000,
  })
  return response.data
}

export async function getImagePlaygroundTask(taskId: string): Promise<ImagePlaygroundTask> {
  const response = await apiClient.get<ImagePlaygroundTask>(
    `/image-playground/tasks/${encodeURIComponent(taskId)}`,
  )
  return response.data
}

export async function listImagePlaygroundTasks(): Promise<ImagePlaygroundTask[]> {
  const response = await apiClient.get<ImagePlaygroundTask[]>('/image-playground/tasks')
  return response.data
}

export async function downloadImagePlaygroundImage(
  taskId: string,
  imageRef: string | number,
): Promise<Blob> {
  const response = await apiClient.get<Blob>(
    `/image-playground/tasks/${encodeURIComponent(taskId)}/images/${encodeURIComponent(String(imageRef))}/download`,
    { responseType: 'blob', timeout: 60000 },
  )
  return response.data
}

export async function getImagePlaygroundImagePreview(
  taskId: string,
  imageRef: string | number,
): Promise<Blob> {
  const response = await apiClient.get<Blob>(
    `/image-playground/tasks/${encodeURIComponent(taskId)}/images/${encodeURIComponent(String(imageRef))}`,
    { responseType: 'blob', timeout: 60000 },
  )
  return response.data
}

export async function deleteImagePlaygroundTask(taskId: string): Promise<void> {
  await apiClient.delete(`/image-playground/tasks/${encodeURIComponent(taskId)}`)
}

export async function deleteImagePlaygroundImage(taskId: string, imageRef: string | number): Promise<ImagePlaygroundTask | null> {
  const response = await apiClient.delete<ImagePlaygroundTask>(
    `/image-playground/tasks/${encodeURIComponent(taskId)}/images/${encodeURIComponent(String(imageRef))}`,
  )
  return response.status === 204 ? null : response.data
}

export async function listAdminImagePlaygroundTasks(page = 1, pageSize = 24): Promise<AdminImagePlaygroundPage> {
  const response = await apiClient.get<AdminImagePlaygroundPage>('/admin/image-playground/tasks', {
    params: { page, page_size: pageSize },
  })
  return response.data
}

export async function getAdminImagePlaygroundPreview(taskId: string, imageRef: string | number): Promise<Blob> {
  const response = await apiClient.get<Blob>(
    `/admin/image-playground/tasks/${encodeURIComponent(taskId)}/images/${encodeURIComponent(String(imageRef))}`,
    { responseType: 'blob', timeout: 60000 },
  )
  return response.data
}

export async function deleteAdminImagePlaygroundTask(taskId: string): Promise<void> {
  await apiClient.delete(`/admin/image-playground/tasks/${encodeURIComponent(taskId)}`)
}

export async function deleteAdminImagePlaygroundImage(taskId: string, imageRef: string | number): Promise<ImagePlaygroundTask | null> {
  const response = await apiClient.delete<ImagePlaygroundTask>(
    `/admin/image-playground/tasks/${encodeURIComponent(taskId)}/images/${encodeURIComponent(String(imageRef))}`,
  )
  return response.status === 204 ? null : response.data
}
