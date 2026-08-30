export const DISPLAY_PRICING_SYNC_CHANNEL = 'sub2api-display-pricing'
export const DISPLAY_PRICING_SYNC_EVENT = 'sub2api:display-pricing-updated'
export const DISPLAY_PRICING_SYNC_STORAGE_KEY = 'sub2api:display-pricing-updated-at'

export interface DisplayPricingSyncMessage {
  updatedAt: number
}

function isSyncMessage(value: unknown): value is DisplayPricingSyncMessage {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as DisplayPricingSyncMessage).updatedAt === 'number'
  )
}

/** Notify every open customer catalogue after an administrator changes display-only pricing. */
export function notifyDisplayPricingUpdated(): void {
  if (typeof window === 'undefined') return

  const message: DisplayPricingSyncMessage = { updatedAt: Date.now() }

  if (typeof BroadcastChannel !== 'undefined') {
    try {
      const channel = new BroadcastChannel(DISPLAY_PRICING_SYNC_CHANNEL)
      channel.postMessage(message)
      channel.close()
    } catch {
      // Continue with storage + same-document event fallbacks.
    }
  }

  try {
    window.localStorage.setItem(DISPLAY_PRICING_SYNC_STORAGE_KEY, String(message.updatedAt))
  } catch {
    // Storage may be unavailable in hardened/private browser contexts.
  }

  window.dispatchEvent(new CustomEvent<DisplayPricingSyncMessage>(DISPLAY_PRICING_SYNC_EVENT, { detail: message }))
}

/**
 * BroadcastChannel handles other tabs, storage is a compatibility fallback, and
 * the custom event covers another catalogue mounted in the current document.
 */
export function subscribeDisplayPricingUpdates(callback: () => void): () => void {
  if (typeof window === 'undefined') return () => undefined

  let lastSeen = 0
  let channel: BroadcastChannel | null = null
  if (typeof BroadcastChannel !== 'undefined') {
    try {
      channel = new BroadcastChannel(DISPLAY_PRICING_SYNC_CHANNEL)
      channel.addEventListener('message', onBroadcast)
    } catch {
      channel = null
    }
  }

  function onBroadcast(event: MessageEvent<unknown>): void {
    if (isSyncMessage(event.data)) emitOnce(event.data.updatedAt)
  }

  function onStorage(event: StorageEvent): void {
    if (event.key !== DISPLAY_PRICING_SYNC_STORAGE_KEY || !event.newValue) return
    const updatedAt = Number(event.newValue)
    if (Number.isFinite(updatedAt)) emitOnce(updatedAt)
  }

  function onLocalEvent(event: Event): void {
    const detail = (event as CustomEvent<unknown>).detail
    if (isSyncMessage(detail)) emitOnce(detail.updatedAt)
  }

  function emitOnce(updatedAt: number): void {
    if (updatedAt <= lastSeen) return
    lastSeen = updatedAt
    callback()
  }

  window.addEventListener('storage', onStorage)
  window.addEventListener(DISPLAY_PRICING_SYNC_EVENT, onLocalEvent)

  return () => {
    channel?.removeEventListener('message', onBroadcast)
    channel?.close()
    window.removeEventListener('storage', onStorage)
    window.removeEventListener(DISPLAY_PRICING_SYNC_EVENT, onLocalEvent)
  }
}
