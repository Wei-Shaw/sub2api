/**
 * Shared host module type shims for sub2api plugins.
 *
 * Plugins reference this via `/// <reference types="@sub2api/plugin-sdk/types/host-modules" />`
 * (or `tsconfig.json` types entry) so vue-tsc can resolve modules that:
 *   1. live in host frontend/node_modules but not in plugin/node_modules
 *      (e.g. @tanstack/vue-virtual — only used inside SDK components and
 *      shared at runtime through host importmap), or
 *   2. are global window properties injected by the host index.html
 *      (e.g. window.__APP_CONFIG__).
 *
 * These declarations are type-only; runtime resolution is handled by the
 * host bundle / importmap, not by the plugin.
 */

declare module '@tanstack/vue-virtual' {
  // SDK DataTable only uses useVirtualizer; loose any-stub is sufficient.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  export function useVirtualizer(options: any): any
}

interface Window {
  __APP_CONFIG__?: Record<string, unknown>
}
