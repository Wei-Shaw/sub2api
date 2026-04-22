import { describe, expect, it, vi REDACTED from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
REDACTED))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  cachedPublicSettings: null as null | Record<string, unknown>,
REDACTED))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
REDACTED))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    customMenuItems: [],
  REDACTED),
REDACTED))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false REDACTED,
  REDACTED),
REDACTED))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  REDACTED),
REDACTED))

describe('router WeChat OAuth route', () => {
  it('registers the WeChat callback route as a public route', async () => {
    const { default: router REDACTED = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === 'WeChatOAuthCallback')

    expect(route?.path).toBe('/auth/wechat/callback')
    expect(route?.meta.requiresAuth).toBe(false)
    expect(route?.meta.title).toBe('WeChat OAuth Callback')
  REDACTED)

  it('registers the WeChat payment callback route as a public route', async () => {
    const { default: router REDACTED = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === 'WeChatPaymentOAuthCallback')

    expect(route?.path).toBe('/auth/wechat/payment/callback')
    expect(route?.meta.requiresAuth).toBe(false)
    expect(route?.meta.title).toBe('WeChat Payment Callback')
  REDACTED)
REDACTED)
