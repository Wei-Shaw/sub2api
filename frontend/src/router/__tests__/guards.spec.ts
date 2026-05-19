import { describe, it, expect, vi, beforeEach REDACTED from 'vitest'
import { setActivePinia, createPinia REDACTED from 'pinia'
import { resolveCompletedSetupRedirectPath REDACTED from '@/router/setupRedirect'

// Mock 导航加载状态
vi.mock('@/composables/useNavigationLoading', () => {
  const mockStart = vi.fn()
  const mockEnd = vi.fn()
  return {
    useNavigationLoadingState: () => ({
      startNavigation: mockStart,
      endNavigation: mockEnd,
      isLoading: { value: false REDACTED,
    REDACTED),
    useNavigationLoading: () => ({
      startNavigation: mockStart,
      endNavigation: mockEnd,
      isLoading: { value: false REDACTED,
    REDACTED),
  REDACTED
REDACTED)

// Mock 路由预加载
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  REDACTED),
REDACTED))

// Mock API 相关模块
vi.mock('@/api', () => ({
  authAPI: {
    getCurrentUser: vi.fn().mockResolvedValue({ data: {REDACTED REDACTED),
    logout: vi.fn(),
  REDACTED,
  isTotp2FARequired: () => false,
REDACTED))

vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn(),
REDACTED))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn(),
REDACTED))


// 用于测试的 auth 状态
interface MockAuthState {
  isAuthenticated: boolean
  isAdmin: boolean
  isSimpleMode: boolean
  backendModeEnabled: boolean
  hasPendingAuthSession: boolean
  setupNeedsSetup?: boolean
REDACTED

/**
 * 将 router/index.ts 中 beforeEach 守卫的核心逻辑提取为可测试的函数
 */
function simulateGuard(
  toPath: string,
  toMeta: Record<string, any>,
  authState: MockAuthState
): string | null {
  const requiresAuth = toMeta.requiresAuth !== false
  const requiresAdmin = toMeta.requiresAdmin === true

  if (toPath === '/setup' && authState.setupNeedsSetup === false) {
    return resolveCompletedSetupRedirectPath(authState.isAuthenticated, authState.isAdmin)
  REDACTED

  // 不需要认证的路由
  if (!requiresAuth) {
    if (
      authState.isAuthenticated &&
      (toPath === '/login' || toPath === '/register')
    ) {
      if (authState.backendModeEnabled && !authState.isAdmin) {
        return null
      REDACTED
      return authState.isAdmin ? '/admin/dashboard' : '/dashboard'
    REDACTED
    if (authState.backendModeEnabled && !authState.isAuthenticated) {
      const allowed = ['/login', '/key-usage', '/setup', '/payment/result']
      const callbackPaths = [
        '/auth/callback',
        '/auth/linuxdo/callback',
        '/auth/oidc/callback',
        '/auth/wechat/callback',
        '/auth/wechat/payment/callback',
      ]
      const pendingAuthPaths = ['/register', '/email-verify']
      const isAllowed =
        allowed.some((path) => toPath === path || toPath.startsWith(path)) ||
        callbackPaths.includes(toPath) ||
        (authState.hasPendingAuthSession && pendingAuthPaths.includes(toPath))
      if (!isAllowed) {
        return '/login'
      REDACTED
    REDACTED
    return null // 允许通过
  REDACTED

  // 需要认证但未登录
  if (!authState.isAuthenticated) {
    return '/login'
  REDACTED

  // 需要管理员但不是管理员
  if (requiresAdmin && !authState.isAdmin) {
    return '/dashboard'
  REDACTED

  // 简易模式限制
  if (authState.isSimpleMode) {
    const restrictedPaths = [
      '/admin/groups',
      '/admin/subscriptions',
      '/admin/redeem',
      '/subscriptions',
      '/redeem',
    ]
    if (restrictedPaths.some((path) => toPath.startsWith(path))) {
      return authState.isAdmin ? '/admin/dashboard' : '/dashboard'
    REDACTED
  REDACTED

  // Backend mode: admin gets full access, non-admin blocked
  if (authState.backendModeEnabled) {
    if (authState.isAuthenticated && authState.isAdmin) {
      return null
    REDACTED
    const allowed = ['/login', '/key-usage', '/setup', '/payment/result']
    const callbackPaths = [
      '/auth/callback',
      '/auth/linuxdo/callback',
      '/auth/oidc/callback',
      '/auth/wechat/callback',
      '/auth/wechat/payment/callback',
    ]
    const pendingAuthPaths = ['/register', '/email-verify']
    const isAllowed =
      allowed.some((path) => toPath === path || toPath.startsWith(path)) ||
      callbackPaths.includes(toPath) ||
      (authState.hasPendingAuthSession && pendingAuthPaths.includes(toPath))
    if (!isAllowed) {
      return '/login'
    REDACTED
  REDACTED

  return null // 允许通过
REDACTED

describe('路由守卫逻辑', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  REDACTED)

  // --- 未认证用户 ---

  describe('未认证用户', () => {
    const authState: MockAuthState = {
      isAuthenticated: false,
      isAdmin: false,
      isSimpleMode: false,
      backendModeEnabled: false,
      hasPendingAuthSession: false,
    REDACTED

    it('访问需要认证的页面重定向到 /login', () => {
      const redirect = simulateGuard('/dashboard', {REDACTED, authState)
      expect(redirect).toBe('/login')
    REDACTED)

    it('访问管理页面重定向到 /login', () => {
      const redirect = simulateGuard('/admin/dashboard', { requiresAdmin: true REDACTED, authState)
      expect(redirect).toBe('/login')
    REDACTED)

    it('访问公开页面允许通过', () => {
      const redirect = simulateGuard('/login', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('访问 /home 公开页面允许通过', () => {
      const redirect = simulateGuard('/home', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)
  REDACTED)

  // --- 已认证普通用户 ---

  describe('已认证普通用户', () => {
    const authState: MockAuthState = {
      isAuthenticated: true,
      isAdmin: false,
      isSimpleMode: false,
      backendModeEnabled: false,
      hasPendingAuthSession: false,
    REDACTED

    it('访问 /login 重定向到 /dashboard', () => {
      const redirect = simulateGuard('/login', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/dashboard')
    REDACTED)

    it('访问 /register 重定向到 /dashboard', () => {
      const redirect = simulateGuard('/register', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/dashboard')
    REDACTED)

    it('访问 /dashboard 允许通过', () => {
      const redirect = simulateGuard('/dashboard', {REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('访问管理页面被拒绝，重定向到 /dashboard', () => {
      const redirect = simulateGuard('/admin/dashboard', { requiresAdmin: true REDACTED, authState)
      expect(redirect).toBe('/dashboard')
    REDACTED)

    it('访问 /admin/users 被拒绝', () => {
      const redirect = simulateGuard('/admin/users', { requiresAdmin: true REDACTED, authState)
      expect(redirect).toBe('/dashboard')
    REDACTED)
  REDACTED)

  // --- 已认证管理员 ---

  describe('已认证管理员', () => {
    const authState: MockAuthState = {
      isAuthenticated: true,
      isAdmin: true,
      isSimpleMode: false,
      backendModeEnabled: false,
      hasPendingAuthSession: false,
    REDACTED

    it('访问 /login 重定向到 /admin/dashboard', () => {
      const redirect = simulateGuard('/login', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/admin/dashboard')
    REDACTED)

    it('访问管理页面允许通过', () => {
      const redirect = simulateGuard('/admin/dashboard', { requiresAdmin: true REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('访问用户页面允许通过', () => {
      const redirect = simulateGuard('/dashboard', {REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)
  REDACTED)

  // --- 简易模式 ---

  describe('简易模式受限路由', () => {
    it('普通用户简易模式访问 /subscriptions 重定向到 /dashboard', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: false,
        isSimpleMode: true,
        backendModeEnabled: false,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/subscriptions', {REDACTED, authState)
      expect(redirect).toBe('/dashboard')
    REDACTED)

    it('普通用户简易模式访问 /redeem 重定向到 /dashboard', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: false,
        isSimpleMode: true,
        backendModeEnabled: false,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/redeem', {REDACTED, authState)
      expect(redirect).toBe('/dashboard')
    REDACTED)

    it('管理员简易模式访问 /admin/groups 重定向到 /admin/dashboard', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: true,
        isSimpleMode: true,
        backendModeEnabled: false,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/admin/groups', { requiresAdmin: true REDACTED, authState)
      expect(redirect).toBe('/admin/dashboard')
    REDACTED)

    it('管理员简易模式访问 /admin/subscriptions 重定向', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: true,
        isSimpleMode: true,
        backendModeEnabled: false,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard(
        '/admin/subscriptions',
        { requiresAdmin: true REDACTED,
        authState
      )
      expect(redirect).toBe('/admin/dashboard')
    REDACTED)

    it('简易模式下非受限页面正常访问', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: false,
        isSimpleMode: true,
        backendModeEnabled: false,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/dashboard', {REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('简易模式下 /keys 正常访问', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: false,
        isSimpleMode: true,
        backendModeEnabled: false,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/keys', {REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)
  REDACTED)

  describe('Backend Mode', () => {
    it('unauthenticated: /home redirects to /login', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/home', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/login')
    REDACTED)

    it('unauthenticated: /login is allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/login', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: /key-usage is allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/key-usage', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: /setup is allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/setup', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: initialized /setup redirects to /login', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
        setupNeedsSetup: false,
      REDACTED
      const redirect = simulateGuard('/setup', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/login')
    REDACTED)

    it('admin: initialized /setup redirects to /admin/dashboard', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: true,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
        setupNeedsSetup: false,
      REDACTED
      const redirect = simulateGuard('/setup', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/admin/dashboard')
    REDACTED)

    it('admin: /admin/dashboard is allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: true,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/admin/dashboard', { requiresAdmin: true REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('admin: /login redirects to /admin/dashboard', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: true,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/login', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/admin/dashboard')
    REDACTED)

    it('non-admin authenticated: /dashboard redirects to /login', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/dashboard', {REDACTED, authState)
      expect(redirect).toBe('/login')
    REDACTED)

    it('non-admin authenticated: /login is allowed (no redirect loop)', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/login', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('non-admin authenticated: /key-usage is allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: true,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/key-usage', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: callback routes are allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/auth/wechat/callback', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: WeChat payment callback route is allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/auth/wechat/payment/callback', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: /payment/result is allowed', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/payment/result', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: /register is allowed when a pending auth session exists', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: true,
      REDACTED
      const redirect = simulateGuard('/register', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBeNull()
    REDACTED)

    it('unauthenticated: /email-verify is blocked without a pending auth session', () => {
      const authState: MockAuthState = {
        isAuthenticated: false,
        isAdmin: false,
        isSimpleMode: false,
        backendModeEnabled: true,
        hasPendingAuthSession: false,
      REDACTED
      const redirect = simulateGuard('/email-verify', { requiresAuth: false REDACTED, authState)
      expect(redirect).toBe('/login')
    REDACTED)
  REDACTED)
REDACTED)
