<template>
  <AppLayout v-if="isAuthenticated">
    <ModelPlazaContent v-bind="contentProps" embedded @refresh="loadData(false)" />
  </AppLayout>

  <div v-else class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 bg-white/90 px-4 py-3 backdrop-blur dark:border-dark-800 dark:bg-dark-900/90 sm:px-6">
      <nav class="mx-auto flex max-w-7xl items-center justify-between gap-4">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-9 w-9 shrink-0 rounded-xl object-contain" />
          <span class="truncate font-bold text-gray-950 dark:text-white">{{ siteName }}</span>
        </RouterLink>
        <div class="flex items-center gap-2">
          <LocaleSwitcher />
          <RouterLink
            to="/login"
            class="inline-flex min-h-9 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ t('home.login') }}
          </RouterLink>
        </div>
      </nav>
    </header>
    <main class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
      <ModelPlazaContent v-bind="contentProps" @refresh="loadData(false)" />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import ModelPlazaContent from '@/components/modelPlaza/ModelPlazaContent.vue'
import { getModelPrices, type ModelPricesResponse } from '@/api/modelPrices'
import { useAppStore, useAuthStore } from '@/stores'
import { subscribeDisplayPricingUpdates } from '@/utils/displayPricingSync'
import { sanitizeUrl } from '@/utils/url'

const AUTO_REFRESH_MS = 30_000

const data = ref<ModelPricesResponse | null>(null)
const loading = ref(true)
const refreshing = ref(false)
const loadFailed = ref(false)
const lastUpdated = ref<Date | null>(null)
const authStore = useAuthStore()
const appStore = useAppStore()
const { t } = useI18n()

const isAuthenticated = computed(() => authStore.isAuthenticated)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(
  appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '',
  { allowRelative: true, allowDataUrl: true }
))
const contentProps = computed(() => ({
  response: data.value,
  loading: loading.value,
  error: loadFailed.value,
  refreshing: refreshing.value,
  lastUpdated: lastUpdated.value
}))

let refreshTimer: number | undefined
let requestInFlight = false
let refreshQueued = false
let stopPricingSync: (() => void) | undefined

async function loadData(initial: boolean): Promise<void> {
  if (requestInFlight) {
    refreshQueued = true
    return
  }
  requestInFlight = true
  if (initial) loading.value = true
  else refreshing.value = true

  try {
    data.value = await getModelPrices()
    loadFailed.value = false
    lastUpdated.value = new Date()
  } catch {
    if (!data.value) loadFailed.value = true
  } finally {
    loading.value = false
    refreshing.value = false
    requestInFlight = false
    if (refreshQueued) {
      refreshQueued = false
      void loadData(false)
    }
  }
}

function refreshWhenVisible(): void {
  if (document.visibilityState === 'visible') void loadData(false)
}

onMounted(() => {
  void loadData(true)
  refreshTimer = window.setInterval(() => void loadData(false), AUTO_REFRESH_MS)
  window.addEventListener('focus', refreshWhenVisible)
  document.addEventListener('visibilitychange', refreshWhenVisible)
  stopPricingSync = subscribeDisplayPricingUpdates(() => void loadData(false))
})

onBeforeUnmount(() => {
  if (refreshTimer !== undefined) window.clearInterval(refreshTimer)
  window.removeEventListener('focus', refreshWhenVisible)
  document.removeEventListener('visibilitychange', refreshWhenVisible)
  stopPricingSync?.()
})
</script>
