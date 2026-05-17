/**
 * Common component types
 */

/**
 * Row type for DataTable internals. Used by sort utilities, composables,
 * and sub-components that need to index rows by column key.
 */
// Vue/Volar cannot infer generic row types through dynamic slot names.
// DataRow must be `any` so slot consumers can pass rows to typed functions
// (e.g., `formatDate(row as Account)`) without casting every access.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type DataRow = any

/**
 * Input type for DataTable's `data` prop. Wider than DataRow so that
 * TypeScript interfaces (which lack implicit index signatures) are accepted
 * without consumers needing to cast.  `object` is the base type of every
 * non-primitive, so `Account[]` is assignable to `object[]`.
 */
export type DataTableInputRow = object

export interface Column {
  key: string
  label: string
  sortable?: boolean
  class?: string
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  formatter?: (value: any, row: DataRow) => string
}

/**
 * Option shape for the shared Select component.
 *
 * 抽取到 types/index.ts 而非 Select.vue 内部, 因为 host (vue-tsc -b 项目引用
 * 模式) 不会走 Volar 解析 packages/ 下的 .vue 文件, 只会按 `*.vue` 通配模块
 * 处理 (default export only). 因此与 Select 共用的 type 必须放在 .ts 文件.
 */
export interface SelectOption {
  value: string | number | boolean | null
  label: string
  disabled?: boolean
  [key: string]: unknown
}
