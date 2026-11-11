export interface OpenAIActiveSession {
  session_id: string
  applications: string[]
  device_name: string
  description?: string
  platform?: string
  os_version?: string
  city?: string
  region?: string
  country?: string
  last_signed_in?: number
  is_current: boolean
  can_revoke: boolean
}

export interface OpenAIActiveSessions {
  sessions: OpenAIActiveSession[]
  current_session_identified: boolean
  fetched_at: number
}
