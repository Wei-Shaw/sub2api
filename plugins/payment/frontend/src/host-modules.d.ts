/**
 * Host module type shims (V3 后).
 *
 * V3: HostSdk 类型已搬入 @sub2api/plugin-sdk (含内联 User 最小 shape), plugin
 *     不再 import any '@/types', 因此移除原来的 `declare module '@/types'` shim.
 *     仍保留:
 *
 * 1. SDK 包内的 DataTable.vue 用 @tanstack/vue-virtual, 该 dep 安装在 host
 *    frontend, plugin 没有; 给一个 module shim 让 vue-tsc 通过.
 * 2. SDK utils/tablePreferences 引用 window.__APP_CONFIG__, 给全局 stub.
 *
 * 这些 shim 仅用于 vue-tsc 类型检查; 运行时 vue-virtual 由 SDK 包源码
 * 通过 host bundle 复用 (host frontend 已安装), tablePreferences 由
 * host index.html 注入 __APP_CONFIG__.
 */

declare module '@tanstack/vue-virtual' {
  // SDK DataTable 只用了 useVirtualizer; 给宽松 any 类型 stub.
  // 真实类型来自 host frontend/node_modules, plugin 没装该 dep, 仅靠 host 运行时复用.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  export function useVirtualizer(options: any): any
}

interface Window {
  __APP_CONFIG__?: Record<string, unknown>
}
