<template>
  <aside class="sticky top-4 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="flex items-center justify-between border-b border-gray-100 px-4 py-4 dark:border-dark-700/70">
      <div class="flex items-center gap-2">
        <Icon name="filter" size="sm" class="text-primary-600 dark:text-primary-400" />
        <h2 class="font-bold text-gray-950 dark:text-white">{{ t('modelPlaza.filters.title') }}</h2>
      </div>
      <button
        type="button"
        class="text-xs font-medium text-gray-400 transition hover:text-primary-600 dark:text-dark-500 dark:hover:text-primary-400"
        @click="$emit('reset')"
      >
        {{ t('modelPlaza.filters.reset') }}
      </button>
    </div>

    <div class="space-y-5 p-4">
      <section>
        <div class="mb-2.5 flex items-center gap-2">
          <span class="h-px flex-1 bg-gray-100 dark:bg-dark-700"></span>
          <h3 class="text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-400 dark:text-dark-500">
            {{ t('modelPlaza.filters.billingMode') }}
          </h3>
          <span class="h-px flex-1 bg-gray-100 dark:bg-dark-700"></span>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <button
            v-for="option in billingOptions"
            :key="option.value"
            type="button"
            class="rounded-xl border px-2 py-2.5 text-center transition"
            :class="
              billingMode === option.value
                ? 'border-primary-300 bg-primary-50 text-primary-700 shadow-sm ring-1 ring-primary-200 dark:border-primary-500/40 dark:bg-primary-500/10 dark:text-primary-300 dark:ring-primary-500/20'
                : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800/50 dark:text-dark-300 dark:hover:bg-dark-800'
            "
            @click="$emit('update:billingMode', option.value)"
          >
            <span class="block text-sm font-semibold">{{ option.label }}</span>
            <span class="mt-0.5 block text-[11px] opacity-65">
              {{ t('modelPlaza.filters.modelCount', { count: option.count }) }}
            </span>
          </button>
        </div>
      </section>

      <section>
        <div class="mb-2.5 flex items-center gap-2">
          <span class="h-px flex-1 bg-gray-100 dark:bg-dark-700"></span>
          <h3 class="text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-400 dark:text-dark-500">
            {{ t('modelPlaza.filters.providerNavigation') }}
          </h3>
          <span class="h-px flex-1 bg-gray-100 dark:bg-dark-700"></span>
        </div>
        <div class="max-h-64 space-y-1 overflow-y-auto pr-1">
          <button
            type="button"
            class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm transition"
            :class="providerButtonClass(platform === 'all')"
            @click="$emit('update:platform', 'all')"
          >
            <span class="flex items-center gap-2">
              <span class="flex h-7 w-7 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300">
                <Icon name="grid" size="xs" />
              </span>
              <span class="font-medium">{{ t('modelPlaza.filters.allProviders') }}</span>
            </span>
            <span class="text-xs opacity-60">{{ totalCount }}</span>
          </button>

          <button
            v-for="provider in providers"
            :key="provider.value"
            type="button"
            class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm transition"
            :class="providerButtonClass(platform === provider.value)"
            @click="$emit('update:platform', provider.value)"
          >
            <span class="flex min-w-0 items-center gap-2">
              <span
                class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg"
                :class="platformBadgeLightClass(provider.value)"
              >
                <ProviderLogo
                  :provider="provider.value"
                  :logo-key="provider.logoKey"
                  :logo-url="provider.logoUrl"
                  :alt="provider.label"
                  size="md"
                />
              </span>
              <span class="truncate font-medium">{{ provider.label }}</span>
            </span>
            <span class="ml-2 shrink-0 text-xs opacity-60">{{ provider.count }}</span>
          </button>
        </div>
      </section>

      <section>
        <div class="mb-2.5 flex items-center gap-2">
          <span class="h-px flex-1 bg-gray-100 dark:bg-dark-700"></span>
          <h3 class="text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-400 dark:text-dark-500">
            {{ t('modelPlaza.filters.modelSearch') }}
          </h3>
          <span class="h-px flex-1 bg-gray-100 dark:bg-dark-700"></span>
        </div>
        <div class="relative">
          <Icon
            name="search"
            size="sm"
            class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500"
          />
          <input
            :value="search"
            type="text"
            :placeholder="t('modelPlaza.filters.searchPlaceholder')"
            class="input rounded-xl py-2 pl-9 pr-9 text-sm"
            @input="$emit('update:search', ($event.target as HTMLInputElement).value)"
          />
          <button
            v-if="search"
            type="button"
            class="absolute right-2.5 top-1/2 -translate-y-1/2 text-gray-400 transition hover:text-gray-700 dark:text-dark-500 dark:hover:text-white"
            @click="$emit('update:search', '')"
          >
            <Icon name="x" size="xs" />
          </button>
        </div>
      </section>

      <section class="space-y-2 rounded-xl bg-gray-50 px-3 py-3 text-xs dark:bg-dark-800/70">
        <div class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.results') }}</span>
          <span class="font-semibold text-gray-700 dark:text-dark-200">
            {{ t('modelPlaza.filters.modelCount', { count: resultCount }) }}
          </span>
        </div>
        <div class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.currentStatus') }}</span>
          <span class="inline-flex items-center gap-1.5 font-semibold text-emerald-600 dark:text-emerald-400">
            <span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
            {{ t('modelPlaza.filters.operational') }}
          </span>
        </div>
        <div class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('modelPlaza.filters.updatedAt') }}</span>
          <span class="font-medium text-gray-700 dark:text-dark-200">{{ lastUpdatedLabel }}</span>
        </div>
        <button
          type="button"
          class="mt-1 flex w-full items-center justify-center gap-1.5 rounded-lg border border-gray-200 bg-white py-2 font-medium text-gray-600 transition hover:border-primary-200 hover:text-primary-600 disabled:cursor-wait disabled:opacity-60 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300 dark:hover:border-primary-500/30 dark:hover:text-primary-300"
          :disabled="refreshing"
          @click="$emit('refresh')"
        >
          <Icon name="refresh" size="xs" :class="refreshing ? 'animate-spin' : ''" />
          {{ refreshing ? t('modelPlaza.filters.refreshing') : t('modelPlaza.filters.refresh') }}
        </button>
      </section>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ProviderLogo from './ProviderLogo.vue'
import { platformBadgeLightClass } from '@/utils/platformColors'

defineProps<{
  billingOptions: Array<{ value: string; label: string; count: number }>
  billingMode: string
  providers: Array<{
    value: string
    label: string
    count: number
    logoKey?: string
    logoUrl?: string
  }>
  platform: string
  search: string
  resultCount: number
  totalCount: number
  lastUpdatedLabel: string
  refreshing?: boolean
}>()

defineEmits<{
  'update:billingMode': [value: string]
  'update:platform': [value: string]
  'update:search': [value: string]
  reset: []
  refresh: []
}>()

const { t } = useI18n()

function providerButtonClass(active: boolean): string {
  return active
    ? 'bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
    : 'text-gray-600 hover:bg-gray-50 hover:text-gray-950 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
}
</script>
