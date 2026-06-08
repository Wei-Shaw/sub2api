/** @type {import('tailwindcss').Config} */
// Host Tailwind configuration. All design tokens live in the shared preset
// (frontend/packages/plugin-sdk/tailwind-preset.cjs) so plugin frontends pull
// from the same source of truth. Do NOT add token overrides here without also
// updating the preset, otherwise host and plugins will visually drift.
//
// content 必须包含 packages/plugin-sdk/src — host 直接 import SDK 共享组件
// (DataTable/EmptyState/BaseDialog/...), 它们的 .vue 模板里大量使用
// Tailwind utility class (md:hidden / md:block / space-y-3 / hidden / flex 等).
// 不扫 SDK source 时 host 主 bundle 不会编出这些 utility 的样式定义,
// 浏览器找不到 .md\:hidden / .md\:block, 导致 DataTable 的 mobile/desktop 分支
// 同时显示 — 表现为页面出现重复的 EmptyState 或表头/卡片混叠.
//
// 历史: V1 后端往 host head 注入了 plugin-sdk.css, 这份 SDK 自己 build 时跑过
// Tailwind 含完整 utility, 实际是给 host 用 SDK 组件兜底. V2 把 plugin-sdk.css
// 改为只在 plugin shadow root 内注入, host 主页面就丢了那份兜底, 必须靠 host
// tailwind 自己扫 SDK source 来补.
import preset from './packages/plugin-sdk/tailwind-preset.cjs'

export default {
  presets: [preset],
  content: [
    './index.html',
    './src/**/*.{vue,js,ts,jsx,tsx}',
    './packages/plugin-sdk/src/**/*.{vue,js,ts,jsx,tsx}',
  ],
  darkMode: 'class'
}
