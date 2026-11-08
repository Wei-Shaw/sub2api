<template>
  <!-- 用量页"用户排行"tab 内容：无卡片外观，依赖父级统一卡片；筛选/时间范围复用页面级筛选栏 -->
  <div>
    <!-- Toolbar -->
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700/50 sm:px-6">
      <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('admin.usage.tokenRanking.subtitle') }}</p>
      <div class="flex items-center gap-3">
        <span v-if="!loading && items.length > 0" class="text-xs text-gray-400 dark:text-gray-500">
          {{ t('admin.usage.tokenRanking.userCount', { count: items.length }) }}
        </span>
        <div class="w-28">
          <Select v-model="limit" :options="limitOptions" @change="load" />
        </div>
      </div>
    </div>

    <!-- Table -->
    <div class="overflow-x-auto">
      <table class="w-full min-w-max divide-y divide-gray-200 dark:divide-dark-700">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th class="w-16 px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400 sm:px-6">#</th>
            <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">
              {{ t('admin.usage.tokenRanking.columns.user') }}
            </th>
            <th
              v-for="col in sortableColumns"
              :key="col.key"
              class="cursor-pointer select-none whitespace-nowrap px-4 py-3 text-right text-xs font-medium uppercase tracking-wider transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
              :class="sortBy === col.key ? 'text-primary-600 dark:text-primary-400' : 'text-gray-500 dark:text-dark-400'"
              @click="setSort(col.key)"
            >
              {{ t(col.label) }}
              <span v-if="sortBy === col.key" aria-hidden="true">↓</span>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
          <tr v-if="loading">
            <td :colspan="sortableColumns.length + 2" class="py-12 text-center">
              <LoadingSpinner />
            </td>
          </tr>
          <tr v-else-if="items.length === 0">
            <td :colspan="sortableColumns.length + 2" class="py-12 text-center text-sm text-gray-400">
              {{ t('admin.dashboard.noDataAvailable') }}
            </td>
          </tr>
          <tr
            v-for="(item, index) in items"
            v-else
            :key="item.user_id"
            class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
            :title="t('admin.usage.tokenRanking.rowHint')"
            @click="$emit('select-user', item.user_id, item.email)"
          >
            <td class="px-4 py-3 sm:px-6">
              <span
                v-if="index < 3"
                class="inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold"
                :class="RANK_BADGE_CLASSES[index]"
              >{{ index + 1 }}</span>
              <span v-else class="inline-block w-6 text-center text-sm tabular-nums text-gray-400">{{ index + 1 }}</span>
            </td>
            <td class="max-w-[260px] truncate px-4 py-3 text-sm font-medium text-gray-700 dark:text-gray-200" :title="item.email">
              {{ item.email || `User #${item.user_id}` }}
              <span class="ml-1 font-normal text-gray-400 dark:text-gray-500">#{{ item.user_id }}</span>
            </td>
            <td class="whitespace-nowrap px-4 py-3 text-right text-sm tabular-nums text-gray-500 dark:text-gray-400">{{ item.requests.toLocaleString() }}</td>
            <td class="whitespace-nowrap px-4 py-3 text-right text-sm tabular-nums text-gray-500 dark:text-gray-400">{{ fmtTokens(item.input_tokens) }}</td>
            <td class="whitespace-nowrap px-4 py-3 text-right text-sm tabular-nums text-gray-500 dark:text-gray-400">{{ fmtTokens(item.output_tokens) }}</td>
            <td class="whitespace-nowrap px-4 py-3 text-right text-sm tabular-nums text-gray-500 dark:text-gray-400">{{ fmtTokens(item.cache_tokens) }}</td>
            <td class="whitespace-nowrap px-4 py-3 text-right text-sm font-medium tabular-nums text-gray-900 dark:text-gray-100">{{ fmtTokens(item.total_tokens) }}</td>
            <td class="whitespace-nowrap px-4 py-3 text-right text-sm font-medium tabular-nums text-green-600 dark:text-green-400">${{ fmtCost(item.actual_cost) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getUserBreakdown, type UserBreakdownParams } from '@/api/admin/dashboard'
import { formatCompactNumber, formatCostFixed } from '@/utils/format'
import type { UserBreakdownItem } from '@/types'
import Select from '@/components/common/Select.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const props = defineProps<{
  startDate: string
  endDate: string
  filters: Record<string, unknown>
  model?: string
}>()

defineEmits<{ (e: 'select-user', userId: number, email: string): void }>()

const { t } = useI18n()

type SortKey = NonNullable<UserBreakdownParams['sort_by']>
const sortableColumns: { key: SortKey; label: string }[] = [
  { key: 'requests', label: 'admin.usage.tokenRanking.columns.requests' },
  { key: 'input_tokens', label: 'admin.usage.tokenRanking.columns.inputTokens' },
  { key: 'output_tokens', label: 'admin.usage.tokenRanking.columns.outputTokens' },
  { key: 'cache_tokens', label: 'admin.usage.tokenRanking.columns.cacheTokens' },
  { key: 'total_tokens', label: 'admin.usage.tokenRanking.columns.totalTokens' },
  { key: 'actual_cost', label: 'admin.usage.tokenRanking.columns.cost' },
]

const limitOptions = [
  { value: 20, label: 'Top 20' },
  { value: 50, label: 'Top 50' },
  { value: 100, label: 'Top 100' },
  { value: 200, label: 'Top 200' },
]

// 前三名金/银/铜徽章
const RANK_BADGE_CLASSES = [
  'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400',
  'bg-gray-200 text-gray-600 dark:bg-gray-500/20 dark:text-gray-300',
  'bg-orange-100 text-orange-700 dark:bg-orange-500/20 dark:text-orange-400',
]

const items = ref<UserBreakdownItem[]>([])
const loading = ref(false)
const sortBy = ref<SortKey>('total_tokens')
const limit = ref(50)
let reqSeq = 0

const fmtTokens = (v: number) => formatCompactNumber(v)
const fmtCost = (v: number) => formatCostFixed(v, 4)

const setSort = (key: SortKey) => {
  if (sortBy.value === key) return
  sortBy.value = key
  load()
}

const load = async () => {
  const seq = ++reqSeq
  loading.value = true
  try {
    const params: UserBreakdownParams = {
      ...props.filters,
      start_date: props.startDate,
      end_date: props.endDate,
      sort_by: sortBy.value,
      limit: limit.value,
    }
    if (props.model) params.model = props.model
    const res = await getUserBreakdown(params)
    if (seq !== reqSeq) return
    items.value = res.users || []
  } catch {
    if (seq !== reqSeq) return
    items.value = []
  } finally {
    if (seq === reqSeq) loading.value = false
  }
}

// Reload when the shared filters / date range / model change.
watch(
  () => [props.startDate, props.endDate, props.model, JSON.stringify(props.filters)],
  () => load(),
  { immediate: true }
)

defineExpose({ reload: load })
</script>
