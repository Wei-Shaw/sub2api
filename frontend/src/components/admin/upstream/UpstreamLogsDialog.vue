<template>
  <BaseDialog
    :show="show"
    :title="t('admin.upstreams.logs.title', { name: station?.name || '' })"
    width="wide"
    @close="emit('close')"
  >
    <div class="max-h-[60vh] overflow-y-auto">
      <div v-if="loading" class="py-12 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="logs.length === 0" class="py-12 text-center text-sm text-gray-500">{{ t('admin.upstreams.logs.empty') }}</div>
      <ol v-else class="divide-y divide-gray-100 dark:divide-dark-700">
        <li v-for="log in logs" :key="log.id" class="grid gap-2 py-4 sm:grid-cols-[120px_1fr_auto] sm:items-start">
          <time class="font-mono text-xs text-gray-400">{{ formatTime(log.created_at) }}</time>
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span :class="log.success ? 'bg-emerald-500' : 'bg-red-500'" class="h-2 w-2 rounded-full"></span>
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ actionLabel(log.action) }}</span>
            </div>
            <p v-if="log.message" class="mt-1 break-words text-sm text-gray-600 dark:text-dark-300">{{ log.message }}</p>
            <pre v-if="log.detail" class="mt-2 overflow-x-auto whitespace-pre-wrap rounded-md bg-gray-50 p-2 text-xs text-gray-500 dark:bg-dark-900 dark:text-dark-300">{{ log.detail }}</pre>
          </div>
          <span :class="log.success ? successBadge : failureBadge">{{ log.success ? t('common.success') : t('common.failed') }}</span>
        </li>
      </ol>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { UpstreamStation, UpstreamSyncLog } from '@/api/admin/upstreamStations'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean; station: UpstreamStation | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t, locale } = useI18n()
const appStore = useAppStore()
const logs = ref<UpstreamSyncLog[]>([])
const loading = ref(false)
const successBadge = 'rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
const failureBadge = 'rounded-md bg-red-50 px-2 py-1 text-xs font-medium text-red-700 dark:bg-red-500/10 dark:text-red-300'

watch(() => [props.show, props.station?.id] as const, ([show]) => {
  if (show) void load()
}, { immediate: true })

async function load() {
  if (!props.station) return
  loading.value = true
  try {
    logs.value = await adminAPI.upstreamStations.listLogs(props.station.id)
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.logsFailed')))
  } finally {
    loading.value = false
  }
}

function formatTime(value: string): string {
  if (!value) return '—'
  return new Intl.DateTimeFormat(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}

function actionLabel(action: string): string {
  return t(`admin.upstreams.logs.actions.${action}`, action)
}
</script>
