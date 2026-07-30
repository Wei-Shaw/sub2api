<template>
  <section
    class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800/70"
  >
    <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700/70">
      <div class="flex items-center gap-2 overflow-x-auto pb-1">
        <button
          v-for="option in platformOptions"
          :key="`platform-${option.value}`"
          type="button"
          data-test="platform-filter-option"
          class="inline-flex h-9 shrink-0 items-center gap-2 rounded-md px-3 text-sm font-medium transition-colors"
          :class="
            platform === option.value
              ? 'bg-primary-50 text-primary-700 ring-1 ring-inset ring-primary-200 dark:bg-primary-500/15 dark:text-primary-300 dark:ring-primary-500/30'
              : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'
          "
          @click="$emit('update:platform', option.value)"
        >
          <PlatformIcon
            v-if="option.value !== 'all'"
            :platform="option.value as GroupPlatform"
            size="xs"
          />
          <Icon v-else name="grid" size="xs" />
          <span>{{ option.label }}</span>
          <span
            class="rounded px-1.5 py-0.5 text-[10px] tabular-nums"
            :class="
              platform === option.value
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-400/20 dark:text-primary-200'
                : 'bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-dark-400'
            "
          >
            {{ option.count }}
          </span>
        </button>
      </div>
    </div>

    <div class="flex flex-col gap-3 px-4 py-3 lg:flex-row lg:items-center">
      <div class="relative min-w-0 flex-1 lg:max-w-md">
        <Icon
          name="search"
          size="sm"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500"
        />
        <input
          :value="search"
          type="search"
          :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          class="input h-10 rounded-md pl-9 pr-9"
          @input="$emit('update:search', ($event.target as HTMLInputElement).value)"
        />
        <button
          v-if="search"
          type="button"
          class="absolute right-2.5 top-1/2 flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :aria-label="t('modelPlaza.filters.clearSearch')"
          @click="$emit('update:search', '')"
        >
          <Icon name="x" size="xs" class="h-3.5 w-3.5" />
        </button>
      </div>

      <div class="flex min-w-0 flex-wrap items-center gap-2 lg:ml-auto lg:flex-nowrap">
        <span class="mr-auto text-xs text-gray-400 dark:text-dark-500 lg:mr-1">
          {{ t('modelPlaza.filters.resultCount', { count: resultCount }) }}
        </span>
        <button
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-md border px-3 text-sm font-medium transition-colors"
          :class="
            advancedOpen || activeFilterCount > 0
              ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-primary-300'
              : 'border-gray-200 text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:border-dark-700 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'
          "
          :aria-expanded="advancedOpen"
          @click="advancedOpen = !advancedOpen"
        >
          <Icon name="filter" size="xs" />
          {{ t('modelPlaza.filters.advanced') }}
          <span
            v-if="activeFilterCount > 0"
            class="flex h-5 min-w-5 items-center justify-center rounded bg-primary-600 px-1 text-[10px] font-semibold text-white"
          >
            {{ activeFilterCount }}
          </span>
        </button>
        <button
          v-if="activeFilterCount > 0"
          type="button"
          class="inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :aria-label="t('modelPlaza.filters.reset')"
          :title="t('modelPlaza.filters.reset')"
          @click="$emit('reset')"
        >
          <Icon name="refresh" size="sm" />
        </button>
        <div
          class="inline-flex h-9 shrink-0 items-center rounded-md border border-gray-200 bg-white p-0.5 dark:border-dark-700 dark:bg-dark-800"
          role="group"
          :aria-label="t('modelPlaza.view.label')"
        >
          <button
            v-for="option in viewOptions"
            :key="option.value"
            type="button"
            data-test="view-mode-option"
            class="flex h-8 w-8 items-center justify-center rounded transition-colors"
            :class="
              viewMode === option.value
                ? 'bg-primary-50 text-primary-700 ring-1 ring-inset ring-primary-200 dark:bg-primary-500/15 dark:text-primary-300 dark:ring-primary-500/30'
                : 'text-gray-400 hover:bg-gray-50 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-gray-200'
            "
            :aria-label="option.label"
            :aria-pressed="viewMode === option.value"
            :title="option.label"
            @click="$emit('update:viewMode', option.value)"
          >
            <Icon :name="option.icon" size="sm" />
          </button>
        </div>
      </div>
    </div>

    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="-translate-y-1 opacity-0"
      enter-to-class="translate-y-0 opacity-100"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="translate-y-0 opacity-100"
      leave-to-class="-translate-y-1 opacity-0"
    >
      <div
        v-if="advancedOpen"
        class="grid gap-4 border-t border-gray-100 bg-gray-50/70 px-4 py-4 dark:border-dark-700/70 dark:bg-dark-900/30 md:grid-cols-3"
      >
        <FilterChipGroup :label="t('modelPlaza.filters.groupLabel')">
          <button
            type="button"
            :class="chipClass(groupId === 'all')"
            @click="$emit('update:groupId', 'all')"
          >
            {{ t('modelPlaza.filters.all') }}
          </button>
          <button
            v-for="group in groups"
            :key="`group-${group.id}`"
            type="button"
            :class="chipClass(groupId === group.id)"
            :disabled="!groupEnabled(group)"
            @click="$emit('update:groupId', group.id)"
          >
            {{ group.name }}
          </button>
        </FilterChipGroup>

        <FilterChipGroup :label="t('modelPlaza.filters.rateLabel')">
          <button
            type="button"
            :class="chipClass(rate === 'all')"
            @click="$emit('update:rate', 'all')"
          >
            {{ t('modelPlaza.filters.all') }}
          </button>
          <button
            v-for="item in rates"
            :key="`rate-${item}`"
            type="button"
            class="font-mono"
            :class="chipClass(rate === item)"
            :disabled="!rateEnabled(item)"
            @click="$emit('update:rate', item)"
          >
            {{ item }}x
          </button>
        </FilterChipGroup>

        <FilterChipGroup :label="t('modelPlaza.filters.billingLabel')">
          <button
            type="button"
            :class="chipClass(billingMode === 'all')"
            @click="$emit('update:billingMode', 'all')"
          >
            {{ t('modelPlaza.filters.all') }}
          </button>
          <button
            v-for="mode in billingModes"
            :key="`billing-${mode}`"
            type="button"
            :class="chipClass(billingMode === mode)"
            @click="$emit('update:billingMode', mode)"
          >
            {{ billingModeLabel(mode) }}
          </button>
        </FilterChipGroup>
      </div>
    </Transition>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import type { BillingMode } from '@/constants/channel'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST
} from '@/constants/channel'
import type { ModelPlazaViewMode } from './viewMode'

