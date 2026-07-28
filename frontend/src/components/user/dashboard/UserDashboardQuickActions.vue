<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('dashboard.quickActions') }}</h2>
    </div>
    <div class="grid grid-cols-2 gap-3 p-4 lg:grid-cols-4">
      <button @click="router.push('/keys')" class="group flex flex-col items-center gap-2 rounded-xl bg-gray-50 p-4 text-center transition-all hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-100 transition-transform group-hover:scale-105 dark:bg-primary-900/30">
          <Icon name="key" size="md" class="text-primary-600 dark:text-primary-400" />
        </div>
        <span class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.createApiKey') }}</span>
      </button>

      <button @click="router.push('/usage')" class="group flex flex-col items-center gap-2 rounded-xl bg-gray-50 p-4 text-center transition-all hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-100 transition-transform group-hover:scale-105 dark:bg-emerald-900/30">
          <Icon name="chart" size="md" class="text-emerald-600 dark:text-emerald-400" />
        </div>
        <span class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.viewUsage') }}</span>
      </button>

      <button v-if="canUseBatchImage" @click="router.push('/batch-image')" class="group flex flex-col items-center gap-2 rounded-xl bg-gray-50 p-4 text-center transition-all hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-100 transition-transform group-hover:scale-105 dark:bg-sky-900/30">
          <Icon name="sparkles" size="md" class="text-sky-600 dark:text-sky-400" />
        </div>
        <span class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.batchImageAgent') }}</span>
      </button>

      <button @click="router.push('/redeem')" class="group flex flex-col items-center gap-2 rounded-xl bg-gray-50 p-4 text-center transition-all hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-100 transition-transform group-hover:scale-105 dark:bg-amber-900/30">
          <Icon name="gift" size="md" class="text-amber-600 dark:text-amber-400" />
        </div>
        <span class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.redeemCode') }}</span>
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
