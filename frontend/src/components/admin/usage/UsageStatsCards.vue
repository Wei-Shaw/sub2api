<template>
  <!--
    One panel, four metrics — not four floating cards each led by a pastel icon
    tile. The tile carried no information: `document`, `dollar` and `clock` each
    restated the label sitting next to it.
  -->
  <Surface data-testid="usage-stats">
    <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
      <div class="min-w-0">
        <Metric
          :label="t('usage.totalRequests')"
          :value="numOrNull(stats?.total_requests)"
          :caption="t('usage.inSelectedRange')"
        />
      </div>

      <div class="min-w-0">
        <Metric :label="t('usage.totalTokens')" :value="numOrNull(stats?.total_tokens)" />
        <!--
          The cache split used to live in a hover-only tooltip: unreachable on
          touch, invisible to anyone reading the page rather than pointing at
          it, and it took a 224px floating panel to say four numbers. Four
          hairline rows cost less space and are always readable.
        -->
        <dl class="mt-2 space-y-0.5">
          <div
            v-for="row in tokenRows"
            :key="row.label"
            class="flex items-baseline justify-between gap-2 text-xs"
          >
            <dt class="min-w-0 truncate text-2xs text-ink-tertiary">{{ row.label }}</dt>
            <dd class="shrink-0"><NumCell :value="row.value" compact /></dd>
          </div>
        </dl>
      </div>

      <div class="min-w-0">
        <Metric
          :label="t('usage.totalCost')"
          :value="numOrNull(stats?.total_actual_cost)"
          :precision="4"
          unit="USD"
        />
        <dl class="mt-2 space-y-0.5">
          <div
            v-if="showAccountCost && totalAccountCost != null"
            class="flex items-baseline justify-between gap-2 text-xs"
          >
            <dt class="min-w-0 truncate text-2xs text-ink-tertiary">
              {{ t('usage.accountCost') }}
            </dt>
            <dd class="shrink-0"><NumCell :value="totalAccountCost" :precision="4" /></dd>
          </div>
          <div class="flex items-baseline justify-between gap-2 text-xs">
            <dt class="min-w-0 truncate text-2xs text-ink-tertiary">
              {{ t('usage.standardCost') }}
            </dt>
            <!--
              The strike-through is the point of this row: it says the list
              price is not what was charged. That is a line, not a colour.
            -->
            <dd class="shrink-0" :class="strikeStandardCost && 'line-through'">
              <NumCell :value="numOrNull(stats?.total_cost)" :precision="4" />
            </dd>
          </div>
        </dl>
      </div>

      <div class="min-w-0">
        <!--
          Milliseconds throughout. The old formatter switched to seconds past
          1000, so the same measurement could not be compared with itself.
        -->
        <Metric
          :label="t('usage.avgDuration')"
          :value="numOrNull(stats?.average_duration_ms)"
          unit="ms"
        />
      </div>
    </div>
  </Surface>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { UsageStatsResponse } from '@/types'
import Metric from '@/components/common/Metric.vue'
import NumCell from '@/components/common/NumCell.vue'
import Surface from '@/components/common/Surface.vue'

const props = withDefaults(
  defineProps<{
    stats: (AdminUsageStatsResponse | UsageStatsResponse) | null
    showAccountCost?: boolean
    strikeStandardCost?: boolean
  }>(),
  {
    showAccountCost: true,
    strikeStandardCost: false,
  }
)

const { t } = useI18n()

/** A stat the backend has not reported is not a stat that is zero. */
const numOrNull = (value: unknown): number | null => {
  if (value === null || value === undefined || value === '') return null
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}

const totalAccountCost = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_account_cost?: number }) | null
  return stats?.total_account_cost ?? null
})
const showAccountCost = computed(() => props.showAccountCost)
const strikeStandardCost = computed(() => props.strikeStandardCost)

const tokenRows = computed(() => [
  { label: t('usage.in'), value: numOrNull(props.stats?.total_input_tokens) },
  { label: t('usage.out'), value: numOrNull(props.stats?.total_output_tokens) },
  { label: t('usage.cacheTotal'), value: numOrNull(props.stats?.total_cache_tokens) },
  {
    label: t('usage.cacheCreationTokensLabel'),
    value: numOrNull(props.stats?.total_cache_creation_tokens),
  },
  {
    label: t('usage.cacheReadTokensLabel'),
    value: numOrNull(props.stats?.total_cache_read_tokens),
  },
])
</script>
