/**
 * Vitest 测试环境设置
 * 提供全局 mock 和测试工具
 */
import { config } from '@vue/test-utils'
import { vi } from 'vitest'

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()

  return {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key: string) {
      return values.has(key) ? values.get(key)! : null
    },
    key(index: number) {
      return Array.from(values.keys())[index] ?? null
    },
    removeItem(key: string) {
      values.delete(key)
    },
    setItem(key: string, value: string) {
      values.set(key, String(value))
    }
  }
}

if (typeof globalThis.localStorage === 'undefined' || typeof globalThis.localStorage.getItem !== 'function') {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: createMemoryStorage()
  })
}

if (typeof window !== 'undefined' && typeof window.localStorage.getItem !== 'function') {
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: globalThis.localStorage
  })
}

// Mock requestIdleCallback (Safari < 15 不支持)
if (typeof globalThis.requestIdleCallback === 'undefined') {
  globalThis.requestIdleCallback = ((callback: IdleRequestCallback) => {
    return window.setTimeout(() => callback({ didTimeout: false, timeRemaining: () => 50 }), 1)
  }) as unknown as typeof requestIdleCallback
}

if (typeof globalThis.cancelIdleCallback === 'undefined') {
  globalThis.cancelIdleCallback = ((id: number) => {
    window.clearTimeout(id)
  }) as unknown as typeof cancelIdleCallback
}

// Mock matchMedia (jsdom 未实现;DataTable 等组件依赖它做桌面/移动分支)
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) => ({
    matches: true, // 测试默认按桌面视口渲染表格
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia
}

// Mock IntersectionObserver
class MockIntersectionObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
}

globalThis.IntersectionObserver = MockIntersectionObserver as unknown as typeof IntersectionObserver

// Mock ResizeObserver
class MockResizeObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
}

globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

// Vue Test Utils 全局配置
config.global.stubs = {
  // 可以在这里添加全局 stub
}

/*
 * Install a real i18n instance for every mount.
 *
 * The Tier 0 primitives in `components/common/**` call `t()` for their a11y
 * labels and translated prop fallbacks. Without a plugin, `useI18n()` throws,
 * so every spec that mounted such a primitive — directly or through any view
 * that happens to contain one — would have to hand-roll a `vue-i18n` mock. That
 * pushes an i18n concern into hundreds of unrelated specs and makes adding a
 * translated string to a primitive a breaking change for the whole suite.
 *
 * Deliberately the *real* instance from `@/i18n` with the real message files,
 * so assertions read the strings the product actually ships. Specs that still
 * mock `vue-i18n` themselves keep working: their mock replaces `useI18n`, and
 * the plugin install below only touches this already-constructed instance.
 *
 * Imported dynamically rather than at the top of the file: ESM hoists static
 * imports above the localStorage shim defined earlier, and `@/i18n` reads
 * `localStorage` while resolving the boot locale. The dynamic import also lets
 * the JIT flag below be set first, which is the whole reason it has to be here.
 */

/*
 * Match the production i18n build. `vite.config.ts` aliases `vue-i18n` to the
 * runtime-only bundle (no `unsafe-eval`, so it survives our CSP) and turns the
 * JIT compiler back on with `define: { __INTLIFY_JIT_COMPILATION__: true }`,
 * which compiles messages to an AST instead of to JS source.
 *
 * `vitest.config.ts` copies the alias but not the `define`. Without the flag,
 * vue-i18n registers no message compiler at all and every `t()` silently
 * returns the key path it was handed — so the suite would assert dotted key
 * paths while the shipped app renders sentences. vue-i18n reads the flag off
 * `globalThis` when it is not statically defined (its `initFeatureFlags` only
 * defaults it when the value is not already a boolean), so setting it before
 * the first `vue-i18n` import is enough.
 */
;(globalThis as unknown as { __INTLIFY_JIT_COMPILATION__: boolean }).__INTLIFY_JIT_COMPILATION__ =
  true

const { default: i18n, loadLocaleMessages } = await import('@/i18n')
await loadLocaleMessages('en')
await loadLocaleMessages('zh')
config.global.plugins = [i18n]

// 设置全局测试超时
vi.setConfig({ testTimeout: 10000 })
