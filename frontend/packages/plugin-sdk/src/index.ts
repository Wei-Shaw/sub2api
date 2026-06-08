/**
 * @sub2api/plugin-sdk public entry.
 *
 * Named re-export of UI components, utilities and types shared between
 * sub2api host frontend and plugin frontends.
 */
import type { Component } from 'vue'

import IconComponent from './components/Icon.vue'
import DataTableComponent from './components/DataTable.vue'
import BaseDialogComponent from './components/BaseDialog.vue'
import ConfirmDialogComponent from './components/ConfirmDialog.vue'
import SelectComponent from './components/Select.vue'
import PaginationComponent from './components/Pagination.vue'
import EmptyStateComponent from './components/EmptyState.vue'
import ToggleComponent from './components/Toggle.vue'
import PlatformIconComponent from './components/PlatformIcon.vue'
import GroupBadgeComponent from './components/GroupBadge.vue'

export const Icon = IconComponent
export const DataTable = DataTableComponent
export const BaseDialog = BaseDialogComponent
export const ConfirmDialog = ConfirmDialogComponent
export const Select = SelectComponent
export const Pagination = PaginationComponent
export const EmptyState = EmptyStateComponent
export const Toggle = ToggleComponent
export const PlatformIcon = PlatformIconComponent
export const GroupBadge = GroupBadgeComponent

/**
 * SDK-provided components registered globally on plugin Vue apps so SFCs
 * can reference them by tag name without import boilerplate.
 *
 * Plugins should pass this map to `sdk.runtime.createApp(view, target, {
 *   components: SDK_GLOBAL_COMPONENTS,
 * })` (or use the helper {@link registerSdkGlobals}).
 *
 * Single source of truth — adding a new SDK component to the global set
 * means appending one line here, not touching every plugin index.ts.
 */
export const SDK_GLOBAL_COMPONENTS: Record<string, Component> = {
  Icon,
  DataTable,
  BaseDialog,
  ConfirmDialog,
  Select,
  Pagination,
  EmptyState,
  Toggle,
  PlatformIcon,
  GroupBadge,
}

// Layout primitives (Plan B D1)
export { default as PluginPageLayout } from './components/PluginPageLayout.vue'
export { default as TablePageLayout } from './components/TablePageLayout.vue'
export { default as PageActions } from './components/PageActions.vue'
export { default as FilterBar } from './components/FilterBar.vue'

// Widgets (Plan B D2)
export { default as SearchInput } from './components/SearchInput.vue'
export { default as StatusBadge } from './components/StatusBadge.vue'

export type { Column, DataRow, DataTableInputRow, SelectOption } from './types'

export * as tablePreferences from './utils/tablePreferences'

export {
  extractApiErrorCode,
  extractApiErrorMetadata,
  extractI18nErrorMessage,
  extractApiErrorMessage,
} from './utils/apiError'

export {
  type Platform,
  isPlatform,
  PLATFORM_TINT,
  platformBadgeClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformTextClass,
  platformTagClass,
  platformTintTextClass,
  platformPickerClass,
  platformAccentBarClass,
  platformIconClass,
  platformButtonClass,
  platformDiscountClass,
  platformGradientClass,
  platformHeaderGradientClass,
  platformGradientTextClass,
  platformGradientSubtextClass,
  platformLabel,
} from './utils/platformColors'

export * from './account-form-types'
export * from './account-form-helpers'

export {
  type ProtocolDefinition,
  type ProtocolModel,
  type ProtocolPresetMapping,
  type ResolvedProtocolGroup,
  BUILTIN_PROTOCOLS,
  PROTOCOL_ANTHROPIC,
  PROTOCOL_OPENAI,
  PROTOCOL_GEMINI,
  getProtocol,
  resolveProtocolModels,
  resolveProtocolPresets,
  resolveAllProtocolModelIds,
  findProtocolForModel,
} from './protocols'

// Account form widget components
export { default as ToggleCard } from './components/account/ToggleCard.vue'
export { default as PoolModeSection } from './components/account/PoolModeSection.vue'
export { default as CustomErrorCodesSection } from './components/account/CustomErrorCodesSection.vue'
export { default as TempUnschedSection } from './components/account/TempUnschedSection.vue'
export { default as ModelRestrictionSection } from './components/account/ModelRestrictionSection.vue'
export { default as VertexServiceAccount } from './components/account/VertexServiceAccount.vue'
export { default as AccountCommonFields } from './components/account/AccountCommonFields.vue'
export { default as GroupSelect } from './components/account/GroupSelect.vue'
export { default as ProxySelect } from './components/account/ProxySelect.vue'

// Account usage display components
export { default as UsageProgressBar } from './components/account/UsageProgressBar.vue'

// Account constants
export { VERTEX_LOCATION_OPTIONS } from './constants/account'

// Subscription type constants — shared between gateway plugins (group config
// gating) and the payment plugin (plan binding).
export {
  SUBSCRIPTION_TYPE_SUBSCRIPTION,
  SUBSCRIPTION_TYPE_STANDARD,
  type SubscriptionType,
} from './constants/subscription'

// Mount helper for V2 Shadow DOM plugins. Encapsulates the duplicated
// componentPath dispatch / fallback / shadow root lookup boilerplate.
export {
  createPluginMount,
  type PluginMountFn,
  type CreatePluginMountOptions,
} from './createPluginMount'

// Utilities used by account form widgets
export { createStableObjectKeyResolver } from './utils/stableObjectKey'

// Utilities used by account usage display
export { formatCompactNumber } from './utils/formatCompact'

// Shared formatting helpers (date/time + host locale).
export { formatDateTime, getHostLocale } from './utils/format'

export * from './host-sdk'

// Plugin frontend bootstrap helpers (sdk / axios accessor factories)
export {
  createSdkAccessor,
  createApiClient,
  type SdkAccessor,
  type ApiClientAccessor,
} from './accessor'

// Group config shared components
export {
  SharedImagePricing,
  SharedAccountFilters,
  SharedInvalidRequestFallback,
} from './components/group-config'
export type {
  GroupConfigGroup,
  GroupConfigProps,
  GroupConfigExposed,
} from './components/group-config'

// Composables
export { useKeyedDebouncedSearch } from './composables/useKeyedDebouncedSearch'
export type { KeyedDebouncedSearchContext } from './composables/useKeyedDebouncedSearch'

// Account test composable & terminal component
export {
  useAccountTest,
  type AccountTestStream,
  type TestOutputLine,
  type TestImage,
  type TestStreamOptions,
} from './composables/useAccountTest'
export { default as AccountTestTerminal } from './components/account/AccountTestTerminal.vue'
export type { AccountTestExposed, SdkTestContext } from './account-form-types'
