<template>
  <div class="mb-4 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
    <!-- Header: title + dimension tabs + collapse -->
    <div class="mb-3 flex items-center justify-between">
      <div class="flex items-center gap-3">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.accounts.health.summaryTitle') }}
        </h3>
        <div class="inline-flex rounded-md bg-gray-100 p-0.5 dark:bg-dark-700">
          <button
            v-for="dim in dimensions"
            :key="dim"
            type="button"
            :class="[
              'rounded px-2.5 py-1 text-xs font-medium transition-colors',
              activeDim === dim
                ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-600 dark:text-primary-400'
                : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
            @click="activeDim = dim"
          >
            {{ t(`admin.accounts.health.dimension.${dim}`) }}
          </button>
        </div>
      </div>
      <button
        type="button"
        class="flex items-center gap-1 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
        @click="collapsed = !collapsed"
      >
        <Icon :name="collapsed ? 'chevronDown' : 'chevronUp'" size="xs" />
        {{ collapsed ? t('common.expand') : t('common.collapse') }}
      </button>
    </div>

    <div v-if="!collapsed">
      <!-- Loading -->
      <div v-if="loading" class="py-6 text-center text-sm text-gray-400">
        {{ t('common.loading') }}
      </div>
      <!-- Error -->
      <div v-else-if="error" class="py-6 text-center text-sm text-red-500">
        {{ error }}
      </div>
      <!-- Cards -->
      <div v-else-if="buckets.length > 0" class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        <button
          v-for="bucket in buckets"
          :key="bucket.key"
          type="button"
          class="rounded-lg border border-gray-200 p-3 text-left transition-shadow hover:shadow-md dark:border-dark-600"
          @click="onCardClick(bucket)"
        >
          <div class="mb-2 flex items-center justify-between">
            <span class="truncate text-sm font-medium text-gray-900 dark:text-gray-100" :title="bucket.label">
              {{ bucket.label }}
            </span>
            <span class="text-xs text-gray-400">{{ t('admin.accounts.health.total', { n: bucket.counts.total }) }}</span>
          </div>

          <!-- Health rate -->
          <div class="mb-2 flex items-baseline gap-1">
            <span class="text-2xl font-bold" :class="rateColor(bucket.counts.health_rate)">
              {{ formatRate(bucket.counts.health_rate) }}
            </span>
            <span class="text-xs text-gray-400">{{ t('admin.accounts.health.healthRate') }}</span>
          </div>

          <!-- Segmented progress bar -->
          <div class="mb-2 flex h-1.5 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
            <div
              v-for="seg in segments(bucket.counts)"
              :key="seg.key"
              :class="seg.color"
              :style="{ width: seg.pct + '%' }"
              :title="`${seg.label}: ${seg.count}`"
            ></div>
          </div>

          <!-- Counts breakdown -->
          <div class="flex flex-wrap gap-x-3 gap-y-0.5 text-xs">
            <span v-for="seg in segments(bucket.counts)" :key="seg.key" class="flex items-center gap-1 text-gray-500 dark:text-gray-400">
              <span :class="['inline-block h-1.5 w-1.5 rounded-full', seg.dot]"></span>
              {{ seg.label }} {{ seg.count }}
            </span>
          </div>
        </button>
      </div>
      <div v-else class="py-6 text-center text-sm text-gray-400">
        {{ t('admin.accounts.health.noData') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { AccountHealthSummary, HealthCounts, HealthSummaryBucket } from '@/types'

const { t } = useI18n()

const emit = defineEmits<{
  // 点击卡片时按维度联动列表筛选(需求 §7.2.7)
  (e: 'filter', payload: { dimension: 'platform' | 'group'; key: string }): void
}>()

type Dimension = 'platform' | 'group'
const dimensions: Dimension[] = ['platform', 'group']
const activeDim = ref<Dimension>('platform')
const collapsed = ref(false)

const summary = ref<AccountHealthSummary | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
let abortController: AbortController | null = null

const buckets = computed<HealthSummaryBucket[]>(() => {
  if (!summary.value) return []
  return activeDim.value === 'platform' ? summary.value.by_platform : summary.value.by_group
})

const segmentDefs: Array<{ key: keyof HealthCounts; color: string; dot: string }> = [
  { key: 'healthy', color: 'bg-emerald-500', dot: 'bg-emerald-500' },
  { key: 'error', color: 'bg-red-500', dot: 'bg-red-500' },
  { key: 'limited', color: 'bg-amber-500', dot: 'bg-amber-500' },
  { key: 'untested', color: 'bg-gray-300 dark:bg-gray-500', dot: 'bg-gray-300 dark:bg-gray-500' },
  { key: 'paused', color: 'bg-gray-400', dot: 'bg-gray-400' }
]

const segments = (counts: HealthCounts) => {
  const total = counts.total || 1
  return segmentDefs.map(def => {
    const count = (counts[def.key] as number) ?? 0
    return {
      key: def.key as string,
      count,
      pct: (count / total) * 100,
      color: def.color,
      dot: def.dot,
      label: t(`admin.accounts.health.status.${def.key}`)
    }
  })
}

const formatRate = (rate: number | null): string => {
  if (rate === null || rate === undefined) return '—'
  return `${Math.round(rate * 100)}%`
}

const rateColor = (rate: number | null): string => {
  if (rate === null || rate === undefined) return 'text-gray-400'
  if (rate >= 0.9) return 'text-emerald-600 dark:text-emerald-400'
  if (rate >= 0.6) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

const onCardClick = (bucket: HealthSummaryBucket) => {
  emit('filter', { dimension: activeDim.value, key: bucket.key })
}

const fetchSummary = async () => {
  abortController?.abort()
  abortController = new AbortController()
  loading.value = true
  error.value = null
  try {
    summary.value = await adminAPI.accounts.getHealthSummary({ signal: abortController.signal })
  } catch (e: unknown) {
    const err = e as { code?: string; message?: string }
    if (err?.code === 'ERR_CANCELED') return
    error.value = err?.message || t('admin.accounts.health.loadFailed')
  } finally {
    loading.value = false
  }
}

defineExpose({ refresh: fetchSummary })

onMounted(fetchSummary)
</script>
