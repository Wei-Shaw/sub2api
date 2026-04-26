/**
 * Common component types
 */

export interface Column {
  key: string
  label: string
  sortable?: boolean
  class?: string
  formatter?: (value: any, row: any) => string
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
