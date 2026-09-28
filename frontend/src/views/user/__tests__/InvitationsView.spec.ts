import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import InvitationsView from '../InvitationsView.vue'

const { copyToClipboard, getOverview, create, showError, showSuccess } = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  getOverview: vi.fn(),
  create: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/invitations', () => ({
  default: { getOverview, create },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function mountView() {
  return mount(InvitationsView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: true,
      },
    },
  })
}

function overview(overrides: Record<string, unknown> = {}) {
  return {
    enabled: true,
    max_codes_per_user: 3,
    used_count: 1,
    remaining: 2,
    code_validity_days: 7,
    codes: [
      {
        code: 'pending-code',
        status: 'unused',
        created_at: '2026-09-28T00:00:00Z',
        expires_at: '2026-10-05T00:00:00Z',
        used_at: null,
      },
      {
        code: 'used-code',
        status: 'used',
        created_at: '2026-09-20T00:00:00Z',
        expires_at: null,
        used_at: '2026-09-21T00:00:00Z',
        used_by_email_mask: 'fr***@example.com',
      },
    ],
    ...overrides,
  }
}

describe('InvitationsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    copyToClipboard.mockResolvedValue(true)
  })

  it('renders quota and codes, with copy actions only for pending codes', async () => {
    getOverview.mockResolvedValue(overview())
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="invitation-used"]').text()).toContain('1')
    expect(wrapper.get('[data-testid="invitation-used"]').text()).toContain('/ 3')
    expect(wrapper.get('[data-testid="invitation-remaining"]').text()).toBe('2')
    expect(wrapper.text()).toContain('fr***@example.com')

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].findAll('button')).toHaveLength(2)
    expect(rows[1].findAll('button')).toHaveLength(0)

    await rows[0].findAll('button')[1].trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(
      `${window.location.origin}/register?invitation_code=pending-code`,
      'userInvitation.list.linkCopied',
    )
  })

  it('generates a code, reloads the list and copies the invite link', async () => {
    getOverview.mockResolvedValue(overview())
    create.mockResolvedValue({
      code: 'fresh-code',
      status: 'unused',
      created_at: '2026-09-28T00:00:00Z',
      expires_at: null,
      used_at: null,
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="invitation-generate"]').trigger('click')
    await flushPromises()

    expect(create).toHaveBeenCalledTimes(1)
    expect(getOverview).toHaveBeenCalledTimes(2)
    expect(showSuccess).toHaveBeenCalledWith('userInvitation.created')
    expect(copyToClipboard).toHaveBeenCalledWith(
      `${window.location.origin}/register?invitation_code=fresh-code`,
      'userInvitation.list.linkCopied',
    )
  })

  it('disables generation when the feature is off or the quota is exhausted', async () => {
    getOverview.mockResolvedValue(overview({ enabled: false }))
    let wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[data-testid="invitation-generate"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('userInvitation.disabled')

    getOverview.mockResolvedValue(overview({ used_count: 3, remaining: 0 }))
    wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[data-testid="invitation-generate"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('userInvitation.limitReached')
  })

  it('shows unlimited quota when the admin sets no limit', async () => {
    getOverview.mockResolvedValue(overview({ max_codes_per_user: 0, remaining: -1 }))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="invitation-remaining"]').text()).toBe('userInvitation.stats.unlimited')
    expect(wrapper.get('[data-testid="invitation-used"]').text()).not.toContain('/')
    expect(wrapper.get('[data-testid="invitation-generate"]').attributes('disabled')).toBeUndefined()
  })
})
