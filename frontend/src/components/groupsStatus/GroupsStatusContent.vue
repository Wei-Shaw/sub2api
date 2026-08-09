<template>
  <div class="space-y-4">
    <header class="flex flex-col gap-3 border-b border-primary-100 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
      <div class="min-w-0">
        <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">
          {{ t('groupsStatus.title') }}
        </h1>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-400">
          {{ t('groupsStatus.description') }}
        </p>
      </div>
      <div v-if="response" class="flex shrink-0 flex-col items-start gap-0.5 text-xs text-gray-500 dark:text-dark-400 sm:items-end">
        <span class="font-medium">
          {{ t('groupsStatus.overview', { groups: response.summary.group_count, available: response.summary.available_group_count }) }}
        </span>
        <span v-if="lastUpdatedAt">{{ t('groupsStatus.lastUpdated', { time: formattedUpdatedAt }) }}</span>
      </div>
    </header>

    <div
      v-if="loading"
      data-testid="groups-status-loading"
      class="min-h-[320px] overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900"
      role="status"
      aria-live="polite"
      :aria-label="t('groupsStatus.loading')"
    >
      <div class="animate-pulse divide-y divide-gray-100 dark:divide-dark-800">
        <div v-for="row in 6" :key="row" class="grid grid-cols-[1.6fr_0.8fr_0.6fr_0.55fr_0.55fr_0.55fr_0.8fr] gap-4 px-5 py-5">
          <span class="h-4 rounded bg-gray-100 dark:bg-dark-800"></span>
          <span class="h-4 rounded bg-gray-100 dark:bg-dark-800"></span>
          <span class="h-4 rounded bg-gray-100 dark:bg-dark-800"></span>
          <span class="h-4 rounded bg-gray-100 dark:bg-dark-800"></span>
          <span class="h-4 rounded bg-gray-100 dark:bg-dark-800"></span>
          <span class="h-4 rounded bg-gray-100 dark:bg-dark-800"></span>
          <span class="h-4 rounded bg-gray-100 dark:bg-dark-800"></span>
        </div>
      </div>
    </div>

    <div
      v-else-if="error"
      data-testid="groups-status-error"
      class="rounded-lg border border-red-200 bg-red-50 px-5 py-10 text-center dark:border-red-500/30 dark:bg-red-500/10"
      role="alert"
    >
      <p class="text-sm text-red-700 dark:text-red-300">{{ t('groupsStatus.loadFailed') }}</p>
      <button type="button" class="btn btn-secondary mt-4" @click="$emit('retry')">
        <Icon name="refresh" size="sm" />
        {{ t('groupsStatus.retry') }}
      </button>
    </div>

    <template v-else-if="response">
      <section class="rounded-lg border border-gray-200 bg-white px-4 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:px-5">
        <div class="grid gap-4 sm:grid-cols-[repeat(3,minmax(0,1fr))_minmax(180px,1.25fr)] sm:items-center sm:gap-0">
          <div class="border-b border-gray-100 pb-3 sm:border-b-0 sm:border-r sm:px-4 sm:pb-0 sm:first:pl-0 dark:border-dark-800">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('groupsStatus.summary.accounts') }}</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-gray-950 dark:text-white">{{ response.summary.account_count }}</p>
          </div>
          <div class="border-b border-gray-100 pb-3 sm:border-b-0 sm:border-r sm:px-4 sm:pb-0 dark:border-dark-800">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('groupsStatus.summary.available') }}</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{{ response.summary.available_account_count }}</p>
          </div>
          <div class="border-b border-gray-100 pb-3 sm:border-b-0 sm:border-r sm:px-4 sm:pb-0 dark:border-dark-800">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('groupsStatus.summary.rateLimited') }}</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-amber-600 dark:text-amber-400">{{ response.summary.rate_limited_account_count }}</p>
          </div>
          <div class="sm:pl-5">
            <div class="flex items-center justify-between gap-3">
              <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('groupsStatus.summary.availabilityRate') }}</p>
              <span class="text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ overallAvailabilityRate }}%</span>
            </div>
            <div
              class="mt-2 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-800"
              role="progressbar"
              :aria-label="t('groupsStatus.summary.availabilityRate')"
              :aria-valuenow="overallAvailabilityRate"
              aria-valuemin="0"
              aria-valuemax="100"
            >
              <div class="h-full rounded-full bg-emerald-500 transition-[width] duration-300" :style="{ width: `${overallAvailabilityRate}%` }"></div>
            </div>
          </div>
        </div>
      </section>

      <section class="rounded-lg border border-primary-100 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-4">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <span class="flex h-7 w-7 items-center justify-center rounded-md bg-gray-100 text-gray-500 dark:bg-dark-800 dark:text-dark-300">
              <Icon name="filter" size="xs" />
            </span>
            <div>
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('groupsStatus.filters.title') }}</h2>
              <p class="text-[11px] text-gray-400 dark:text-dark-500">
                {{ t('groupsStatus.filters.resultSummary', { shown: filteredGroups.length, total: groups.length }) }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              v-if="hasActiveFilters"
              type="button"
              class="inline-flex min-h-11 items-center gap-1 rounded-md px-3 py-1 text-xs font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white sm:min-h-0 sm:px-2"
              data-testid="reset-filters"
              @click="resetFilters"
            >
              <Icon name="x" size="xs" />
              {{ t('groupsStatus.filters.reset') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary h-11 px-3 sm:h-8 sm:px-2.5"
              :title="t('groupsStatus.refresh')"
              :aria-label="t('groupsStatus.refresh')"
              data-testid="refresh-status"
              @click="$emit('retry')"
            >
              <Icon name="refresh" size="sm" />
            </button>
          </div>
        </div>

        <label class="block min-w-0">
          <span class="mb-1 block text-[11px] font-medium text-gray-500 dark:text-dark-400">{{ t('groupsStatus.filters.searchLabel') }}</span>
          <span class="relative block max-w-xl">
            <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500" />
            <input
              v-model="searchQuery"
              type="search"
              class="input h-11 rounded-md py-1.5 pl-9 pr-11 sm:h-9 sm:pr-9"
              :placeholder="t('groupsStatus.filters.searchPlaceholder')"
              data-testid="group-search"
            />
            <button
              v-if="searchQuery"
              type="button"
              class="absolute right-0 top-1/2 flex h-11 w-11 -translate-y-1/2 items-center justify-center text-gray-400 transition-colors hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-dark-500 dark:hover:text-gray-200 sm:h-9 sm:w-9"
              :aria-label="t('groupsStatus.filters.clearSearch')"
              @click="searchQuery = ''"
            >
              <Icon name="x" size="xs" class="h-3.5 w-3.5" />
            </button>
          </span>
        </label>

        <div class="mt-3 border-t border-gray-100 pt-3 dark:border-dark-800">
          <div class="flex min-w-0 items-center gap-2">
            <span class="w-14 shrink-0 text-[11px] font-medium text-gray-500 dark:text-dark-400">{{ t('groupsStatus.filters.channelLabel') }}</span>
            <div class="flex min-w-0 flex-1 gap-1.5 overflow-x-auto pb-0.5" data-testid="channel-filters">
              <button
                v-for="platform in ['all', ...platforms]"
                :key="platform"
                type="button"
                class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 sm:min-h-0 sm:px-2.5"
                :class="platform === 'all' ? neutralFilterClass(selectedPlatform === 'all') : platformFilterClass(platform, selectedPlatform === platform)"
                :style="platform === 'all' ? undefined : { '--channel-accent': platformAccentColor(platform) }"
                :data-testid="`channel-filter-${platform}`"
                :aria-pressed="selectedPlatform === platform"
                @click="selectedPlatform = platform"
              >
                <PlatformIcon v-if="platform !== 'all'" :platform="platform as GroupPlatform" size="xs" />
                {{ platform === 'all' ? t('groupsStatus.filters.allChannels') : platform }}
              </button>
            </div>
          </div>

          <div class="mt-2 flex min-w-0 items-center gap-2">
            <span class="w-14 shrink-0 text-[11px] font-medium text-gray-500 dark:text-dark-400">{{ t('groupsStatus.filters.statusLabel') }}</span>
            <div class="flex min-w-0 flex-1 gap-1.5 overflow-x-auto pb-0.5" data-testid="status-filters">
              <button
                v-for="availability in availabilityOptions"
                :key="availability"
                type="button"
                class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 sm:min-h-0 sm:px-2.5"
                :class="neutralFilterClass(selectedAvailability === availability)"
                :data-testid="`status-filter-${availability}`"
                :aria-pressed="selectedAvailability === availability"
                @click="selectedAvailability = availability"
              >
                <span v-if="availability !== 'all'" class="h-1.5 w-1.5 rounded-full" :class="statusDotClass(availability)"></span>
                {{ availability === 'all' ? t('groupsStatus.filters.allStatuses') : t(`groupsStatus.status.${availability}`) }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <div v-if="groups.length === 0" data-testid="groups-status-empty" class="rounded-lg border border-dashed border-gray-300 bg-white px-5 py-12 text-center dark:border-dark-600 dark:bg-dark-900">
        <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-300 dark:text-dark-600" />
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('groupsStatus.empty') }}</p>
      </div>

      <div v-else-if="filteredGroups.length === 0" data-testid="groups-status-no-results" class="rounded-lg border border-dashed border-gray-300 bg-white px-5 py-12 text-center dark:border-dark-600 dark:bg-dark-900">
        <Icon name="search" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-300 dark:text-dark-600" />
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('groupsStatus.noResults') }}</p>
      </div>

      <div v-else class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <table data-testid="groups-status-desktop" class="hidden w-full table-fixed border-collapse text-sm md:table">
          <thead>
            <tr class="border-b border-gray-100 bg-gray-50/70 text-xs font-medium text-gray-500 dark:border-dark-700 dark:bg-dark-800/60 dark:text-dark-400">
              <th class="w-[25%] px-4 py-3 text-left">{{ t('groupsStatus.table.group') }}</th>
              <th class="w-[14%] px-4 py-3 text-left">{{ t('groupsStatus.table.channel') }}</th>
              <th class="w-[10%] px-4 py-3 text-right">{{ t('groupsStatus.table.rate') }}</th>
              <th class="w-[11%] px-4 py-3 text-right">{{ t('groupsStatus.table.accounts') }}</th>
              <th class="w-[11%] px-4 py-3 text-right">{{ t('groupsStatus.table.available') }}</th>
              <th class="w-[11%] px-4 py-3 text-right">{{ t('groupsStatus.table.rateLimited') }}</th>
              <th class="w-[18%] px-4 py-3 text-left">{{ t('groupsStatus.table.status') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
            <tr v-for="group in filteredGroups" :key="group.id" :data-testid="`group-row-${group.id}`" class="transition-colors hover:bg-gray-50/60 dark:hover:bg-dark-800/40">
              <td class="px-4 py-3.5 align-middle">
                <div class="min-w-0">
                  <p class="truncate font-medium text-gray-900 dark:text-white">{{ group.name }}</p>
                  <p v-if="group.description" class="mt-0.5 truncate text-xs text-gray-400 dark:text-dark-500">{{ group.description }}</p>
                </div>
              </td>
              <td class="px-4 py-3.5 align-middle">
                <span class="inline-flex max-w-full items-center gap-1.5 rounded-md bg-gray-50 px-2 py-1 text-xs font-medium text-gray-600 ring-1 ring-inset ring-gray-200 dark:bg-dark-800 dark:text-dark-300 dark:ring-dark-700">
                  <PlatformIcon :platform="group.platform as GroupPlatform" size="xs" />
                  <span class="truncate">{{ group.platform }}</span>
                </span>
              </td>
              <td class="px-4 py-3.5 text-right align-middle font-mono text-sm font-semibold tabular-nums text-gray-800 dark:text-gray-100">
                {{ formatRate(group.rate_multiplier) }}x
              </td>
              <td class="px-4 py-3.5 text-right align-middle font-medium tabular-nums text-gray-800 dark:text-gray-100">{{ group.account_count }}</td>
              <td class="px-4 py-3.5 text-right align-middle font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{{ group.available_account_count }}</td>
              <td class="px-4 py-3.5 text-right align-middle font-semibold tabular-nums" :class="group.rate_limited_account_count > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-gray-400 dark:text-dark-500'">
                {{ group.rate_limited_account_count }}
              </td>
              <td class="px-4 py-3.5 align-middle">
                <div class="min-w-0">
                  <span class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-semibold" :class="statusBadgeClass(group.availability)" :title="t(`groupsStatus.statusHint.${group.availability}`)">
                    <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass(group.availability)"></span>
                    {{ t(`groupsStatus.status.${group.availability}`) }}
                  </span>
                  <div class="mt-2 flex h-1.5 w-full max-w-32 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-800" :title="accountBreakdown(group)">
                    <span class="bg-emerald-500" :style="{ width: segmentWidth(group.available_account_count, group.account_count) }"></span>
                    <span class="bg-amber-400" :style="{ width: segmentWidth(group.rate_limited_account_count, group.account_count) }"></span>
                  </div>
                </div>
              </td>
            </tr>
          </tbody>
        </table>

        <div data-testid="groups-status-mobile" class="divide-y divide-gray-100 md:hidden dark:divide-dark-800">
          <article v-for="group in filteredGroups" :key="`mobile-${group.id}`" :data-testid="`mobile-group-${group.id}`" class="px-4 py-4">
            <div class="flex min-w-0 flex-col items-start gap-2">
              <GroupBadge
                class="max-w-full"
                :name="group.name"
                :platform="group.platform as GroupPlatform"
                :subscription-type="(group.subscription_type || 'standard') as SubscriptionType"
                :rate-multiplier="group.rate_multiplier"
                :peak-rate-enabled="group.peak_rate_enabled"
                :peak-start="group.peak_start"
                :peak-end="group.peak_end"
                :peak-rate-multiplier="group.peak_rate_multiplier"
                always-show-rate
              />
              <span class="inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1 text-xs font-semibold" :class="statusBadgeClass(group.availability)">
                <span class="h-1.5 w-1.5 rounded-full" :class="statusDotClass(group.availability)"></span>
                {{ t(`groupsStatus.status.${group.availability}`) }}
              </span>
            </div>
            <p v-if="group.description" class="mt-2 break-words text-xs leading-5 text-gray-500 dark:text-dark-400">{{ group.description }}</p>
            <dl class="mt-4 grid grid-cols-3 divide-x divide-gray-100 rounded-md bg-gray-50/80 py-3 text-center dark:divide-dark-700 dark:bg-dark-800/60">
              <div class="px-2">
                <dt class="text-[10px] font-medium text-gray-400 dark:text-dark-500">{{ t('groupsStatus.table.accounts') }}</dt>
                <dd class="mt-1 font-semibold tabular-nums text-gray-900 dark:text-white">{{ group.account_count }}</dd>
              </div>
              <div class="px-2">
                <dt class="text-[10px] font-medium text-gray-400 dark:text-dark-500">{{ t('groupsStatus.table.available') }}</dt>
                <dd class="mt-1 font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{{ group.available_account_count }}</dd>
              </div>
              <div class="px-2">
                <dt class="text-[10px] font-medium text-gray-400 dark:text-dark-500">{{ t('groupsStatus.table.rateLimited') }}</dt>
                <dd class="mt-1 font-semibold tabular-nums" :class="group.rate_limited_account_count > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-gray-400 dark:text-dark-500'">{{ group.rate_limited_account_count }}</dd>
              </div>
            </dl>
            <p class="mt-2 text-[11px] text-gray-400 dark:text-dark-500">{{ accountBreakdown(group) }}</p>
          </article>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import { platformAccentColor } from '@/utils/platformColors'
import type { GroupPlatform, SubscriptionType } from '@/types'
import type { GroupAvailability, GroupsStatusResponse, PublicGroupStatus } from '@/api/groupsStatus'

const props = defineProps<{
  response: GroupsStatusResponse | null
  loading: boolean
  error?: boolean
  lastUpdatedAt?: Date | null
}>()

defineEmits<{
  retry: []
}>()

const { t, locale } = useI18n()

const selectedPlatform = ref('all')
const selectedAvailability = ref<GroupAvailability | 'all'>('all')
const searchQuery = ref('')
const availabilityOptions: Array<GroupAvailability | 'all'> = [
  'all',
  'available',
  'degraded',
  'rate_limited',
  'unavailable'
]

const groups = computed(() => props.response?.groups ?? [])
const platforms = computed(() => [...new Set(groups.value.map((group) => group.platform).filter(Boolean))].sort())
const hasActiveFilters = computed(
  () => selectedPlatform.value !== 'all' || selectedAvailability.value !== 'all' || searchQuery.value.trim() !== ''
)

const filteredGroups = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return groups.value.filter((group) => {
    if (selectedPlatform.value !== 'all' && group.platform !== selectedPlatform.value) return false
    if (selectedAvailability.value !== 'all' && group.availability !== selectedAvailability.value) return false
    if (!query) return true
    return (
      group.name.toLowerCase().includes(query) ||
      group.description.toLowerCase().includes(query) ||
      group.platform.toLowerCase().includes(query)
    )
  })
})