const props = defineProps<{
  platforms: Array<{ name: string; count: number }>
  groups: Array<{ id: number; name: string; platform: string; rate: number }>
  rates: number[]
  billingModes: BillingMode[]
  platform: string
  groupId: number | 'all'
  rate: number | 'all'
  billingMode: BillingMode | 'all'
  search: string
  viewMode: ModelPlazaViewMode
  resultCount: number
  totalCount: number
  activeFilterCount: number
}>()

defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number | 'all']
  'update:rate': [value: number | 'all']
  'update:billingMode': [value: BillingMode | 'all']
  'update:search': [value: string]
  'update:viewMode': [value: ModelPlazaViewMode]
  reset: []
}>()

const { t } = useI18n()
const advancedOpen = ref(false)

const FilterChipGroup = defineComponent({
  props: { label: { type: String, required: true } },
  setup(groupProps, { slots }) {
    return () =>
      h('div', [
        h(
          'div',
          { class: 'mb-2 text-xs font-semibold text-gray-500 dark:text-dark-300' },
          groupProps.label
        ),
        h('div', { class: 'flex flex-wrap gap-1.5' }, slots.default?.())
      ])
  }
})

const platformOptions = computed(() => [
  { value: 'all', label: t('modelPlaza.filters.all'), count: props.totalCount },
  ...props.platforms.map((item) => ({
    value: item.name,
    label: item.name,
    count: item.count
  }))
])

const viewOptions = [
  { value: 'list', icon: 'list', label: t('modelPlaza.view.list') },
  { value: 'card', icon: 'grid', label: t('modelPlaza.view.card') }
] as const

function groupEnabled(group: { platform: string; rate: number }): boolean {
  return (
    (props.platform === 'all' || group.platform === props.platform) &&
    (props.rate === 'all' || group.rate === props.rate)
  )
}

function rateEnabled(value: number): boolean {
  return props.groups.some(
    (group) =>
      group.rate === value &&
      (props.platform === 'all' || group.platform === props.platform) &&
      (props.groupId === 'all' || group.id === props.groupId)
  )
}

function billingModeLabel(mode: BillingMode): string {
  if (mode === BILLING_MODE_IMAGE) return t('modelPlaza.table.perImage')
  if (mode === BILLING_MODE_PER_REQUEST) return t('modelPlaza.table.perRequest')
  return t('modelPlaza.filters.tokenBilling')
}

function chipClass(active: boolean): string {
  return [
    'rounded-md border px-2.5 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-35',
    active
      ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-primary-300'
      : 'border-gray-200 bg-white text-gray-500 hover:border-gray-300 hover:text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300 dark:hover:border-dark-600 dark:hover:text-white'
  ].join(' ')
}
</script>
