/**
 * Plugin host-bridge "stores".
 *
 * The migrated payment views were written against the host SPA's pinia stores
 * (`useAppStore`, `useAuthStore`, `useSubscriptionStore`). Inside the plugin
 * runtime there is no host store registry available, so we expose a tiny
 * adapter layer over the host SDK that mimics the surface the views relied
 * on. This keeps the migrated SFC bodies untouched while routing all calls
 * through SDK-injected capabilities.
 */

import { computed, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { getSdk } from '../api/sdk'

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

interface AuthStoreLike {
  user: ComputedRef<unknown>
  refreshUser(): Promise<void>
}

/** Bridge authStore to sdk.auth (refreshUser is a no-op — host owns refresh). */
export function useAuthStore(): AuthStoreLike {
  const sdk = getSdk()
  return {
    user: computed(() => sdk.auth.user.value),
    refreshUser: async () => {
      // The host SDK's auth state is reactive and refreshed by the host on
      // its own cadence. The plugin should not push manual refreshes.
    },
  }
}

interface SubscriptionStoreLike {
  activeSubscriptions: Ref<unknown[]>
  fetchActiveSubscriptions(force?: boolean): Promise<unknown[]>
}

/**
 * Stub subscription store — TODO Phase 4: surface a real `/me/subscriptions`
 * call via the plugin's host-extension proxy. For now we expose an empty list
 * so PaymentView's "active subscriptions" section gracefully renders blank.
 */
export function useSubscriptionStore(): SubscriptionStoreLike {
  const empty = ref<unknown[]>([])
  return {
    activeSubscriptions: empty,
    fetchActiveSubscriptions: async () => empty.value,
  }
}
