<template>
  <div class="group/usage relative text-sm">
    <div class="flex items-center gap-1.5">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.users.today') REDACTEDREDACTED:</span>
      <span class="font-medium text-gray-900 dark:text-white">${{ today.toFixed(4) REDACTEDREDACTED</span>
      <Icon
        v-if="hasBreakdown"
        name="infoCircle"
        size="xs"
        class="text-gray-400 dark:text-gray-500"
      />
    </div>
    <div class="mt-0.5 flex items-center gap-1.5">
      <span class="text-gray-500 dark:text-gray-400">{{ t('admin.users.total') REDACTEDREDACTED:</span>
      <span class="font-medium text-gray-900 dark:text-white">${{ total.toFixed(4) REDACTEDREDACTED</span>
    </div>

    <div
      v-if="hasBreakdown"
      class="pointer-events-none absolute left-full top-0 z-50 ml-2 min-w-[220px] whitespace-nowrap rounded-md bg-gray-900 px-3 py-2 text-xs text-white opacity-0 shadow-xl transition-opacity duration-100 group-hover/usage:opacity-100 dark:bg-dark-600"
    >
      <div class="mb-1.5 flex items-center justify-between gap-3 border-b border-white/10 pb-1 text-[11px] opacity-80">
        <span>{{ t('admin.users.platformBreakdown') REDACTEDREDACTED</span>
        <span class="font-mono">{{ t('admin.users.today') REDACTEDREDACTED / {{ t('admin.users.total') REDACTEDREDACTED</span>
      </div>
      <div
        v-for="item in sortedBreakdown"
        :key="item.platform"
        class="flex items-center justify-between gap-3 py-0.5"
        :class="{ 'opacity-70 italic': item.isOther REDACTED"
      >
        <span class="capitalize">
          {{ item.isOther ? t('admin.users.platformOther') : platformLabel(item.platform) REDACTEDREDACTED
        </span>
        <span class="font-mono">
          ${{ item.today_actual_cost.toFixed(4) REDACTEDREDACTED
          <span class="opacity-50">/</span>
          ${{ item.total_actual_cost.toFixed(4) REDACTEDREDACTED
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { PlatformUsage REDACTED from '@/api/admin/dashboard'

const props = defineProps<{
  today: number
  total: number
  byPlatform?: PlatformUsage[]
REDACTED>()

const { t REDACTED = useI18n()

// 与 UserDashboardStats 保持一致：把"总值 - 各平台之和"的差作为"其他"行展示，
// 避免 tooltip 内各平台费用加总与列首总值对不上。
const OTHER_THRESHOLD = 0.0001

interface BreakdownRow {
  platform: string
  today_actual_cost: number
  total_actual_cost: number
  isOther?: boolean
REDACTED

const sortedBreakdown = computed<BreakdownRow[]>(() => {
  const list = props.byPlatform ?? []
  const rows: BreakdownRow[] = [...list]
    .sort((a, b) => b.total_actual_cost - a.total_actual_cost)
    .map((p) => ({ ...p REDACTED))

  const sumTotal = rows.reduce((s, r) => s + r.total_actual_cost, 0)
  const sumToday = rows.reduce((s, r) => s + r.today_actual_cost, 0)
  const diffTotal = Math.max(0, props.total - sumTotal)
  const diffToday = Math.max(0, props.today - sumToday)
  if (diffTotal > OTHER_THRESHOLD || diffToday > OTHER_THRESHOLD) {
    rows.push({
      platform: '__other__',
      today_actual_cost: diffToday,
      total_actual_cost: diffTotal,
      isOther: true
    REDACTED)
  REDACTED
  return rows
REDACTED)

const hasBreakdown = computed(() => sortedBreakdown.value.length > 0)

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity'
REDACTED

function platformLabel(platform: string): string {
  return PLATFORM_LABELS[platform] ?? platform
REDACTED
</script>
