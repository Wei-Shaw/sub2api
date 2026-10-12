<template>
  <div class="space-y-3">
    <template v-for="block in document.blocks" :key="block.id">
      <div v-if="block.type === 'status'" class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium" :class="toneClasses(block.tone).soft">
        <span class="h-2 w-2 rounded-full" :class="toneClasses(block.tone).dot"></span>{{ block.title }}
      </div>
      <h3 v-else-if="block.type === 'heading'" class="text-lg font-semibold leading-7 text-gray-900 dark:text-white">{{ block.title }}</h3>
      <p v-else-if="block.type === 'text'" class="whitespace-pre-line text-sm leading-6 text-gray-600 dark:text-gray-300">{{ block.content }}</p>
      <div v-else-if="block.type === 'metrics'" class="grid grid-cols-2 gap-2 sm:grid-cols-3">
        <div v-for="metric in block.metrics" :key="`${metric.label}-${metric.value}`" class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/60">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ metric.label }}</div><div class="mt-1 text-base font-semibold text-gray-900 dark:text-white">{{ metric.value }}</div>
        </div>
      </div>
      <div v-else-if="block.type === 'feature'" class="long-notice-feature rounded-xl border px-4 py-4" :data-tone="block.tone">
        <div class="flex items-start gap-3">
          <span v-if="block.number" class="shrink-0 text-xs font-semibold tracking-[0.16em] text-gray-400 dark:text-gray-500">{{ block.number }}</span>
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <h4 v-if="block.title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ block.title }}</h4>
              <span v-for="tag in block.tags || []" :key="tag" class="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ tag }}</span>
            </div>
            <p v-if="block.content" class="mt-2 whitespace-pre-line text-xs leading-5 text-gray-600 dark:text-gray-300">{{ block.content }}</p>
            <p v-if="block.highlight" class="mt-2 text-xs font-medium text-blue-600 dark:text-blue-300">{{ block.highlight }}</p>
          </div>
        </div>
      </div>
      <div v-else-if="block.type === 'callout'" class="rounded-lg border px-3 py-3" :class="toneClasses(block.tone).panel">
        <div v-if="block.title" class="text-sm font-semibold">{{ block.title }}</div><p class="mt-1 whitespace-pre-line text-xs leading-5">{{ block.content }}</p>
      </div>
      <div v-else-if="block.type === 'buttons'" class="flex flex-wrap gap-2">
        <a v-for="button in block.buttons" :key="`${button.label}-${button.url}`" :href="safeUrl(button.url)" target="_blank" rel="noopener noreferrer" class="inline-flex min-h-9 items-center rounded-lg px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90" :class="toneClasses(block.tone).button">{{ button.label }}<span class="ml-1">↗</span></a>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import type { LongNoticeDocument, LongNoticeTone } from '@/types/longNotice'
defineProps<{ document: LongNoticeDocument }>()

const tones = {
  blue: { soft: 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300', dot: 'bg-blue-500', panel: 'border-blue-200 bg-blue-50 text-blue-800 dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-200', button: 'bg-blue-600' },
  green: { soft: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300', dot: 'bg-emerald-500', panel: 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200', button: 'bg-emerald-600' },
  amber: { soft: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300', dot: 'bg-amber-500', panel: 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-200', button: 'bg-amber-600' },
  red: { soft: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300', dot: 'bg-red-500', panel: 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200', button: 'bg-red-600' },
  gray: { soft: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300', dot: 'bg-gray-500', panel: 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-300', button: 'bg-gray-700' },
}
function toneClasses(tone?: LongNoticeTone) { return tones[tone || 'blue'] }
function safeUrl(value: string) { try { const url = new URL(value); return ['http:', 'https:'].includes(url.protocol) ? url.toString() : '#' } catch { return '#' } }
</script>
