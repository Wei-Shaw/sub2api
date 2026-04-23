/**
 * Vitest 测试环境设置
 * 提供全局 mock 和测试工具
 */
import { config REDACTED from '@vue/test-utils'
import { vi REDACTED from 'vitest'

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
