// Export all common components.
//
// Every line here must stay a side-effect-free `export { default as X } from`
// re-export, and package.json must keep `"sideEffects": ["*.css"]`. Otherwise
// importing one component pulls the module graph for all of them and the
// first-paint chunk grows — the manual chunking in vite.config.ts splits
// node_modules only, so app-code bloat lands in the entry.

// ── Primitives (Swiss/editorial design system) ──────────────────────
export { default as Button } from './Button.vue'
export { default as Badge } from './Badge.vue'
export { default as StatusDot } from './StatusDot.vue'
export { default as NumCell } from './NumCell.vue'
export { default as Metric } from './Metric.vue'
export { default as Meter } from './Meter.vue'
export { default as FormField } from './FormField.vue'
export { default as Surface } from './Surface.vue'

// ── Pre-existing components ─────────────────────────────────────────
export { default as DataTable } from './DataTable.vue'
export { default as Pagination } from './Pagination.vue'
export { default as BaseDialog } from './BaseDialog.vue'
export { default as ConfirmDialog } from './ConfirmDialog.vue'
export { default as StatCard } from './StatCard.vue'
export { default as Toast } from './Toast.vue'
export { default as LoadingSpinner } from './LoadingSpinner.vue'
export { default as EmptyState } from './EmptyState.vue'
export { default as LocaleSwitcher } from './LocaleSwitcher.vue'
export { default as ExportProgressDialog } from './ExportProgressDialog.vue'

// Export types
export type { Column } from './types'
export type { Density, Size, Tone, Variant } from './primitives'
