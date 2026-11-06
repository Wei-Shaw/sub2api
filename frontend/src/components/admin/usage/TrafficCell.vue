<template>
  <!--
    One shared grid holds the optional admin label and every measurement part of every
    line, so request and response byte values form right-aligned numeric columns. User
    rows omit the visible label; their direction icons retain the full client direction
    as hover and accessibility text. A line without a compression rate leaves that track
    empty instead of shifting the response column. The wrappers stay grouped in the DOM
    (and keep their test ids) through `display: contents`.
  -->
  <div
    data-testid="traffic-cell"
    class="grid items-center gap-x-1 gap-y-1 text-xs"
    :class="showUpstream
      ? 'grid-cols-[max-content_max-content_max-content_max-content_max-content_max-content_max-content]'
      : 'grid-cols-[max-content_max-content_max-content]'"
    :title="t('usage.traffic.bodyOnlyHint')"
  >
    <div
      v-for="row in rows"
      :key="row.key"
      :data-testid="`traffic-${row.key}`"
      class="contents"
    >
      <span v-if="showUpstream" class="pr-2 text-gray-500 dark:text-gray-400">{{ row.label }}</span>
      <div
        v-for="(measurement, index) in row.measurements"
        :key="measurement.key"
        :data-testid="`traffic-${row.key}-${measurement.key}`"
        class="contents"
      >
        <span
          v-if="measurement.label"
          role="img"
          class="inline-flex items-center"
          :class="index > 0 ? 'pl-2' : ''"
          :aria-label="measurement.label"
          :title="measurement.label"
          :data-testid="`traffic-${row.key}-${measurement.key}-direction`"
        >
          <Icon
            :name="measurement.key === 'request' ? 'arrowUp' : 'arrowDown'"
            size="sm"
            class="h-3.5 w-3.5"
            :class="measurement.key === 'request' ? 'text-emerald-500' : 'text-violet-500'"
            aria-hidden="true"
          />
        </span>
        <span data-testid="traffic-actual" class="justify-self-end whitespace-nowrap font-medium tabular-nums text-gray-900 dark:text-white">
          {{ formatTrafficBytes(measurement.value?.actual_bytes) }}
        </span>
        <span class="inline-flex items-center gap-1 whitespace-nowrap">
          <template v-if="compressionRate(measurement.value) !== null">
            <span
              data-testid="traffic-compression"
              class="whitespace-nowrap tabular-nums text-gray-500 dark:text-gray-400"
              :aria-label="`${t('usage.traffic.compressionRate')} ${compressionRate(measurement.value)}`"
            >{{ compressionRate(measurement.value) }}</span>
            <HelpTooltip
              class="ml-0"
              width-class="w-max max-w-[calc(100vw-2rem)]"
              :content="`${t('usage.traffic.original')} ${formatTrafficBytes(measurement.value?.original_bytes)}`"
              data-testid="traffic-original-tooltip"
            >
              <template #trigger>
                <span
                  role="img"
                  class="inline-flex cursor-help items-center text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
                  :aria-label="t('usage.traffic.original')"
                  :title="t('usage.traffic.original')"
                  data-testid="traffic-original-icon"
                >
                  <Icon name="infoCircle" size="xs" aria-hidden="true" />
                </span>
              </template>
            </HelpTooltip>
          </template>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import type { TrafficLink, TrafficMeasurement, UsageTraffic } from '@/types'
import { formatTrafficBytes, formatTrafficCompressionRate } from '@/utils/traffic'

interface TrafficRow {
  key: string
  label: string
  measurements: {
    key: string
    label?: string
    value?: TrafficMeasurement | null
  }[]
}

const props = withDefaults(defineProps<{
  traffic?: UsageTraffic | null
  showUpstream?: boolean
}>(), {
  showUpstream: false,
})
const { t } = useI18n()

function compressionRate(value?: TrafficMeasurement | null): string | null {
  if (value?.compressed !== true) return null
  const rate = formatTrafficCompressionRate(value.original_bytes, value.actual_bytes)
  return rate === '—' ? null : rate
}

const linkRow = (key: 'upstream' | 'downstream', link?: TrafficLink | null): TrafficRow => ({
  key,
  label: t(`usage.traffic.${key}`),
  measurements: [
    { key: 'request', label: t('usage.traffic.request'), value: link?.request },
    { key: 'response', label: t('usage.traffic.response'), value: link?.response },
  ],
})

const rows = computed<TrafficRow[]>(() => {
  if (props.showUpstream) {
    return [linkRow('upstream', props.traffic?.upstream), linkRow('downstream', props.traffic?.downstream)]
  }
  // Deliberately project only downstream, even if an API accidentally sends upstream.
  return [
    {
      key: 'client-upload',
      label: t('usage.traffic.clientUpload'),
      measurements: [{ key: 'request', label: t('usage.traffic.clientUpload'), value: props.traffic?.downstream?.request }],
    },
    {
      key: 'client-download',
      label: t('usage.traffic.clientDownload'),
      measurements: [{ key: 'response', label: t('usage.traffic.clientDownload'), value: props.traffic?.downstream?.response }],
    },
  ]
})
</script>
