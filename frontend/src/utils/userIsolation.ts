import type { AccountPlatform, AccountType } from '@/types'

export const USER_ISOLATION_EXTRA_KEY = 'user_isolation_enabled'

export type UserIsolationRisk = 'oauth' | 'coding_plan' | null

export interface UserIsolationCapability {
  available: boolean
  experimental: boolean
  risk: UserIsolationRisk
}

const unavailableCapability: UserIsolationCapability = {
  available: false,
  experimental: false,
  risk: null
}

export function getUserIsolationCapability(
  platform: AccountPlatform,
  type: AccountType,
  accountMode?: string,
  apiProtocol?: string
): UserIsolationCapability {
  const supported = (risk: UserIsolationRisk = null, experimental = false): UserIsolationCapability => ({
    available: true,
    experimental,
    risk
  })

  switch (platform) {
    case 'anthropic':
    case 'openai':
      if (type === 'apikey') return supported()
      if (type === 'oauth' || type === 'setup-token') return supported('oauth')
      return unavailableCapability
    case 'grok':
      if (type === 'apikey') return supported()
      if (type === 'oauth') return supported('oauth')
      return unavailableCapability
    case 'kimi':
    case 'zhipu': {
      if (type !== 'apikey') return unavailableCapability
      const codingPlan = accountMode === 'coding'
      const unconfirmedProtocol = apiProtocol !== undefined && apiProtocol !== 'chat_completions'
      return supported(codingPlan ? 'coding_plan' : null, codingPlan || unconfirmedProtocol)
    }
    case 'deepseek':
      return type === 'apikey' && accountMode !== 'coding' ? supported() : unavailableCapability
    default:
      return unavailableCapability
  }
}

export function supportsUserIsolation(
  platform: AccountPlatform,
  type: AccountType,
  accountMode?: string,
  apiProtocol?: string
): boolean {
  return getUserIsolationCapability(platform, type, accountMode, apiProtocol).available
}
