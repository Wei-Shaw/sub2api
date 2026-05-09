<template>
  <BaseDialog
    :show="show"
    :title="dialogTitle"
    width="wide"
    @close="handleClose"
  >
    <div v-if="group" class="space-y-4">
      <div class="flex flex-wrap items-center gap-3 rounded-lg bg-gray-50 px-4 py-2.5 text-sm dark:bg-dark-700">
        <span class="inline-flex items-center gap-1.5" :class="platformColorClass">
          <PlatformIcon :platform="group.platform" size="sm" />
          {{ t('admin.groups.platforms.' + group.platform) }}
        </span>
        <span class="text-gray-400">|</span>
        <span class="font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
      </div>

      <div v-if="loading" class="flex justify-center py-6">
        <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>

      <template v-else>
        <div v-if="summary && !summary.supported" class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/10 dark:text-amber-300">
          <div class="font-medium">{{ t('admin.groups.quotaSummaryUnsupported') }}</div>
          <div class="mt-1">{{ t('admin.groups.quotaSummaryOnlyOpenAI') }}</div>
        </div>

        <template v-else-if="summary">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.groups.quotaSummaryDescription') }}
          </p>

          <div class="border-b border-gray-200 dark:border-dark-700">
            <nav class="-mb-px flex space-x-6" aria-label="Quota Windows">
              <button
                v-for="tab in tabs"
                :key="tab.window"
                type="button"
                class="whitespace-nowrap border-b-2 px-1 py-2.5 text-sm font-medium transition-colors"
                :class="activeTab === tab.window
                  ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                  : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
                @click="activeTab = tab.window"
              >
                {{ tabLabel(tab.window) }}
              </button>
            </nav>
          </div>

          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="mb-4 flex flex-wrap justify-center gap-4 text-sm text-gray-600 dark:text-gray-400">
              <span>{{ t('admin.groups.quotaSummaryMatchedAccounts') }}: {{ activeSummary?.matched_account_count ?? 0 }}</span>
              <span>{{ t('admin.groups.quotaSummarySkippedAccounts') }}: {{ activeSummary?.skipped_account_count ?? 0 }}</span>
            </div>

            <div class="space-y-2">
              <div
                v-for="bucket in normalizedBucketCounts"
                :key="bucket.used_percent"
                :data-testid="`quota-bucket-row-${bucket.used_percent}`"
                class="grid grid-cols-[auto_4rem_auto] items-center justify-center gap-x-2 rounded-lg bg-gray-50 px-4 py-3 text-sm dark:bg-dark-700"
              >
                <span class="text-right text-gray-700 dark:text-gray-300">
                  {{ t('admin.groups.quotaSummaryBucketPrefix') }}
                  <span class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ bucket.used_percent }}%</span>
                  {{ t('admin.groups.quotaSummaryBucketSuffix') }}
                </span>
                <span class="text-center font-semibold tabular-nums text-gray-900 dark:text-white">{{ bucket.account_count }}</span>
                <span class="text-left text-gray-700 dark:text-gray-300">{{ t('admin.groups.quotaSummaryBucketUnit') }}</span>
              </div>
            </div>
          </div>
        </template>
      </template>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { GroupQuotaSummaryResponse } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

const props = defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const summary = ref<GroupQuotaSummaryResponse | null>(null)
const activeTab = ref<'5h' | '7d'>('5h')
const bucketPercents = [100, 90, 80, 70, 60, 50, 40, 30, 20, 10, 0] as const

const dialogTitle = computed(() => t('admin.groups.quotaSummaryTitle', { name: props.group?.name || '' }))

const platformColorClass = computed(() => {
  switch (props.group?.platform) {
    case 'anthropic': return 'text-orange-700 dark:text-orange-400'
    case 'openai': return 'text-emerald-700 dark:text-emerald-400'
    case 'antigravity': return 'text-purple-700 dark:text-purple-400'
    default: return 'text-blue-700 dark:text-blue-400'
  }
})

const tabs = computed(() => summary.value?.tabs ?? [])
const activeSummary = computed(() => tabs.value.find((tab) => tab.window === activeTab.value) ?? tabs.value[0] ?? null)
const normalizedBucketCounts = computed(() => {
  const counts = new Map((activeSummary.value?.bucket_counts ?? []).map((bucket) => [bucket.used_percent, bucket.account_count]))
  return bucketPercents.map((usedPercent) => ({
    used_percent: usedPercent,
    account_count: counts.get(usedPercent) ?? 0
  }))
})

const tabLabel = (window: '5h' | '7d') => {
  return window === '5h'
    ? t('admin.groups.quotaSummaryWindows.fiveHour')
    : t('admin.groups.quotaSummaryWindows.sevenDay')
}

const loadSummary = async () => {
  if (!props.group) return
  loading.value = true
  summary.value = null
  activeTab.value = '5h'
  try {
    summary.value = await adminAPI.groups.getQuotaSummary(props.group.id)
  } catch (error) {
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error('Error loading group quota summary:', error)
  } finally {
    loading.value = false
  }
}

watch(
  [() => props.show, () => props.group?.id],
  ([show, groupID]) => {
    if (show && groupID) {
      loadSummary()
      return
    }

    if (!show) {
      summary.value = null
      activeTab.value = '5h'
    }
  },
  { immediate: true }
)

const handleClose = () => {
  emit('close')
}
</script>
