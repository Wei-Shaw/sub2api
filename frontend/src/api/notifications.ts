import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'
import type { UserNotification, UserNotificationListParams } from '@/types/notification'

export const notificationAPI = {
  list(params?: UserNotificationListParams) {
    return apiClient.get<BasePaginationResponse<UserNotification>>('/notifications', { params })
  },

  unreadCount(category?: string) {
    return apiClient.get<{ count: number }>('/notifications/unread-count', {
      params: category ? { category } : undefined
    })
  },

  markRead(id: number) {
    return apiClient.post(`/notifications/${id}/read`)
  },

  markAllRead(category?: string) {
    return apiClient.post('/notifications/read-all', null, {
      params: category ? { category } : undefined
    })
  }
}
