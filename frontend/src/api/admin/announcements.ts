/**
 * Admin Announcements API endpoints
 */

import { apiClient REDACTED from '../client'
import type {
  Announcement,
  AnnouncementUserReadStatus,
  BasePaginationResponse,
  CreateAnnouncementRequest,
  UpdateAnnouncementRequest
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
): Promise<BasePaginationResponse<Announcement>> {
  const { data REDACTED = await apiClient.get<BasePaginationResponse<Announcement>>('/admin/announcements', {
    params: { page, page_size: pageSize, ...filters REDACTED,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

export async function getById(id: number): Promise<Announcement> {
  const { data REDACTED = await apiClient.get<Announcement>(`/admin/announcements/${idREDACTED`)
  return data
REDACTED

export async function create(request: CreateAnnouncementRequest): Promise<Announcement> {
  const { data REDACTED = await apiClient.post<Announcement>('/admin/announcements', request)
  return data
REDACTED

export async function update(id: number, request: UpdateAnnouncementRequest): Promise<Announcement> {
  const { data REDACTED = await apiClient.put<Announcement>(`/admin/announcements/${idREDACTED`, request)
  return data
REDACTED

export async function deleteAnnouncement(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/announcements/${idREDACTED`)
  return data
REDACTED

export async function getReadStatus(
  id: number,
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<BasePaginationResponse<AnnouncementUserReadStatus>> {
  const { data REDACTED = await apiClient.get<BasePaginationResponse<AnnouncementUserReadStatus>>(
    `/admin/announcements/${idREDACTED/read-status`,
    {
      params: { page, page_size: pageSize, ...filters REDACTED,
      signal: options?.signal
    REDACTED
  )
  return data
REDACTED

const announcementsAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteAnnouncement,
  getReadStatus
REDACTED

export default announcementsAPI
