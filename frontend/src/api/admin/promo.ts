/**
 * Admin Promo Codes API endpoints
 */

import { apiClient REDACTED from '../client'
import type {
  PromoCode,
  PromoCodeUsage,
  CreatePromoCodeRequest,
  UpdatePromoCodeRequest,
  BasePaginationResponse
REDACTED from '@/types'

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<BasePaginationResponse<PromoCode>> {
  const { data REDACTED = await apiClient.get<BasePaginationResponse<PromoCode>>('/admin/promo-codes', {
    params: { page, page_size: pageSize, ...filters REDACTED,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

export async function getById(id: number): Promise<PromoCode> {
  const { data REDACTED = await apiClient.get<PromoCode>(`/admin/promo-codes/${idREDACTED`)
  return data
REDACTED

export async function create(request: CreatePromoCodeRequest): Promise<PromoCode> {
  const { data REDACTED = await apiClient.post<PromoCode>('/admin/promo-codes', request)
  return data
REDACTED

export async function update(id: number, request: UpdatePromoCodeRequest): Promise<PromoCode> {
  const { data REDACTED = await apiClient.put<PromoCode>(`/admin/promo-codes/${idREDACTED`, request)
  return data
REDACTED

export async function deleteCode(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/promo-codes/${idREDACTED`)
  return data
REDACTED

export async function getUsages(
  id: number,
  page: number = 1,
  pageSize: number = 20
): Promise<BasePaginationResponse<PromoCodeUsage>> {
  const { data REDACTED = await apiClient.get<BasePaginationResponse<PromoCodeUsage>>(
    `/admin/promo-codes/${idREDACTED/usages`,
    { params: { page, page_size: pageSize REDACTED REDACTED
  )
  return data
REDACTED

const promoAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteCode,
  getUsages
REDACTED

export default promoAPI
