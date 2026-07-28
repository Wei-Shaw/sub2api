<template>
  <div class="card p-5">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('dashboard.quickAccess.title') }}</h2>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('dashboard.quickAccess.subtitle') }}</p>
      </div>
      <span class="inline-flex items-center gap-1.5 rounded-full bg-green-50 px-2 py-0.5 text-xs font-medium text-green-600 dark:bg-green-900/30 dark:text-green-400">
        <span class="h-1.5 w-1.5 rounded-full bg-green-500" />
        {{ t('dashboard.quickAccess.serviceOk') }}
      </span>
    </div>
    <div class="space-y-2">
      <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800/50">
        <div class="min-w-0">
          <p class="text-[10px] uppercase tracking-wide text-gray-400">{{ t('dashboard.quickAccess.baseUrl') }}</p>
          <p class="truncate font-mono text-sm text-gray-900 dark:text-white">{{ baseUrl }}</p>
        </div>
      </div>
      <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800/50">
        <div class="min-w-0">
          <p class="text-[10px] uppercase tracking-wide text-gray-400">{{ t('dashboard.quickAccess.authorization') }}</p>
          <p v-if="maskedKey" class="truncate font-mono text-sm text-gray-900 dark:text-white">Bearer {{ maskedKey }}</p>
          <button v-else @click="router.push('/keys')" class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400">
            {{ t('dashboard.quickAccess.noKey') }} · {{ t('dashboard.quickAccess.createKey') }}
          </button>
        </div>
      </div>
    </div>
    <div class="mt-3 flex gap-2">
      <button @click="copyUrl" class="btn btn-primary flex-1">{{ t('dashboard.quickAccess.copyUrl') }}</button>
      <button @click="router.push('/keys')" class="btn btn-secondary flex-1">{{ t('dashboard.quickAccess.tutorial') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores'
import { maskApiKey } from '@/utils/maskApiKey'
import type { ApiKey } from '@/types'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const firstKey = ref<ApiKey | null>(null)

const baseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || `${window.location.origin}/v1`)
const maskedKey = computed(() => (firstKey.value ? maskApiKey(firstKey.value.key) : ''))

async function copyUrl() {
  try {
    await navigator.clipboard.writeText(baseUrl.value)
  } catch {
    // Clipboard unavailable (permissions / non-secure context); ignore.
  }
}

onMounted(async () => {
  try {
    // keysAPI.list(page, pageSize, filters, options) — positional args.
    const res = await keysAPI.list(1, 1, { status: 'active' })
    firstKey.value = res?.items?.[0] || null
  } catch {
    firstKey.value = null
  }
})
</script>
