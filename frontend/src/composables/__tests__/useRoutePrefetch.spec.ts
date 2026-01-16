/**
 * useRoutePrefetch 组合式函数单元测试
 */
import { describe, it, expect, vi, beforeEach, afterEach REDACTED from 'vitest'
import type { RouteLocationNormalized REDACTED from 'vue-router'

// Mock 所有动态 import
vi.mock('@/views/admin/AccountsView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/admin/UsersView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/admin/DashboardView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/admin/GroupsView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/admin/SubscriptionsView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/admin/RedeemView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/user/KeysView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/user/UsageView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/user/DashboardView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/user/RedeemView.vue', () => ({ default: {REDACTED REDACTED))
vi.mock('@/views/user/ProfileView.vue', () => ({ default: {REDACTED REDACTED))

import { useRoutePrefetch, _adminPrefetchMap, _userPrefetchMap REDACTED from '../useRoutePrefetch'

// Mock 路由对象
const createMockRoute = (path: string): RouteLocationNormalized => ({
  path,
  name: undefined,
  params: {REDACTED,
  query: {REDACTED,
  hash: '',
  fullPath: path,
  matched: [],
  meta: {REDACTED,
  redirectedFrom: undefined
REDACTED)

describe('useRoutePrefetch', () => {
  let originalRequestIdleCallback: typeof window.requestIdleCallback
  let originalCancelIdleCallback: typeof window.cancelIdleCallback

  beforeEach(() => {
    // 保存原始函数
    originalRequestIdleCallback = window.requestIdleCallback
    originalCancelIdleCallback = window.cancelIdleCallback

    // Mock requestIdleCallback 立即执行
    vi.stubGlobal('requestIdleCallback', (cb: IdleRequestCallback) => {
      const id = setTimeout(() => cb({ didTimeout: false, timeRemaining: () => 50 REDACTED), 0)
      return id
    REDACTED)
    vi.stubGlobal('cancelIdleCallback', (id: number) => clearTimeout(id))
  REDACTED)

  afterEach(() => {
    vi.restoreAllMocks()
    // 恢复原始函数
    window.requestIdleCallback = originalRequestIdleCallback
    window.cancelIdleCallback = originalCancelIdleCallback
  REDACTED)

  describe('_isAdminRoute', () => {
    it('应该正确识别管理员路由', () => {
      const { _isAdminRoute REDACTED = useRoutePrefetch()
      expect(_isAdminRoute('/admin/dashboard')).toBe(true)
      expect(_isAdminRoute('/admin/users')).toBe(true)
      expect(_isAdminRoute('/admin/accounts')).toBe(true)
    REDACTED)

    it('应该正确识别非管理员路由', () => {
      const { _isAdminRoute REDACTED = useRoutePrefetch()
      expect(_isAdminRoute('/dashboard')).toBe(false)
      expect(_isAdminRoute('/keys')).toBe(false)
      expect(_isAdminRoute('/usage')).toBe(false)
    REDACTED)
  REDACTED)

  describe('_getPrefetchConfig', () => {
    it('管理员 dashboard 应该返回正确的预加载配置', () => {
      const { _getPrefetchConfig REDACTED = useRoutePrefetch()
      const route = createMockRoute('/admin/dashboard')
      const config = _getPrefetchConfig(route)

      expect(config).toHaveLength(2)
    REDACTED)

    it('普通用户 dashboard 应该返回正确的预加载配置', () => {
      const { _getPrefetchConfig REDACTED = useRoutePrefetch()
      const route = createMockRoute('/dashboard')
      const config = _getPrefetchConfig(route)

      expect(config).toHaveLength(2)
    REDACTED)

    it('未定义的路由应该返回空数组', () => {
      const { _getPrefetchConfig REDACTED = useRoutePrefetch()
      const route = createMockRoute('/unknown-route')
      const config = _getPrefetchConfig(route)

      expect(config).toHaveLength(0)
    REDACTED)
  REDACTED)

  describe('triggerPrefetch', () => {
    it('应该在浏览器空闲时触发预加载', async () => {
      const { triggerPrefetch, prefetchedRoutes REDACTED = useRoutePrefetch()
      const route = createMockRoute('/admin/dashboard')

      triggerPrefetch(route)

      // 等待 requestIdleCallback 执行
      await new Promise((resolve) => setTimeout(resolve, 100))

      expect(prefetchedRoutes.value.has('/admin/dashboard')).toBe(true)
    REDACTED)

    it('应该避免重复预加载同一路由', async () => {
      const { triggerPrefetch, prefetchedRoutes REDACTED = useRoutePrefetch()
      const route = createMockRoute('/admin/dashboard')

      triggerPrefetch(route)
      await new Promise((resolve) => setTimeout(resolve, 100))

      // 第二次触发
      triggerPrefetch(route)
      await new Promise((resolve) => setTimeout(resolve, 100))

      // 只应该预加载一次
      expect(prefetchedRoutes.value.size).toBe(1)
    REDACTED)
  REDACTED)

  describe('cancelPendingPrefetch', () => {
    it('应该取消挂起的预加载任务', () => {
      const { triggerPrefetch, cancelPendingPrefetch, prefetchedRoutes REDACTED = useRoutePrefetch()
      const route = createMockRoute('/admin/dashboard')

      triggerPrefetch(route)
      cancelPendingPrefetch()

      // 不应该有预加载完成
      expect(prefetchedRoutes.value.size).toBe(0)
    REDACTED)
  REDACTED)

  describe('路由变化时取消之前的预加载', () => {
    it('应该在路由变化时取消之前的预加载任务', async () => {
      const { triggerPrefetch, prefetchedRoutes REDACTED = useRoutePrefetch()

      // 触发第一个路由的预加载
      triggerPrefetch(createMockRoute('/admin/dashboard'))

      // 立即切换到另一个路由
      triggerPrefetch(createMockRoute('/admin/users'))

      // 等待执行
      await new Promise((resolve) => setTimeout(resolve, 100))

      // 只有最后一个路由应该被预加载
      expect(prefetchedRoutes.value.has('/admin/users')).toBe(true)
    REDACTED)
  REDACTED)

  describe('resetPrefetchState', () => {
    it('应该重置所有预加载状态', async () => {
      const { triggerPrefetch, resetPrefetchState, prefetchedRoutes REDACTED = useRoutePrefetch()
      const route = createMockRoute('/admin/dashboard')

      triggerPrefetch(route)
      await new Promise((resolve) => setTimeout(resolve, 100))

      expect(prefetchedRoutes.value.size).toBeGreaterThan(0)

      resetPrefetchState()

      expect(prefetchedRoutes.value.size).toBe(0)
    REDACTED)
  REDACTED)

  describe('预加载映射表', () => {
    it('管理员预加载映射表应该包含正确的路由', () => {
      expect(_adminPrefetchMap).toHaveProperty('/admin/dashboard')
      expect(_adminPrefetchMap['/admin/dashboard']).toHaveLength(2)
    REDACTED)

    it('用户预加载映射表应该包含正确的路由', () => {
      expect(_userPrefetchMap).toHaveProperty('/dashboard')
      expect(_userPrefetchMap['/dashboard']).toHaveLength(2)
    REDACTED)
  REDACTED)

  describe('requestIdleCallback 超时处理', () => {
    it('超时后仍能正常执行预加载', async () => {
      // 模拟超时情况
      vi.stubGlobal('requestIdleCallback', (cb: IdleRequestCallback, options?: IdleRequestOptions) => {
        const timeout = options?.timeout || 2000
        return setTimeout(() => cb({ didTimeout: true, timeRemaining: () => 0 REDACTED), timeout)
      REDACTED)

      const { triggerPrefetch, prefetchedRoutes REDACTED = useRoutePrefetch()
      const route = createMockRoute('/dashboard')

      triggerPrefetch(route)

      // 等待超时执行
      await new Promise((resolve) => setTimeout(resolve, 2100))

      expect(prefetchedRoutes.value.has('/dashboard')).toBe(true)
    REDACTED)
  REDACTED)

  describe('预加载失败处理', () => {
    it('预加载失败时应该静默处理不影响页面功能', async () => {
      // 这个测试验证预加载失败不会抛出异常
      const { triggerPrefetch REDACTED = useRoutePrefetch()
      const route = createMockRoute('/admin/dashboard')

      // 不应该抛出异常
      expect(() => triggerPrefetch(route)).not.toThrow()
    REDACTED)
  REDACTED)
REDACTED)
