/**
 * Model routing composable: manages routing rules state, API format
 * conversion, and account search. Extracted from ModelRoutingSection
 * to keep the .vue file under 300 lines.
 */
import { ref } from 'vue'
import {
  createStableObjectKeyResolver,
  useKeyedDebouncedSearch,
} from '@sub2api/plugin-sdk'
import { searchAccounts, getAccountById } from '../../api/adminAccounts'
import type { SimpleAccount, ModelRoutingRule } from './modelRoutingTypes'

export function useModelRouting(mode: string) {
  const routingRules = ref<ModelRoutingRule[]>([])

  const resolveRuleKey = createStableObjectKeyResolver<ModelRoutingRule>(
    `${mode}-rule`,
  )
  const getRuleRenderKey = (rule: ModelRoutingRule) => resolveRuleKey(rule)
  const getRuleSearchKey = (rule: ModelRoutingRule) =>
    `${mode}-${resolveRuleKey(rule)}`

  const accountSearchKeyword = ref<Record<string, string>>({})
  const accountSearchResults = ref<Record<string, SimpleAccount[]>>({})
  const showAccountDropdown = ref<Record<string, boolean>>({})

  const accountSearchRunner = useKeyedDebouncedSearch<SimpleAccount[]>({
    delay: 300,
    search: async (keyword, { signal }) => {
      return searchAccounts(keyword, 'anthropic', { signal })
    },
    onSuccess: (key, result) => {
      accountSearchResults.value[key] = result
    },
    onError: (key) => {
      accountSearchResults.value[key] = []
    },
  })

  const searchAccountsByRule = (rule: ModelRoutingRule) => {
    const key = getRuleSearchKey(rule)
    accountSearchRunner.trigger(key, accountSearchKeyword.value[key] || '')
  }

  const onAccountSearchFocus = (rule: ModelRoutingRule) => {
    const key = getRuleSearchKey(rule)
    showAccountDropdown.value[key] = true
    if (!accountSearchResults.value[key]?.length) {
      accountSearchRunner.trigger(key, accountSearchKeyword.value[key] || '')
    }
  }

  const selectAccount = (rule: ModelRoutingRule, account: SimpleAccount) => {
    if (!rule.accounts.some((a) => a.id === account.id)) {
      rule.accounts.push(account)
    }
    const key = getRuleSearchKey(rule)
    accountSearchKeyword.value[key] = ''
    showAccountDropdown.value[key] = false
  }

  const removeSelectedAccount = (rule: ModelRoutingRule, accountId: number) => {
    rule.accounts = rule.accounts.filter((a) => a.id !== accountId)
  }

  const addRoutingRule = () => {
    routingRules.value.push({ pattern: '', accounts: [] })
  }

  const removeRoutingRule = (rule: ModelRoutingRule) => {
    const index = routingRules.value.indexOf(rule)
    if (index === -1) return
    const key = getRuleSearchKey(rule)
    accountSearchRunner.clearKey(key)
    delete accountSearchKeyword.value[key]
    delete accountSearchResults.value[key]
    delete showAccountDropdown.value[key]
    routingRules.value.splice(index, 1)
  }

  // --- API Format Conversion ---

  const getRoutingRulesApiFormat = (): Record<string, number[]> | null => {
    const result: Record<string, number[]> = {}
    let hasValid = false
    for (const rule of routingRules.value) {
      const pattern = rule.pattern.trim()
      if (!pattern) continue
      const ids = rule.accounts.map((a) => a.id).filter((id) => id > 0)
      if (ids.length > 0) {
        result[pattern] = ids
        hasValid = true
      }
    }
    return hasValid ? result : null
  }

  const loadRoutingRules = async (
    apiFormat: Record<string, number[]> | null,
  ) => {
    if (!apiFormat) {
      routingRules.value = []
      return
    }
    const rules: ModelRoutingRule[] = []
    for (const [pattern, accountIds] of Object.entries(apiFormat)) {
      const accounts: SimpleAccount[] = []
      for (const id of accountIds) {
        try {
          const account = await getAccountById(id)
          accounts.push({ id: account.id, name: account.name })
        } catch {
          accounts.push({ id, name: `#${id}` })
        }
      }
      rules.push({ pattern, accounts })
    }
    routingRules.value = rules
  }

  const resetRoutingRules = () => {
    routingRules.value.forEach((rule) => {
      accountSearchRunner.clearKey(getRuleSearchKey(rule))
    })
    accountSearchKeyword.value = {}
    accountSearchResults.value = {}
    showAccountDropdown.value = {}
    routingRules.value = []
  }

  const clearAll = () => {
    accountSearchRunner.clearAll()
  }

  return {
    routingRules,
    accountSearchKeyword,
    accountSearchResults,
    showAccountDropdown,
    getRuleRenderKey,
    getRuleSearchKey,
    searchAccountsByRule,
    onAccountSearchFocus,
    selectAccount,
    removeSelectedAccount,
    addRoutingRule,
    removeRoutingRule,
    getRoutingRulesApiFormat,
    loadRoutingRules,
    resetRoutingRules,
    clearAll,
  }
}
