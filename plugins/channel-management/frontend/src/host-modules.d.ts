/**
 * Module shims for host frontend `@/...` imports.
 *
 * Plugin 通过 vite alias 把 `@` 指向 host frontend/src/, 让 ChannelsView.vue
 * 能复用 host 的通用组件 (DataTable / Select / ...) 与 store / API.
 * 然而 vue-tsc 严格类型检查时会沿着 import 链一路跟到 host 的 .vue / .ts,
 * 把 host 自身遗留的 type 警告暴露给 plugin typecheck.
 *
 * 这些 host 模块在 plugin 这边只关心"存在 + 可以渲染", 不需要精确类型;
 * 用宽松的 module shim 让 vue-tsc 跳过 host 文件的深度检查, 同时保留
 * plugin 自身代码的严格检查.
 */

declare module '@/components/common/*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

declare module '@/components/icons/*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

declare module '@/components/layout/*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

declare module '@/components/common/types' {
  export interface Column {
    key: string
    label: string
    sortable?: boolean
    width?: string
    align?: 'left' | 'center' | 'right'
  }
}

declare module '@/types' {
  export type GroupPlatform = string
  export interface AdminGroup {
    id: number
    name: string
    platform: GroupPlatform
    [key: string]: unknown
  }
  export interface User {
    id: number
    username: string
    email?: string
    role?: string
    [key: string]: unknown
  }
}

declare module '@/api/admin' {
  import type { AdminGroup } from '@/types'
  export interface AdminAPIShape {
    groups: {
      getAll: () => Promise<AdminGroup[]>
    }
    [key: string]: unknown
  }
  export const adminAPI: AdminAPIShape
}

declare module '@/stores/app' {
  export interface AppStoreShape {
    showError: (msg: string, duration?: number) => void
    showSuccess: (msg: string, duration?: number) => void
    showWarning: (msg: string, duration?: number) => void
    showInfo: (msg: string, duration?: number) => void
    [key: string]: unknown
  }
  export function useAppStore(): AppStoreShape
}

declare module '@/composables/usePersistedPageSize' {
  export function getPersistedPageSize(fallback?: number): number
}
