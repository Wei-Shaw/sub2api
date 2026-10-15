import { inject, type InjectionKey } from 'vue'
import { adminAPI } from '@/api/admin'

// The same account forms run against an explicitly selected management scope.
export type AccountWorkspaceAPI = {
  accounts: Pick<typeof adminAPI.accounts,
    'create' | 'update' | 'generateAuthUrl' | 'exchangeCode' | 'refreshOpenAIToken' |
    'syncUpstreamModels' | 'syncUpstreamModelsPreview' | 'getAntigravityDefaultModelMapping' |
    'applyOAuthCredentials' | 'clearError' | 'getAvailableModels' | 'getStats' | 'getUsage' | 'refreshCredentials' |
    'getOpenCodeGoUsage' | 'setOpenCodeGoUsageAutoRefresh' | 'refreshOpenCodeGoUsage' |
    'getGrokMediaEligibility' | 'updateGrokMediaEligibility' | 'checkMixedChannelRisk' |
    'probeUpstreamBilling' | 'importCodexSession' | 'createOpenAICodexPAT'>
  gemini: typeof adminAPI.gemini
  antigravity: typeof adminAPI.antigravity
  grok: typeof adminAPI.grok
}

export interface AccountWorkspace {
  api: AccountWorkspaceAPI
  isRelay: boolean
  basePath: '/admin' | '/relay'
}

export const accountWorkspaceKey: InjectionKey<AccountWorkspace> = Symbol('accountWorkspace')

export function useAccountWorkspace(): AccountWorkspace {
  return inject(accountWorkspaceKey, { api: adminAPI, isRelay: false, basePath: '/admin' })
}
