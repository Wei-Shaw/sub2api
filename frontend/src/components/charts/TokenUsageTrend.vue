<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.dashboard.tokenUsageTrend') REDACTEDREDACTED
    </h3>
    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="trendData.length > 0 && chartData" class="h-48">
      <Line :data="chartData" :options="lineOptions" />
    </div>
    <div
      v-else
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') REDACTEDREDACTED
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
REDACTED from 'chart.js'
import { Line REDACTED from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { TrendDataPoint REDACTED from '@/types'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const { t REDACTED = useI18n()

const props = defineProps<{
  trendData: TrendDataPoint[]
  loading?: boolean
REDACTED>()

const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
REDACTED)

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  input: '#3b82f6',
  output: '#10b981',
  cacheCreation: '#f59e0b',
  cacheRead: '#06b6d4',
  cacheHitRate: '#8b5cf6'
REDACTED))

const chartData = computed(() => {
  if (!props.trendData?.length) return null

  return {
    labels: props.trendData.map((d) => d.date),
    datasets: [
      {
        label: 'Input',
        data: props.trendData.map((d) => d.input_tokens),
        borderColor: chartColors.value.input,
        backgroundColor: `${chartColors.value.inputREDACTED20`,
        fill: true,
        tension: 0.3
      REDACTED,
      {
        label: 'Output',
        data: props.trendData.map((d) => d.output_tokens),
        borderColor: chartColors.value.output,
        backgroundColor: `${chartColors.value.outputREDACTED20`,
        fill: true,
        tension: 0.3
      REDACTED,
      {
        label: 'Cache Creation',
        data: props.trendData.map((d) => d.cache_creation_tokens),
        borderColor: chartColors.value.cacheCreation,
        backgroundColor: `${chartColors.value.cacheCreationREDACTED20`,
        fill: true,
        tension: 0.3
      REDACTED,
      {
        label: 'Cache Read',
        data: props.trendData.map((d) => d.cache_read_tokens),
        borderColor: chartColors.value.cacheRead,
        backgroundColor: `${chartColors.value.cacheReadREDACTED20`,
        fill: true,
        tension: 0.3
      REDACTED,
      {
        label: 'Cache Hit Rate',
        data: props.trendData.map((d) => {
          const totalPromptTokens = d.input_tokens + d.cache_read_tokens + d.cache_creation_tokens
          return totalPromptTokens > 0 ? (d.cache_read_tokens / totalPromptTokens) * 100 : 0
        REDACTED),
        borderColor: chartColors.value.cacheHitRate,
        backgroundColor: `${chartColors.value.cacheHitRateREDACTED20`,
        borderDash: [5, 5],
        fill: false,
        tension: 0.3,
        yAxisID: 'yPercent'
      REDACTED
    ]
  REDACTED
REDACTED)

const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  REDACTED,
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        REDACTED
      REDACTED
    REDACTED,
    tooltip: {
      callbacks: {
        label: (context: any) => {
          if (context.dataset.yAxisID === 'yPercent') {
            return `${context.dataset.labelREDACTED: ${context.raw.toFixed(1)REDACTED%`
          REDACTED
          return `${context.dataset.labelREDACTED: ${formatTokens(context.raw)REDACTED`
        REDACTED,
        footer: (tooltipItems: any) => {
          const dataIndex = tooltipItems[0]?.dataIndex
          if (dataIndex !== undefined && props.trendData[dataIndex]) {
            const data = props.trendData[dataIndex]
            return `Actual: $${formatCost(data.actual_cost)REDACTED | Standard: $${formatCost(data.cost)REDACTED`
          REDACTED
          return ''
        REDACTED
      REDACTED
    REDACTED
  REDACTED,
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      REDACTED,
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        REDACTED
      REDACTED
    REDACTED,
    y: {
      grid: {
        color: chartColors.value.grid
      REDACTED,
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        REDACTED,
        callback: (value: string | number) => formatTokens(Number(value))
      REDACTED
    REDACTED,
    yPercent: {
      position: 'right' as const,
      min: 0,
      max: 100,
      grid: {
        drawOnChartArea: false
      REDACTED,
      ticks: {
        color: chartColors.value.cacheHitRate,
        font: {
          size: 10
        REDACTED,
        callback: (value: string | number) => `${valueREDACTED%`
      REDACTED
    REDACTED
  REDACTED
REDACTED))

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)REDACTEDB`
  REDACTED else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)REDACTEDM`
  REDACTED else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)REDACTEDK`
  REDACTED
  return value.toLocaleString()
REDACTED

const formatCost = (value: number): string => {
  if (value >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  REDACTED else if (value >= 1) {
    return value.toFixed(2)
  REDACTED else if (value >= 0.01) {
    return value.toFixed(3)
  REDACTED
  return value.toFixed(4)
REDACTED
</script>
