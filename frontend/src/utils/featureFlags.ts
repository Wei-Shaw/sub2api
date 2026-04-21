/**
 * Feature flag registry — single source of truth for public-settings-driven
 * feature switches used by the sidebar, routes, and views.
 *
 * ## Why this module exists
 *
 * `public settings` reach the frontend through two channels:
 *
 *   1. **SSR injection** — the backend embeds `window.__APP_CONFIG__` into the
 *      HTML. `main.ts` calls `appStore.initFromInjectedConfig()` synchronously
 *      before Vue mounts, so `cachedPublicSettings` is populated on first
 *      render.
 *   2. **Async API** — `App.vue` awaits `appStore.fetchPublicSettings()` on
 *      mount as a fallback (used when injection is missing or stale).
 *
 * If the SSR injection struct forgets to include a feature flag field — the
 * exact bug that hid the "可用渠道" menu after every refresh — the frontend
 * reads `undefined` until the async call resolves. An opt-in flag written as
 * `settings?.xxx_enabled === true` then evaluates to `false` and the menu
 * disappears. An opt-out flag written as `settings?.xxx_enabled !== false`
 * evaluates to `true` (menu stays) but will flicker off if the backend sends
 * `false`.
 *
 * This module hides that `undefined` handling behind two explicit modes.
 *
 * ## Modes
 *
 *   - **`opt-out`** (default enabled) — menu visible when settings unloaded,
 *     hidden only when the backend explicitly sends `false`. Use for features
 *     that ship enabled by default (Channel Monitor, Payment).
 *   - **`opt-in`**  (default disabled) — menu hidden when settings unloaded,
 *     visible only when the backend explicitly sends `true`. Use for features
 *     that ship disabled (Available Channels).
 *
 * For `opt-in` flags to render immediately on refresh, the backend **must**
 * inject the field through `PublicSettingsInjectionPayload`. A drift test in
 * `backend/internal/handler/dto/public_settings_injection_schema_test.go`
 * catches omissions.
 *
 * ## Adding a new flag
 *
 *   1. Backend `service/domain_constants.go`  → `SettingKey<Name>Enabled`
 *   2. Backend `service/settings_view.go`      → `PublicSettings` + `SystemSettings`
 *   3. Backend `service/setting_service.go`    → `GetPublicSettings` / `UpdateSettings` /
 *                                                 `GetAllSettings` / `InitDefaultSettings` /
 *                                                 **`PublicSettingsInjectionPayload`**
 *                                                 (the drift test enforces this)
 *   4. Backend `handler/dto/settings.go`       → `PublicSettings` + `SystemSettings`
 *   5. Backend `handler/setting_handler.go`    → handler response
 *   6. Backend `handler/admin/setting_handler.go` → update request + audit diff
 *   7. Frontend `types/index.ts`               → `PublicSettings` typings
 *   8. Frontend `api/admin/settings.ts`        → admin DTO typings
 *   9. **Frontend `utils/featureFlags.ts` (this file)** → register via `defineFlag`
 *  10. Frontend `views/admin/SettingsView.vue` → Toggle UI + form defaults + save payload
 *  11. Frontend `components/layout/AppSidebar.vue` → attach via `makeSidebarFlag`
 *
 * ## Usage
 *
 * ```ts
 * import { FeatureFlags, makeSidebarFlag } from '@/utils/featureFlags'
 *
 * const flagAvailableChannels = makeSidebarFlag(FeatureFlags.availableChannels)
 * // ...
 * { path: '/available-channels', label: ..., featureFlag: flagAvailableChannels }
 * ```
 *
 * `isFeatureFlagEnabled(flag)` returns the resolved boolean (`true` = show).
 * `makeSidebarFlag(flag)` returns a `() => boolean | undefined` compatible with
 * `AppSidebar.NavItem.featureFlag`, where `false` hides the menu entry.
 */

import { useAppStore } from '@/stores/app'
import type { PublicSettings } from '@/types'

export type FeatureFlagMode = 'opt-in' | 'opt-out'

export interface FeatureFlagDefinition {
  /** Public-settings key used for lookup. */
  readonly key: keyof PublicSettings
  /** Resolution mode when the key is missing/undefined. */
  readonly mode: FeatureFlagMode
  /** Short human label for logs and debug tooling. */
  readonly label: string
}

function defineFlag<K extends keyof PublicSettings>(
  def: { key: K; mode: FeatureFlagMode; label: string },
): FeatureFlagDefinition {
  return def
}

/**
 * Registered feature flags. Add a new entry here when introducing a new
 * public-settings-driven switch; see the "Adding a new flag" checklist above.
 */
export const FeatureFlags = {
  channelMonitor: defineFlag({
    key: 'channel_monitor_enabled',
    mode: 'opt-out',
    label: 'Channel Monitor',
  }),
  availableChannels: defineFlag({
    key: 'available_channels_enabled',
    mode: 'opt-in',
    label: 'Available Channels',
  }),
  payment: defineFlag({
    key: 'payment_enabled',
    mode: 'opt-out',
    label: 'Payment',
  }),
} as const

export type RegisteredFeatureFlag = keyof typeof FeatureFlags

/**
 * Read the current value of a flag, honoring the mode's fallback.
 * `true`  → the feature is enabled (menu/route should render).
 * `false` → the feature is disabled (menu/route should hide).
 */
export function isFeatureFlagEnabled(flag: FeatureFlagDefinition): boolean {
  const appStore = useAppStore()
  const raw = appStore.cachedPublicSettings?.[flag.key] as
    | boolean
    | undefined
  if (typeof raw === 'boolean') return raw
  // Settings not yet loaded → fall back to the flag's declared mode:
  //   opt-out → visible by default, opt-in → hidden by default.
  return flag.mode === 'opt-out'
}

/**
 * Sidebar NavItem.featureFlag accepts a getter that returns
 * `false` to hide. Keeping the same contract lets callers swap in
 * registry-backed flags without changing AppSidebar's filter logic.
 */
export function makeSidebarFlag(flag: FeatureFlagDefinition): () => boolean {
  return () => isFeatureFlagEnabled(flag)
}
