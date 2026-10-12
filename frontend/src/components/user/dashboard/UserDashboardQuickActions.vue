<template>
  <div class="newapi-quick-actions card">
    <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
      <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.quickActions') }}</h2>
    </div>
    <div class="divide-y divide-gray-100 dark:divide-dark-700">
      <button @click="router.push('/keys')" class="group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-blue-50/70 dark:hover:bg-blue-900/15">
        <div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded bg-blue-50 dark:bg-blue-900/30">
          <Icon name="key" size="sm" class="text-blue-600 dark:text-blue-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.createApiKey') }}</p>
          <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('dashboard.generateNewKey') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-blue-500 dark:text-dark-500"
        />
      </button>

      <button @click="router.push('/usage')" class="group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-emerald-50/70 dark:hover:bg-emerald-900/15">
        <div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded bg-emerald-50 dark:bg-emerald-900/30">
          <Icon name="chart" size="sm" class="text-emerald-600 dark:text-emerald-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.viewUsage') }}</p>
          <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('dashboard.checkDetailedLogs') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-emerald-500 dark:text-dark-500"
        />
      </button>

      <button v-if="canUseBatchImage" @click="router.push('/batch-image')" class="group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-sky-50/70 dark:hover:bg-sky-900/15">
        <div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded bg-sky-50 dark:bg-sky-900/30">
          <Icon name="sparkles" size="sm" class="text-sky-600 dark:text-sky-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.batchImageAgent') }}</p>
          <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('dashboard.batchImageAgentDesc') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-sky-500 dark:text-dark-500"
        />
      </button>

      <button @click="router.push('/redeem')" class="group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-amber-50/70 dark:hover:bg-amber-900/15">
        <div class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded bg-amber-50 dark:bg-amber-900/30">
          <Icon name="gift" size="sm" class="text-amber-600 dark:text-amber-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.redeemCode') }}</p>
          <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('dashboard.addBalanceWithCode') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-amber-500 dark:text-dark-500"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
const router = useRouter()
const { t } = useI18n()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()

onMounted(() => {
  void refreshBatchImageAccess()
})
</script>