const overallAvailabilityRate = computed(() => {
  const summary = props.response?.summary
  if (!summary || summary.account_count <= 0) return 0
  return Math.round((summary.available_account_count / summary.account_count) * 100)
})

const formattedUpdatedAt = computed(() => {
  if (!props.lastUpdatedAt) return ''
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  }).format(props.lastUpdatedAt)
})

function resetFilters(): void {
  selectedPlatform.value = 'all'
  selectedAvailability.value = 'all'
  searchQuery.value = ''
}

function formatRate(value: number): string {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(4)))
}

function segmentWidth(value: number, total: number): string {
  if (total <= 0 || value <= 0) return '0%'
  return `${Math.min(100, Math.max(0, (value / total) * 100))}%`
}

function otherUnavailableCount(group: PublicGroupStatus): number {
  return Math.max(0, group.account_count - group.available_account_count - group.rate_limited_account_count)
}

function accountBreakdown(group: PublicGroupStatus): string {
  return t('groupsStatus.accountBreakdown', {
    available: group.available_account_count,
    limited: group.rate_limited_account_count,
    other: otherUnavailableCount(group)
  })
}

function statusBadgeClass(status: GroupAvailability): string {
  switch (status) {
    case 'available':
      return 'bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-500/20'
    case 'degraded':
      return 'bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-200 dark:bg-amber-500/10 dark:text-amber-300 dark:ring-amber-500/20'
    case 'rate_limited':
      return 'bg-orange-50 text-orange-700 ring-1 ring-inset ring-orange-200 dark:bg-orange-500/10 dark:text-orange-300 dark:ring-orange-500/20'
    default:
      return 'bg-gray-100 text-gray-600 ring-1 ring-inset ring-gray-200 dark:bg-dark-800 dark:text-dark-300 dark:ring-dark-700'
  }
}

