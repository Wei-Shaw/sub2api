<template>
  <div>
    <!-- 列表骨架屏 -->
    <div
      v-if="loading && items.length === 0"
      class="flex flex-col gap-2"
    >
      <div
        v-for="i in 10"
        :key="i"
        class="px-4 py-2 rounded-xl h-[60px] bg-white/70 dark:bg-dark-800/60 border border-gray-200/80 dark:border-dark-700/70 animate-pulse flex items-center gap-3"
      >
        <div class="w-8 h-8 rounded-lg bg-gray-200 dark:bg-dark-700 flex-shrink-0"></div>
        <div class="h-4 w-48 rounded bg-gray-200 dark:bg-dark-700"></div>
        <div class="h-5 w-16 rounded-full bg-gray-200 dark:bg-dark-700"></div>
        <div class="ml-auto h-4 w-40 rounded bg-gray-100 dark:bg-dark-900/40"></div>
        <div class="h-4 w-32 rounded bg-gray-100 dark:bg-dark-900/40"></div>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('channelStatus.empty.title')"
      :description="t('channelStatus.empty.description')"
    />

    <!-- 紧凑列表（默认） -->
    <div v-else-if="viewMode === 'list'" class="flex flex-col gap-2">
      <MonitorRow
        v-for="item in items"
        :key="item.id"
        :item="item"
        :window="window"
        :availability-value="resolveAvailability(item)"
        :countdown-seconds="countdownSeconds"
        @click="emit('cardClick', item)"
      />
    </div>

    <!-- 卡片网格（可切换回去） -->
    <div
      v-else
      class="grid gap-5 grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4"
    >
      <MonitorCard
        v-for="item in items"
        :key="item.id"
        :item="item"
        :window="window"
        :availability-value="resolveAvailability(item)"
        :countdown-seconds="countdownSeconds"
        @click="emit('cardClick', item)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UserMonitorView, UserMonitorDetail } from '@/api/channelMonitor'
import EmptyState from '@/components/common/EmptyState.vue'
import MonitorCard from './MonitorCard.vue'
import MonitorRow from './MonitorRow.vue'

const props = withDefaults(defineProps<{
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  countdownSeconds: number
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
  viewMode?: 'list' | 'card'
}>(), {
  viewMode: 'list',
})

const emit = defineEmits<{
  (e: 'cardClick', item: UserMonitorView): void
}>()

const { t } = useI18n()

function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') {
    return item.availability_7d ?? null
  }
  const detail = props.detailCache[item.id]
  if (!detail) return null
  const primary = detail.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}
</script>
