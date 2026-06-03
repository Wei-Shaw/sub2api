/**
 * Auth-aware navigation helper.
 *
 * Anonymous visitors clicking on plaza CTAs (`Use this group`, `Buy now`)
 * must be funnelled through `LoginView`, which already understands a
 * `?redirect=…` query and post-login does `router.push(redirect)`.
 *
 * `gotoOrLogin(target)` resolves the target to a `fullPath` (so any nested
 * query like `?openCreate=1&group_id=42` is preserved verbatim) and either:
 *   - pushes directly to the target when authenticated, or
 *   - pushes `/login?redirect=<fullPath>` otherwise.
 *
 * The composable is intentionally tiny: callers stay declarative and we keep
 * a single canonical place to wire the auth wall, so adding things like
 * a captcha or a "session expiring" interstitial later is one-touch.
 */

import { useRouter, type RouteLocationRaw } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

export interface UseAuthRedirect {
  /**
   * Navigate to `target` if authenticated, else send the visitor to
   * `/login?redirect=<resolved fullPath of target>`.
   *
   * Returns the underlying `router.push` promise so callers can `await` if
   * they want to chain UI feedback (e.g. close a modal once routing settles).
   */
  gotoOrLogin: (target: RouteLocationRaw) => Promise<void | unknown>
}

export function useAuthRedirect(): UseAuthRedirect {
  const router = useRouter()
  const authStore = useAuthStore()

  function gotoOrLogin(target: RouteLocationRaw): Promise<void | unknown> {
    if (authStore.isAuthenticated) {
      return router.push(target)
    }
    // `router.resolve(target).fullPath` already includes the encoded query
    // string. vue-router will URL-encode the *value* of the `redirect` query
    // automatically when serialising the outer location.
    const fullPath = router.resolve(target).fullPath
    return router.push({ path: '/login', query: { redirect: fullPath } })
  }

  return { gotoOrLogin }
}

export default useAuthRedirect
