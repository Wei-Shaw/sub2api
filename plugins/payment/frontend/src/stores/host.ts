/**
 * Plugin host-bridge "stores".
 *
 * The migrated payment views were written against the host SPA's pinia stores
 * (`useAppStore`, `useAuthStore`, `useSubscriptionStore`). Inside the plugin
 * runtime there is no host store registry available, so we expose a tiny
 * adapter layer over the host SDK that mimics the surface the views relied
 * on. This keeps the migrated SFC bodies untouched while routing all calls
 * through SDK-injected capabilities.
 *
 * The subscription store is a real pinia store backed by the host's
 * `/subscriptions/active` endpoint. The plugin runtime shares the host's
 * pinia instance via `sdk.runtime.createApp`, so this defineStore call
 * registers on the same registry as the host's own stores.
 */

import { computed, ref } from 'vue'
import type { Ref } from 'vue'
import { defineStore } from 'pinia'
import type { User as SdkUser } from '@sub2api/plugin-sdk'
import { getSdk } from '../api/sdk'
import { getClient } from '../api/client'
import type { UserSubscription } from '../types/index'

/**
 * Host injects its full `User` (frontend/src/types) which has more fields
 * than the SDK's minimal shape (`balance`, `concurrency`, ...). The SDK
 * exposes the minimum guaranteed shape (`id`, `username`, ...) plus an
 * `[key: string]: unknown` index signature so plugin code can read extras.
 *
 * PaymentView reads `user.username` (typed) and `user.balance` (accessed
 * through the index signature). We widen the bridge so plugin views can
 * pull `balance` off without `as` casts.
 *
 * `balance` is typed as `string | number` because the host frontend has not
 * yet migrated its own `User.balance` to a string — once it does, plugin
 * views still work via the `money()` wrapper, which accepts both. The host
 * store edge converts whichever shape it receives.
 */
type AuthUser = SdkUser & { balance?: string | number }

interface AppStoreLike {
  showSuccess(message: string): void
  showError(message: string): void
  showInfo(message: string): void
  showWarning(message: string): void
  /** Cached public settings — host exposes payment_enabled etc. */
  cachedPublicSettings?: Record<string, unknown> | null
}

/** Bridge appStore.showXxx to sdk.notify.* */
export function useAppStore(): AppStoreLike {
  const sdk = getSdk()
  return {
    showSuccess: (msg: string) => sdk.notify.success(msg),
    showError: (msg: string) => sdk.notify.error(msg),
    showInfo: (msg: string) => sdk.notify.info(msg),
    showWarning: (msg: string) => sdk.notify.warning(msg),
    cachedPublicSettings: null,
  }
}

/**
 * Bridge authStore to sdk.auth.
 *
 * Returns a real pinia store (matching the host's setup-style store shape)
 * so that `authStore.user` is auto-unwrapped to `AuthUser | null` rather
 * than `Ref<AuthUser | null>`. PaymentView does
 * `const user = computed(() => authStore.user)`, which only typechecks
 * cleanly when `authStore.user` is the unwrapped value.
 *
 * `refreshUser` is a no-op — the host SDK's auth state is reactive and
 * refreshed by the host on its own cadence.
 */
export const useAuthStore = defineStore('payment-host-auth', () => {
  const sdk = getSdk()
  const user = computed<AuthUser | null>(() => sdk.auth.user.value as AuthUser | null)
  async function refreshUser(): Promise<void> {
    // intentional no-op: see jsdoc above.
  }
  return { user, refreshUser }
})

// ---------------------------------------------------------------------------
// Subscription store
// ---------------------------------------------------------------------------
//
// Backed by GET /api/v1/subscriptions/active (host endpoint, no /admin prefix).
// Cache TTL mirrors the host store (60s) so the user's purchase view feels
// consistent across host pages and the plugin-rendered /recharge route.
//
// Note on collisions: pinia store id 'subscriptions' is owned by the host's
// frontend/src/stores/subscriptions.ts. We register under a separate id
// ('payment-host-subscriptions') to avoid clobbering the host registry while
// still benefiting from the shared pinia instance (so reactive state lives
// for the lifetime of the app, not just the plugin mount).

const SUBSCRIPTION_CACHE_TTL_MS = 60_000

export const useSubscriptionStore = defineStore('payment-host-subscriptions', () => {
  const activeSubscriptions: Ref<UserSubscription[]> = ref([])
  const loading = ref(false)
  const loaded = ref(false)
  const lastFetchedAt: Ref<number | null> = ref(null)

  // In-flight request deduplication.
  let activePromise: Promise<UserSubscription[]> | null = null

  const hasActiveSubscriptions = computed(() => activeSubscriptions.value.length > 0)

  async function fetchActiveSubscriptions(force = false): Promise<UserSubscription[]> {
    const now = Date.now()
    if (
      !force &&
      loaded.value &&
      lastFetchedAt.value &&
      now - lastFetchedAt.value < SUBSCRIPTION_CACHE_TTL_MS
    ) {
      return activeSubscriptions.value
    }
    if (activePromise && !force) {
      return activePromise
    }

    loading.value = true
    const requestPromise = getClient()
      .get<UserSubscription[]>('/subscriptions/active')
      .then((res) => {
        const data = Array.isArray(res.data) ? res.data : []
        activeSubscriptions.value = data
        loaded.value = true
        lastFetchedAt.value = Date.now()
        return data
      })
      .catch((err) => {
        // Swallow: PaymentView gracefully renders an empty active-subscriptions
        // section, and the host's auth interceptor already surfaces 401s.
        console.error('[plugin-payment] failed to fetch active subscriptions:', err)
        activeSubscriptions.value = []
        return [] as UserSubscription[]
      })
      .finally(() => {
        if (activePromise === requestPromise) {
          loading.value = false
          activePromise = null
        }
      })

    activePromise = requestPromise
    return requestPromise
  }

  return {
    activeSubscriptions,
    loading,
    hasActiveSubscriptions,
    fetchActiveSubscriptions,
  }
})
