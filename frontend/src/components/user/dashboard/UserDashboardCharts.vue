<template>
  <div class="space-y-6">
    <div class="card p-4 flex flex-wrap items-center gap-4">
      <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
      <div class="ml-auto w-28"><Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')REDACTED, {value:'hour', label:t('dashboard.hour')REDACTED]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" /></div>
    </div>
    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <div class="card p-4 min-h-[300px] relative"><div v-if="loading" class="absolute inset-0 flex items-center justify-center bg-white/50"><LoadingSpinner /></div>
        <h3 class="mb-4 font-semibold">{{ t('dashboard.modelDistribution') REDACTEDREDACTED</h3>
        <div class="h-48"><Doughnut v-if="modelData" :data="modelData" :options="{maintainAspectRatio:falseREDACTED" /></div>
      </div>
      <div class="card p-4 min-h-[300px] relative"><div v-if="loading" class="absolute inset-0 flex items-center justify-center bg-white/50"><LoadingSpinner /></div>
        <h3 class="mb-4 font-semibold">{{ t('dashboard.tokenUsageTrend') REDACTEDREDACTED</h3>
        <div class="h-48"><Line v-if="trendData" :data="trendData" :options="{maintainAspectRatio:falseREDACTED" /></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'; import { useI18n REDACTED from 'vue-i18n'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'; import DateRangePicker from '@/components/common/DateRangePicker.vue'; import Select from '@/components/common/Select.vue'; import { Line, Doughnut REDACTED from 'vue-chartjs'
import type { TrendDataPoint, ModelStat REDACTED from '@/types'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler REDACTED from 'chart.js'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler)

const props = defineProps<{ loading: boolean, startDate: string, endDate: string, granularity: string, trend: TrendDataPoint[], models: ModelStat[] REDACTED>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange'])
const { t REDACTED = useI18n()
const modelData = computed(() => !props.models?.length ? null : { labels: props.models.map((m:ModelStat) => m.model), datasets: [{ data: props.models.map((m:ModelStat) => m.total_tokens), backgroundColor: ['#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6'] REDACTED] REDACTED)
const trendData = computed(() => !props.trend?.length ? null : { labels: props.trend.map((d:TrendDataPoint) => d.date), datasets: [{ label: 'Input', data: props.trend.map((d:TrendDataPoint) => d.input_tokens), borderColor: '#3b82f6', tension: 0.3 REDACTED] REDACTED)
</script>
