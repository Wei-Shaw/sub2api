<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-4">
          <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            <div class="relative sm:col-span-2 xl:col-span-2">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="filters.search"
                type="text"
                :placeholder="t('modelMarket.searchPlaceholder')"
                class="input pl-10"
              />
            </div>

            <select v-model="filters.platform" class="input">
              <option value="">{{ t('modelMarket.filters.allPlatforms') }}</option>
              <option v-for="platform in filterOptions.platforms" :key="platform" :value="platform">
                {{ platformLabel(platform) }}
              </option>
            </select>

            <select v-model="filters.channel" class="input">
              <option value="">{{ t('modelMarket.filters.allChannels') }}</option>
              <option v-for="channel in filterOptions.channels" :key="channel" :value="channel">
                {{ channel }}
              </option>
            </select>

            <select v-model="filters.pricing" class="input">
              <option value="all">{{ t('modelMarket.filters.allPricing') }}</option>
              <option value="with">{{ t('modelMarket.filters.withPricing') }}</option>
              <option value="without">{{ t('modelMarket.filters.withoutPricing') }}</option>
            </select>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span class="rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-800">
                {{ t('modelMarket.summary.groups', { count: filteredItems.length }) }}
              </span>
              <span class="rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-800">
                {{ t('modelMarket.summary.models', { count: filteredModelCount }) }}
              </span>
              <span class="rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-800">
                {{ t('modelMarket.summary.channels', { count: channels.length }) }}
              </span>
            </div>

            <button
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('common.refresh', 'Refresh')"
              @click="loadMarket"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div class="table-wrapper">
          <table class="w-full table-fixed border-collapse text-sm">
            <thead>
              <tr class="border-b border-gray-100 bg-gray-50/70 text-xs font-medium uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/70 dark:text-gray-400">
                <th class="w-[260px] px-4 py-3 text-left">{{ t('modelMarket.columns.group') }}</th>
                <th class="w-[140px] px-4 py-3 text-left">{{ t('modelMarket.columns.platform') }}</th>
                <th class="w-[220px] px-4 py-3 text-left">{{ t('modelMarket.columns.channels') }}</th>
                <th class="min-w-[320px] px-4 py-3 text-left">{{ t('modelMarket.columns.models') }}</th>
                <th class="w-[120px] px-4 py-3 text-left">{{ t('modelMarket.columns.pricing') }}</th>
              </tr>
            </thead>

            <tbody v-if="loading">
              <tr>
                <td colspan="5" class="py-10 text-center">
                  <Icon name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
                </td>
              </tr>
            </tbody>

            <tbody v-else-if="filteredItems.length === 0">
              <tr>
                <td colspan="5" class="py-12 text-center">
                  <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
                  <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('modelMarket.empty') }}</p>
                </td>
              </tr>
            </tbody>

            <tbody v-else>
              <tr
                v-for="item in filteredItems"
                :key="item.group.id"
                class="border-b border-gray-100 transition-colors hover:bg-gray-50/50 dark:border-dark-800 dark:hover:bg-dark-800/50"
              >
                <td class="px-4 py-3 align-top">
                  <GroupBadge
                    :name="item.group.name"
                    :platform="item.group.platform as GroupPlatform"
                    :subscription-type="(item.group.subscription_type || 'standard') as SubscriptionType"
                    :rate-multiplier="item.group.rate_multiplier"
                    :user-rate-multiplier="userGroupRates[item.group.id] ?? null"
                    always-show-rate
                  />
                </td>

                <td class="px-4 py-3 align-top">
                  <span
                    :class="[
                      'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase',
                      platformBadgeClass(item.platform)
                    ]"
                  >
                    <PlatformIcon :platform="item.platform as GroupPlatform" size="xs" />
                    {{ item.platform }}
                  </span>
                </td>

                <td class="px-4 py-3 align-top">
                  <div class="flex flex-col gap-1">
                    <span
                      v-for="channel in item.channels"
                      :key="channel.name"
                      class="truncate text-xs font-medium text-gray-700 dark:text-gray-300"
                      :title="channel.description || channel.name"
                    >
                      {{ channel.name }}
                    </span>
                  </div>
                </td>

                <td class="px-4 py-3 align-top">
                  <div class="flex flex-wrap gap-1">
                    <SupportedModelChip
                      v-for="model in item.models"
                      :key="`${model.platform}-${model.name}`"
                      :model="model"
                      pricing-key-prefix="availableChannels.pricing"
                      :no-pricing-label="model.pricing_conflict
                        ? t('modelMarket.pricingVaries')
                        : t('availableChannels.noPricing')"
                      :show-platform="false"
                      :platform-hint="item.platform"
                    />
                  </div>
                </td>

                <td class="px-4 py-3 align-top">
                  <span
                    class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium"
                    :class="item.has_pricing
                      ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
                      : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'"
                  >
                    {{ item.has_pricing ? t('modelMarket.pricingConfigured') : t('availableChannels.noPricing') }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import SupportedModelChip from '@/components/channels/SupportedModelChip.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import { useAppStore } from '@/stores/app'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import {
  buildModelMarketItems,
  countModelMarketModels,
  filterModelMarketItems,
  getModelMarketFilterOptions,
  type ModelMarketFilters
} from '@/utils/modelMarket'

const { t } = useI18n()
const appStore = useAppStore()

const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const filters = reactive<ModelMarketFilters>({
  search: '',
  platform: '',
  channel: '',
  pricing: 'all'
})

const marketItems = computed(() => buildModelMarketItems(channels.value))
const filterOptions = computed(() => getModelMarketFilterOptions(marketItems.value))
const filteredItems = computed(() => filterModelMarketItems(marketItems.value, filters))
const filteredModelCount = computed(() => countModelMarketModels(filteredItems.value))

async function loadMarket() {
  loading.value = true
  try {
    const [list, rates] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates().catch((err: unknown) => {
        console.error('Failed to load user group rates:', err)
        return {} as Record<number, number>
      })
    ])
    channels.value = list
    userGroupRates.value = rates
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadMarket)
</script>