function statusDotClass(status: GroupAvailability | 'all'): string {
  switch (status) {
    case 'available':
      return 'bg-emerald-500'
    case 'degraded':
      return 'bg-amber-500'
    case 'rate_limited':
      return 'bg-orange-500'
    case 'unavailable':
      return 'bg-gray-400 dark:bg-dark-500'
    default:
      return 'bg-primary-500'
  }
}

function neutralFilterClass(active: boolean): string {
  return active
    ? 'bg-primary-600 text-white shadow-sm dark:bg-primary-500 dark:text-white'
    : 'bg-white text-gray-600 ring-1 ring-inset ring-gray-200 hover:bg-gray-50 hover:text-gray-900 hover:ring-gray-300 dark:bg-dark-800 dark:text-dark-300 dark:ring-dark-700 dark:hover:text-white'
}

function platformFilterClass(_platform: string, active: boolean): string {
  return active ? 'channel-filter-active' : 'channel-filter'
}
</script>

<style scoped>
.channel-filter {
  color: color-mix(in srgb, var(--channel-accent) 78%, black);
  background-color: color-mix(in srgb, var(--channel-accent) 9%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--channel-accent) 25%, transparent);
}

.channel-filter:hover {
  background-color: color-mix(in srgb, var(--channel-accent) 16%, transparent);
}

.channel-filter-active {
  color: #fff;
  background-color: color-mix(in srgb, var(--channel-accent) 84%, black);
  box-shadow: 0 1px 2px color-mix(in srgb, var(--channel-accent) 30%, transparent);
}

.dark .channel-filter {
  color: color-mix(in srgb, var(--channel-accent) 72%, white);
  background-color: color-mix(in srgb, var(--channel-accent) 12%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--channel-accent) 30%, transparent);
}

.dark .channel-filter-active {
  background-color: color-mix(in srgb, var(--channel-accent) 80%, transparent);
}
</style>
