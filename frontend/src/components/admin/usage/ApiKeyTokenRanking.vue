<template>
  <!-- 用量页"API Key 排行"tab 内容：无卡片外观，依赖父级统一卡片；时间范围/用户筛选复用页面级筛选栏 -->
  <div>
    <!-- Toolbar -->
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700/50 sm:px-6">
      <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('admin.usage.keyRanking.subtitle') }}</p>
      <div class="flex items-center gap-3">
        <span v-if="!loading && items.length > 0" class="text-xs text-gray-400 dark:text-gray-500">
          {{ t('admin.usage.keyRanking.keyCount', { count: items.length }) }}
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
              {{ t('admin.usage.keyRanking.columns.key') }}
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
            :key="item.api_key_id"
            class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/40"
            :title="t('admin.usage.keyRanking.rowHint')"
            @click="$emit('select-key', item.api_key_id, item.key_name)"
          >
            <td class="px-4 py-3 sm:px-6">
              <span
                v-if="index < 3"
                class="inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold"
                :class="RANK_BADGE_CLASSES[index]"
              >{{ index + 1 }}</span>
              <span v-else class="inline-block w-6 text-center text-sm tabular-nums text-gray-400">{{ index + 1 }}</span>
            </td>
            <td class="max-w-[260px] px-4 py-3 text-sm font-medium text-gray-700 dark:text-gray-200">
              <span class="block truncate" :title="item.key_name">
                {{ item.key_name || t('admin.dashboard.apiKeyPrefix', { id: item.api_key_id }) }}
                <span
                  v-if="item.key_deleted"
                  class="ml-1 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-normal text-gray-500 dark:bg-dark-700 dark:text-gray-400"
                >{{ t('admin.dashboard.apiKeyDeletedBadge') }}</span>
              </span>
              <span class="block truncate text-xs font-normal text-gray-400 dark:text-gray-500" :title="item.email">
                #{{ item.api_key_id }}<template v-if="item.email"> · {{ item.email }}</template>
              </span>
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
import { getApiKeysRanking, type ApiKeyUsageRankingParams } from '@/api/admin/dashboard'
import { formatCompactNumber, formatCostFixed } from '@/utils/format'
import type { ApiKeyUsageRankingItem } from '@/types'
import Select from '@/components/common/Select.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

// 仅 user_id 会作用于本排行；页面级的模型/分组等其它筛选不生效(接口按 api_key_id 全量聚合)。
const props = defineProps<{
  startDate: string
  endDate: string
  userId?: number
}>()

defineEmits<{ (e: 'select-key', apiKeyId: number, keyName: string): void }>()

const { t } = useI18n()

type SortKey = NonNullable<ApiKeyUsageRankingParams['sort_by']>
const sortableColumns: { key: SortKey; label: string }[] = [
  { key: 'requests', label: 'admin.usage.keyRanking.columns.requests' },
  { key: 'input_tokens', label: 'admin.usage.keyRanking.columns.inputTokens' },
  { key: 'output_tokens', label: 'admin.usage.keyRanking.columns.outputTokens' },
  { key: 'cache_tokens', label: 'admin.usage.keyRanking.columns.cacheTokens' },
  { key: 'total_tokens', label: 'admin.usage.keyRanking.columns.totalTokens' },
  { key: 'actual_cost', label: 'admin.usage.keyRanking.columns.cost' },
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

const items = ref<ApiKeyUsageRankingItem[]>([])
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
    const params: ApiKeyUsageRankingParams = {
      start_date: props.startDate,
      end_date: props.endDate,
      sort_by: sortBy.value,
      limit: limit.value,
    }
    if (props.userId) params.user_id = props.userId
    const res = await getApiKeysRanking(params)
    if (seq !== reqSeq) return
    items.value = res.ranking || []
  } catch {
    if (seq !== reqSeq) return
    items.value = []
  } finally {
    if (seq === reqSeq) loading.value = false
  }
}

// Reload when the shared date range / user filter changes.
watch(
  () => [props.startDate, props.endDate, props.userId],
  () => load(),
  { immediate: true }
)

defineExpose({ reload: load })
</script>
