/**
 * Host module type shims (V2 SDK 改造后).
 *
 * V2 之前: 大量 `@/...` shim, 让 vue-tsc 跳过 host 实现细节.
 * V2: plugin 通过 @sub2api/plugin-sdk 共享 UI 组件, 不再 import 任何 host @/
 *     源码; 这里只保留 type-only HostSdk 引用所需的 shim (相对路径直接到
 *     host frontend/src/plugins/sdk/host-sdk).
 *
 * Note: HostSdk 类型本身在 host frontend 源码里维护; plugin 通过相对路径
 * type-only import. 该路径的 .ts 文件在 plugin pnpm install 时不会被解析
 * (跨 workspace), 但 vue-tsc 走 fileSystem 直接读源码, 不依赖 module resolution.
 */

// 故意不再写任何 declare module '@/...'; 见上方说明.
export {}
