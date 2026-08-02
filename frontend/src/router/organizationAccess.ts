import type { User } from '@/types'

const iamFinancialRoutePrefixes = [
  '/subscriptions',
  '/purchase',
  '/orders',
  '/redeem',
  '/affiliate',
]

export function isIAMFinancialRouteRestricted(path: string, user?: User | null): boolean {
  if (user?.identity_type !== 'iam') return false
  return iamFinancialRoutePrefixes.some(prefix => path === prefix || path.startsWith(`${prefix}/`))
}

export function canOpenCompanyUpgrade(user?: User | null, featureEnabled = false): boolean {
  return Boolean(featureEnabled && user && user.identity_type !== 'iam' && !user.organization)
}

export function canAccessOrganizationRoute(
  user?: User | null,
  requiredAction?: string,
  ownerRequired = false,
): boolean {
  const organization = user?.organization
  if (!organization || organization.organization_status !== 'active' || organization.membership_status !== 'active') return false
  if (ownerRequired && organization.role !== 'owner') return false
  return organization.role === 'owner' || !requiredAction || organization.actions.includes(requiredAction)
}
