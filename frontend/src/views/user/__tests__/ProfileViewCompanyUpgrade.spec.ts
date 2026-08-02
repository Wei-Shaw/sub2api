import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ProfileView from '@/views/user/ProfileView.vue'

const stores = vi.hoisted(() => ({
  app: {
    cachedPublicSettings: { company_applications_enabled: false } as { company_applications_enabled: boolean },
    fetchPublicSettings: vi.fn(),
  },
  auth: {
    user: { id: 1, identity_type: 'root', role: 'user' } as {
      id: number
      identity_type: 'root' | 'iam'
      role: 'user'
      organization?: object
    },
    refreshUser: vi.fn(),
  },
}))
const currentRoute = vi.hoisted(() => ({ name: 'Profile', path: '/profile' }))

vi.mock('@/stores/app', () => ({ useAppStore: () => stores.app }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => stores.auth }))
vi.mock('@/api/auth', () => ({ isWeChatWebOAuthEnabled: () => false }))
vi.mock('vue-router', () => ({ useRoute: () => currentRoute }))

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
  missingWarn: false,
  fallbackWarn: false,
})

function mountProfile() {
  return mount(ProfileView, {
    global: {
      plugins: [i18n],
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        CompanyUpgradeView: { template: '<div data-testid="company-upgrade-form" class="w-full" />' },
        Icon: true,
        ProfileBalanceNotifyCard: true,
        ProfileIAMRecoveryEmailCard: true,
        ProfileInfoCard: true,
        ProfilePasswordForm: true,
        ProfileTotpCard: true,
        RouterLink: {
          props: ['to'],
          template: '<a :data-to="to"><slot /></a>',
        },
      },
    },
  })
}

describe('ProfileView company upgrade section', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stores.app.cachedPublicSettings = { company_applications_enabled: false }
    stores.app.fetchPublicSettings.mockResolvedValue({ company_applications_enabled: false })
    stores.auth.user = { id: 1, identity_type: 'root', role: 'user' }
    stores.auth.refreshUser.mockResolvedValue(undefined)
    currentRoute.name = 'Profile'
    currentRoute.path = '/profile'
  })

  it('replaces the profile content with the upgrade review page and a back button', async () => {
    stores.app.cachedPublicSettings = { company_applications_enabled: true }
    stores.app.fetchPublicSettings.mockResolvedValue({ company_applications_enabled: true })
    currentRoute.name = 'CompanyUpgrade'
    currentRoute.path = '/profile/company-upgrade'

    const wrapper = mountProfile()
    await flushPromises()

    expect(wrapper.get('[data-testid="company-upgrade-page"]').classes()).toContain('max-w-[950px]')
    expect(wrapper.get('[data-testid="company-upgrade-form"]').classes()).toContain('w-full')
    expect(wrapper.get('[data-testid="back-to-profile"]').attributes('data-to')).toBe('/profile')
    expect(wrapper.find('[data-testid="profile-shell"]').exists()).toBe(false)
  })

  it('keeps the ordinary profile content on the profile route', async () => {
    stores.app.cachedPublicSettings = { company_applications_enabled: true }
    stores.app.fetchPublicSettings.mockResolvedValue({ company_applications_enabled: true })

    const wrapper = mountProfile()
    await flushPromises()

    expect(wrapper.get('[data-testid="profile-shell"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="company-upgrade-page"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="company-upgrade-form"]').exists()).toBe(false)
  })
})
