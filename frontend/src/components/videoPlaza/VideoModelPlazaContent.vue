<template>
  <div class="space-y-6">
    <div v-if="!embedded && !selectedCard">
      <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white sm:text-3xl">
        {{ t('videoPlaza.title') }}
      </h1>
      <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">
        {{ t('videoPlaza.description') }}
      </p>
    </div>

    <div v-if="loading" class="flex min-h-[240px] items-center justify-center">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600" />
    </div>
    <div
      v-else-if="error"
      class="rounded-2xl border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
    >
      {{ t('videoPlaza.loadFailed') }}
    </div>
    <template v-else>
      <template v-if="selectedCard">
        <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 sm:flex-row sm:items-center">
          <label class="flex items-center gap-3 text-sm text-gray-500 dark:text-dark-400">
            {{ t('videoPlaza.groupFilter') }}
            <select v-model="selectedGroup" class="input min-w-[210px] py-2 text-sm text-gray-800 dark:text-dark-100">
              <option value="all">{{ t('videoPlaza.allGroups') }}</option>
              <option v-for="group in groups" :key="group.id" :value="String(group.id)">{{ group.name }}</option>
            </select>
          </label>
          <span class="text-xs text-gray-400 dark:text-dark-500">{{ t('videoPlaza.groupPriceHint') }}</span>
        </div>
        <div class="flex flex-wrap items-center gap-3 text-sm">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 py-2 font-medium text-gray-600 transition hover:border-primary-300 hover:text-primary-600 dark:border-dark-700 dark:bg-dark-800/60 dark:text-dark-200 dark:hover:border-primary-600 dark:hover:text-primary-300"
            @click="closeDetails"
          >
            <Icon name="arrowLeft" size="sm" />
            {{ t('videoPlaza.backToList') }}
          </button>
          <span class="text-gray-400 dark:text-dark-500">{{ t('videoPlaza.title') }}</span>
          <span class="text-gray-300 dark:text-dark-700">/</span>
          <span class="font-medium text-gray-800 dark:text-dark-100">{{ selectedCard.model.name }}</span>
        </div>

        <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-card dark:border-dark-700/70 dark:bg-dark-800/60">
          <div class="border-b border-gray-100 px-6 py-6 dark:border-dark-700/60 sm:px-8">
            <div class="flex flex-wrap items-start gap-4">
              <span class="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300">
                <PlatformIcon :platform="selectedCard.model.platform as GroupPlatform" size="lg" />
              </span>
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <h1 class="break-all text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                    {{ selectedCard.model.name }}
                  </h1>
                  <span v-if="discountLabel(selectedCard.startingOffer)" class="rounded-md bg-primary-500 px-2 py-1 text-xs font-semibold text-white">
                    {{ discountLabel(selectedCard.startingOffer) }}
                  </span>
                </div>
                <p class="mt-1 flex items-center gap-1.5 text-sm text-gray-500 dark:text-dark-400">
                  <PlatformIcon :platform="selectedCard.model.platform as GroupPlatform" size="xs" />
                  {{ platformLabel(selectedCard.model.platform) }}
                </p>
              </div>
            </div>

            <div class="mt-8 space-y-5">
              <div v-if="selectedGroupIntroductions.length" class="rounded-xl border border-primary-100 bg-primary-50/60 px-4 py-4 dark:border-primary-900/40 dark:bg-primary-900/10">
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('videoPlaza.groupIntroduction') }}</h2>
                <div class="mt-3 space-y-3">
                  <div v-for="group in selectedGroupIntroductions" :key="group.id">
                    <p class="text-sm font-medium text-gray-800 dark:text-dark-100">{{ group.name }}</p>
                    <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ group.description || t('videoPlaza.groupIntroductionFallback') }}</p>
                  </div>
                </div>
              </div>
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('videoPlaza.modelIntroduction') }}</h2>
                <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-300">{{ t('videoPlaza.modelIntroductionText') }}</p>
              </div>
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('videoPlaza.aboutModel') }}</h2>
                <div class="mt-3 grid overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700 sm:grid-cols-2">
                  <div class="flex border-b border-gray-100 dark:border-dark-700 sm:border-r">
                    <span class="w-28 shrink-0 bg-gray-50 px-3 py-3 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">{{ t('videoPlaza.capability') }}</span>
                    <span class="px-3 py-3 text-sm text-gray-800 dark:text-dark-100">{{ t('videoPlaza.videoGeneration') }}</span>
                  </div>
                  <div class="flex border-b border-gray-100 dark:border-dark-700">
                    <span class="w-28 shrink-0 bg-gray-50 px-3 py-3 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">{{ t('videoPlaza.brand') }}</span>
                    <span class="px-3 py-3 text-sm text-gray-800 dark:text-dark-100">{{ platformLabel(selectedCard.model.platform) }}</span>
                  </div>
                  <div class="flex sm:border-r sm:border-gray-100 sm:dark:border-dark-700">
                    <span class="w-28 shrink-0 bg-gray-50 px-3 py-3 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">{{ t('videoPlaza.pricingPlans') }}</span>
                    <span class="px-3 py-3 text-sm text-gray-800 dark:text-dark-100">{{ selectedCard.model.offers.length }} {{ t('videoPlaza.pricingPlanCount') }}</span>
                  </div>
                  <div class="flex">
                    <span class="w-28 shrink-0 bg-gray-50 px-3 py-3 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">{{ t('videoPlaza.billingRule') }}</span>
                    <span class="px-3 py-3 text-sm text-gray-800 dark:text-dark-100">{{ billingRule(selectedCard.startingOffer) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="px-6 py-6 sm:px-8">
            <div class="flex flex-wrap items-end justify-between gap-3">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('videoPlaza.pricingInfo') }}</h2>
              <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('videoPlaza.currentRate') }} ×{{ formatPrice(selectedCard.startingOffer.rate_multiplier) }}</span>
            </div>
            <div class="mt-3 overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
              <table class="w-full min-w-[680px] text-sm">
                <thead class="border-b border-gray-200 bg-gray-50 text-left text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400">
                  <tr>
                    <th class="px-4 py-3 font-medium">{{ t('videoPlaza.serviceGroup') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('videoPlaza.billingRule') }}</th>
                    <th class="px-4 py-3 font-medium">{{ t('videoPlaza.priceDetails') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                  <tr v-for="offer in selectedCard.model.offers" :key="offer.group_id" class="align-top">
                    <td class="px-4 py-4">
                      <p class="font-medium text-gray-900 dark:text-white">{{ offer.group_name }}</p>
                      <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('videoPlaza.siteRelay') }}</p>
                    </td>
                    <td class="px-4 py-4 text-gray-600 dark:text-dark-300">{{ billingRule(offer) }}</td>
                    <td class="px-4 py-4">
                      <div v-if="offer.tiers?.length" class="grid gap-x-8 gap-y-1 sm:grid-cols-2">
                        <div v-for="tier in offer.tiers" :key="tier.label" class="whitespace-nowrap">
                          <span class="text-gray-500 dark:text-dark-400">{{ formatVideoTierLabel(tier.label) }}</span>
                          <span v-if="originalPrice(tier.price, offer.rate_multiplier) !== null" class="ml-2 text-xs text-gray-400 line-through dark:text-dark-500">¥{{ formatPrice(originalPrice(tier.price, offer.rate_multiplier) ?? 0) }}</span>
                          <span class="ml-1 font-mono font-semibold text-primary-600 dark:text-primary-300">¥{{ formatPrice(tier.price) }}</span>
                          <span class="ml-1 text-xs text-gray-500 dark:text-dark-400">{{ unitLabel(offer) }}</span>
                        </div>
                      </div>
                      <div v-else class="whitespace-nowrap">
                        <span v-if="originalPrice(offer.default_price ?? 0, offer.rate_multiplier) !== null" class="text-xs text-gray-400 line-through dark:text-dark-500">¥{{ formatPrice(originalPrice(offer.default_price ?? 0, offer.rate_multiplier) ?? 0) }}</span>
                        <span class="ml-2 font-mono font-semibold text-primary-600 dark:text-primary-300">¥{{ formatPrice(offer.default_price ?? 0) }}</span>
                        <span class="ml-1 text-xs text-gray-500 dark:text-dark-400">{{ unitLabel(offer) }}</span>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p class="mt-3 text-xs text-gray-400 dark:text-dark-500">{{ t('videoPlaza.priceNote') }}</p>
          </div>
        </section>
      </template>

      <template v-else>
        <div class="grid gap-6 lg:grid-cols-[190px_minmax(0,1fr)]">
          <aside class="h-fit rounded-xl border border-gray-200 bg-white p-2 shadow-card dark:border-dark-700/70 dark:bg-dark-800/60">
            <button type="button" class="flex w-full items-center justify-between rounded-lg px-3 py-2.5 text-left text-sm font-medium transition" :class="category === 'all' ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-700/60'" @click="category = 'all'">
              <span class="flex items-center gap-2"><Icon name="grid" size="sm" />{{ t('videoPlaza.allModels') }}</span>
              <span class="text-xs text-gray-400 dark:text-dark-500">{{ totalModelCount }}</span>
            </button>
            <button type="button" class="mt-1 flex w-full items-center justify-between rounded-lg px-3 py-2.5 text-left text-sm font-medium transition" :class="category === 'video' ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-700/60'" @click="category = 'video'">
              <span class="flex items-center gap-2"><Icon name="play" size="sm" />{{ t('videoPlaza.videoCategory') }}</span>
              <span class="text-xs text-gray-400 dark:text-dark-500">{{ totalModelCount }}</span>
            </button>
            <div class="my-2 border-t border-gray-100 dark:border-dark-700/60" />
            <div class="space-y-1 px-3 py-2 text-xs text-gray-400 dark:text-dark-500">
              <div class="flex items-center justify-between"><span>{{ t('videoPlaza.imageCategory') }}</span><span>0</span></div>
              <div class="flex items-center justify-between"><span>{{ t('videoPlaza.textCategory') }}</span><span>0</span></div>
              <div class="flex items-center justify-between"><span>{{ t('videoPlaza.embeddingCategory') }}</span><span>0</span></div>
              <div class="flex items-center justify-between"><span>{{ t('videoPlaza.audioCategory') }}</span><span>0</span></div>
            </div>
          </aside>

          <div class="min-w-0 space-y-5">
            <div class="flex flex-col gap-4">
              <div class="flex flex-wrap items-center gap-2">
                <span class="mr-2 text-xs font-semibold text-gray-400 dark:text-dark-500">{{ t('videoPlaza.brandFilter') }}</span>
                <button type="button" class="rounded-lg px-3 py-1.5 text-sm font-medium transition" :class="selectedPlatform === 'all' ? 'bg-primary-600 text-white shadow-sm' : 'bg-gray-50 text-gray-600 hover:bg-gray-100 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700'" @click="selectedPlatform = 'all'">
                  {{ t('videoPlaza.allModels') }}
                </button>
                <button v-for="platform in platforms" :key="platform" type="button" class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition" :class="selectedPlatform === platform ? 'bg-primary-600 text-white shadow-sm' : 'bg-gray-50 text-gray-600 hover:bg-gray-100 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700'" @click="selectedPlatform = platform">
                  <PlatformIcon :platform="platform as GroupPlatform" size="xs" />
                  {{ platformLabel(platform) }}
                  <span class="text-xs opacity-70">{{ platformModelCount(platform) }}</span>
                </button>
              </div>
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
                <label class="flex items-center gap-2 text-xs font-semibold text-gray-400 dark:text-dark-500">
                  {{ t('videoPlaza.groupFilter') }}
                  <select v-model="selectedGroup" class="input min-w-[190px] py-1.5 text-sm font-normal">
                    <option value="all">{{ t('videoPlaza.allGroups') }}</option>
                    <option v-for="group in groups" :key="group.id" :value="String(group.id)">{{ group.name }}</option>
                  </select>
                </label>
                <label class="relative block sm:ml-auto sm:w-72">
                  <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                  <input v-model="search" class="input w-full py-1.5 pl-9" :placeholder="t('videoPlaza.searchPlaceholder')" />
                </label>
              </div>
            </div>

            <section
              v-if="selectedGroupDetails"
              class="rounded-xl border border-primary-100 bg-primary-50/60 px-4 py-4 dark:border-primary-900/40 dark:bg-primary-900/10"
            >
              <div class="flex items-start gap-3">
                <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0 text-primary-600 dark:text-primary-300" />
                <div class="min-w-0">
                  <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
                    {{ t('videoPlaza.groupIntroduction') }}：{{ selectedGroupDetails.name }}
                  </h2>
                  <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">
                    {{ selectedGroupDetails.description || t('videoPlaza.groupIntroductionFallback') }}
                  </p>
                </div>
              </div>
            </section>

            <div v-if="cards.length" class="space-y-4">
              <div class="flex items-center gap-2 text-sm font-semibold text-gray-700 dark:text-dark-200">
                <Icon name="play" size="sm" class="text-primary-600 dark:text-primary-300" />
                {{ t('videoPlaza.videoCategory') }}
                <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-normal text-gray-500 dark:bg-dark-800 dark:text-dark-400">{{ cards.length }}</span>
              </div>
              <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                <button v-for="card in cards" :key="card.key" type="button" class="group min-h-[184px] rounded-xl border border-gray-200 bg-white p-4 text-left shadow-card transition-all hover:-translate-y-0.5 hover:border-primary-300 hover:shadow-lg dark:border-dark-700/70 dark:bg-dark-800/60 dark:hover:border-primary-600" @click="openDetails(card)">
                  <div class="flex items-start gap-3">
                    <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300">
                      <PlatformIcon :platform="card.model.platform as GroupPlatform" size="lg" />
                    </span>
                    <div class="min-w-0">
                      <h2 class="break-all text-base font-semibold text-gray-900 dark:text-white">{{ card.model.name }}</h2>
                      <p class="mt-1 flex items-center gap-1 text-xs text-gray-500 dark:text-dark-400">
                        {{ platformLabel(card.model.platform) }} <span aria-hidden="true">·</span> <span class="truncate">{{ card.groupNames.join(' / ') }}</span>
                      </p>
                    </div>
                  </div>
                  <p class="mt-5 text-xs text-gray-500 dark:text-dark-400">{{ t('videoPlaza.modelDescription') }}</p>
                  <div class="mt-5 flex items-end justify-between gap-3">
                    <div>
                      <p class="text-[11px] text-gray-400 dark:text-dark-500">{{ t('videoPlaza.startingPrice') }}</p>
                      <div class="mt-1 flex items-baseline gap-1.5">
                        <span v-if="originalPrice(card.startingPrice, card.startingOffer.rate_multiplier) !== null" class="text-xs text-gray-400 line-through dark:text-dark-500">¥{{ formatPrice(originalPrice(card.startingPrice, card.startingOffer.rate_multiplier) ?? 0) }}</span>
                        <span class="font-mono text-lg font-semibold text-primary-600 dark:text-primary-300">¥{{ formatPrice(card.startingPrice) }}</span>
                        <span class="text-xs font-normal text-gray-400">{{ unitLabel(card.startingOffer) }}</span>
                      </div>
                    </div>
                    <span v-if="discountLabel(card.startingOffer)" class="rounded-md bg-primary-500 px-2 py-1 text-xs font-semibold text-white">{{ discountLabel(card.startingOffer) }}</span>
                    <span class="text-xs text-gray-400 transition-colors group-hover:text-primary-500">{{ t('videoPlaza.viewDetails') }} <Icon name="chevronRight" size="xs" class="inline" /></span>
                  </div>
                </button>
              </div>
            </div>
            <div v-else class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">{{ t('videoPlaza.empty') }}</div>
          </div>
        </div>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { platformLabel as getPlatformLabel } from '@/utils/platformColors'
