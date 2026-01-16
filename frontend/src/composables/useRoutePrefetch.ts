/**
 * 路由预加载组合式函数
 * 在浏览器空闲时预加载可能访问的下一个页面，提升导航体验
 */
import { ref, readonly REDACTED from 'vue'
import type { RouteLocationNormalized, RouteRecordRaw REDACTED from 'vue-router'

/**
 * 组件导入函数类型
 */
type ComponentImportFn = () => Promise<unknown>

/**
 * 预加载配置类型
 */
interface PrefetchConfig {
  [path: string]: ComponentImportFn[]
REDACTED

/**
 * 路由预加载元数据扩展
 * 在路由 meta 中可以指定 prefetch 配置
 */
declare module 'vue-router' {
  interface RouteMeta {
    /** 需要预加载的路由路径列表 */
    prefetch?: string[]
  REDACTED
REDACTED

/**
 * requestIdleCallback 的返回类型
 * 在支持的浏览器中返回 number，polyfill 中使用 ReturnType<typeof setTimeout>
 */
type IdleCallbackHandle = number | ReturnType<typeof setTimeout>

/**
 * requestIdleCallback polyfill
 * Safari < 15 不支持 requestIdleCallback
 */
const scheduleIdleCallback = (
  callback: IdleRequestCallback,
  options?: IdleRequestOptions
): IdleCallbackHandle => {
  if (typeof window.requestIdleCallback === 'function') {
    return window.requestIdleCallback(callback, options)
  REDACTED
  // Fallback: 使用 setTimeout 模拟，延迟 1 秒执行
  return setTimeout(() => {
    callback({
      didTimeout: false,
      timeRemaining: () => 50
    REDACTED)
  REDACTED, 1000)
REDACTED

const cancelScheduledCallback = (handle: IdleCallbackHandle): void => {
  if (typeof window.cancelIdleCallback === 'function' && typeof handle === 'number') {
    window.cancelIdleCallback(handle)
  REDACTED else {
    clearTimeout(handle)
  REDACTED
REDACTED

/**
 * 从路由配置自动生成预加载映射表
 * 根据路由的 meta.prefetch 配置和同级路由自动生成
 *
 * @param routes - 路由配置数组
 * @returns 预加载映射表
 */
export function generatePrefetchMap(routes: RouteRecordRaw[]): PrefetchConfig {
  const prefetchMap: PrefetchConfig = {REDACTED
  const routeComponentMap = new Map<string, ComponentImportFn>()

  // 第一遍：收集所有路由的组件导入函数
  const collectComponents = (routeList: RouteRecordRaw[], prefix = '') => {
    for (const route of routeList) {
      if (route.redirect) continue

      const fullPath = prefix + route.path
      if (route.component && typeof route.component === 'function') {
        routeComponentMap.set(fullPath, route.component as ComponentImportFn)
      REDACTED

      // 递归处理子路由
      if (route.children) {
        collectComponents(route.children, fullPath)
      REDACTED
    REDACTED
  REDACTED

  collectComponents(routes)

  // 第二遍：根据 meta.prefetch 或同级路由生成预加载映射
  const generateMapping = (routeList: RouteRecordRaw[], siblings: RouteRecordRaw[] = []) => {
    for (let i = 0; i < routeList.length; i++) {
      const route = routeList[i]
      if (route.redirect || !route.component) continue

      const path = route.path
      const prefetchPaths: string[] = []

      // 优先使用 meta.prefetch 配置
      if (route.meta?.prefetch && Array.isArray(route.meta.prefetch)) {
        prefetchPaths.push(...route.meta.prefetch)
      REDACTED else {
        // 自动预加载相邻的同级路由（前后各一个）
        const siblingRoutes = siblings.length > 0 ? siblings : routeList
        const currentIndex = siblingRoutes.findIndex((r) => r.path === path)

        if (currentIndex > 0) {
          const prev = siblingRoutes[currentIndex - 1]
          if (prev && !prev.redirect && prev.component) {
            prefetchPaths.push(prev.path)
          REDACTED
        REDACTED
        if (currentIndex < siblingRoutes.length - 1) {
          const next = siblingRoutes[currentIndex + 1]
          if (next && !next.redirect && next.component) {
            prefetchPaths.push(next.path)
          REDACTED
        REDACTED
      REDACTED

      // 转换为组件导入函数
      const importFns: ComponentImportFn[] = []
      for (const prefetchPath of prefetchPaths) {
        const importFn = routeComponentMap.get(prefetchPath)
        if (importFn) {
          importFns.push(importFn)
        REDACTED
      REDACTED

      if (importFns.length > 0) {
        prefetchMap[path] = importFns
      REDACTED

      // 递归处理子路由
      if (route.children) {
        generateMapping(route.children, route.children)
      REDACTED
    REDACTED
  REDACTED

  // 分别处理用户路由和管理员路由
  const userRoutes = routes.filter(
    (r) => !r.path.startsWith('/admin') && !r.path.startsWith('/auth') && !r.path.startsWith('/setup')
  )
  const adminRoutes = routes.filter((r) => r.path.startsWith('/admin'))

  generateMapping(userRoutes, userRoutes)
  generateMapping(adminRoutes, adminRoutes)

  return prefetchMap
REDACTED

/**
 * 默认预加载映射表（手动配置，优先级更高）
 * 可以覆盖自动生成的映射
 */
const defaultAdminPrefetchMap: PrefetchConfig = {
  '/admin/dashboard': [
    () => import('@/views/admin/AccountsView.vue'),
    () => import('@/views/admin/UsersView.vue')
  ],
  '/admin/accounts': [
    () => import('@/views/admin/DashboardView.vue'),
    () => import('@/views/admin/UsersView.vue')
  ],
  '/admin/users': [
    () => import('@/views/admin/GroupsView.vue'),
    () => import('@/views/admin/DashboardView.vue')
  ]
REDACTED

const defaultUserPrefetchMap: PrefetchConfig = {
  '/dashboard': [
    () => import('@/views/user/KeysView.vue'),
    () => import('@/views/user/UsageView.vue')
  ],
  '/keys': [
    () => import('@/views/user/DashboardView.vue'),
    () => import('@/views/user/UsageView.vue')
  ],
  '/usage': [
    () => import('@/views/user/KeysView.vue'),
    () => import('@/views/user/RedeemView.vue')
  ]
REDACTED

/**
 * 路由预加载组合式函数
 *
 * @param customPrefetchMap - 自定义预加载映射表（可选）
 */
export function useRoutePrefetch(customPrefetchMap?: PrefetchConfig) {
  // 合并预加载映射表：自定义 > 默认管理员 > 默认用户
  const prefetchMap: PrefetchConfig = {
    ...defaultUserPrefetchMap,
    ...defaultAdminPrefetchMap,
    ...customPrefetchMap
  REDACTED

  // 当前挂起的预加载任务句柄
  const pendingPrefetchHandle = ref<IdleCallbackHandle | null>(null)

  // 已预加载的路由集合（避免重复预加载）
  const prefetchedRoutes = ref<Set<string>>(new Set())

  /**
   * 判断是否为管理员路由
   */
  const isAdminRoute = (path: string): boolean => {
    return path.startsWith('/admin')
  REDACTED

  /**
   * 获取当前路由对应的预加载配置
   */
  const getPrefetchConfig = (route: RouteLocationNormalized): ComponentImportFn[] => {
    return prefetchMap[route.path] || []
  REDACTED

  /**
   * 执行单个组件的预加载
   * 静默处理错误，不影响页面功能
   */
  const prefetchComponent = async (importFn: ComponentImportFn): Promise<void> => {
    try {
      await importFn()
    REDACTED catch (error) {
      // 静默处理预加载错误
      if (import.meta.env.DEV) {
        console.debug('[Prefetch] Failed to prefetch component:', error)
      REDACTED
    REDACTED
  REDACTED

  /**
   * 取消挂起的预加载任务
   */
  const cancelPendingPrefetch = (): void => {
    if (pendingPrefetchHandle.value !== null) {
      cancelScheduledCallback(pendingPrefetchHandle.value)
      pendingPrefetchHandle.value = null
    REDACTED
  REDACTED

  /**
   * 触发路由预加载
   * 在浏览器空闲时执行，超时 2 秒后强制执行
   */
  const triggerPrefetch = (route: RouteLocationNormalized): void => {
    // 取消之前的预加载任务
    cancelPendingPrefetch()

    const prefetchList = getPrefetchConfig(route)
    if (prefetchList.length === 0) {
      return
    REDACTED

    // 在浏览器空闲时执行预加载
    pendingPrefetchHandle.value = scheduleIdleCallback(
      () => {
        pendingPrefetchHandle.value = null

        // 过滤掉已预加载的组件
        const routePath = route.path
        if (prefetchedRoutes.value.has(routePath)) {
          return
        REDACTED

        // 执行预加载
        Promise.all(prefetchList.map(prefetchComponent)).then(() => {
          prefetchedRoutes.value.add(routePath)
        REDACTED)
      REDACTED,
      { timeout: 2000 REDACTED // 2 秒超时
    )
  REDACTED

  /**
   * 重置预加载状态（用于测试）
   */
  const resetPrefetchState = (): void => {
    cancelPendingPrefetch()
    prefetchedRoutes.value.clear()
  REDACTED

  return {
    prefetchedRoutes: readonly(prefetchedRoutes),
    triggerPrefetch,
    cancelPendingPrefetch,
    resetPrefetchState,
    // 导出用于测试
    _getPrefetchConfig: getPrefetchConfig,
    _isAdminRoute: isAdminRoute
  REDACTED
REDACTED

// 导出预加载映射表（用于测试）
export const _adminPrefetchMap = defaultAdminPrefetchMap
export const _userPrefetchMap = defaultUserPrefetchMap
