<template>
  <component :is="layoutComponent">
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div class="flex flex-1 flex-col gap-3">
            <div class="relative w-full">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('modelMarketplace.searchPlaceholder')"
                class="input pl-10"
              />
            </div>

            <div class="grid gap-3 sm:grid-cols-3">
              <select v-model="platformFilter" class="input">
                <option value="all">{{ t('modelMarketplace.filters.allPlatforms') }}</option>
                <option v-for="platform in platformOptions" :key="platform" :value="platform">
                  {{ platformLabel(platform) }}
                </option>
              </select>

              <select v-model="accessStateFilter" class="input">
                <option value="all">{{ t('modelMarketplace.filters.allAccess') }}</option>
                <option value="available">{{ t('modelMarketplace.access.available') }}</option>
                <option value="subscribed">{{ t('modelMarketplace.access.subscribed') }}</option>
                <option value="purchasable">{{ t('modelMarketplace.access.purchasable') }}</option>
              </select>

              <select v-model="billingModeFilter" class="input">
                <option value="all">{{ t('modelMarketplace.filters.allBillingModes') }}</option>
                <option value="token">{{ t('modelMarketplace.billingMode.token') }}</option>
                <option value="per_request">{{ t('modelMarketplace.billingMode.perRequest') }}</option>
                <option value="image">{{ t('modelMarketplace.billingMode.image') }}</option>
              </select>
            </div>
          </div>

          <button
            data-testid="marketplace-refresh-button"
            type="button"
            @click="loadMarketplace"
            :disabled="loading"
            class="btn btn-secondary self-end"
            :title="t('common.refresh', 'Refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #table>
        <div v-if="!isMobile" data-testid="marketplace-desktop-table">
          <div class="card overflow-hidden">
            <table class="w-full table-fixed border-collapse text-sm">
              <thead>
                <tr class="border-b border-gray-100 bg-gray-50/50 text-xs font-medium uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-400">
                  <th class="w-[220px] px-4 py-3 text-left">{{ t('modelMarketplace.columns.model') }}</th>
                  <th class="w-[120px] px-4 py-3 text-left">{{ t('modelMarketplace.columns.platform') }}</th>
                  <th class="w-[140px] px-4 py-3 text-left">{{ t('modelMarketplace.columns.access') }}</th>
                  <th class="w-[220px] px-4 py-3 text-left">{{ t('modelMarketplace.columns.channels') }}</th>
                  <th class="px-4 py-3 text-left">{{ t('modelMarketplace.columns.groups') }}</th>
                  <th class="w-[220px] px-4 py-3 text-left">{{ t('modelMarketplace.columns.pricing') }}</th>
                </tr>
              </thead>
              <tbody v-if="loading">
                <tr>
                  <td colspan="6" class="py-10 text-center">
                    <Icon name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
                  </td>
                </tr>
              </tbody>
              <tbody v-else-if="filteredModels.length === 0">
                <tr>
                  <td colspan="6" class="py-12 text-center">
                    <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
                    <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('modelMarketplace.empty') }}</p>
                  </td>
                </tr>
              </tbody>
              <tbody v-else>
                <tr
                  v-for="model in filteredModels"
                  :key="`${model.platform}-${model.name}`"
                  class="border-b border-gray-100 align-top transition-colors hover:bg-gray-50/40 dark:border-dark-700 dark:hover:bg-dark-800/40"
                >
                  <td class="px-4 py-4">
                    <div class="font-medium text-gray-900 dark:text-white">{{ model.name }}</div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ model.channels.length }} {{ t('modelMarketplace.channelsCount') }}</div>
                  </td>
                  <td class="px-4 py-4">
                    <span :class="['inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase', platformBadgeClass(model.platform)]">
                      <PlatformIcon :platform="model.platform as GroupPlatform" size="xs" />
                      {{ platformLabel(model.platform) }}
                    </span>
                  </td>
                  <td class="px-4 py-4">
                    <span :class="accessStateClass(model.access_state)" class="inline-flex rounded-full px-2.5 py-1 text-xs font-semibold">
                      {{ accessStateLabel(model.access_state) }}
                    </span>
                  </td>
                  <td class="px-4 py-4">
                    <div class="flex flex-wrap gap-1.5">
                      <span
                        v-for="channel in model.channels"
                        :key="channel.name"
                        class="inline-flex rounded-md border border-gray-200 bg-white px-2 py-0.5 text-xs text-gray-600 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300"
                      >
                        {{ channel.name }}
                      </span>
                    </div>
                  </td>
                  <td class="px-4 py-4">
                    <div class="flex flex-col gap-2">
                      <div class="flex flex-wrap gap-1.5">
                        <GroupBadge
                          v-for="group in model.groups"
                          :key="group.id"
                          :name="group.name"
                          :platform="group.platform as GroupPlatform"
                          :subscription-type="group.subscription_type as SubscriptionType"
                          :rate-multiplier="group.rate_multiplier"
                          :user-rate-multiplier="group.user_rate_multiplier ?? null"
                          always-show-rate
                        />
                      </div>
                      <div v-if="uniquePlans(model.groups).length > 0" class="flex flex-wrap gap-2">
                        <button
                          v-for="plan in uniquePlans(model.groups)"
                          :key="plan.id"
                          :data-testid="`marketplace-plan-button-${plan.id}`"
                          type="button"
                          class="btn btn-xs btn-primary"
                          @click="goPurchase(plan.id)"
                        >
                          {{ t('modelMarketplace.buyPlan', { name: plan.name }) }}
                        </button>
                      </div>
                    </div>
                  </td>
                  <td class="px-4 py-4">
                    <SupportedModelChip
                      :model="toSupportedModel(model)"
                      pricing-key-prefix="modelMarketplace.pricing"
                      :no-pricing-label="t('modelMarketplace.noPricing')"
                      :show-platform="false"
                      :platform-hint="model.platform"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div v-else data-testid="marketplace-mobile-cards" class="grid gap-4">
          <div v-if="loading" class="card p-10 text-center">
            <Icon name="refresh" size="lg" class="mx-auto animate-spin text-gray-400" />
          </div>
          <div v-else-if="filteredModels.length === 0" class="card p-8 text-center">
            <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('modelMarketplace.empty') }}</p>
          </div>
          <div
            v-else
            v-for="model in filteredModels"
            :key="`${model.platform}-${model.name}-mobile`"
            data-testid="marketplace-mobile-card"
            class="card overflow-hidden"
          >
            <div class="h-1" :class="platformAccentBarClass(model.platform)"></div>
            <div class="space-y-4 p-4">
              <div class="flex items-start justify-between gap-3">
                <div>
                  <div class="font-semibold text-gray-900 dark:text-white">{{ model.name }}</div>
                  <div class="mt-1 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span :class="['inline-flex items-center gap-1 rounded-md border px-2 py-0.5 font-medium uppercase', platformBadgeClass(model.platform)]">
                      <PlatformIcon :platform="model.platform as GroupPlatform" size="xs" />
                      {{ platformLabel(model.platform) }}
                    </span>
                    <span :class="accessStateClass(model.access_state)" class="inline-flex rounded-full px-2 py-0.5 font-semibold">
                      {{ accessStateLabel(model.access_state) }}
                    </span>
                  </div>
                </div>
                <SupportedModelChip
                  :model="toSupportedModel(model)"
                  pricing-key-prefix="modelMarketplace.pricing"
                  :no-pricing-label="t('modelMarketplace.noPricing')"
                  :show-platform="false"
                  :platform-hint="model.platform"
                />
              </div>

              <div>
                <div class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('modelMarketplace.columns.channels') }}
                </div>
                <div class="flex flex-wrap gap-1.5">
                  <span
                    v-for="channel in model.channels"
                    :key="channel.name"
                    class="inline-flex rounded-md border border-gray-200 bg-white px-2 py-0.5 text-xs text-gray-600 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300"
                  >
                    {{ channel.name }}
                  </span>
                </div>
              </div>

              <div>
                <div class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('modelMarketplace.columns.groups') }}
                </div>
                <div class="flex flex-wrap gap-1.5">
                  <GroupBadge
                    v-for="group in model.groups"
                    :key="group.id"
                    :name="group.name"
                    :platform="group.platform as GroupPlatform"
                    :subscription-type="group.subscription_type as SubscriptionType"
                    :rate-multiplier="group.rate_multiplier"
                    :user-rate-multiplier="group.user_rate_multiplier ?? null"
                    always-show-rate
                  />
                </div>
              </div>

              <div v-if="uniquePlans(model.groups).length > 0" class="flex flex-wrap gap-2">
                <button
                  v-for="plan in uniquePlans(model.groups)"
                  :key="plan.id"
                  :data-testid="`marketplace-plan-button-${plan.id}`"
                  type="button"
                  class="btn btn-sm btn-primary"
                  @click="goPurchase(plan.id)"
                >
                  {{ t('modelMarketplace.buyPlan', { name: plan.name }) }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </TablePageLayout>
  </component>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import SupportedModelChip from '@/components/channels/SupportedModelChip.vue'
import modelMarketplaceAPI, { type MarketplaceAccessState, type MarketplaceGroup, type MarketplaceModel, type MarketplacePlan, type ModelMarketplaceResponse } from '@/api/modelMarketplace'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { useAppStore, useAuthStore } from '@/stores'
import { platformAccentBarClass, platformBadgeClass, platformLabel } from '@/utils/platformColors'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const router = useRouter()

const marketplace = ref<ModelMarketplaceResponse | null>(null)
const loading = ref(false)
const searchQuery = ref('')
const platformFilter = ref<'all' | string>('all')
const accessStateFilter = ref<'all' | MarketplaceAccessState>('all')
const billingModeFilter = ref<'all' | string>('all')
const isMobile = ref(false)

const layoutComponent = computed(() => (authStore.isAuthenticated ? AppLayout : 'div'))

const models = computed(() => marketplace.value?.models ?? [])

const platformOptions = computed(() => {
  const set = new Set(models.value.map((item) => item.platform).filter(Boolean))
  return Array.from(set).sort((a, b) => a.localeCompare(b))
})

const filteredModels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return models.value.filter((model) => {
    if (platformFilter.value !== 'all' && model.platform !== platformFilter.value) return false
    if (accessStateFilter.value !== 'all' && model.access_state !== accessStateFilter.value) return false
    if (billingModeFilter.value !== 'all' && model.pricing?.billing_mode !== billingModeFilter.value) return false
    if (!query) return true

    return [
      model.name,
      model.platform,
      ...model.channels.map((channel) => channel.name),
      ...model.groups.map((group) => group.name),
      ...uniquePlans(model.groups).map((plan) => plan.name),
    ].some((text) => text.toLowerCase().includes(query))
  })
})

