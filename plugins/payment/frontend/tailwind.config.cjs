/** @type {import('tailwindcss').Config} */
// Plugin Tailwind config consumes the shared preset from @sub2api/plugin-sdk
// so colors / shadows / animations stay aligned with the host frontend.
// Do NOT redeclare design tokens here — change them in the shared preset.
//
// content 必须包含 @sub2api/plugin-sdk/src — plugin 内部使用的 SDK 共享组件
// (DataTable / EmptyState / BaseDialog / Pagination / Select / ...) 模板里
// 大量用 Tailwind utility class (md:hidden / md:block / space-y-3 / hidden / ...).
// 不扫 SDK source 时这些 class 不会进 plugin entry.css. 之前 host 头部注入了
// 一份 plugin-sdk.css 兜底, V2 取消了 head 注入 (改注入 plugin shadow root),
// shadow root 内必须由 plugin entry.css 自带这些 utility, 否则 DataTable 的
// mobile / desktop 分支 (md:hidden / hidden md:block) 同时显示, 出现重复
// EmptyState / 表头错乱.
//
// SDK 通过 file: 链接到 host 的 frontend/packages/plugin-sdk, pnpm install 通过
// symlink 把它链到 plugin 自己的 node_modules/@sub2api/plugin-sdk. fast-glob (tailwind
// 内部使用) 默认不跟随 node_modules 中的 symlink, 直接写
// './node_modules/@sub2api/plugin-sdk/src/**' 在 pnpm 下扫不到任何文件.
// 改用 require.resolve('@sub2api/plugin-sdk/package.json') 拿到包真实的物理路径
// (pnpm 解析后落在 .pnpm 或 host 仓库 frontend/packages/plugin-sdk), 再拼 src glob,
// 保证两种 install 形态 (pnpm symlink / npm hoisting) 都能扫到 SDK source.
const path = require('path')
const sdkPath = path.dirname(require.resolve('@sub2api/plugin-sdk/package.json'))

module.exports = {
  presets: [require('@sub2api/plugin-sdk/tailwind-preset.cjs')],
  content: [
    './src/**/*.{vue,js,ts,jsx,tsx}',
    './index.html',
    path.join(sdkPath, 'src/**/*.{vue,js,ts,jsx,tsx}'),
  ],
  darkMode: 'class',
}