import type { VideoPlazaGroup, VideoPlazaModel, VideoPlazaOffer } from '@/api/videoModelPlaza'

interface VideoCard {
  key: string
  model: VideoPlazaModel
  groupNames: string[]
  startingOffer: VideoPlazaOffer
  startingPrice: number
}

const props = defineProps<{ groups: VideoPlazaGroup[]; loading: boolean; error?: boolean; embedded?: boolean }>()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const selectedPlatform = ref('all')
const selectedGroup = ref<string>('all')
const category = ref<'all' | 'video'>('all')
const search = ref('')

const platforms = computed(() => [...new Set(props.groups.map((group) => group.platform).filter(Boolean))].sort())

const visibleGroups = computed(() => props.groups.filter((group) => {
  if (selectedGroup.value !== 'all' && String(group.id) !== selectedGroup.value) return false
  return selectedPlatform.value === 'all' || group.platform === selectedPlatform.value
}))

const cards = computed<VideoCard[]>(() => {
  const modelMap = new Map<string, VideoCard>()
  const query = search.value.trim().toLowerCase()
  for (const group of visibleGroups.value) {
    for (const model of group.models) {
      if (query && !`${model.name} ${model.platform} ${group.name}`.toLowerCase().includes(query)) continue
      const key = `${model.platform}:${model.name}`
      const existing = modelMap.get(key)
      if (existing) {
        existing.model.offers.push(...model.offers.map((offer) => ({ ...offer, tiers: offer.tiers?.map((tier) => ({ ...tier })) })))
        if (!existing.groupNames.includes(group.name)) existing.groupNames.push(group.name)
        const candidate = cheapestOffer(model)
        if (cheapestPrice(candidate) < existing.startingPrice) {
          existing.startingOffer = candidate
          existing.startingPrice = cheapestPrice(candidate)
        }
      } else {
        const copiedModel = { ...model, offers: model.offers.map((offer) => ({ ...offer, tiers: offer.tiers?.map((tier) => ({ ...tier })) })) }
        const startingOffer = cheapestOffer(copiedModel)
        modelMap.set(key, { key, model: copiedModel, groupNames: [group.name], startingOffer, startingPrice: cheapestPrice(startingOffer) })
      }
    }
  }
  return [...modelMap.values()].sort((a, b) => a.model.name.localeCompare(b.model.name))
})