function accessStateLabel(state: MarketplaceAccessState) {
  return t(`modelMarketplace.access.${state}`)
}

function accessStateClass(state: MarketplaceAccessState) {
  switch (state) {
    case 'subscribed':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'available':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'purchasable':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}

function uniquePlans(groups: MarketplaceGroup[]): MarketplacePlan[] {
  const map = new Map<number, MarketplacePlan>()
  for (const group of groups) {
    for (const plan of group.plans) {
      if (!map.has(plan.id)) {
        map.set(plan.id, plan)
      }
    }
  }
  return Array.from(map.values()).sort((a, b) => a.sort_order - b.sort_order || a.price - b.price)
}

function toSupportedModel(model: MarketplaceModel) {
  return {
    name: model.name,
    platform: model.platform,
    pricing: model.pricing,
  }
}

function goPurchase(planID: number) {
  router.push({ path: '/purchase', query: { plan: String(planID) } })
}

function syncViewportMode() {
  isMobile.value = window.innerWidth < 1024
}

async function loadMarketplace() {
  loading.value = true
  try {
    marketplace.value = await modelMarketplaceAPI.getModelMarketplace()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  syncViewportMode()
  window.addEventListener('resize', syncViewportMode)
  loadMarketplace()
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewportMode)
})
</script>
