import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import ProfileView from '@/views/user/ProfileView.vue'

const {
  fetchPublicSettingsMock,
  refreshUserMock,
  authState
REDACTED = vi.hoisted(() => ({
  fetchPublicSettingsMock: vi.fn(),
  refreshUserMock: vi.fn(),
  authState: {
    user: null as Record<string, unknown> | null,
    refreshUser: vi.fn()
  REDACTED
REDACTED))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    fetchPublicSettings: fetchPublicSettingsMock
  REDACTED)
REDACTED))

vi.mock('@/utils/format', () => ({
  formatDate: () => 'April 2026'
REDACTED))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

describe('ProfileView', () => {
  beforeEach(() => {
    refreshUserMock.mockReset()
    fetchPublicSettingsMock.mockReset()
    refreshUserMock.mockResolvedValue(undefined)
    authState.refreshUser = refreshUserMock
    authState.user = {
      id: 1,
      username: 'alice',
      email: 'alice@example.com',
      role: 'user',
      balance: 10,
      concurrency: 2,
      status: 'active',
      allowed_groups: null,
      balance_notify_enabled: true,
      balance_notify_threshold: null,
      balance_notify_extra_emails: [],
      created_at: '2026-04-20T00:00:00Z',
      updated_at: '2026-04-20T00:00:00Z'
    REDACTED
    fetchPublicSettingsMock.mockResolvedValue({
      contact_info: '',
      balance_low_notify_enabled: false,
      balance_low_notify_threshold: 0,
      linuxdo_oauth_enabled: true,
      wechat_oauth_enabled: true,
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
      oidc_oauth_enabled: true,
      oidc_oauth_provider_name: 'OIDC'
    REDACTED)
  REDACTED)

  it('renders info, avatar, and account binding cards as separate sections', async () => {
    const wrapper = mount(ProfileView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' REDACTED,
          StatCard: { template: '<div class="stat-card" />' REDACTED,
          ProfileInfoCard: { template: '<div data-testid="profile-info-card" />' REDACTED,
          ProfileAvatarCard: { template: '<div data-testid="profile-avatar-card" />' REDACTED,
          ProfileAccountBindingsCard: { template: '<div data-testid="profile-account-bindings-card" />' REDACTED,
          ProfileEditForm: true,
          ProfileBalanceNotifyCard: true,
          ProfilePasswordForm: true,
          ProfileTotpCard: true,
          Icon: true
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()

    const html = wrapper.html()
    expect(html.indexOf('profile-info-card')).toBeGreaterThan(-1)
    expect(html.indexOf('profile-avatar-card')).toBeGreaterThan(html.indexOf('profile-info-card'))
    expect(html.indexOf('profile-account-bindings-card')).toBeGreaterThan(html.indexOf('profile-avatar-card'))
  REDACTED)
REDACTED)
