import { ref, computed, onMounted, onBeforeUnmount, onUnmounted, watch, type Ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo, GeminiCredentials } from '@/types'
import { buildOpenAIUsageRefreshKey } from '@/utils/accountUsageRefresh'
import { enqueueUsageRequest } from '@/utils/usageLoadQueue'
import { isOfficialGeminiBaseUrl } from '@/utils/geminiExplicitCache'
import { isMultiProtocolApiKeyPlatform } from './credentialsBuilder'

const USAGE_CACHE_TTL = 5 * 60 * 1000
const desktopViewportQuery = '(min-width: 768px)'

export interface UseAccountUsageLoaderProps {
  account: Account
  manualRefreshToken?: number
  batchedUsage?: AccountUsageInfo | null
  batchedUsageError?: string | null
  batchedUsageLoading?: boolean
  requestBatchedUsage?: ((account: Account, options?: { force?: boolean }) => void) | null
}

export function useAccountUsageLoader(
  props: UseAccountUsageLoaderProps,
  rootRef: Ref<HTMLElement | null>,
  t: (key: string) => string,
  onUsageLoaded: (usage: AccountUsageInfo) => void
) {
  // 实例级缓存（与原 <script setup> 作用域一致，避免跨组件重挂载共享旧缓存导致不请求新数据）
  const _usageCache = new Map<number, { data: AccountUsageInfo; ts: number }>()

  const unmounted = ref(false)
  onBeforeUnmount(() => {
    unmounted.value = true
  })

  const loading = ref(false)
  const activeQueryLoading = ref(false)
  const error = ref<string | null>(null)
  const usageInfo = ref<AccountUsageInfo | null>(null)
  let requestVersion = 0
  const acceptsResponse = (version: number, accountID: number) =>
    !unmounted.value && version === requestVersion && props.account.id === accountID
  watch(usageInfo, (usage) => {
    if (usage) onUsageLoaded(usage)
  })

  const isDesktopViewport = ref(
    typeof window === 'undefined' ? true : window.matchMedia(desktopViewportQuery).matches
  )
  const hasEnteredViewport = ref(false)
  const pendingAutoLoad = ref(false)
  const pendingAutoLoadSource = ref<'passive' | 'active' | undefined>(undefined)

  let desktopViewportMediaQuery: MediaQueryList | null = null
  let desktopViewportListener: ((event: MediaQueryListEvent) => void) | null = null
  let visibilityObserver: IntersectionObserver | null = null

  const isGeminiThirdPartyApiKey = computed(() => {
    if (props.account.platform !== 'gemini' || props.account.type !== 'apikey') return false
    const creds = props.account.credentials as GeminiCredentials | undefined
    return !isOfficialGeminiBaseUrl(creds?.base_url)
  })

  const isGeminiVertex = computed(() => {
    return props.account.platform === 'gemini' && props.account.type === 'service_account'
  })

  const showUsageWindows = computed(() => {
    if (props.account.platform === 'gemini') return !isGeminiThirdPartyApiKey.value
    if (isMultiProtocolApiKeyPlatform(props.account.platform)) return true
    return props.account.type === 'oauth' || props.account.type === 'setup-token'
  })

  const shouldFetchUsage = computed(() => {
    if (props.account.platform === 'anthropic') {
      return props.account.type === 'oauth' || props.account.type === 'setup-token'
    }
    if (props.account.platform === 'gemini') {
      return !isGeminiThirdPartyApiKey.value && !isGeminiVertex.value
    }
    if (
      props.account.platform === 'antigravity' ||
      props.account.platform === 'grok' ||
      props.account.platform === 'openai'
    ) {
      return props.account.type === 'oauth'
    }
    return false
  })

  const isBatchManaged = computed(() => typeof props.requestBatchedUsage === 'function')

  const hasOpenAIUsageFallback = computed(() => {
    if (props.account.platform !== 'openai' || props.account.type !== 'oauth') return false
    return !!usageInfo.value?.five_hour || !!usageInfo.value?.seven_day
  })

  const openAISevenDayEstimatedTotalCost = computed(() => {
    const sevenDay = usageInfo.value?.seven_day
    const utilization = sevenDay?.utilization
    const currentCost = sevenDay?.window_stats?.cost
    if (
      typeof utilization !== 'number' ||
      typeof currentCost !== 'number' ||
      !Number.isFinite(utilization) ||
      !Number.isFinite(currentCost) ||
      utilization <= 0 ||
      currentCost <= 0
    ) {
      return null
    }
    const estimate = (currentCost * 100) / utilization
    return Number.isFinite(estimate) && estimate > 0 ? estimate : null
  })

  const openAIUsageRefreshKey = computed(() => buildOpenAIUsageRefreshKey(props.account))
  const shouldAutoLoadUsageOnMount = computed(() => shouldFetchUsage.value)
  const shouldLazyLoadOnMobile = computed(() => shouldFetchUsage.value && !isDesktopViewport.value)
  const isAnthropicOAuthOrSetupToken = computed(() => {
    return (
      props.account.platform === 'anthropic' &&
      (props.account.type === 'oauth' || props.account.type === 'setup-token')
    )
  })

  const requestParentBatchUsage = (options?: { force?: boolean }) => {
    if (!isBatchManaged.value || !shouldFetchUsage.value) return
    props.requestBatchedUsage?.(props.account, options)
  }

  const syncManagedUsageState = () => {
    if (!isBatchManaged.value) return
    usageInfo.value = props.batchedUsage ?? null
    error.value = props.batchedUsageError ?? null
    loading.value = props.batchedUsageLoading === true
  }

  const loadUsage = async (options?: { source?: 'passive' | 'active'; bypassCache?: boolean }) => {
    if (!shouldFetchUsage.value || unmounted.value) return
    if (isBatchManaged.value) {
      requestParentBatchUsage({ force: options?.bypassCache === true })
      return
    }

    const account = props.account
    const version = ++requestVersion
    activeQueryLoading.value = false
    error.value = null

    if (!options?.bypassCache) {
      const cached = _usageCache.get(props.account.id)
      if (cached && Date.now() - cached.ts < USAGE_CACHE_TTL) {
        usageInfo.value = cached.data
        loading.value = false
        return
      }
    }

    loading.value = true
    error.value = null

    try {
      const fetchFn = () =>
        options?.source || (account.platform === 'antigravity' && options?.bypassCache)
          ? adminAPI.accounts.getUsage(
              account.id,
              options.source,
              options.bypassCache === true
            )
          : adminAPI.accounts.getUsage(account.id)
      const result = await enqueueUsageRequest(account, fetchFn)
      if (acceptsResponse(version, account.id)) {
        usageInfo.value = result
        _usageCache.set(account.id, { data: result, ts: Date.now() })
      }
    } catch (e: unknown) {
      if (acceptsResponse(version, account.id)) {
        error.value = t('common.error')
        console.error('Failed to load usage:', e)
      }
    } finally {
      if (acceptsResponse(version, account.id)) loading.value = false
    }
  }

  const flushPendingAutoLoad = () => {
    if (!pendingAutoLoad.value) return
    const source = pendingAutoLoadSource.value
    pendingAutoLoad.value = false
    pendingAutoLoadSource.value = undefined
    loadUsage({ source }).catch((e) => {
      console.error('Failed to load deferred usage:', e)
    })
  }

  const requestAutoLoad = (source?: 'passive' | 'active') => {
    if (!shouldFetchUsage.value) return
    if (shouldLazyLoadOnMobile.value && !hasEnteredViewport.value) {
      pendingAutoLoad.value = true
      pendingAutoLoadSource.value = source
      return
    }
    loadUsage({ source }).catch((e) => {
      console.error('Failed to auto load usage:', e)
    })
  }

  const detachVisibilityObserver = () => {
    visibilityObserver?.disconnect()
    visibilityObserver = null
  }

  const attachVisibilityObserver = () => {
    detachVisibilityObserver()
    if (!shouldLazyLoadOnMobile.value || hasEnteredViewport.value) return
    if (typeof window === 'undefined' || typeof IntersectionObserver === 'undefined') {
      hasEnteredViewport.value = true
      flushPendingAutoLoad()
      return
    }
    if (!rootRef.value) return

    visibilityObserver = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return
        hasEnteredViewport.value = true
        detachVisibilityObserver()
        flushPendingAutoLoad()
      },
      { root: null, rootMargin: '200px 0px', threshold: 0.01 }
    )
    visibilityObserver.observe(rootRef.value)
  }

  const loadActiveUsage = async () => {
    if (activeQueryLoading.value || unmounted.value) return
    const accountID = props.account.id
    const version = ++requestVersion
    activeQueryLoading.value = true
    loading.value = false
    error.value = null
    try {
      const result = await adminAPI.accounts.getUsage(accountID, 'active', true)
      if (!acceptsResponse(version, accountID)) return
      usageInfo.value = result
      _usageCache.set(accountID, { data: result, ts: Date.now() })
    } catch (e: unknown) {
      if (!acceptsResponse(version, accountID)) return
      error.value = t('common.error')
      console.error('Failed to load active usage:', e)
    } finally {
      if (acceptsResponse(version, accountID)) activeQueryLoading.value = false
    }
  }

  const handleGrokProbed = async () => {
    await loadUsage({ source: 'active', bypassCache: true })
  }

  onMounted(() => {
    if (typeof window !== 'undefined') {
      desktopViewportMediaQuery = window.matchMedia(desktopViewportQuery)
      isDesktopViewport.value = desktopViewportMediaQuery.matches
      desktopViewportListener = (event: MediaQueryListEvent) => {
        isDesktopViewport.value = event.matches
      }
      if (typeof desktopViewportMediaQuery.addEventListener === 'function') {
        desktopViewportMediaQuery.addEventListener('change', desktopViewportListener)
      } else {
        desktopViewportMediaQuery.addListener(desktopViewportListener)
      }
    }

    if (isBatchManaged.value) {
      syncManagedUsageState()
      requestParentBatchUsage()
      return
    }

    if (!shouldAutoLoadUsageOnMount.value) return
    const source = isAnthropicOAuthOrSetupToken.value ? 'passive' : undefined
    requestAutoLoad(source)
  })

  watch(
    () =>
      [
        props.batchedUsage,
        props.batchedUsageError,
        props.batchedUsageLoading,
        isBatchManaged.value
      ] as const,
    () => {
      syncManagedUsageState()
    },
    { immediate: true, deep: true }
  )

  watch(isBatchManaged, (managed, wasManaged) => {
    if (managed && !wasManaged) {
      syncManagedUsageState()
      requestParentBatchUsage()
    }
  })

  watch(
    () =>
      [props.account.id, props.account.platform, props.account.type, isBatchManaged.value] as const,
    (
      [accountID, platform, accountType, managed],
      [previousAccountID, previousPlatform, previousAccountType]
    ) => {
      if (
        accountID === previousAccountID &&
        platform === previousPlatform &&
        accountType === previousAccountType
      ) {
        return
      }
      ++requestVersion
      usageInfo.value = null
      error.value = null
      loading.value = false
      activeQueryLoading.value = false
      if (!shouldFetchUsage.value) return
      if (!managed) {
        requestAutoLoad()
        return
      }
      syncManagedUsageState()
      requestParentBatchUsage()
    },
    { flush: 'post' }
  )

  watch(openAIUsageRefreshKey, (nextKey, prevKey) => {
    if (!prevKey || nextKey === prevKey) return
    if (props.account.platform !== 'openai' || props.account.type !== 'oauth') return

    if (isBatchManaged.value) {
      requestParentBatchUsage({ force: true })
      return
    }

    _usageCache.delete(props.account.id)
    requestAutoLoad()
  })

  watch(
    () => props.manualRefreshToken,
    (nextToken, prevToken) => {
      if (nextToken === prevToken) return
      if (!shouldFetchUsage.value) return

      if (isBatchManaged.value) {
        requestParentBatchUsage({ force: true })
        return
      }

      const source = isAnthropicOAuthOrSetupToken.value ? 'passive' : undefined
      _usageCache.delete(props.account.id)
      loadUsage({ source, bypassCache: true }).catch((e) => {
        console.error('Failed to refresh usage after manual refresh:', e)
      })
    }
  )

  watch(
    [rootRef, shouldLazyLoadOnMobile],
    () => {
      if (shouldLazyLoadOnMobile.value) {
        attachVisibilityObserver()
        return
      }
      detachVisibilityObserver()
    },
    { immediate: true, flush: 'post' }
  )

  watch(isDesktopViewport, (isDesktop) => {
    if (isDesktop) {
      detachVisibilityObserver()
      hasEnteredViewport.value = true
      flushPendingAutoLoad()
      return
    }
    hasEnteredViewport.value = false
    attachVisibilityObserver()
  })

  onUnmounted(() => {
    detachVisibilityObserver()
    if (desktopViewportMediaQuery && desktopViewportListener) {
      if (typeof desktopViewportMediaQuery.removeEventListener === 'function') {
        desktopViewportMediaQuery.removeEventListener('change', desktopViewportListener)
      } else {
        desktopViewportMediaQuery.removeListener(desktopViewportListener)
      }
    }
    desktopViewportListener = null
    desktopViewportMediaQuery = null
  })

  return {
    loading,
    activeQueryLoading,
    error,
    usageInfo,
    showUsageWindows,
    hasOpenAIUsageFallback,
    openAISevenDayEstimatedTotalCost,
    loadActiveUsage,
    handleGrokProbed
  }
}
