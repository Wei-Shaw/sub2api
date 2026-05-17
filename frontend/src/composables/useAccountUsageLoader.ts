import { ref, computed, onMounted, onBeforeUnmount, onUnmounted, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo } from '@/types'
import { usePlatforms } from '@/composables/usePlatforms'
import { enqueueUsageRequest } from '@/utils/usageLoadQueue'
import { getAccountTypeMeta } from '@/utils/platformFrontendMeta'

// Module-level cache shared across all AccountUsageCell instances
const _usageCache = new Map<number, { data: AccountUsageInfo; ts: number }>()
const USAGE_CACHE_TTL = 5 * 60 * 1000 // 5 minutes

const DESKTOP_VIEWPORT_QUERY = '(min-width: 768px)'

interface UseAccountUsageLoaderOptions {
  account: Ref<Account>
  manualRefreshToken: Ref<number>
  rootRef: Ref<HTMLElement | null>
}

export function useAccountUsageLoader(options: UseAccountUsageLoaderOptions) {
  const { account, manualRefreshToken, rootRef } = options
  const { t } = useI18n()
  const { getPlatformDecl, getAccountTypeDecl } = usePlatforms()

  const unmounted = ref(false)
  onBeforeUnmount(() => { unmounted.value = true })

  const loading = ref(false)
  const activeQueryLoading = ref(false)
  const error = ref<string | null>(null)
  const usageInfo = ref<AccountUsageInfo | null>(null)
  const isDesktopViewport = ref(
    typeof window === 'undefined' ? true : window.matchMedia(DESKTOP_VIEWPORT_QUERY).matches
  )
  const hasEnteredViewport = ref(false)
  const pendingAutoLoad = ref(false)
  const pendingAutoLoadSource = ref<'passive' | 'active' | undefined>(undefined)

  let desktopViewportMediaQuery: MediaQueryList | null = null
  let desktopViewportListener: ((event: MediaQueryListEvent) => void) | null = null
  let visibilityObserver: IntersectionObserver | null = null

  /**
   * Determine if this account should fetch usage data.
   * Data-driven: checks platform declaration's usage_display config.
   */
  const shouldFetchUsage = computed(() => {
    const decl = getPlatformDecl(account.value.platform)
    if (!decl?.usage_display) return false
    return decl.usage_display.show_req_count || decl.usage_display.show_cost
  })

  const shouldLazyLoadOnMobile = computed(() => shouldFetchUsage.value && !isDesktopViewport.value)

  /**
   * Determine if this account uses passive source for initial load.
   * Checks AccountTypeDeclaration.frontend_meta.default_usage_source first,
   * falls back to hardcoded: Anthropic OAuth/SetupToken use passive sampling.
   */
  const usesPassiveSource = computed(() => {
    const typeDecl = getAccountTypeDecl(account.value.platform, account.value.type)
    const meta = getAccountTypeMeta(typeDecl)
    if (meta.default_usage_source !== undefined) {
      return meta.default_usage_source === 'passive'
    }
    // Fallback: Anthropic OAuth/SetupToken
    return account.value.platform === 'anthropic' &&
      (account.value.type === 'oauth' || account.value.type === 'setup-token')
  })

  /**
   * Build a usage refresh key from account fields that, when changed,
   * indicate cached usage data is stale and should be re-fetched.
   * Checks AccountTypeDeclaration.frontend_meta.usage_refresh_extra_fields first,
   * falls back to hardcoded OpenAI codex fields.
   */
  const usageRefreshKey = computed(() => {
    const a = account.value
    const parts = [a.id, a.updated_at, a.last_used_at, a.rate_limit_reset_at]
    const extra = a.extra ?? {}

    // Check metadata for declared extra fields
    const typeDecl = getAccountTypeDecl(a.platform, a.type)
    const meta = getAccountTypeMeta(typeDecl)
    if (meta.usage_refresh_extra_fields && meta.usage_refresh_extra_fields.length > 0) {
      for (const field of meta.usage_refresh_extra_fields) {
        parts.push(extra[field] != null ? String(extra[field]) : '')
      }
    } else if (extra.codex_usage_updated_at !== undefined) {
      // Fallback: OpenAI-specific codex fields
      parts.push(
        extra.codex_usage_updated_at as string,
        String(extra.codex_5h_used_percent ?? ''),
        extra.codex_5h_reset_at as string,
        String(extra.codex_7d_used_percent ?? ''),
        extra.codex_7d_reset_at as string,
      )
    }
    return parts.map(v => v == null ? '' : String(v)).join('|')
  })

  const loadUsage = async (opts?: { source?: 'passive' | 'active'; bypassCache?: boolean }) => {
    if (!shouldFetchUsage.value) return
    if (!opts?.bypassCache) {
      const cached = _usageCache.get(account.value.id)
      if (cached && Date.now() - cached.ts < USAGE_CACHE_TTL) {
        usageInfo.value = cached.data
        loading.value = false
        return
      }
    }
    loading.value = true
    error.value = null
    try {
      const fetchFn = () => adminAPI.accounts.getUsage(account.value.id, opts?.source)
      const result = await enqueueUsageRequest(account.value, fetchFn)
      if (!unmounted.value) {
        usageInfo.value = result
        _usageCache.set(account.value.id, { data: result, ts: Date.now() })
      }
    } catch (e: unknown) {
      if (!unmounted.value) {
        error.value = t('common.error')
        console.error('Failed to load usage:', e)
      }
    } finally {
      if (!unmounted.value) loading.value = false
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
    visibilityObserver = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return
      hasEnteredViewport.value = true
      detachVisibilityObserver()
      flushPendingAutoLoad()
    }, { root: null, rootMargin: '200px 0px', threshold: 0.01 })
    visibilityObserver.observe(rootRef.value)
  }

  const loadActiveUsage = async () => {
    activeQueryLoading.value = true
    try {
      usageInfo.value = await adminAPI.accounts.getUsage(account.value.id, 'active')
    } catch (e: unknown) {
      console.error('Failed to load active usage:', e)
    } finally {
      activeQueryLoading.value = false
    }
  }

  // ===== Lifecycle =====

  onMounted(() => {
    if (typeof window !== 'undefined') {
      desktopViewportMediaQuery = window.matchMedia(DESKTOP_VIEWPORT_QUERY)
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
    if (!shouldFetchUsage.value) return
    const source = usesPassiveSource.value ? 'passive' : undefined
    requestAutoLoad(source)
  })

  // Watch for account data changes that indicate stale usage
  watch(usageRefreshKey, (nextKey, prevKey) => {
    if (!prevKey || nextKey === prevKey) return
    if (!shouldFetchUsage.value) return
    _usageCache.delete(account.value.id)
    requestAutoLoad()
  })

  watch(
    () => manualRefreshToken.value,
    (nextToken, prevToken) => {
      if (nextToken === prevToken) return
      if (!shouldFetchUsage.value) return
      const source = usesPassiveSource.value ? 'passive' : undefined
      _usageCache.delete(account.value.id)
      loadUsage({ source, bypassCache: true }).catch((e) => {
        console.error('Failed to refresh usage after manual refresh:', e)
      })
    }
  )

  watch(
    [rootRef, shouldLazyLoadOnMobile],
    () => {
      if (shouldLazyLoadOnMobile.value) { attachVisibilityObserver(); return }
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
    loadActiveUsage,
    shouldFetchUsage,
  }
}