const totalModelCount = computed(() => {
  const names = new Set<string>()
  for (const group of props.groups) {
    for (const model of group.models) names.add(`${model.platform}:${model.name}`)
  }
  return names.size
})

const selectedCard = computed(() => {
  const model = typeof route.query.model === 'string' ? route.query.model : ''
  const platform = typeof route.query.platform === 'string' ? route.query.platform : ''
  return cards.value.find((card) => card.model.name === model && (!platform || card.model.platform === platform)) ?? null
})

const selectedGroupDetails = computed(() => {
  if (selectedGroup.value === 'all') return null
  return props.groups.find((group) => String(group.id) === selectedGroup.value) ?? null
})

const selectedGroupIntroductions = computed(() => {
  if (!selectedCard.value) return []
  const groupIDs = selectedGroup.value === 'all'
    ? selectedCard.value.model.offers.map((offer) => offer.group_id)
    : [Number(selectedGroup.value)]
  const seen = new Set<number>()
  return groupIDs.reduce<VideoPlazaGroup[]>((result, groupID) => {
    if (seen.has(groupID)) return result
    const group = props.groups.find((item) => item.id === groupID)
    if (group) {
      seen.add(groupID)
      result.push(group)
    }
    return result
  }, [])
})

watch([() => props.groups, cards], () => {
  if (route.query.model && !selectedCard.value) void router.replace({ query: { ...route.query, model: undefined, platform: undefined } })
})

