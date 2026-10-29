<template>
  <div
    class="quota-pools text-[11px] leading-5"
    :class="{
      'quota-pools-expanded': expanded,
      'quota-pools-many': groups.some((group) => group.length > 2)
    }"
    :title="updatedAt ? t('admin.quota.collected', { time: date(updatedAt) }) : undefined"
  >
    <div v-for="(group, index) in groups" :key="index" :class="{ 'mt-2': index > 0 }">
      <div
        class="quota-grid gap-x-3 text-[10px] text-gray-400 dark:text-gray-500"
        :style="gridStyle(group)"
      >
        <span class="whitespace-nowrap" :title="t('admin.quota.legend')">{{
          group.every((pool) => pool.windows.every((window) => window.id === 'balance'))
            ? t('admin.quota.item')
            : t('admin.quota.period')
        }}</span>
        <span v-for="pool in group" :key="pool.id" class="text-right truncate" :title="pool.name">{{
          pool.name
        }}</span>
      </div>
      <div
        v-for="id in windowIds(group)"
        :key="id"
        class="quota-grid items-center gap-x-3"
        :style="gridStyle(group)"
      >
        <span
          class="whitespace-nowrap font-medium text-gray-600 dark:text-gray-300"
          :title="windowTitle(id)"
          >{{ label(id) }}</span
        >
        <div
          v-for="pool in group"
          :key="pool.id"
          class="quota-metric flex items-center justify-end"
          :title="detail(pool, id)"
        >
          <QuotaMeter
            :value="remaining(pool, id)"
            :text="windowFor(pool, id)?.display"
            :wide="expanded"
          />
          <span
            v-if="hasReset(pool)"
            class="quota-reset whitespace-nowrap text-right text-[9px] tabular-nums text-gray-400"
            :aria-label="t('admin.quota.resetIn', { time: countdown(pool, id) })"
            >{{ countdown(pool, id) }}</span
          >
        </div>
      </div>
    </div>
    <span v-if="!groups.length" class="text-gray-400">--</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import QuotaMeter from './QuotaMeter.vue'
import { useQuotaNow } from '@/composables/useQuotaNow'
import {
  sortQuotaWindows,
  quotaPoolGroups,
  quotaWindowLabel,
  type QuotaPool
} from '@/utils/quotaPools'
import { quotaCountdown } from '@/utils/quotaCountdown'

const props = defineProps<{
  pools: QuotaPool[]
  updatedAt?: string | null
  expanded?: boolean
  maxColumns?: number
}>()
const { t } = useI18n()
const now = useQuotaNow()
const wideScreen = useMediaQuery('(min-width: 768px)')
const groups = computed(() =>
  quotaPoolGroups(props.pools, props.maxColumns ?? (wideScreen.value ? props.pools.length : 2))
)
const hasReset = (pool: QuotaPool) => pool.windows.some((window) => !!window.resets_at)
const windowIds = (pools: QuotaPool[]) =>
  sortQuotaWindows(
    [...new Set(pools.flatMap((pool) => pool.windows.map((window) => window.id)))].map((id) => ({
      id
    }))
  ).map((w) => w.id)
const columnWidth = (pool: QuotaPool) =>
  Math.max(
    hasReset(pool) ? (props.expanded ? 100 : 106) : 56,
    Math.min(140, pool.name.length * 5 + 8),
    ...pool.windows.map((window) => (window.display?.length ?? 0) * 6 + 8)
  )
const gridStyle = (pools: QuotaPool[]) => ({
  gridTemplateColumns: `minmax(24px, max-content) ${pools
    .map((pool) =>
      props.expanded ? `minmax(${columnWidth(pool)}px, 1fr)` : `${columnWidth(pool)}px`
    )
    .join(' ')}`
})
const label = quotaWindowLabel
const windowTitle = (id: string) =>
  id === 'weekly'
    ? t('admin.quota.weeklyQuota')
    : id === '5h'
      ? t('admin.quota.fiveHourQuota')
      : t('admin.quota.windowQuota', { window: label(id) })
const windowFor = (pool: QuotaPool, id: string) => pool.windows.find((window) => window.id === id)
const remaining = (pool: QuotaPool, id: string) => windowFor(pool, id)?.remaining ?? null
const date = (value: string) => new Date(value).toLocaleString()
const countdown = (pool: QuotaPool, id: string) =>
  quotaCountdown(windowFor(pool, id)?.resets_at, now.value.getTime())
const detail = (pool: QuotaPool, id: string) => {
  const window = windowFor(pool, id)
  return [
    t('admin.quota.cellDetail', {
      name: pool.name,
      window: windowTitle(id),
      value:
        window?.display ??
        (window?.remaining == null
          ? '--'
          : t('admin.quota.remaining', { value: (window.remaining * 100).toFixed(1) }))
    }),
    window?.resets_at ? t('admin.quota.resetAt', { time: date(window.resets_at) }) : '',
    props.updatedAt ? t('admin.quota.collected', { time: date(props.updatedAt) }) : ''
  ]
    .filter(Boolean)
    .join('\n')
}
</script>
<style scoped>
.quota-pools {
  width: max-content;
  max-width: 100%;
}
.quota-pools-expanded {
  width: 100%;
  max-width: 520px;
}
.quota-pools-expanded.quota-pools-many {
  max-width: 100%;
}
.quota-grid {
  display: grid;
}
.quota-metric {
  column-gap: 3px;
}
.quota-reset {
  width: 40px;
  flex-shrink: 0;
  text-align: right;
}
.quota-pools-expanded .quota-metric {
  padding-inline-start: 8px;
  column-gap: 4px;
}
.quota-pools-expanded .quota-reset {
  width: 44px;
}
@media (min-width: 640px) {
  .quota-pools-expanded .quota-grid {
    column-gap: 16px;
  }
  .quota-pools-expanded {
    font-size: 12px;
    line-height: 24px;
  }
  .quota-pools-expanded .quota-reset {
    font-size: 10px;
  }
}
</style>
