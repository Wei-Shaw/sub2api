<template>
  <div class="flex flex-wrap gap-2">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm transition-colors"
      :class="modelValue == null
        ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
        : 'border-gray-200 bg-white text-gray-500 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:bg-dark-700'"
      @click="emit('update:modelValue', null)"
    >
      {{ t('common.all') }}
    </button>
    <button
      v-for="p in platforms"
      :key="p"
      type="button"
      class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm transition-colors"
      :class="platformButtonClass(p, modelValue === p)"
      @click="emit('update:modelValue', p)"
    >
      <PlatformIcon :platform="p as GroupPlatform" size="sm" />
      <span>{{ p }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()

defineProps<{
  modelValue: string | null | undefined
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | null): void
}>()

const platforms = ['anthropic', 'openai', 'gemini', 'antigravity']

const palette: Record<string, { on: string; off: string }> = {
  anthropic: {
    on: 'border-orange-500 bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
    off: 'border-gray-200 bg-white text-gray-600 hover:border-orange-300 hover:text-orange-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:text-orange-300',
  },
  openai: {
    on: 'border-emerald-500 bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    off: 'border-gray-200 bg-white text-gray-600 hover:border-emerald-300 hover:text-emerald-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:text-emerald-300',
  },
  gemini: {
    on: 'border-blue-500 bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
    off: 'border-gray-200 bg-white text-gray-600 hover:border-blue-300 hover:text-blue-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:text-blue-300',
  },
  antigravity: {
    on: 'border-purple-500 bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
    off: 'border-gray-200 bg-white text-gray-600 hover:border-purple-300 hover:text-purple-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:text-purple-300',
  },
}

function platformButtonClass(p: string, selected: boolean): string {
  const entry = palette[p]
  if (!entry) return selected ? 'border-primary-500 bg-primary-50' : 'border-gray-200'
  return selected ? entry.on : entry.off
}
</script>