function openDetails(card: VideoCard): void {
  void router.push({ query: { ...route.query, model: card.model.name, platform: card.model.platform } })
}

function closeDetails(): void {
  void router.push({ query: { ...route.query, model: undefined, platform: undefined } })
}

function platformModelCount(platform: string): number {
  const names = new Set<string>()
  for (const group of props.groups) {
    if (selectedGroup.value !== 'all' && String(group.id) !== selectedGroup.value) continue
    if (group.platform === platform) group.models.forEach((model) => names.add(model.name))
  }
  return names.size
}

function cheapestOffer(model: VideoPlazaModel): VideoPlazaOffer {
  return model.offers.reduce((best, offer) => cheapestPrice(offer) < cheapestPrice(best) ? offer : best, model.offers[0])
}

function cheapestPrice(value: VideoPlazaModel | VideoPlazaOffer): number {
  const offers = 'offers' in value ? value.offers : [value]
  return Math.min(...offers.flatMap((offer) => offer.tiers?.map((tier) => tier.price) ?? [offer.default_price ?? Number.POSITIVE_INFINITY]))
}

function originalPrice(price: number, rate: number): number | null {
  return rate > 0 && rate !== 1 && Number.isFinite(price) ? price / rate : null
}

function discountLabel(offer: VideoPlazaOffer): string {
  if (!offer || offer.rate_multiplier === 1) return ''
  return offer.rate_multiplier < 1 ? `${formatPrice(offer.rate_multiplier * 10)}${t('videoPlaza.discountSuffix')}` : `×${formatPrice(offer.rate_multiplier)}`
}

function formatPrice(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return value.toFixed(value < 0.01 ? 6 : value < 1 ? 4 : 3).replace(/0+$/, '').replace(/\.$/, '')
}

function unitLabel(offer: VideoPlazaOffer): string {
  return offer.billing_mode === 'video' ? t('videoPlaza.perSecond') : t('videoPlaza.perRequest')
}

function billingRule(offer: VideoPlazaOffer): string {
  return offer.billing_mode === 'video' ? t('videoPlaza.byDuration') : t('videoPlaza.byRequest')
}

function formatVideoTierLabel(label: string): string {
  return label.split('|', 1)[0]?.trim() || label.trim()
}

function platformLabel(platform: string): string {
  return getPlatformLabel(platform)
}
</script>
