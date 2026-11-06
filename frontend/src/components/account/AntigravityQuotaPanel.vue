<template>
  <div class="w-56 max-w-full space-y-2.5" data-test="antigravity-quota-panel">
    <p v-if="usage?.antigravity_quota_stale" role="status" class="text-[10px] text-amber-600 dark:text-amber-400">
      {{ t('antigravityQuota.stale') }}
    </p>
    <p v-if="!hasWindows" class="whitespace-normal text-[10px] text-gray-500 dark:text-gray-400">
      {{ sections.length ? t('antigravityQuota.fallback') : t('antigravityQuota.empty') }}
    </p>
    <section v-for="section in sections" :key="section.key" class="space-y-1" :data-test="section.buckets.length ? 'quota-group' : 'model-group'">
      <div class="flex min-w-0 items-center justify-between gap-2">
        <button
          v-if="section.models.length"
          type="button"
          class="group flex min-w-0 items-center gap-1 rounded text-left text-xs font-medium text-gray-700 hover:text-primary-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-gray-200"
          :aria-expanded="expanded.has(section.key)"
          :aria-label="t('antigravityQuota.modelDetails', { group: sectionName(section) })"
          :title="section.description || t('antigravityQuota.modelHint')"
          data-test="quota-model-toggle"
          @click="toggle(section.key)"
        >
          <svg :class="['h-3 w-3 shrink-0 transition-transform', expanded.has(section.key) ? 'rotate-90' : '']" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path d="m6 4 4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span class="truncate">{{ sectionName(section) }}</span>
          <span class="text-[10px] font-normal text-gray-400">{{ section.models.length }}</span>
        </button>
        <span v-else class="text-xs font-medium text-gray-700 dark:text-gray-200" :title="section.description">{{ sectionName(section) }}</span>
        <span class="shrink-0 text-[9px] text-gray-400">{{ t('antigravityQuota.heading') }}</span>
      </div>
      <div class="space-y-1 pl-4">
        <div v-for="bucket in section.buckets" :key="bucket.id" :data-bucket="bucket.id">
          <UsageProgressBar
            v-if="validQuota(bucket)"
            :label="windowName(bucket)"
            :utilization="bucket.utilization!"
            :resets-at="bucket.resets_at"
            :color="bucket.window === 'weekly' ? 'emerald' : 'indigo'"
            :title="resetTitle(bucket.resets_at)"
            label-width="auto"
            precise
          />
          <div v-else class="flex flex-wrap gap-x-2 text-[10px] text-gray-500 dark:text-gray-400">
            <span>{{ windowName(bucket) }}</span>
            <span>{{ bucket.disabled ? t('antigravityQuota.disabled') : t('antigravityQuota.unknown') }}</span>
          </div>
        </div>
      </div>
      <div v-if="expanded.has(section.key)" class="ml-1.5 space-y-1 border-l border-gray-200 py-1 pl-2 dark:border-gray-700" data-test="quota-models" :title="t('antigravityQuota.modelHint')">
        <div v-for="[model, quota] in section.models" :key="model" class="flex min-w-0 items-center justify-between gap-2 text-[10px] leading-4" :data-model="model">
          <span class="min-w-0 truncate text-gray-500 dark:text-gray-400" :title="model">{{ model }}</span>
          <span class="shrink-0 font-medium tabular-nums" :class="quotaColor(quota.utilization)" :title="resetTitle(quota.reset_time)">{{ formatQuotaPercent(quota.utilization) }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AntigravityQuotaBucket, AntigravityUsageInfo } from '@/types/antigravityUsage'
import { formatQuotaPercent } from '@/utils/quotaPercent'
import { antigravityQuotaSections, type QuotaSection } from './antigravityQuotaSections'
import UsageProgressBar from './UsageProgressBar.vue'

const props = defineProps<{ usage: AntigravityUsageInfo | null }>()
const { t, locale } = useI18n()
const sections = computed(() => antigravityQuotaSections(props.usage))
const hasWindows = computed(() => sections.value.some(section => section.buckets.length > 0))
const expanded = ref(new Set<string>())
const sectionName = (section: QuotaSection) => section.labelKey ? t(section.labelKey) : section.name

function toggle(key: string) {
  if (expanded.value.has(key)) expanded.value.delete(key)
  else expanded.value.add(key)
}

function windowName(bucket: AntigravityQuotaBucket): string {
  if (bucket.window === '5h') return t('antigravityQuota.fiveHour')
  if (bucket.window === 'weekly') return t('antigravityQuota.weekly')
  return bucket.name || bucket.window || bucket.id
}

function validQuota(bucket: AntigravityQuotaBucket): boolean {
  return !bucket.disabled && typeof bucket.utilization === 'number' && Number.isFinite(bucket.utilization)
}

function resetTitle(raw?: string | null): string | undefined {
  if (!raw || !Number.isFinite(Date.parse(raw))) return undefined
  return t('antigravityQuota.resetsAt') + ' ' + new Date(raw).toLocaleString(locale.value)
}

function quotaColor(value: number): string {
  if (value >= 90) return 'text-red-600 dark:text-red-400'
  if (value >= 75) return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-600 dark:text-gray-300'
}
</script>
