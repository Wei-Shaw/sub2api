export interface UserNotification {
  id: number
  category: string
  title: string
  body?: string
  link?: string
  metadata?: Record<string, unknown>
  read_at?: string | null
  created_at: string
}

export interface UserNotificationListParams {
  page?: number
  page_size?: number
  category?: string
  unread_only?: boolean
}
