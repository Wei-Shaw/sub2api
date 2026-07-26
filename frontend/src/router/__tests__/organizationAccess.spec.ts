import { describe, expect, it } from 'vitest'
import { canAccessOrganizationRoute, canOpenCompanyUpgrade, isIAMFinancialRouteRestricted } from '@/router/organizationAccess'
import type { User } from '@/types'

function user(overrides: Partial<User> = {}): User {
  return { identity_type: 'root', ...overrides } as User
}

describe('organization route access', () => {
  it.each(['/subscriptions', '/purchase', '/orders/1', '/redeem', '/affiliate'])(
    'blocks IAM direct navigation to %s',
    path => expect(isIAMFinancialRouteRestricted(path, user({ identity_type: 'iam' }))).toBe(true),
  )

  it('does not apply IAM financial restrictions to root accounts', () => {
    expect(isIAMFinancialRouteRestricted('/purchase', user())).toBe(false)
  })

  it('only permits personal roots to open company upgrade', () => {
    expect(canOpenCompanyUpgrade(user(), true)).toBe(true)
    expect(canOpenCompanyUpgrade(user({ identity_type: 'iam' }), true)).toBe(false)
    expect(canOpenCompanyUpgrade(user({ organization: { organization_id: 1 } as User['organization'] }), true)).toBe(false)
  })

  it('hides company upgrade when the feature is disabled or unknown', () => {
    expect(canOpenCompanyUpgrade(user(), false)).toBe(false)
    expect(canOpenCompanyUpgrade(user())).toBe(false)
  })

  it('enforces active organization, owner, and action requirements', () => {
    const owner = user({ organization: {
      organization_id: 1, organization_status: 'active', membership_status: 'active', role: 'owner', actions: [],
    } as User['organization'] })
    const financeMember = user({ identity_type: 'iam', organization: {
      organization_id: 1, organization_status: 'active', membership_status: 'active', role: 'member', actions: ['organization.finance.balance.read'],
    } as User['organization'] })
    const suspended = user({ organization: {
      organization_id: 1, organization_status: 'suspended', membership_status: 'active', role: 'owner', actions: [],
    } as User['organization'] })

    expect(canAccessOrganizationRoute(owner, 'organization.finance.balance.read', true)).toBe(true)
    expect(canAccessOrganizationRoute(financeMember, 'organization.finance.balance.read')).toBe(true)
    expect(canAccessOrganizationRoute(financeMember, undefined, true)).toBe(false)
    expect(canAccessOrganizationRoute(financeMember, 'organization.members.manage')).toBe(false)
    expect(canAccessOrganizationRoute(suspended)).toBe(false)
    expect(canAccessOrganizationRoute(user())).toBe(false)
  })
})
