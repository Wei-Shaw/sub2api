/**
 * Admin Batches API endpoints
 * Handles batch (批次标签) management for account classification
 */

import { apiClient } from '../client'

export interface Batch {
  id: number
  name: string
  description: string
  source: string
  account_count: number
  created_at: string
  updated_at: string
}

export interface CreateBatchRequest {
  name: string
  description?: string
  source?: string
}

export interface UpdateBatchRequest {
  name?: string
  description?: string
}

/**
 * List all batches
 */
export async function list(options?: { signal?: AbortSignal }): Promise<Batch[]> {
  const { data } = await apiClient.get<Batch[]>('/admin/batches', {
    signal: options?.signal,
  })
  return data
}

/**
 * Create a new batch
 */
export async function create(
  request: CreateBatchRequest,
  options?: { signal?: AbortSignal }
): Promise<{ data: Batch }> {
  const { data } = await apiClient.post<{ data: Batch }>('/admin/batches', request, {
    signal: options?.signal,
  })
  return data
}

/**
 * Update a batch
 */
export async function update(
  id: number,
  request: UpdateBatchRequest,
  options?: { signal?: AbortSignal }
): Promise<{ data: Batch }> {
  const { data } = await apiClient.put<{ data: Batch }>(`/admin/batches/${id}`, request, {
    signal: options?.signal,
  })
  return data
}

/**
 * Delete a batch
 */
export async function remove(id: number, options?: { signal?: AbortSignal }): Promise<void> {
  await apiClient.delete(`/admin/batches/${id}`, {
    signal: options?.signal,
  })
}
