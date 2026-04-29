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

export type { Column, SelectOption } from './types'

export * as tablePreferences from './utils/tablePreferences'

export * from './host-sdk'
