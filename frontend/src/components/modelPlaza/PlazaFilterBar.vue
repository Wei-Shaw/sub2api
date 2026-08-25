<template>
  <div class="space-y-3 rounded-2xl border border-gray-100 bg-white p-4 shadow-card dark:border-dark-700/50 dark:bg-dark-800/50">
    <div class="flex items-center gap-2">
      <span class="text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.label') }}</span>
      <button v-if="hasActiveFilters" type="button" class="rounded-md px-2 py-1 text-xs font-semibold text-primary-600 hover:bg-primary-50 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:text-primary-300 dark:hover:bg-primary-500/10" @click="$emit('clear')">
        {{ t('modelPlaza.filters.clear') }}
      </button>
    </div>

    <div class="flex items-start gap-3">
      <span class="w-16 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.platformLabel') }}</span>
      <div class="flex flex-wrap items-center gap-2">
        <button v-for="p in ['all', ...platforms]" :key="`platform-${p}`" type="button" class="inline-flex min-h-10 items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40" :class="p === 'all' ? chipClass(platform === 'all') : platform === p ? 'chip-tinted-active' : 'chip-tinted'" :style="p === 'all' ? undefined : { '--chip-accent': platformAccentColor(p) }" :disabled="p !== 'all' && !platformEnabled(p)" :aria-pressed="platform === p" @click="$emit('update:platform', p)">
          <PlatformIcon v-if="p !== 'all'" :platform="p as GroupPlatform" size="xs" />
          {{ p === 'all' ? t('modelPlaza.filters.all') : p }}
        </button>
      </div>
    </div>

    <div class="flex items-start gap-3">
      <span class="w-16 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.groupLabel') }}</span>
      <div class="flex flex-wrap items-center gap-2">
        <button type="button" class="min-h-10 rounded-lg px-3 py-2 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-primary-500" :class="chipClass(groupId === 'all')" :aria-pressed="groupId === 'all'" @click="$emit('update:groupId', 'all')">{{ t('modelPlaza.filters.all') }}</button>
        <button v-for="g in groups" :key="`group-${g.id}`" type="button" class="min-h-10 rounded-lg px-3 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40" :class="groupId === g.id ? 'chip-tinted-active' : 'chip-tinted'" :style="{ '--chip-accent': platformAccentColor(g.platform) }" :disabled="!groupEnabled(g)" :aria-pressed="groupId === g.id" @click="$emit('update:groupId', g.id)">{{ g.name }}</button>
      </div>
    </div>

    <div class="flex items-start gap-3">
      <span class="w-16 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.rateLabel') }}</span>
      <div class="flex flex-wrap items-center gap-2">
        <button type="button" class="min-h-10 rounded-lg px-3 py-2 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-primary-500" :class="chipClass(rate === 'all')" :aria-pressed="rate === 'all'" @click="$emit('update:rate', 'all')">{{ t('modelPlaza.filters.all') }}</button>
        <button v-for="r in rates" :key="`rate-${r}`" type="button" class="min-h-10 rounded-lg px-3 py-2 font-mono text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40" :class="chipClass(rate === r)" :disabled="!rateEnabled(r)" :aria-pressed="rate === r" @click="$emit('update:rate', r)">{{ r }}x</button>
      </div>
    </div>

    <div class="flex flex-wrap items-start gap-3">
      <label for="model-plaza-search" class="w-16 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.modelLabel') }}</label>
      <div class="relative w-full sm:w-80">
        <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500" />
        <input id="model-plaza-search" :value="search" type="search" :placeholder="t('modelPlaza.filters.searchPlaceholder')" class="input min-h-10 rounded-lg py-2 pl-9 pr-9" @input="$emit('update:search', ($event.target as HTMLInputElement).value)" />
        <button v-if="search" type="button" class="absolute right-2.5 top-1/2 min-h-8 min-w-8 -translate-y-1/2 rounded-md text-gray-400 transition-colors hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:text-dark-500 dark:hover:text-gray-300" :aria-label="t('modelPlaza.filters.clearSearch')" @click="$emit('update:search', '')">
          <Icon name="x" size="xs" class="mx-auto h-3.5 w-3.5" />
        </button>
      </div>
    </div>

    <div class="flex flex-wrap items-start gap-3">
      <label for="model-plaza-sort" class="w-16 shrink-0 pt-2 text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.sortLabel') }}</label>
      <select id="model-plaza-sort" :value="sort" class="input min-h-10 w-full rounded-lg py-2 sm:w-80" @change="$emit('update:sort', ($event.target as HTMLSelectElement).value as PlazaSort)">
        <option value="default">{{ t('modelPlaza.sort.default') }}</option>
        <option value="name">{{ t('modelPlaza.sort.name') }}</option>
        <option value="platform">{{ t('modelPlaza.sort.platform') }}</option>
        <option value="offers">{{ t('modelPlaza.sort.offers') }}</option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { platformAccentColor } from '@/utils/platformColors'
