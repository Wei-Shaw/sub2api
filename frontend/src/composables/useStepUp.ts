/**
 * Step-up (sudo) 2FA composable.
 *
 * Wraps a sensitive admin action so that when the backend responds with a
 * STEP_UP_REQUIRED error, the caller can prompt for a TOTP code, obtain a
 * short-lived grant, and transparently retry the original action.
 *
 * Usage in a view:
 *   const stepUp = useStepUp()
 *   async function exportData() {
 *     await stepUp.run(() => adminAPI.accounts.exportData(...))
 *   }
 *   // template: <TotpStepUpDialog :controller="stepUp" />
 */
import { ref } from 'vue'

/** Error codes the backend uses to signal step-up state. */
const STEP_UP_REQUIRED = 'STEP_UP_REQUIRED'
const STEP_UP_TOTP_NOT_ENABLED = 'STEP_UP_TOTP_NOT_ENABLED'
const STEP_UP_ADMIN_API_KEY_FORBIDDEN = 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'

/**
 * Thrown by run() when the user dismisses the TOTP dialog.
 * Callers should treat it as a silent no-op, not an error to toast.
 */
export class StepUpCancelledError extends Error {
  readonly code = 'STEP_UP_CANCELLED'
  constructor() {
    super('step-up verification cancelled by user')
    this.name = 'StepUpCancelledError'
  }
}

export function isStepUpCancelled(err: unknown): boolean {
  return err instanceof StepUpCancelledError
}

interface ApiError {
  status?: number
  code?: string | number
  reason?: string
  message?: string
}

/** Extract the semantic error marker from either envelope shape (code or reason). */
function markerOf(err: unknown): string {
  const e = (err ?? {}) as ApiError
  const candidates = [e.code, e.reason].map((v) => (typeof v === 'string' ? v : ''))
  return candidates.find((v) => v.startsWith('STEP_UP')) || ''
}

export function isStepUpRequired(err: unknown): boolean {
  return markerOf(err) === STEP_UP_REQUIRED
}

export function isStepUpBlocked(err: unknown): boolean {
  const m = markerOf(err)
  return m === STEP_UP_TOTP_NOT_ENABLED || m === STEP_UP_ADMIN_API_KEY_FORBIDDEN
}

export function stepUpBlockReason(err: unknown): string {
  return markerOf(err)
}

export type StepUpController = ReturnType<typeof useStepUp>

export function useStepUp() {
  const visible = ref(false)
  const blockedReason = ref<string>('')
  let resolver: ((ok: boolean) => void) | null = null

  /** Open the TOTP dialog and resolve true once a grant is obtained. */
  function prompt(): Promise<boolean> {
    visible.value = true
    return new Promise<boolean>((resolve) => {
      resolver = resolve
    })
  }

  function onVerified() {
    visible.value = false
    resolver?.(true)
    resolver = null
  }

  function onCancel() {
    visible.value = false
    resolver?.(false)
    resolver = null
  }

  /**
   * Run a sensitive action. On STEP_UP_REQUIRED, prompt for a TOTP code and
   * retry once. STEP_UP_TOTP_NOT_ENABLED / admin-api-key errors are surfaced
   * to the caller (they cannot be resolved by entering a code). If the user
   * cancels the prompt, a StepUpCancelledError is thrown so callers can
   * distinguish "user changed their mind" from real failures.
   */
  async function run<T>(action: () => Promise<T>): Promise<T> {
    try {
      return await action()
    } catch (err) {
      if (isStepUpBlocked(err)) {
        blockedReason.value = markerOf(err)
        throw err
      }
      if (!isStepUpRequired(err)) {
        throw err
      }
      const ok = await prompt()
      if (!ok) {
        throw new StepUpCancelledError()
      }
      // Retry once now that the session holds a step-up grant.
      return await action()
    }
  }

  return {
    visible,
    blockedReason,
    prompt,
    onVerified,
    onCancel,
    run
  }
}
