<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <DateRangePicker
          v-model:start-date="startDate"
          v-model:end-date="endDate"
          @change="handleDateRangeChange"
        />
        <div class="w-24">
          <Select v-model="granularity" :options="granularityOptions" @change="loadSummary" />
        </div>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadSummary">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
        <span>{{ t('common.refresh') }}</span>
      </button>
    </div>

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <ModelDistributionChart
        v-model:source="modelSource"
        v-model:metric="modelMetric"
        :model-stats="models"
        :loading="loading"
        :show-source-toggle="true"
        :show-metric-toggle="true"
      />
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { ModelStat, TrendDataPoint } from '@/types'

type DistributionMetric = 'tokens' | 'actual_cost'
type ModelSource = 'requested' | 'upstream' | 'mapping'

const { t } = useI18n()

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const now = new Date()
const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

const startDate = ref(formatLocalDate(yesterday))
const endDate = ref(formatLocalDate(now))
const granularity = ref<'day' | 'hour'>('hour')
const modelSource = ref<ModelSource>('requested')
const modelMetric = ref<DistributionMetric>('tokens')
const loading = ref(false)
const models = ref<ModelStat[]>([])
const trend = ref<TrendDataPoint[]>([])
const reqSeq = ref(0)

const granularityOptions = computed(() => [
  { value: 'hour', label: t('admin.dashboard.hour') },
  { value: 'day', label: t('admin.dashboard.day') }
])

const loadSummary = async () => {
  const seq = ++reqSeq.value
  loading.value = true
  try {
    const summary = await adminAPI.accounts.getUsageViewerSummary({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      model_source: modelSource.value
    })
    if (seq !== reqSeq.value) return
    models.value = summary.models || []
    trend.value = summary.trend || []
  } catch (error) {
    if (seq !== reqSeq.value) return
    models.value = []
    trend.value = []
    console.error('Failed to load usage viewer summary:', error)
  } finally {
    if (seq === reqSeq.value) {
      loading.value = false
    }
  }
}

const handleDateRangeChange = (range: { startDate: string; endDate: string }) => {
  startDate.value = range.startDate
  endDate.value = range.endDate
  loadSummary()
}

watch(modelSource, () => {
  loadSummary()
})

onMounted(() => {
  loadSummary()
})
</script>
