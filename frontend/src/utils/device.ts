/**
 * Detect whether the current device is mobile.
 * Uses navigator.userAgentData (modern API) with UA regex fallback.
 */
export function isMobileDevice(): boolean {
  const nav = navigator as unknown as Record<string, unknown>
  if (nav.userAgentData && typeof (nav.userAgentData as Record<string, unknown>).mobile === 'boolean') {
    return (nav.userAgentData as Record<string, unknown>).mobile as boolean
  }
  return /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent)
}
