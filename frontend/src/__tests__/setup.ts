/**
 * Vitest 测试环境设置
 * 提供全局 mock 和测试工具
 */
import { config REDACTED from '@vue/test-utils'
import { vi REDACTED from 'vitest'

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()

  return {
    get length() {
      return values.size
    REDACTED,
    clear() {
      values.clear()
    REDACTED,
    getItem(key: string) {
      return values.has(key) ? values.get(key)! : null
    REDACTED,
    key(index: number) {
      return Array.from(values.keys())[index] ?? null
    REDACTED,
    removeItem(key: string) {
      values.delete(key)
    REDACTED,
    setItem(key: string, value: string) {
      values.set(key, String(value))
    REDACTED
  REDACTED
REDACTED

if (typeof globalThis.localStorage === 'undefined' || typeof globalThis.localStorage.getItem !== 'function') {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: createMemoryStorage()
  REDACTED)
REDACTED

if (typeof window !== 'undefined' && typeof window.localStorage.getItem !== 'function') {
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: globalThis.localStorage
  REDACTED)
REDACTED

// Mock requestIdleCallback (Safari < 15 不支持)
if (typeof globalThis.requestIdleCallback === 'undefined') {
  globalThis.requestIdleCallback = ((callback: IdleRequestCallback) => {
    return window.setTimeout(() => callback({ didTimeout: false, timeRemaining: () => 50 REDACTED), 1)
  REDACTED) as unknown as typeof requestIdleCallback
REDACTED

if (typeof globalThis.cancelIdleCallback === 'undefined') {
  globalThis.cancelIdleCallback = ((id: number) => {
    window.clearTimeout(id)
  REDACTED) as unknown as typeof cancelIdleCallback
REDACTED

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
  REDACTED)) as unknown as typeof window.matchMedia
REDACTED

// Mock IntersectionObserver
class MockIntersectionObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
REDACTED

globalThis.IntersectionObserver = MockIntersectionObserver as unknown as typeof IntersectionObserver

// Mock ResizeObserver
class MockResizeObserver {
  observe = vi.fn()
  disconnect = vi.fn()
  unobserve = vi.fn()
REDACTED

globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

// Vue Test Utils 全局配置
config.global.stubs = {
  // 可以在这里添加全局 stub
REDACTED

// 设置全局测试超时
vi.setConfig({ testTimeout: 10000 REDACTED)
