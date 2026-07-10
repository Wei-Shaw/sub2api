import { beforeAll, beforeEach, describe, expect, it, vi REDACTED from 'vitest'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
REDACTED))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: true,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
REDACTED))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: false,
  cachedPublicSettings: null as null | {
    payment_enabled?: boolean
    risk_control_enabled?: boolean
    custom_menu_items?: []
  REDACTED,
  fetchPublicSettings: vi.fn(),
REDACTED))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({REDACTED)),
  createRouter: vi.fn(() => ({
    beforeEach: vi.fn((guard: NavigationGuard) => {
      routerHarness.guard = guard
    REDACTED),
    afterEach: vi.fn(),
    onError: vi.fn(),
  REDACTED)),
REDACTED))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
REDACTED))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] REDACTED),
REDACTED))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
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

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  REDACTED)
  return { promise, resolve REDACTED
REDACTED

function runGuard(meta: Record<string, unknown>, path: string) {
  if (!routerHarness.guard) {
    throw new Error('router guard was not registered')
  REDACTED

  const next = vi.fn()
  const navigation = routerHarness.guard(
    {
      path,
      fullPath: path,
      name: 'FeatureRoute',
      params: {REDACTED,
      meta: { requiresAuth: true, ...meta REDACTED,
    REDACTED,
    {REDACTED,
    next
  )
  return { navigation, next REDACTED
REDACTED

describe('feature route guard', () => {
  beforeAll(async () => {
    await import('@/router')
  REDACTED)

  beforeEach(() => {
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.fetchPublicSettings.mockReset()
  REDACTED)

  it('waits for the first public-settings request before deciding payment access', async () => {
    const deferred = createDeferred<{ payment_enabled: boolean REDACTED>()
    appStore.fetchPublicSettings.mockImplementation(async () => {
      const settings = await deferred.promise
      appStore.cachedPublicSettings = settings
      appStore.publicSettingsLoaded = true
      return settings
    REDACTED)

    const { navigation, next REDACTED = runGuard({ requiresPayment: true REDACTED, '/purchase')

    await vi.waitFor(() => expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1))
    expect(next).not.toHaveBeenCalled()

    deferred.resolve({ payment_enabled: true REDACTED)
    await navigation
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  REDACTED)

  it.each([
    ['payment', { requiresPayment: true REDACTED, '/purchase'],
    ['risk control', { requiresRiskControl: true REDACTED, '/admin/risk-control'],
  ])('does not treat a failed %s settings load as explicitly disabled', async (_name, meta, path) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next REDACTED = runGuard(meta, path)
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  REDACTED)

  it.each([
    ['payment', { requiresPayment: true REDACTED, { payment_enabled: false REDACTED, '/dashboard'],
    [
      'risk control',
      { requiresRiskControl: true REDACTED,
      { risk_control_enabled: false REDACTED,
      '/admin/settings',
    ],
  ])('redirects when loaded settings explicitly disable %s', async (_name, meta, settings, target) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.cachedPublicSettings = settings
    appStore.publicSettingsLoaded = true

    const { navigation, next REDACTED = runGuard(meta, '/feature')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith(target)
  REDACTED)
REDACTED)
