import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import type { User } from '@/types'

vi.mock('vue-router', () => ({
  useRoute: () => ({
    fullPath: '/profile'
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: null
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'profile.accountBalance') return 'Account Balance'
        if (key === 'profile.concurrencyLimit') return 'Concurrency Limit'
        if (key === 'profile.memberSince') return 'Member Since'
        if (key === 'profile.administrator') return 'Administrator'
        if (key === 'profile.user') return 'User'
        if (key === 'profile.authBindings.providers.email') return 'Email'
        if (key === 'profile.authBindings.providers.linuxdo') return 'LinuxDo'
        if (key === 'profile.authBindings.providers.wechat') return 'WeChat'
        if (key === 'profile.authBindings.providers.oidc') return params?.providerName || 'OIDC'
        if (key === 'organization.accountId') return 'Account ID'
        if (key === 'organization.accountType.label') return 'Account type'
        if (key === 'organization.accountType.personal') return 'Personal account'
        if (key === 'organization.accountType.company') return 'Company account'
        if (key === 'organization.accountIdentity.label') return 'Account identity'
        if (key === 'organization.accountIdentity.root') return 'Main account'
        if (key === 'organization.accountIdentity.iam') return 'Sub-account'
        if (key === 'organization.upgrade.title') return 'Upgrade to company account'
        if (key === 'profile.authBindings.source.avatar') {
          return `Avatar synced from ${params?.providerName || 'provider'}`
        }
        if (key === 'profile.authBindings.source.username') {
          return `Username synced from ${params?.providerName || 'provider'}`
        }
        return key
      }
    })
  }
})

function createUser(overrides: Partial<User> = {}): User {
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
  }
}

describe('ProfileInfoCard', () => {
  it('renders basic account information inside the new overview shell', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser()
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        }
      }
    })

    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('User')
    expect(wrapper.get('[data-testid="profile-basics-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-auth-bindings-panel"]').exists()).toBe(true)
  })

  it('shows account ID, personal account type, and the company upgrade action', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({ account_id: '1719905235756637', identity_type: 'root' }),
        companyApplicationsEnabled: true,
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: {
            props: ['to'],
            template: '<a :data-to="to"><slot /></a>',
          },
        },
      },
    })

    expect(wrapper.get('[data-testid="profile-account-id"]').text()).toBe('1719905235756637')
    expect(wrapper.get('[data-testid="profile-account-type"]').text()).toBe('Personal account')
    expect(wrapper.get('[data-testid="profile-account-identity-type"]').text()).toBe('Main account')
    expect(wrapper.get('[data-testid="profile-company-upgrade"]').attributes('data-to')).toBe('/profile/company-upgrade')
    expect(wrapper.get('[data-testid="profile-company-upgrade"]').classes()).toContain('btn-sm')
    expect(wrapper.get('[data-testid="profile-account-identity"]').element.parentElement).toBe(
      wrapper.get('[data-testid="profile-primary-email"]').element.parentElement,
    )
  })

  it('shows company account type without the upgrade action for organization users', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          account_id: '1719905235756637',
          identity_type: 'root',
          organization: { organization_id: 1 } as User['organization'],
        }),
        companyApplicationsEnabled: true,
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        },
      },
    })

    expect(wrapper.get('[data-testid="profile-account-type"]').text()).toBe('Company account')
    expect(wrapper.find('[data-testid="profile-company-upgrade"]').exists()).toBe(false)
  })

  it('identifies IAM users as sub-accounts', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          account_id: '1719905235756637',
          external_user_id: '201705485041478971',
          identity_type: 'iam',
          organization: { organization_id: 1 } as User['organization'],
        }),
        companyApplicationsEnabled: true,
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        },
      },
    })

    expect(wrapper.get('[data-testid="profile-account-type"]').text()).toBe('Company account')
    expect(wrapper.get('[data-testid="profile-account-identity-type"]').text()).toBe('Sub-account')
    expect(wrapper.find('[data-testid="profile-company-upgrade"]').exists()).toBe(false)
  })

  it('renders third-party source hints from profile sources', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          avatar_url: 'https://cdn.example.com/linuxdo.png',
          profile_sources: {
            avatar: { provider: 'linuxdo', source: 'linuxdo' },
            username: { provider: 'linuxdo', source: 'linuxdo' }
          }
        })
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        }
      }
    })

    expect(wrapper.text()).toContain('Avatar synced from LinuxDo')
    expect(wrapper.text()).toContain('Username synced from LinuxDo')
  })

  it('uses the configured OIDC provider name in source hints', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          profile_sources: {
            username: { provider: 'oidc', source: 'oidc' }
          }
        }),
        oidcProviderName: 'ExampleID'
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        }
      }
    })

    expect(wrapper.text()).toContain('Username synced from ExampleID')
  })

  it('does not display synthetic oauth-only emails as a real bound email', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          email: 'legacy-user@oidc-connect.invalid',
          email_bound: false,
          auth_bindings: {
            email: { bound: false }
          }
        })
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        }
      }
    })

    expect(wrapper.text()).not.toContain('legacy-user@oidc-connect.invalid')
  })

  it('does not display synthetic oauth-only emails when only legacy identity bindings mark email as unbound', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          email: 'legacy-user@wechat-connect.invalid',
          identity_bindings: {
            email: { bound: false }
          }
        })
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        }
      }
    })

    expect(wrapper.text()).not.toContain('legacy-user@wechat-connect.invalid')
  })

  it('renders the approved overview hero and two-column content shell', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser()
      },
      global: {
        stubs: {
          Icon: true,
          RouterLink: true,
        }
      }
    })

    expect(wrapper.get('[data-testid="profile-overview-hero"]').text()).toContain('alice@example.com')
    expect(wrapper.get('[data-testid="profile-overview-metric-balance"]').text()).toContain('Account Balance')
    expect(wrapper.get('[data-testid="profile-overview-metric-concurrency"]').text()).toContain('Concurrency Limit')
    expect(wrapper.get('[data-testid="profile-overview-metric-member-since"]').text()).toContain('Member Since')
    expect(wrapper.find('[data-testid="profile-info-summary-grid"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="profile-main-column"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-side-column"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-basics-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-auth-bindings-panel"]').exists()).toBe(true)
  })
})
