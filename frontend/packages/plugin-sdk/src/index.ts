/**
 * @sub2api/plugin-sdk public entry.
 *
 * Named re-export of UI components, utilities and types shared between
 * sub2api host frontend and plugin frontends.
 */
export { default as Icon } from './components/Icon.vue'
export { default as DataTable } from './components/DataTable.vue'
export { default as BaseDialog } from './components/BaseDialog.vue'
export { default as ConfirmDialog } from './components/ConfirmDialog.vue'
export { default as Select } from './components/Select.vue'
export { default as Pagination } from './components/Pagination.vue'
export { default as EmptyState } from './components/EmptyState.vue'
export { default as Toggle } from './components/Toggle.vue'
export { default as PlatformIcon } from './components/PlatformIcon.vue'

// Layout primitives (Plan B D1)
export { default as PluginPageLayout } from './components/PluginPageLayout.vue'
export { default as TablePageLayout } from './components/TablePageLayout.vue'
export { default as PageActions } from './components/PageActions.vue'
export { default as FilterBar } from './components/FilterBar.vue'

// Widgets (Plan B D2)
export { default as SearchInput } from './components/SearchInput.vue'
export { default as StatusBadge } from './components/StatusBadge.vue'

export type { Column, SelectOption } from './types'

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

// Account form widget components
export { default as ToggleCard } from './components/account/ToggleCard.vue'
export { default as PoolModeSection } from './components/account/PoolModeSection.vue'
export { default as CustomErrorCodesSection } from './components/account/CustomErrorCodesSection.vue'
export { default as TempUnschedSection } from './components/account/TempUnschedSection.vue'
export { default as ModelRestrictionSection } from './components/account/ModelRestrictionSection.vue'
export { default as VertexServiceAccount } from './components/account/VertexServiceAccount.vue'

// Account usage display components
export { default as UsageProgressBar } from './components/account/UsageProgressBar.vue'

// Account constants
export { VERTEX_LOCATION_OPTIONS } from './constants/account'

// Utilities used by account form widgets
export { createStableObjectKeyResolver } from './utils/stableObjectKey'

// Utilities used by account usage display
export { formatCompactNumber } from './utils/formatCompact'

export * from './host-sdk'

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
