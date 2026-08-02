import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppHeader from '@/components/layout/AppHeader.vue'

const stores = vi.hoisted(() => ({
  app: {
    cachedPublicSettings: { company_applications_enabled: false } as { company_applications_enabled: boolean },
    contactInfo: '',
    docUrl: '',
    toggleMobileSidebar: vi.fn(),
  },
  auth: {
    user: { id: 1, identity_type: 'root', role: 'user', email: 'root@example.com' },
    isAdmin: false,
    isSimpleMode: false,
    logout: vi.fn(),
  },
  onboarding: { replay: vi.fn() },
  adminSettings: { customMenuItems: [] },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => stores.app,
  useAuthStore: () => stores.auth,
  useOnboardingStore: () => stores.onboarding,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => stores.adminSettings,
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ name: 'Dashboard', params: {}, meta: {}, path: '/dashboard' }),
}))

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
  missingWarn: false,
  fallbackWarn: false,
})

function mountHeader() {
  return mount(AppHeader, {
    global: {
      plugins: [i18n],
      stubs: {
        AnnouncementBell: true,
        HomeProductsMenu: true,
        Icon: true,
        LocaleSwitcher: true,
        SubscriptionProgressMini: true,
        RouterLink: {
          props: ['to'],
          template: '<a :data-to="to"><slot /></a>',
        },
      },
    },
  })
}

describe('AppHeader company upgrade menu', () => {
  beforeEach(() => {
    stores.app.cachedPublicSettings = { company_applications_enabled: false }
    stores.auth.user = { id: 1, identity_type: 'root', role: 'user', email: 'root@example.com' }
  })

  it('only shows upgrade when company applications are enabled', async () => {
    const disabled = mountHeader()
    await disabled.get('button[aria-label="common.userMenu"]').trigger('click')
    expect(disabled.find('[data-to="/profile/company-upgrade"]').exists()).toBe(false)

    stores.app.cachedPublicSettings = { company_applications_enabled: true }
    const enabled = mountHeader()
    await enabled.get('button[aria-label="common.userMenu"]').trigger('click')
    const upgradeLink = enabled.get('[data-to="/profile/company-upgrade"]')
    const profileLink = enabled.get('[data-to="/profile"]')
    expect(upgradeLink.element.compareDocumentPosition(profileLink.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(enabled.get('[data-testid="company-upgrade-menu-section"]').classes()).toContain('border-b')
  })

  it('shows the account ID immediately below the user email', async () => {
    stores.auth.user = {
      id: 1,
      identity_type: 'root',
      role: 'user',
      email: 'root@example.com',
      account_id: '1719905235756637',
    }

    const wrapper = mountHeader()
    await wrapper.get('button[aria-label="common.userMenu"]').trigger('click')
    const email = wrapper.get('[data-testid="user-menu-email"]')
    const accountID = wrapper.get('[data-testid="user-menu-account-id"]')
    expect(accountID.text()).toContain('1719905235756637')
    expect(accountID.element.previousElementSibling).toBe(email.element)
    expect(wrapper.get('[data-testid="user-menu-account-identity"]').text()).toContain('organization.accountIdentity.root')
  })

  it('identifies IAM users as sub-accounts in the user menu', async () => {
    stores.auth.user = {
      id: 2,
      identity_type: 'iam',
      role: 'user',
      email: '',
      account_id: '1719905235756637',
      external_user_id: '201705485041478971',
    }

    const wrapper = mountHeader()
    await wrapper.get('button[aria-label="common.userMenu"]').trigger('click')
    expect(wrapper.get('[data-testid="user-menu-account-identity"]').text()).toContain('organization.accountIdentity.iam')
  })
})
