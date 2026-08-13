<template>
  <article class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <span class="h-2.5 w-2.5 flex-none rounded-full" :class="statusDotClass" />
          <h3 class="truncate font-semibold text-gray-900 dark:text-white">{{ monitor.name }}</h3>
        </div>
        <p class="mt-1 text-xs font-medium uppercase tracking-wide text-gray-400">{{ typeLabel }}</p>
      </div>
      <button class="btn btn-secondary px-3 py-1.5 text-xs" @click="$emit('edit', monitor)">{{ t('common.edit') }}</button>
    </div>

    <p class="mt-3 truncate text-sm text-gray-500 dark:text-gray-400" :title="monitor.base_url">{{ monitor.base_url }}</p>

    <div class="my-4 border-t border-gray-100 dark:border-dark-700" />

    <dl v-if="monitor.type === 'sub2api'" class="space-y-2 text-sm">
      <div class="flex justify-between gap-3"><dt class="text-gray-500">{{ t('admin.upstreamBalance.remaining') }}</dt><dd class="font-semibold" :class="isLowBalance ? 'text-amber-600' : ''">{{ money(monitor.balance_display?.quota_remaining_usd) }}</dd></div>
      <div class="flex justify-between gap-3"><dt class="text-gray-500">{{ t('admin.upstreamBalance.account') }}</dt><dd class="truncate">{{ monitor.balance_display?.username || monitor.balance_display?.email || '—' }}</dd></div>
    </dl>
    <dl v-else class="space-y-2 text-sm">
      <div class="flex justify-between gap-3"><dt class="text-gray-500">{{ t('admin.upstreamBalance.remaining') }}</dt><dd class="font-semibold" :class="isLowBalance ? 'text-amber-600' : ''">{{ money(monitor.balance_display?.quota_remaining_usd) }}</dd></div>
      <div class="flex justify-between gap-3"><dt class="text-gray-500">{{ t('admin.upstreamBalance.used') }}</dt><dd>{{ money(monitor.balance_display?.used_quota_usd) }}</dd></div>
      <div class="flex justify-between gap-3"><dt class="text-gray-500">{{ t('admin.upstreamBalance.requests') }}</dt><dd>{{ monitor.balance_display?.request_count ?? '—' }}</dd></div>
      <div class="flex justify-between gap-3"><dt class="text-gray-500">{{ t('admin.upstreamBalance.group') }}</dt><dd>{{ monitor.balance_display?.group || '—' }}</dd></div>
    </dl>

    <div v-if="monitor.balance_display?.rates?.length" class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
      <div class="mb-2 flex items-center justify-between text-xs text-gray-500"><span>{{ t('admin.upstreamBalance.groups') }}</span><span>{{ monitor.balance_display.rates.length }}</span></div>
      <div class="flex max-h-28 flex-wrap gap-1 overflow-y-auto">
        <span v-for="rate in sortedRates(monitor.balance_display.rates)" :key="rate.name" class="rounded border px-1.5 py-0.5 text-[11px] leading-4" :class="rateTagClass(rate.ratio)" :title="rate.description">
          {{ rate.name }} <strong>{{ rate.ratio.toFixed(2) }}</strong>
        </span>
      </div>
    </div>

    <div v-if="monitor.last_probe_error" class="mt-3 rounded-lg bg-red-50 p-2 text-xs text-red-600 dark:bg-red-500/10 dark:text-red-400" :title="monitor.last_probe_error">
      {{ monitor.last_probe_error }}
    </div>

    <div class="mt-4 flex items-center justify-between gap-3 border-t border-gray-100 pt-4 text-xs dark:border-dark-700">
      <div>
        <span :class="statusTextClass">{{ statusLabel }}</span>
        <span class="ml-2 text-gray-400">{{ lastUpdated }}</span>
      </div>
      <button class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="probing" @click="$emit('probe', monitor)">
        {{ probing ? t('common.loading') : t('admin.upstreamBalance.probe') }}
      </button>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UpstreamBalanceMonitor, UpstreamRate } from '@/api/admin/upstreamBalance'

const props = defineProps<{ monitor: UpstreamBalanceMonitor; probing?: boolean }>()
defineEmits<{ edit: [monitor: UpstreamBalanceMonitor]; probe: [monitor: UpstreamBalanceMonitor] }>()
const { t, locale } = useI18n()

const typeLabel = computed(() => props.monitor.type === 'sub2api' ? 'Sub2API' : 'New-API')
const isLowBalance = computed(() => typeof props.monitor.balance_display?.quota_remaining_usd === 'number' && props.monitor.balance_display.quota_remaining_usd < props.monitor.low_balance_threshold_usd)
const visualStatus = computed(() => !props.monitor.enabled ? 'disabled' : props.monitor.last_probe_status === 'failed' ? 'failed' : isLowBalance.value ? 'low' : props.monitor.last_probe_status)
const statusDotClass = computed(() => ({ ok: 'bg-emerald-500', low: 'bg-amber-400', failed: 'bg-red-500', pending: 'bg-gray-400', disabled: 'bg-gray-300' }[visualStatus.value] || 'bg-gray-400'))
const statusTextClass = computed(() => ({ ok: 'text-emerald-600', low: 'text-amber-600', failed: 'text-red-600', pending: 'text-gray-500', disabled: 'text-gray-400' }[visualStatus.value]))
const statusLabel = computed(() => t(`admin.upstreamBalance.status.${visualStatus.value}`))
const lastUpdated = computed(() => props.monitor.last_probe_at ? new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(props.monitor.last_probe_at)) : t('admin.upstreamBalance.never'))
function money(value?: number) { return typeof value === 'number' ? `$${value.toFixed(2)}` : '—' }
function sortedRates(rates: UpstreamRate[]) {
  return [...rates].sort((a, b) => {
    const ratio = a.ratio - b.ratio
    const priority = rateProviderPriority(a.name) - rateProviderPriority(b.name)
    return ratio || priority || a.name.localeCompare(b.name, locale.value)
  })
}
function rateProviderPriority(name: string) {
  const normalized = name.toLowerCase()
  if (/anthropic|claude|(^|[^a-z])cc([^a-z]|$)|kiro/.test(normalized)) return 0
  if (/openai|chatgpt|gpt|codex|(^|[^a-z])o[134]([^a-z]|$)/.test(normalized)) return 1
  if (/google|gemini|vertex/.test(normalized)) return 2
  if (/xai|grok/.test(normalized)) return 3
  return 4
}
function rateTagClass(ratio: number) {
  if (ratio < 1) return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
  if (ratio <= 1.05) return 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200'
  if (ratio <= 2) return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
  return 'border-red-200 bg-red-50 text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300'
}
</script>
