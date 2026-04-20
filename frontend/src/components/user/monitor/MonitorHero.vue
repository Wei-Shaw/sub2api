<template>
  <section class="pt-8 pb-10 md:pb-14">
    <div class="text-xs font-medium tracking-widest uppercase text-gray-400 dark:text-gray-500 mb-4">
      {{ t('channelStatus.hero.breadcrumb') }}
    </div>
    <div class="flex flex-col gap-6 md:flex-row md:items-end md:justify-between">
      <div class="min-w-0">
        <h1
          class="text-5xl md:text-6xl xl:text-7xl font-bold leading-[1.05] tracking-tight text-gray-900 dark:text-gray-50"
        >
          {{ t('channelStatus.hero.title') }}
        </h1>
        <p class="mt-4 text-sm md:text-base text-gray-500 dark:text-gray-400 max-w-xl">
          {{ t('channelStatus.hero.subtitleZh') }}
        </p>
        <p class="mt-1 text-xs md:text-sm italic opacity-80 text-gray-500 dark:text-gray-400 max-w-xl">
          {{ t('channelStatus.hero.subtitleEn') }}
        </p>
      </div>

      <div class="flex flex-col items-start md:items-end gap-2.5">
        <div
          role="tablist"
          class="inline-flex p-0.5 rounded-xl bg-gray-100 dark:bg-dark-800 border border-gray-200/60 dark:border-dark-700/60 text-xs"
        >
          <button
            v-for="opt in windowOptions"
            :key="opt.value"
            type="button"
            role="tab"
            :aria-selected="window === opt.value"
            class="px-3 py-1.5 rounded-lg transition-colors"
            :class="window === opt.value
              ? 'bg-white dark:bg-dark-700 shadow-sm text-gray-900 dark:text-white font-semibold'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="emit('update:window', opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>

        <div class="flex items-center gap-2">
          <span
            class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-semibold tracking-wider uppercase"
            :class="overallChipClass"
          >
            <span
              class="w-1.5 h-1.5 rounded-full mr-1.5"
              :class="overallDotClass"
            ></span>
            {{ overallLabel }}
          </span>
          <button
            type="button"
            class="h-8 w-8 rounded-lg flex items-center justify-center text-gray-500 hover:text-gray-700 hover:bg-gray-100 dark:text-gray-400 dark:hover:text-gray-200 dark:hover:bg-dark-700 transition-colors disabled:opacity-50"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="emit('refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>

        <div class="text-xs text-gray-500 dark:text-gray-400 tabular-nums text-right">
          {{ updatedLabel }}<span v-if="intervalSeconds > 0"> · {{ t('monitorCommon.pollEvery', { n: intervalSeconds }) }}</span>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

export type MonitorWindow = '7d' | '15d' | '30d'
export type OverallStatus = 'operational' | 'degraded' | 'unavailable'

const props = defineProps<{
  overallStatus: OverallStatus
  updatedAt: string | null
  intervalSeconds: number
  window: MonitorWindow
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'update:window', value: MonitorWindow): void
  (e: 'refresh'): void
}>()

const { t } = useI18n()
const { formatRelativeTime } = useChannelMonitorFormat()

const windowOptions = computed<{ value: MonitorWindow; label: string }[]>(() => [
  { value: '7d', label: t('channelStatus.windowTab.7d') },
  { value: '15d', label: t('channelStatus.windowTab.15d') },
  { value: '30d', label: t('channelStatus.windowTab.30d') },
])

const overallLabel = computed(() => t(`channelStatus.overall.${props.overallStatus}`))

const overallChipClass = computed(() => {
  switch (props.overallStatus) {
    case 'operational':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
    case 'degraded':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
    case 'unavailable':
    default:
      return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
  }
})

const overallDotClass = computed(() => {
  switch (props.overallStatus) {
    case 'operational':
      return 'bg-emerald-500 animate-pulse'
    case 'degraded':
      return 'bg-amber-500 animate-pulse'
    case 'unavailable':
    default:
      return 'bg-red-500 animate-pulse'
  }
})

const updatedLabel = computed(() => {
  if (!props.updatedAt) return t('monitorCommon.updatedAt', { time: '--' })
  return t('monitorCommon.updatedAt', { time: formatRelativeTime(props.updatedAt) })
})
</script>
