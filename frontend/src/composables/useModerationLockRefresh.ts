import { watch } from 'vue'
import type { ApiKey } from '@/types'

// Only the server releases locks. An open list fetches its current state at the next deadline.
export function useModerationLockRefresh(keys: () => ApiKey[], refresh: () => Promise<void>) {
  watch(() => {
    let next = Infinity
    for (const key of keys()) {
      if (key.moderation_locked_at && key.moderation_locked_until) {
        const until = Date.parse(key.moderation_locked_until)
        if (Number.isFinite(until)) next = Math.min(next, until)
      }
    }
    return next
  }, (next, _, onCleanup) => {
    if (!Number.isFinite(next)) return
    let timer: ReturnType<typeof setTimeout>
    let active = true
    const schedule = () => {
      const remaining = next - Date.now()
      // Browsers only accept signed 32-bit timers. Recompute far-away deadlines on wake.
      timer = setTimeout(async () => {
        if (Date.now() >= next) {
          try {
            await refresh()
          } finally {
            // If the server still reports the same lock (e.g. clock skew), recheck later.
            if (active) schedule()
          }
        } else {
          schedule()
        }
      }, remaining <= 0 ? 30_000 : Math.min(remaining + 50, 2_147_483_647))
    }
    schedule()
    onCleanup(() => { active = false; clearTimeout(timer) })
  }, { immediate: true })
}
