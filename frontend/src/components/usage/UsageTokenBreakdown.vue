<template>
  <div class="flex items-center gap-1.5">
    <div class="space-y-1 text-sm">
      <div class="flex items-center gap-2">
        <span class="inline-flex items-center gap-1"><Icon name="arrowDown" size="sm" class="h-3.5 w-3.5 text-emerald-500" /><b class="font-medium">{{ inputTokens.toLocaleString() }}</b></span>
        <span class="inline-flex items-center gap-1"><Icon name="arrowUp" size="sm" class="h-3.5 w-3.5 text-violet-500" /><b class="font-medium">{{ outputTokens.toLocaleString() }}</b></span>
      </div>
      <div v-if="cacheReadTokens || cacheCreationTokens" class="flex items-center gap-2">
        <span v-if="cacheReadTokens" class="inline-flex items-center gap-1 font-medium text-sky-600 dark:text-sky-400"><svg class="h-3.5 w-3.5 text-sky-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" /></svg>{{ formatCacheTokens(cacheReadTokens) }}</span>
        <span v-if="cacheCreationTokens" class="inline-flex items-center gap-1 font-medium text-amber-600 dark:text-amber-400"><svg class="h-3.5 w-3.5 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>{{ formatCacheTokens(cacheCreationTokens) }} <small v-if="cacheCreation1hTokens" class="rounded bg-orange-100 px-1 text-[10px] dark:bg-orange-500/20">1h</small></span>
      </div>
    </div>
    <div @mouseenter="showTooltip" @mouseleave="open = false">
      <button type="button" class="flex h-4 w-4 items-center justify-center rounded-full bg-gray-100 hover:bg-blue-100 dark:bg-gray-700" :aria-label="t('usage.tokenDetails')" @click="toggleTooltip" @focus="showTooltip" @blur="open = false"><Icon name="infoCircle" size="xs" class="text-gray-400" /></button>
    </div>
  </div>
  <Teleport to="body">
      <div v-if="open" class="fixed z-[9999] w-max -translate-y-1/2 rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl" :style="{ left: `${position.x}px`, top: `${position.y}px` }">
        <div class="mb-1 font-semibold text-gray-300">{{ t('usage.tokenDetails') }}</div>
        <div class="space-y-1.5">
          <div class="flex justify-between gap-8"><span class="text-gray-400">{{ t('admin.usage.inputTokens') }}</span><span>{{ inputTokens.toLocaleString() }}</span></div>
          <div class="flex justify-between gap-8"><span class="text-gray-400">{{ t('admin.usage.outputTokens') }}</span><span>{{ outputTokens.toLocaleString() }}</span></div>
          <div v-if="cacheCreation5mTokens" class="flex justify-between gap-8"><span class="text-gray-400">{{ t('admin.usage.cacheCreation5mTokens') }}</span><span>{{ cacheCreation5mTokens.toLocaleString() }}</span></div>
          <div v-if="cacheCreation1hTokens" class="flex justify-between gap-8"><span class="text-gray-400">{{ t('admin.usage.cacheCreation1hTokens') }}</span><span>{{ cacheCreation1hTokens.toLocaleString() }}</span></div>
          <div v-if="cacheCreationTokens && !cacheCreation5mTokens && !cacheCreation1hTokens" class="flex justify-between gap-8"><span class="text-gray-400">{{ t('admin.usage.cacheCreationTokens') }}</span><span>{{ cacheCreationTokens.toLocaleString() }}</span></div>
          <div v-if="cacheReadTokens" class="flex justify-between gap-8"><span class="text-gray-400">{{ t('admin.usage.cacheReadTokens') }}</span><span>{{ cacheReadTokens.toLocaleString() }}</span></div>
          <div class="flex justify-between gap-8 border-t border-gray-700 pt-1.5"><span class="text-gray-400">{{ t('usage.totalTokens') }}</span><b class="text-blue-400">{{ totalTokens.toLocaleString() }}</b></div>
        </div>
      </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{ inputTokens: number; outputTokens: number; cacheCreationTokens: number; cacheReadTokens: number; cacheCreation5mTokens?: number; cacheCreation1hTokens?: number }>(), { cacheCreation5mTokens: 0, cacheCreation1hTokens: 0 })
const { t } = useI18n()
const open = ref(false)
const position = ref({ x: 0, y: 0 })
const totalTokens = computed(() => props.inputTokens + props.outputTokens + props.cacheCreationTokens + props.cacheReadTokens)
function showTooltip(event: Event) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  position.value = { x: rect.right + 8, y: rect.top + rect.height / 2 }
  open.value = true
}
function toggleTooltip(event: MouseEvent) {
  if (open.value) open.value = false
  else showTooltip(event)
}
function formatCacheTokens(value: number) { return value >= 1e6 ? `${(value / 1e6).toFixed(1)}M` : value >= 1e3 ? `${(value / 1e3).toFixed(1)}K` : value.toLocaleString() }
</script>