import type { GroupPlatform } from '@/types'
import type { PlazaSort } from '@/api/modelPlaza'

const props = defineProps<{
  platforms: string[]
  groups: Array<{ id: number; name: string; platform: string; modelPlatforms?: string[]; rate: number }>
  rates: number[]
  platform: string
  groupId: number | 'all'
  rate: number | 'all'
  search: string
  sort: PlazaSort
}>()

defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number | 'all']
  'update:rate': [value: number | 'all']
  'update:search': [value: string]
  'update:sort': [value: PlazaSort]
  clear: []
}>()

const { t } = useI18n()
const hasActiveFilters = computed(() => props.platform !== 'all' || props.groupId !== 'all' || props.rate !== 'all' || props.search.trim() !== '' || props.sort !== 'default')

function platformEnabled(platform: string): boolean {
  return props.groups.some((group) => (group.platform === platform || group.modelPlatforms?.includes(platform)) && (props.groupId === 'all' || group.id === props.groupId) && (props.rate === 'all' || group.rate === props.rate))
}
function groupEnabled(group: { id: number; platform: string; modelPlatforms?: string[]; rate: number }): boolean {
  return (props.platform === 'all' || group.platform === props.platform || group.modelPlatforms?.includes(props.platform) === true) && (props.rate === 'all' || group.rate === props.rate)
}
function rateEnabled(rate: number): boolean {
  return props.groups.some((group) => group.rate === rate && (props.platform === 'all' || group.platform === props.platform || group.modelPlatforms?.includes(props.platform) === true) && (props.groupId === 'all' || group.id === props.groupId))
}
function chipClass(active: boolean): string {
  return active ? 'bg-gradient-to-r from-primary-500 to-primary-600 text-white shadow-sm shadow-primary-500/30' : 'bg-white text-gray-600 ring-1 ring-inset ring-gray-200 hover:bg-gray-50 hover:text-gray-900 dark:bg-dark-800/60 dark:text-dark-300 dark:ring-dark-700 dark:hover:bg-dark-800 dark:hover:text-white'
}
</script>

<style scoped>
.chip-tinted { color: var(--chip-accent); color: color-mix(in srgb, var(--chip-accent) 78%, black); background-color: color-mix(in srgb, var(--chip-accent) 9%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--chip-accent) 25%, transparent); }
.chip-tinted:not(:disabled):hover { background-color: color-mix(in srgb, var(--chip-accent) 16%, transparent); }
.dark .chip-tinted { color: color-mix(in srgb, var(--chip-accent) 72%, white); background-color: color-mix(in srgb, var(--chip-accent) 12%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--chip-accent) 30%, transparent); }
.chip-tinted-active { color: #fff; background-color: color-mix(in srgb, var(--chip-accent) 85%, black); box-shadow: 0 1px 2px 0 color-mix(in srgb, var(--chip-accent) 35%, transparent); }
</style>
