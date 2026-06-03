export type PendingAuthTokenField = 'pending_auth_token' | 'pending_oauth_token'

export interface RegisterFlowPendingAdoptionDecision {
  adopt_display_name?: boolean
  adopt_avatar?: boolean
}

export interface RegisterFlowStorageData {
  email?: string
  password?: string
  turnstile_token?: string
  promo_code?: string
  invitation_code?: string
  aff_code?: string
  email_verify_sent_at?: number
  email_verify_countdown?: number
  pending_auth_token?: string
  pending_auth_token_field?: PendingAuthTokenField
  pending_provider?: string
  pending_redirect?: string
  pending_adoption_decision?: RegisterFlowPendingAdoptionDecision
}

const REGISTER_FLOW_STORAGE_KEY = 'register_data'

function cleanRegisterFlowStorage(
  data: Partial<RegisterFlowStorageData>
): RegisterFlowStorageData {
  return Object.fromEntries(
    Object.entries(data).filter(([, value]) => value !== undefined)
  ) as RegisterFlowStorageData
}

export function getRegisterFlowStorage(): RegisterFlowStorageData | null {
  if (typeof window === 'undefined') {
    return null
  }

  try {
    const raw = window.sessionStorage.getItem(REGISTER_FLOW_STORAGE_KEY)
    if (!raw) {
      return null
    }
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') {
      clearRegisterFlowStorage()
      return null
    }
    return parsed as RegisterFlowStorageData
  } catch {
    clearRegisterFlowStorage()
    return null
  }
}

export function setRegisterFlowStorage(data: Partial<RegisterFlowStorageData>): void {
  if (typeof window === 'undefined') {
    return
  }

  const cleaned = cleanRegisterFlowStorage(data)

  try {
    if (Object.keys(cleaned).length === 0) {
      window.sessionStorage.removeItem(REGISTER_FLOW_STORAGE_KEY)
      return
    }
    window.sessionStorage.setItem(REGISTER_FLOW_STORAGE_KEY, JSON.stringify(cleaned))
  } catch {
    // Ignore browser storage errors.
  }
}

export function patchRegisterFlowStorage(
  patch: Partial<RegisterFlowStorageData>
): RegisterFlowStorageData | null {
  const current = getRegisterFlowStorage() || {}
  const next = cleanRegisterFlowStorage({
    ...current,
    ...patch,
  })

  setRegisterFlowStorage(next)
  return getRegisterFlowStorage()
}

export function clearRegisterFlowStorage(): void {
  if (typeof window === 'undefined') {
    return
  }

  try {
    window.sessionStorage.removeItem(REGISTER_FLOW_STORAGE_KEY)
  } catch {
    // Ignore browser storage errors.
  }
}

export function getRegisterFlowVerifyCountdownRemaining(now = Date.now()): number {
  const data = getRegisterFlowStorage()
  const countdown = Number(data?.email_verify_countdown) || 0
  const sentAt = Number(data?.email_verify_sent_at) || 0

  if (countdown <= 0 || sentAt <= 0) {
    return 0
  }

  const elapsedSeconds = Math.max(0, Math.floor((now - sentAt) / 1000))
  return Math.max(0, countdown - elapsedSeconds)
}
