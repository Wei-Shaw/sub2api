/**
 * Host module type shims (V2 SDK 改造后).
 *
 * V2: plugin 通过 @sub2api/plugin-sdk 共享 UI 组件, 不再 import 任何 host @/
 *     源码. 但仍需要少量 type-only shim:
 *
 * 1. host-sdk.ts (HostSdk) 内部 import type { User } from '@/types' —— 跨
 *    workspace type-only 引用, 我们给一个最小 User shape stub
 * 2. SDK 包内的 DataTable.vue 用 @tanstack/vue-virtual, 该 dep 安装在 host
 *    frontend, plugin 没有; 给一个 module shim 让 vue-tsc 通过
 * 3. SDK utils/tablePreferences 引用 window.__APP_CONFIG__, 给全局 stub
 *
 * 这些 shim 仅用于 vue-tsc 类型检查; 运行时 vue-virtual 由 SDK 包源码
 * 通过 host bundle 复用 (host frontend 已安装), tablePreferences 由
 * host index.html 注入 __APP_CONFIG__.
 */

declare module '@/types' {
  export interface User {
    id: number
    username: string
    email?: string
    role?: string
    [key: string]: unknown
  }
}

declare module '@tanstack/vue-virtual' {
  // SDK DataTable 只用了 useVirtualizer; 给宽松 any 类型 stub.
  // 真实类型来自 host frontend/node_modules, plugin 没装该 dep, 仅靠 host 运行时复用.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  export function useVirtualizer(options: any): any
}

interface Window {
  __APP_CONFIG__?: Record<string, unknown>
}
