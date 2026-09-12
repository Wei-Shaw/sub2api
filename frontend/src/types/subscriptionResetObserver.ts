export type SubscriptionResetDimension = 'daily' | 'weekly' | 'monthly'

export interface SubscriptionResetPolicy {
  group_id: number
  mode: 'off' | 'observe'
  source: '7d'
  account_ids: number[]
  quorum_percent: number
  aggregation_minutes: number
  reset_dimensions: SubscriptionResetDimension[]
  allow_single_subject: boolean
  version: number
  updated_at: string
}

export type SubscriptionResetPolicyInput = Omit<SubscriptionResetPolicy, 'group_id' | 'updated_at'>

export interface SubscriptionResetAccountState {
  account_id: number
  subject_key: string
  duplicate_of: number | null
  last_observed_at: string | null
  used_percent: number | null
  reset_at: string | null
  window_minutes: number | null
  plan_type: string
  state: 'waiting_baseline' | 'observing' | 'unknown' | 'needs_review' | 'local_reset'
  reason: string
  last_error: string
}

export interface SubscriptionResetEvent {
  id: string
  group_id: number
  policy_version: number
  source: '7d'
  kind: 'natural_reset' | 'early_drop'
  status: 'pending' | 'confirmed' | 'timed_out' | 'needs_review' | 'config_changed'
  reason: string
  opened_at: string
  deadline_at: string
  confirmed_at: string | null
  updated_at: string
  source_reset_at: string
  reset_dimensions: SubscriptionResetDimension[]
  denominator: number
  confirmed_count: number
  required_count: number
  members: {
    subject_key: string
    account_ids: number[]
    confirmed: boolean
    confirmed_at: string | null
    transition_id: string
    state: string
    reason: string
    old_reset_at?: string | null
    new_reset_at?: string | null
    confirmation_samples: number
    last_evidence_at?: string | null
  }[]
}

export interface SubscriptionResetStatus {
  policy: SubscriptionResetPolicy
  accounts: SubscriptionResetAccountState[]
  subject_count: number
  verified_subject_count: number
  ready: boolean
  active_event: SubscriptionResetEvent | null
  events: SubscriptionResetEvent[]
  last_observed_at: string | null
  observation_only: true
}
