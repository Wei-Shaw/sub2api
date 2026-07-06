import { buildGatewayUrl REDACTED from './client'

export type BatchImageStatus =
  | 'queued'
  | 'running'
  | 'indexing'
  | 'processing_results'
  | 'settling'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'output_deleted'
  | string

export interface BatchImageSubmitItem {
  custom_id: string
  prompt: string
REDACTED

export interface BatchImageSubmitRequest {
  model: string
  task_name?: string
  parent_batch_id?: string
  provider?: '' | 'gemini_api' | 'vertex' | string
  image_size?: '1K' | '2K' | '4K' | string
  response_mime_type?: string
  aspect_ratio?: string
  items: BatchImageSubmitItem[]
  metadata?: Record<string, string>
REDACTED

export interface BatchImageJob {
  id: string
  object: string
  task_name: string
  parent_batch_id?: string | null
  status: BatchImageStatus
  model: string
  provider: string
  item_count: number
  success_count: number
  fail_count: number
  estimated_cost: number
  hold_amount: number
  actual_cost: number | null
  created_at: number
  submitted_at: number | null
  settled_at: number | null
  downloaded_at?: number | null
  output_deleted_at?: number | null
REDACTED

export interface BatchImageItem {
  batch_id?: string
  source_task_name?: string
  custom_id: string
  status: string
  prompt_preview?: string | null
  mime_type: string | null
  file_extension: string | null
  image_count: number
  error?: {
    code: string
    message: string
    source?: 'provider' | 'system' | string
  REDACTED | null
REDACTED

export interface BatchImageItemsResponse {
  object: string
  data: BatchImageItem[]
  has_more: boolean
REDACTED

export interface BatchImageJobsResponse {
  object: string
  data: BatchImageJob[]
  has_more: boolean
REDACTED

export interface BatchImageModel {
  id: string
  object: string
  provider: string
REDACTED

export interface BatchImageModelsResponse {
  object: string
  data: BatchImageModel[]
REDACTED

export interface BatchImageJobsListOptions {
  limit?: number
  cursor?: string
  status?: string
  taskName?: string
  downloaded?: '' | 'true' | 'false' | string
  from?: string
  to?: string
REDACTED

async function parseBatchImageError(response: Response): Promise<Error> {
  try {
    const body = await response.json()
    const message = body?.error?.message || body?.message || response.statusText
    const error = new Error(message)
    ;(error as any).code = body?.error?.code || response.status
    ;(error as any).status = response.status
    ;(error as any).requestId = response.headers.get('X-Request-Id') || ''
    return error
  REDACTED catch {
    const error = new Error(response.statusText || `HTTP ${response.statusREDACTED`)
    ;(error as any).code = response.status
    ;(error as any).status = response.status
    ;(error as any).requestId = response.headers.get('X-Request-Id') || ''
    return error
  REDACTED
REDACTED

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKeyREDACTED`,
    ...extra,
  REDACTED
REDACTED

export async function submitBatchImageJob(
  apiKey: string,
  payload: BatchImageSubmitRequest,
  idempotencyKey: string,
): Promise<BatchImageJob> {
  const response = await fetch(buildGatewayUrl('/v1/images/batches'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    REDACTED),
    body: JSON.stringify(payload),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
REDACTED

export async function getBatchImageJob(apiKey: string, batchId: string): Promise<BatchImageJob> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)REDACTED`), {
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
REDACTED

export async function listBatchImageJobs(apiKey: string, options: number | BatchImageJobsListOptions = 20): Promise<BatchImageJobsResponse> {
  const params = new URLSearchParams()
  if (typeof options === 'number') {
    params.set('limit', String(options))
  REDACTED else {
    params.set('limit', String(options.limit || 20))
    if (options.cursor) params.set('cursor', options.cursor)
    if (options.status) params.set('status', options.status)
    if (options.taskName) params.set('task_name', options.taskName)
    if (options.downloaded) params.set('downloaded', options.downloaded)
    if (options.from) params.set('from', options.from)
    if (options.to) params.set('to', options.to)
  REDACTED
  const response = await fetch(buildGatewayUrl(`/v1/images/batches?${params.toString()REDACTED`), {
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
REDACTED

export async function listBatchImageModels(apiKey: string): Promise<BatchImageModelsResponse> {
  const response = await fetch(buildGatewayUrl('/v1/images/batches/models'), {
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
REDACTED

export async function listBatchImageItems(
  apiKey: string,
  batchId: string,
  status = '',
): Promise<BatchImageItemsResponse> {
  const query = status ? `?status=${encodeURIComponent(status)REDACTED` : ''
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)REDACTED/items${queryREDACTED`), {
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
REDACTED

export async function cancelBatchImageJob(apiKey: string, batchId: string): Promise<BatchImageJob> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)REDACTED/cancel`), {
    method: 'POST',
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
REDACTED

export async function downloadBatchImageZip(apiKey: string, batchId: string): Promise<Blob> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)REDACTED/download`), {
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.blob()
REDACTED

export async function getBatchImageItemContent(apiKey: string, batchId: string, customId: string, imageIndex = 0): Promise<Blob> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)REDACTED/items/${encodeURIComponent(customId)REDACTED/content?image_index=${encodeURIComponent(String(imageIndex))REDACTED`), {
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
  return response.blob()
REDACTED

export async function deleteBatchImageJobRecord(apiKey: string, batchId: string): Promise<void> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)REDACTED`), {
    method: 'DELETE',
    headers: authHeaders(apiKey),
  REDACTED)
  if (!response.ok) throw await parseBatchImageError(response)
REDACTED

export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
REDACTED
