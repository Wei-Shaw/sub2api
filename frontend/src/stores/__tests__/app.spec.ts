import { describe, it, expect, vi, beforeEach, afterEach REDACTED from 'vitest'
import { setActivePinia, createPinia REDACTED from 'pinia'
import { useAppStore REDACTED from '@/stores/app'

// Mock API 模块
vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn(),
REDACTED))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn(),
REDACTED))

describe('useAppStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    // 清除 window.__APP_CONFIG__
    delete (window as any).__APP_CONFIG__
  REDACTED)

  afterEach(() => {
    vi.useRealTimers()
  REDACTED)

  // --- Toast 消息管理 ---

  describe('Toast 消息管理', () => {
    it('showSuccess 创建 success 类型 toast', () => {
      const store = useAppStore()
      const id = store.showSuccess('操作成功')

      expect(id).toMatch(/^toast-/)
      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('success')
      expect(store.toasts[0].message).toBe('操作成功')
    REDACTED)

    it('showError 创建 error 类型 toast', () => {
      const store = useAppStore()
      store.showError('出错了')

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('error')
      expect(store.toasts[0].message).toBe('出错了')
    REDACTED)

    it('showWarning 创建 warning 类型 toast', () => {
      const store = useAppStore()
      store.showWarning('警告信息')

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('warning')
    REDACTED)

    it('showInfo 创建 info 类型 toast', () => {
      const store = useAppStore()
      store.showInfo('提示信息')

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('info')
    REDACTED)

    it('toast 在指定 duration 后自动消失', () => {
      const store = useAppStore()
      store.showSuccess('临时消息', 3000)

      expect(store.toasts).toHaveLength(1)

      vi.advanceTimersByTime(3000)

      expect(store.toasts).toHaveLength(0)
    REDACTED)

    it('hideToast 移除指定 toast', () => {
      const store = useAppStore()
      const id = store.showSuccess('消息1')
      store.showError('消息2')

      expect(store.toasts).toHaveLength(2)

      store.hideToast(id)

      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('error')
    REDACTED)

    it('clearAllToasts 清除所有 toast', () => {
      const store = useAppStore()
      store.showSuccess('消息1')
      store.showError('消息2')
      store.showWarning('消息3')

      expect(store.toasts).toHaveLength(3)

      store.clearAllToasts()

      expect(store.toasts).toHaveLength(0)
    REDACTED)

    it('hasActiveToasts 正确反映 toast 状态', () => {
      const store = useAppStore()
      expect(store.hasActiveToasts).toBe(false)

      store.showSuccess('消息')
      expect(store.hasActiveToasts).toBe(true)

      store.clearAllToasts()
      expect(store.hasActiveToasts).toBe(false)
    REDACTED)

    it('多个 toast 的 ID 唯一', () => {
      const store = useAppStore()
      const id1 = store.showSuccess('消息1')
      const id2 = store.showSuccess('消息2')
      const id3 = store.showSuccess('消息3')

      expect(id1).not.toBe(id2)
      expect(id2).not.toBe(id3)
    REDACTED)
  REDACTED)

  // --- 侧边栏 ---

  describe('侧边栏管理', () => {
    it('toggleSidebar 切换折叠状态', () => {
      const store = useAppStore()
      expect(store.sidebarCollapsed).toBe(false)

      store.toggleSidebar()
      expect(store.sidebarCollapsed).toBe(true)

      store.toggleSidebar()
      expect(store.sidebarCollapsed).toBe(false)
    REDACTED)

    it('setSidebarCollapsed 直接设置状态', () => {
      const store = useAppStore()

      store.setSidebarCollapsed(true)
      expect(store.sidebarCollapsed).toBe(true)

      store.setSidebarCollapsed(false)
      expect(store.sidebarCollapsed).toBe(false)
    REDACTED)

    it('toggleMobileSidebar 切换移动端状态', () => {
      const store = useAppStore()
      expect(store.mobileOpen).toBe(false)

      store.toggleMobileSidebar()
      expect(store.mobileOpen).toBe(true)

      store.toggleMobileSidebar()
      expect(store.mobileOpen).toBe(false)
    REDACTED)
  REDACTED)

  // --- Loading 状态 ---

  describe('Loading 状态管理', () => {
    it('setLoading 管理引用计数', () => {
      const store = useAppStore()
      expect(store.loading).toBe(false)

      store.setLoading(true)
      expect(store.loading).toBe(true)

      store.setLoading(true) // 两次 true
      expect(store.loading).toBe(true)

      store.setLoading(false) // 第一次 false，计数还是 1
      expect(store.loading).toBe(true)

      store.setLoading(false) // 第二次 false，计数为 0
      expect(store.loading).toBe(false)
    REDACTED)

    it('setLoading(false) 不会使计数为负', () => {
      const store = useAppStore()

      store.setLoading(false)
      store.setLoading(false)
      expect(store.loading).toBe(false)

      store.setLoading(true)
      expect(store.loading).toBe(true)

      store.setLoading(false)
      expect(store.loading).toBe(false)
    REDACTED)

    it('withLoading 自动管理 loading 状态', async () => {
      const store = useAppStore()

      const result = await store.withLoading(async () => {
        expect(store.loading).toBe(true)
        return 'done'
      REDACTED)

      expect(result).toBe('done')
      expect(store.loading).toBe(false)
    REDACTED)

    it('withLoading 错误时也恢复 loading 状态', async () => {
      const store = useAppStore()

      await expect(
        store.withLoading(async () => {
          throw new Error('操作失败')
        REDACTED)
      ).rejects.toThrow('操作失败')

      expect(store.loading).toBe(false)
    REDACTED)

    it('withLoadingAndError 错误时显示 toast 并返回 null', async () => {
      const store = useAppStore()

      const result = await store.withLoadingAndError(async () => {
        throw new Error('网络错误')
      REDACTED)

      expect(result).toBeNull()
      expect(store.loading).toBe(false)
      expect(store.toasts).toHaveLength(1)
      expect(store.toasts[0].type).toBe('error')
    REDACTED)
  REDACTED)

  // --- reset ---

  describe('reset', () => {
    it('重置所有 UI 状态', () => {
      const store = useAppStore()

      store.setSidebarCollapsed(true)
      store.setLoading(true)
      store.showSuccess('消息')

      store.reset()

      expect(store.sidebarCollapsed).toBe(false)
      expect(store.loading).toBe(false)
      expect(store.toasts).toHaveLength(0)
    REDACTED)
  REDACTED)

  // --- 公开设置 ---

  describe('公开设置加载', () => {
    it('从 window.__APP_CONFIG__ 初始化', () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = {
        site_name: 'TestSite',
        site_logo: '/logo.png',
        version: '1.0.0',
        contact_info: 'test@test.com',
        api_base_url: 'https://api.test.com',
        doc_url: 'https://docs.test.com',
      REDACTED

      const store = useAppStore()
      const result = store.initFromInjectedConfig()

      expect(result).toBe(true)
      expect(store.siteName).toBe('TestSite')
      expect(store.siteLogo).toBe('/logo.png')
      expect(store.siteVersion).toBe('1.0.0')
      expect(store.publicSettingsLoaded).toBe(true)
    REDACTED)

    it('无注入配置时返回 false', () => {
      const store = useAppStore()
      const result = store.initFromInjectedConfig()

      expect(result).toBe(false)
      expect(store.publicSettingsLoaded).toBe(false)
    REDACTED)

    it('clearPublicSettingsCache 清除缓存', () => {
      const windowAny = window as any
      windowAny.__APP_CONFIG__ = { site_name: 'Test' REDACTED
      const store = useAppStore()
      store.initFromInjectedConfig()

      expect(store.publicSettingsLoaded).toBe(true)

      store.clearPublicSettingsCache()

      expect(store.publicSettingsLoaded).toBe(false)
      expect(store.cachedPublicSettings).toBeNull()
    REDACTED)
  REDACTED)
REDACTED)
