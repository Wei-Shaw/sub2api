import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import type { User } from '@/types'

const copyToClipboardMock = vi.fn().mockResolvedValue(true)

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: copyToClipboardMock
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
        if (key === 'profile.serialNumber') return 'Serial Number'
        if (key === 'common.copy') return 'Copy'
        if (key === 'profile.authBindings.providers.email') return 'Email'
        if (key === 'profile.authBindings.providers.linuxdo') return 'LinuxDo'
        if (key === 'profile.authBindings.providers.wechat') return 'WeChat'
        if (key === 'profile.authBindings.providers.oidc') return params?.providerName || 'OIDC'
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

beforeEach(() => {
  copyToClipboardMock.mockClear()
})

describe('ProfileInfoCard', () => {
  it('renders serial number in the overview hero with a copy action', async () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          device_code: 'DLG-ABCD-1234'
        })
      },
      global: {
        stubs: {
          Icon: true,
          ProfileAvatarCard: true,
          ProfileEditForm: true,
          ProfileIdentityBindingsSection: true
        }
      }
    })

    const hero = wrapper.get('[data-testid="profile-overview-hero"]')
    expect(hero.text()).toContain('Serial Number')
    expect(hero.text()).toContain('DLG-ABCD-1234')
    expect(hero.get('button').attributes('aria-label')).toBe('Copy')

    await hero.get('button').trigger('click')
    expect(copyToClipboardMock).toHaveBeenCalledWith('DLG-ABCD-1234')
  })

  it('keeps the existing profile overview layout and source hints', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          profile_sources: {
            avatar: { provider: 'linuxdo', source: 'linuxdo' },
            username: { provider: 'linuxdo', source: 'linuxdo' }
          }
        })
      },
      global: {
        stubs: {
          Icon: true,
          ProfileAvatarCard: true,
          ProfileEditForm: true,
          ProfileIdentityBindingsSection: true
        }
      }
    })

    expect(wrapper.get('[data-testid="profile-overview-hero"]').text()).toContain('Avatar synced from LinuxDo')
    expect(wrapper.get('[data-testid="profile-overview-hero"]').text()).toContain('Username synced from LinuxDo')
    expect(wrapper.find('[data-testid="profile-basics-panel"]').element).toBeTruthy()
    expect(wrapper.find('[data-testid="profile-auth-bindings-panel"]').element).toBeTruthy()
  })
})
