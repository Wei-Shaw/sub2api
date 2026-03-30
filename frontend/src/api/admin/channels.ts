/**
 * Admin Channels API endpoints
 * Handles channel management for administrators
 */

import { apiClient REDACTED from '../client'

export type BillingMode = 'token' | 'per_request' | 'image'

export interface PricingInterval {
  id?: number
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
  sort_order: number
REDACTED

export interface ChannelModelPricing {
  id?: number
  platform: string
  models: string[]
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: PricingInterval[]
REDACTED

export interface Channel {
  id: number
  name: string
  description: string
  status: string
  billing_model_source: string // "requested" | "upstream"
  restrict_models: boolean
  group_ids: number[]
  model_pricing: ChannelModelPricing[]
  model_mapping: Record<string, Record<string, string>> // platform → {src→dstREDACTED
  created_at: string
  updated_at: string
REDACTED

export interface CreateChannelRequest {
  name: string
  description?: string
  group_ids?: number[]
  model_pricing?: ChannelModelPricing[]
  model_mapping?: Record<string, Record<string, string>>
  billing_model_source?: string
  restrict_models?: boolean
REDACTED

export interface UpdateChannelRequest {
  name?: string
  description?: string
  status?: string
  group_ids?: number[]
  model_pricing?: ChannelModelPricing[]
  model_mapping?: Record<string, Record<string, string>>
  billing_model_source?: string
  restrict_models?: boolean
REDACTED

interface PaginatedResponse<T> {
  items: T[]
  total: number
REDACTED

/**
 * List channels with pagination
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    search?: string
  REDACTED,
  options?: { signal?: AbortSignal REDACTED
): Promise<PaginatedResponse<Channel>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<Channel>>('/admin/channels', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    REDACTED,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

/**
 * Get channel by ID
 */
export async function getById(id: number): Promise<Channel> {
  const { data REDACTED = await apiClient.get<Channel>(`/admin/channels/${idREDACTED`)
  return data
REDACTED

/**
 * Create a new channel
 */
export async function create(req: CreateChannelRequest): Promise<Channel> {
  const { data REDACTED = await apiClient.post<Channel>('/admin/channels', req)
  return data
REDACTED

/**
 * Update a channel
 */
export async function update(id: number, req: UpdateChannelRequest): Promise<Channel> {
  const { data REDACTED = await apiClient.put<Channel>(`/admin/channels/${idREDACTED`, req)
  return data
REDACTED

/**
 * Delete a channel
 */
export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/channels/${idREDACTED`)
REDACTED

const channelsAPI = { list, getById, create, update, remove REDACTED
export default channelsAPI
