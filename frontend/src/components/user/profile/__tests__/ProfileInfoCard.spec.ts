import { mount REDACTED from '@vue/test-utils'
import { describe, expect, it, vi REDACTED from 'vitest'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import type { User REDACTED from '@/types'

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: null
  REDACTED)
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  REDACTED)
REDACTED))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'profile.administrator') return 'Administrator'
        if (key === 'profile.user') return 'User'
        if (key === 'profile.authBindings.providers.email') return 'Email'
        if (key === 'profile.authBindings.providers.linuxdo') return 'LinuxDo'
        if (key === 'profile.authBindings.providers.wechat') return 'WeChat'
        if (key === 'profile.authBindings.providers.oidc') return params?.providerName || 'OIDC'
        if (key === 'profile.authBindings.source.avatar') {
          return `Avatar synced from ${params?.providerName || 'provider'REDACTED`
        REDACTED
        if (key === 'profile.authBindings.source.username') {
          return `Username synced from ${params?.providerName || 'provider'REDACTED`
        REDACTED
        return key
      REDACTED
    REDACTED)
  REDACTED
REDACTED)

function createUser(overrides: Partial<User> = {REDACTED): User {
  return {
    id: 5,
    username: 'alice',
    email: 'alice@example.com',
    avatar_url: null,
    role: 'user',
    balance: 10,
    concurrency: 2,
    status: 'active',
    allowed_groups: null,
    balance_notify_enabled: true,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-04-20T00:00:00Z',
    updated_at: '2026-04-20T00:00:00Z',
    ...overrides
  REDACTED
REDACTED

describe('ProfileInfoCard', () => {
  it('renders basic account information without avatar or bindings actions', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser()
      REDACTED,
      global: {
        stubs: {
          Icon: true
        REDACTED
      REDACTED
    REDACTED)

    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('User')
    expect(wrapper.find('[data-testid="profile-avatar-save"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-binding-email-status"]').exists()).toBe(false)
  REDACTED)

  it('renders third-party source hints from profile sources', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          avatar_url: 'https://cdn.example.com/linuxdo.png',
          profile_sources: {
            avatar: { provider: 'linuxdo', source: 'linuxdo' REDACTED,
            username: { provider: 'linuxdo', source: 'linuxdo' REDACTED
          REDACTED
        REDACTED)
      REDACTED,
      global: {
        stubs: {
          Icon: true
        REDACTED
      REDACTED
    REDACTED)

    expect(wrapper.text()).toContain('Avatar synced from LinuxDo')
    expect(wrapper.text()).toContain('Username synced from LinuxDo')
  REDACTED)
REDACTED)
