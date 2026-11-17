export interface OpenAIReferralEligibility {
  should_show: boolean
  remaining_send_capacity: number | null
  remaining_reward_capacity: number | null
  requires_explicit_confirmation: boolean | null
  offer_id?: string
  title?: string
  description?: string
  rules?: string[]
  grants?: { grant_type: string; recipient: string; amount: number }[]
  program_id: string
  available_invites: number | null
  fetched_at: number
}

export interface OpenAIReferralRefreshResult {
  eligibility: OpenAIReferralEligibility | null
  cache_persisted: boolean
}

export interface OpenAIReferralSendResult extends OpenAIReferralRefreshResult {
  email: string
  sent: boolean
  refresh_failed: boolean
}
