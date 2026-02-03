/**
 * User Announcements API endpoints
 */

import { apiClient REDACTED from './client'
import type { UserAnnouncement REDACTED from '@/types'

export async function list(unreadOnly: boolean = false): Promise<UserAnnouncement[]> {
  const { data REDACTED = await apiClient.get<UserAnnouncement[]>('/announcements', {
    params: unreadOnly ? { unread_only: 1 REDACTED : {REDACTED
  REDACTED)
  return data
REDACTED

export async function markRead(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>(`/announcements/${idREDACTED/read`)
  return data
REDACTED

const announcementsAPI = {
  list,
  markRead
REDACTED

export default announcementsAPI

