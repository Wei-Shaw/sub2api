<template>
  <div class="available-models-shell">
    <div v-if="loading" class="empty-state">
      <Icon name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
    </div>

    <div v-else-if="rows.length === 0" class="empty-state">
      <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyLabel }}</p>
    </div>

    <div v-else class="model-page-grid">
      <aside class="channel-tabs-shell">
        <div class="section-eyebrow">{{ viewModeLabel }}</div>
        <button
          v-for="tabItem in sidebarTabs"
          :key="tabItem.key"
          type="button"
          class="group-tab"
          :class="{ active: tabItem.key === activeTabKey }"
          @click="selectTab(tabItem.key)"
        >
          <span class="group-tab-top">
            <span
              class="group-name-pill"
              :class="platformPillClass(tabItem.platform)"
            >
              <PlatformIcon :platform="tabItem.platform as GroupPlatform" size="sm" />
              <span class="truncate">{{ tabItem.name }}</span>
            </span>
            <span class="rate-badge" :class="platformPillClass(tabItem.platform)">
              {{ formatRate(tabItem.rateMultiplier) }} 倍率
            </span>
          </span>
          <span class="group-desc">{{ tabItem.description }}</span>
          <span class="group-tab-meta">
            <span class="mini-badge">{{ tabItem.modelCount }} 模型</span>
            <span class="mini-badge">{{ tabItem.accountCount }} 账号</span>
            <span v-if="tabItem.channelCount > 0" class="mini-badge">{{ tabItem.channelCount }} 渠道</span>
          </span>
        </button>
      </aside>

      <section class="content-shell">
        <div v-if="activeTab" class="content-head">
          <div class="content-title-row">
            <div class="content-icon" :class="platformPillClass(activeTab.platform)">
              <PlatformIcon :platform="activeTab.platform as GroupPlatform" size="lg" />
            </div>
            <div class="content-title-copy">
              <div class="content-heading-line">
                <h2>{{ activeTab.name }}</h2>
                <div class="mobile-group-filter">
                  <button
                    type="button"
                    class="mobile-filter-btn"
                    :aria-expanded="mobileGroupDropdownOpen"
                    @click.stop="mobileGroupDropdownOpen = !mobileGroupDropdownOpen"
                  >
                    <Icon name="filter" size="sm" />
                    <span>筛选</span>
                    <Icon name="chevronDown" size="xs" />
                  </button>
                  <div v-if="mobileGroupDropdownOpen" class="mobile-filter-menu">
                    <button
                      v-for="tabItem in sidebarTabs"
                      :key="`mobile-${tabItem.key}`"
                      type="button"
                      class="mobile-filter-item"
                      :class="{ active: tabItem.key === activeTabKey }"
                      @click="selectTab(tabItem.key)"
                    >
                      <span class="mobile-filter-item-main">
                        <PlatformIcon :platform="tabItem.platform as GroupPlatform" size="xs" />
                        <span>{{ tabItem.name }}</span>
                      </span>
                      <span class="mobile-filter-item-meta">
                        {{ formatRate(tabItem.rateMultiplier) }} · {{ tabItem.modelCount }} 模型
                      </span>
                    </button>
                  </div>
                </div>
              </div>
              <p class="content-meta-line">
                {{ activeTab.platform }} · {{ activeTab.subscriptionType || 'standard' }} ·
                {{ activeTab.isExclusive ? '专属' : '公开' }} · {{ formatRate(activeTab.rateMultiplier) }}
              </p>
              <p class="content-desc">
                {{ activeTab.description }}
              </p>
              <div class="content-tags">
                <span class="mini-badge">
                  <PlatformIcon :platform="activeTab.platform as GroupPlatform" size="xs" />
                  {{ activeTab.platform }}
                </span>
                <span class="mini-badge">
                  <PlatformIcon :platform="activeTab.platform as GroupPlatform" size="xs" />
                  {{ activeTab.subscriptionType || 'standard' }}
                </span>
                <span class="mini-badge">
                  <PlatformIcon :platform="activeTab.platform as GroupPlatform" size="xs" />
                  {{ activeTab.isExclusive ? '专属分组' : '公开分组' }}
                </span>
              </div>
            </div>
          </div>
          <div class="content-side">
            <div class="kpi">
              <div class="value">{{ activeTab.modelCount }}</div>
              <div class="subvalue">模型</div>
            </div>
            <div class="kpi">
              <div class="value">{{ activeTab.accountCount }}</div>
              <div class="subvalue">账号</div>
            </div>
          </div>
        </div>

        <template
          v-for="section in activeTab?.sections || []"
          :key="`${activeTab?.key}-${section.key}`"
        >
          <div class="section-title">
            <span>{{ section.title }}</span>
            <span>{{ section.supported_models.length }} models</span>
          </div>
          <div v-if="section.supported_models.length > 0" class="model-card-grid">
            <article
              v-for="m in section.supported_models"
              :key="`${section.key}-${m.name}`"
              class="model-card"
              :class="{ 'unit-priced-model-card': isUnitPricedModel(m) }"
            >
              <div class="model-header">
                <div class="model-title-row">
                  <div class="model-avatar">
                    <ModelIcon :model="m.name" size="26px" />
                  </div>
                  <div class="min-w-0">
                    <h3 class="model-name">{{ m.name }}</h3>
                    <div class="model-tags">
                      <span class="mini-badge">{{ m.platform || section.platform }}</span>
                      <span class="mini-badge">{{ m.pricing?.billing_mode || 'token' }}</span>
                    </div>
                  </div>
                </div>
                <button
                  class="copy-btn"
                  type="button"
                  :title="copiedModel === m.name ? '已复制' : '复制 ID'"
                  :aria-label="copiedModel === m.name ? '已复制' : '复制 ID'"
                  @click="copyModelId(m.name)"
                >
                  <Icon :name="copiedModel === m.name ? 'check' : 'copy'" size="sm" />
                </button>
              </div>
              <div v-if="isUnitPricedModel(m)" class="price-grid unit-price-grid">
                <div class="price-box unit-price-box">
                  <div class="label unit-price-label">
                    <span>{{ unitPriceTitle(m) }}</span>
                    <span
                      v-for="entry in unitPriceEntries(m)"
                      :key="entry.label"
                      class="tier-label"
                    >
                      {{ entry.label }}
                    </span>
                  </div>
                  <div class="tier-price-list">
                    <div
                      v-for="entry in unitPriceEntries(m)"
                      :key="entry.label"
                      class="tier-price-row"
                    >
                      <div class="price-lines">
                        <div class="price-line">
                          <span>原始</span>
                          <strong>{{ unitPriceDisplay(entry.value) }}</strong>
                        </div>
                        <div class="price-line actual">
                          <span class="actual-label">
                            实际
                            <span
                              class="price-help"
                              :data-tooltip="effectiveUnitPriceTooltip(entry.value)"
                              tabindex="0"
                              aria-label="实际价格计算方式"
                            >
                              <Icon name="questionCircle" size="xs" />
                            </span>
                          </span>
                          <strong>{{ cnyUnitPriceDisplay(effectiveUnitCnyPrice(entry.value)) }}</strong>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              <div v-else class="price-grid">
                <div class="price-box">
                  <div class="label">输入价格 / 1M tokens</div>
                  <div class="price-lines">
                    <div class="price-line">
                      <span>原始</span>
                      <strong>{{ priceDisplay(averagePrice(m.pricing?.input_price ?? null, m.pricing?.intervals, 'input_price')) }}</strong>
                    </div>
                    <div class="price-line actual">
                      <span class="actual-label">
                        实际
                        <span
                          class="price-help"
                          :data-tooltip="effectivePriceTooltip(m, 'input_price')"
                          tabindex="0"
                          aria-label="实际价格计算方式"
                        >
                          <Icon name="questionCircle" size="xs" />
                        </span>
                      </span>
                      <strong>{{ effectiveCnyPriceDisplay(m, 'input_price') }}</strong>
                    </div>
                  </div>
                </div>
                <div class="price-box">
                  <div class="label">输出价格 / 1M tokens</div>
                  <div class="price-lines">
                    <div class="price-line">
                      <span>原始</span>
                      <strong>{{ priceDisplay(averagePrice(m.pricing?.output_price ?? null, m.pricing?.intervals, 'output_price')) }}</strong>
                    </div>
                    <div class="price-line actual">
                      <span class="actual-label">
                        实际
                        <span
                          class="price-help"
                          :data-tooltip="effectivePriceTooltip(m, 'output_price')"
                          tabindex="0"
                          aria-label="实际价格计算方式"
                        >
                          <Icon name="questionCircle" size="xs" />
                        </span>
                      </span>
                      <strong>{{ effectiveCnyPriceDisplay(m, 'output_price') }}</strong>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="priceFoot(m)" class="price-foot">{{ priceFoot(m) }}</div>
            </article>
          </div>
          <div v-else class="empty-state">{{ noModelsLabel }}</div>
        </template>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import type { UserAvailableChannel, UserAvailableGroup, UserSupportedModel } from '@/api/channels'
import type { Group, GroupPlatform } from '@/types'
import { BILLING_MODE_IMAGE, BILLING_MODE_PER_REQUEST } from '@/constants/channel'
import { useAppStore } from '@/stores/app'

type ViewMode = 'channel' | 'group'

interface ViewSection {
  key: string
  title: string
  platform: string
  channel_name: string
  supported_models: UserSupportedModel[]
}

interface ViewTab {
  id?: number
  key: string
  name: string
  description: string
  platform: string
  rateMultiplier: number
  subscriptionType: string
  isExclusive: boolean
  modelCount: number
  accountCount: number
  channelCount: number
  imagePrice1K?: number | null
  imagePrice2K?: number | null
  imagePrice4K?: number | null
  sections: ViewSection[]
}

const props = defineProps<{
  columns: {
    name: string
    description: string
    platform: string
    groups: string
    supportedModels: string
  }
  rows: UserAvailableChannel[]
  loading: boolean
  pricingKeyPrefix: string
  noPricingLabel: string
  noModelsLabel: string
  emptyLabel: string
  userGroupRates: Record<number, number>
  viewMode: ViewMode
  availableGroups: Group[]
}>()

void props.pricingKeyPrefix

const activeChannelName = ref('')
const activeGroupId = ref<number | null>(null)
const copiedModel = ref('')
const appStore = useAppStore()
const mobileGroupDropdownOpen = ref(false)

const groupViewRows = computed(() => buildGroupViewRows(props.rows))

const sidebarTabs = computed(() => {
  return props.viewMode === 'group'
    ? groupViewRows.value
    : props.rows.map((channel) => buildChannelTab(channel))
})

const activeTabKey = computed({
  get() {
    return props.viewMode === 'group'
      ? (activeGroupId.value == null ? '' : `group-${activeGroupId.value}`)
      : activeChannelName.value
  },
  set(value: string) {
    if (props.viewMode === 'group') {
      const parsed = Number(value.replace(/^group-/, ''))
      activeGroupId.value = Number.isFinite(parsed) ? parsed : null
      return
    }
    activeChannelName.value = value
  },
})

const activeTab = computed(() => {
  return props.viewMode === 'group'
    ? groupViewRows.value.find((item) => item.key === activeTabKey.value) || groupViewRows.value[0] || null
    : sidebarTabs.value.find((item) => item.key === activeTabKey.value) || sidebarTabs.value[0] || null
})

const viewModeLabel = computed(() => props.viewMode === 'group' ? '用户可用分组' : props.columns.name)

function selectTab(key: string) {
  activeTabKey.value = key
  mobileGroupDropdownOpen.value = false
}

watch(
  () => props.rows,
  (rows) => {
    if (!rows.some((channel) => channel.name === activeChannelName.value)) {
      activeChannelName.value = rows[0]?.name || ''
    }
    if (!groupViewRows.value.some((row) => row.id === activeGroupId.value)) {
      activeGroupId.value = groupViewRows.value[0]?.id ?? null
    }
  },
  { immediate: true },
)

watch(
  () => props.viewMode,
  (mode) => {
    if (mode === 'group' && activeGroupId.value == null) {
      activeGroupId.value = groupViewRows.value[0]?.id ?? null
    }
    if (mode === 'channel' && !props.rows.some((channel) => channel.name === activeChannelName.value)) {
      activeChannelName.value = props.rows[0]?.name || ''
    }
  },
  { immediate: true },
)

function modelCount(channel: UserAvailableChannel): number {
  return channel.platforms.reduce((sum, section) => sum + section.supported_models.length, 0)
}

function accountCount(channel: UserAvailableChannel): number {
  return channel.platforms.reduce(
    (sum, section) => sum + section.groups.reduce((groupSum, group) => groupSum + groupAccountCount(group), 0),
    0,
  )
}

function groupAccountCount(group: UserAvailableGroup): number {
  const maybeCount = group as UserAvailableGroup & { account_count?: number; accounts_count?: number }
  return maybeCount.account_count ?? maybeCount.accounts_count ?? 0
}

function buildChannelTab(channel: UserAvailableChannel): ViewTab {
  const groups = channel.platforms.flatMap((section) => section.groups)
  const firstGroup = groups[0]
  return {
    key: channel.name,
    name: channel.name,
    description: channel.description || props.columns.supportedModels,
    platform: channel.platforms[0]?.platform || 'openai',
    rateMultiplier: firstGroup ? effectiveGroupRate(firstGroup.id, firstGroup.rate_multiplier) : 1,
    subscriptionType: firstGroup?.subscription_type || 'standard',
    isExclusive: firstGroup?.is_exclusive || false,
    modelCount: modelCount(channel),
    accountCount: accountCount(channel),
    channelCount: 1,
    sections: channel.platforms.map((section) => ({
      key: section.platform,
      title: section.platform,
      platform: section.platform,
      channel_name: channel.name,
      supported_models: section.supported_models,
    })),
  }
}

function buildGroupViewRows(rows: UserAvailableChannel[]): ViewTab[] {
  const availableGroupMap = new Map(props.availableGroups.map((group) => [group.id, group]))
  const map = new Map<number, {
    id: number
    name: string
    platform: string
    description: string
    rateMultiplier: number
    isExclusive: boolean
    subscriptionType: string
    accountCount: number
    imagePrice1K: number | null
    imagePrice2K: number | null
    imagePrice4K: number | null
    channelNames: Set<string>
    sectionMap: Map<string, {
      key: string
      title: string
      platform: string
      channel_name: string
      modelMap: Map<string, UserSupportedModel>
    }>
  }>()

  for (const channel of rows) {
    for (const section of channel.platforms) {
      for (const group of section.groups) {
        const groupId = group.id
        const fullGroup = availableGroupMap.get(groupId)
        const existing = map.get(groupId) || {
          id: groupId,
          name: fullGroup?.name || group.name,
          platform: fullGroup?.platform || group.platform || section.platform,
          description: fullGroup?.description || `${fullGroup?.name || group.name} 可用模型分组`,
          rateMultiplier: effectiveGroupRate(groupId, fullGroup?.rate_multiplier ?? group.rate_multiplier),
          isExclusive: fullGroup?.is_exclusive ?? group.is_exclusive,
          subscriptionType: fullGroup?.subscription_type ?? group.subscription_type,
          accountCount: groupAccountCount(group),
          imagePrice1K: fullGroup?.image_price_1k ?? null,
          imagePrice2K: fullGroup?.image_price_2k ?? null,
          imagePrice4K: fullGroup?.image_price_4k ?? null,
          channelNames: new Set<string>(),
          sectionMap: new Map<string, {
            key: string
            title: string
            platform: string
            channel_name: string
            modelMap: Map<string, UserSupportedModel>
          }>(),
        }
        existing.channelNames.add(channel.name)
        existing.accountCount = Math.max(existing.accountCount, groupAccountCount(group))
        existing.platform = existing.platform || group.platform || section.platform
        const sectionKey = `${channel.name}::${section.platform}`
        if (!existing.sectionMap.has(sectionKey)) {
          existing.sectionMap.set(sectionKey, {
            key: sectionKey,
            title: `${channel.name} / ${section.platform}`,
            platform: section.platform,
            channel_name: channel.name,
            modelMap: new Map<string, UserSupportedModel>(),
          })
        }
        const bucket = existing.sectionMap.get(sectionKey)!
        for (const model of section.supported_models) {
          if (!bucket.modelMap.has(model.name)) {
            bucket.modelMap.set(model.name, model)
          }
        }
        map.set(groupId, existing)
      }
    }
  }

  return Array.from(map.values())
    .sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
    .map((group) => {
      const sections = Array.from(group.sectionMap.values())
        .map((section) => ({
          key: section.key,
          title: section.title,
          platform: section.platform,
          channel_name: section.channel_name,
          supported_models: Array.from(section.modelMap.values()),
        }))
        .sort((a, b) => a.title.localeCompare(b.title, 'zh-Hans-CN'))
      const modelCountTotal = sections.reduce((sum, section) => sum + section.supported_models.length, 0)
      return {
        id: group.id,
        key: `group-${group.id}`,
        name: group.name,
        description: group.description,
        platform: group.platform,
        rateMultiplier: group.rateMultiplier,
        subscriptionType: group.subscriptionType,
        isExclusive: group.isExclusive,
        modelCount: modelCountTotal,
        accountCount: group.accountCount,
        channelCount: group.channelNames.size,
        imagePrice1K: group.imagePrice1K,
        imagePrice2K: group.imagePrice2K,
        imagePrice4K: group.imagePrice4K,
        sections,
      }
    })
}

function effectiveGroupRate(groupId: number, defaultRate: number): number {
  return props.userGroupRates[groupId] ?? defaultRate ?? 1
}

function formatRate(value: number): string {
  return Number.isInteger(value) ? value.toFixed(0) + 'x' : `${Number(value.toFixed(4))}x`
}

function platformPillClass(platform: string): string {
  if (platform === 'anthropic') return 'anthropic'
  if (platform === 'openai') return 'openai'
  if (platform === 'gemini') return 'gemini'
  return 'generic'
}

function formatCompactNumber(value: number): string {
  return Number.isInteger(value)
    ? value.toFixed(0)
    : value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
}

function priceDisplay(value: number | null): string {
  if (value === null) return props.noPricingLabel || '-'
  const perMillion = Math.abs(value) < 0.01 ? value * 1_000_000 : value
  return `$${formatCompactNumber(perMillion)}`
}

function cnyPriceDisplay(value: number | null): string {
  if (value === null) return props.noPricingLabel || '-'
  const perMillion = Math.abs(value) < 0.01 ? value * 1_000_000 : value
  return `¥${formatCompactNumber(perMillion)}`
}

function unitPriceDisplay(value: number | null): string {
  if (value === null) return props.noPricingLabel || '-'
  return `$${formatCompactNumber(value)}`
}

function cnyUnitPriceDisplay(value: number | null): string {
  if (value === null) return props.noPricingLabel || '-'
  return `￥${formatCompactNumber(value)}`
}

type PriceField = 'input_price' | 'output_price'
type UnitPriceEntry = { label: string; value: number | null }

function averagePrice(
  fallback: number | null,
  intervals: Array<Record<PriceField, number | null>> | undefined,
  field: PriceField,
): number | null {
  const values = (intervals || [])
    .map((interval) => interval[field])
    .filter((value): value is number => typeof value === 'number')
  if (values.length === 0) return fallback
  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function averageRechargeMultiplier(): number {
  const settings = appStore.cachedPublicSettings
  const tiers = (settings?.payment_balance_pricing_tiers || [])
    .filter((tier) => tier.enabled !== false && Number.isFinite(tier.multiplier) && tier.multiplier > 0)
  if (tiers.length > 0) {
    return tiers.reduce((sum, tier) => sum + tier.multiplier, 0) / tiers.length
  }
  const fallback = settings?.payment_balance_recharge_multiplier
  return typeof fallback === 'number' && Number.isFinite(fallback) && fallback > 0 ? fallback : 1
}

function effectiveCnyPrice(model: UserSupportedModel, field: PriceField): number | null {
  const original = averagePrice(model.pricing?.[field] ?? null, model.pricing?.intervals, field)
  if (original === null) return null
  const rechargeMultiplier = averageRechargeMultiplier()
  const channelMultiplier = activeTab.value?.rateMultiplier ?? 1
  return original * channelMultiplier * rechargeMultiplier
}

function effectiveCnyPriceDisplay(model: UserSupportedModel, field: PriceField): string {
  return cnyPriceDisplay(effectiveCnyPrice(model, field))
}

function effectivePriceTooltip(model: UserSupportedModel, field: PriceField): string {
  const original = averagePrice(model.pricing?.[field] ?? null, model.pricing?.intervals, field)
  if (original === null) return '暂无价格'
  const rechargeMultiplier = averageRechargeMultiplier()
  const channelMultiplier = activeTab.value?.rateMultiplier ?? 1
  const actual = effectiveCnyPrice(model, field)
  return [
    '实际价格按人民币展示',
    `${priceDisplay(original)} × 渠道倍率 ${formatRate(channelMultiplier)} × 充值倍率 ${formatCompactNumber(rechargeMultiplier)} USD/CNY`,
    `= ${cnyPriceDisplay(actual)}`,
  ].join('\n')
}

function isUnitPricedModel(model: UserSupportedModel): boolean {
  const mode = model.pricing?.billing_mode
  return mode === BILLING_MODE_IMAGE || mode === BILLING_MODE_PER_REQUEST
}

function unitPriceTitle(model: UserSupportedModel): string {
  return model.pricing?.billing_mode === BILLING_MODE_IMAGE ? '图片价格 / 张' : '请求价格 / 次'
}

function unitPriceEntries(model: UserSupportedModel): UnitPriceEntry[] {
  const groupEntries = groupImagePriceEntries(model)
  if (groupEntries.length > 0) return groupEntries

  const pricing = model.pricing
  if (!pricing) return [{ label: '默认', value: null }]
  const intervalEntries = (pricing.intervals || [])
    .filter((interval) => typeof interval.per_request_price === 'number')
    .map((interval) => ({
      label: interval.tier_label || intervalLabel(interval.min_tokens, interval.max_tokens),
      value: interval.per_request_price,
    }))
  if (intervalEntries.length > 0) {
    const resolution = modelResolutionSuffix(model.name)
    if (resolution) {
      const matchedEntries = intervalEntries.filter((entry) => normalizeResolutionLabel(entry.label) === resolution)
      if (matchedEntries.length > 0) return matchedEntries
    }
    return intervalEntries
  }
  if (pricing.per_request_price != null) {
    return [{ label: pricing.billing_mode === BILLING_MODE_IMAGE ? '默认' : '每次', value: pricing.per_request_price }]
  }
  return [{ label: '默认', value: null }]
}

function groupImagePriceEntries(model: UserSupportedModel): UnitPriceEntry[] {
  if (!isUnitPricedModel(model)) return []
  const tab = activeTab.value
  if (!tab) return []
  const prices: Record<string, number | null | undefined> = {
    '1K': tab.imagePrice1K,
    '2K': tab.imagePrice2K,
    '4K': tab.imagePrice4K,
  }
  const resolution = modelResolutionSuffix(model.name)
  if (resolution) {
    const value = prices[resolution]
    return typeof value === 'number' ? [{ label: resolution, value }] : []
  }
  return Object.entries(prices)
    .filter((entry): entry is [string, number] => typeof entry[1] === 'number')
    .map(([label, value]) => ({ label, value }))
}

function intervalLabel(minTokens: number, maxTokens: number | null): string {
  if (maxTokens == null) return `${minTokens}+`
  return `${minTokens}-${maxTokens}`
}

function modelResolutionSuffix(modelName: string): string {
  const match = modelName.trim().match(/-(1k|2k|4k)$/i)
  if (match) return match[1].toUpperCase()
  return isImageModelName(modelName) ? '1K' : ''
}

function normalizeResolutionLabel(label: string): string {
  return label.trim().replace(/\s+/g, '').toUpperCase()
}

function isImageModelName(modelName: string): boolean {
  return /^gpt-image-\d+(?:\.\d+)?$/i.test(modelName.trim())
}

function effectiveUnitCnyPrice(original: number | null): number | null {
  if (original === null) return null
  const rechargeMultiplier = averageRechargeMultiplier()
  const channelMultiplier = activeTab.value?.rateMultiplier ?? 1
  return original * channelMultiplier * rechargeMultiplier
}

function effectiveUnitPriceTooltip(original: number | null): string {
  if (original === null) return '暂无价格'
  const rechargeMultiplier = averageRechargeMultiplier()
  const channelMultiplier = activeTab.value?.rateMultiplier ?? 1
  const actual = effectiveUnitCnyPrice(original)
  return [
    '实际价格按人民币展示',
    `${unitPriceDisplay(original)} × 渠道倍率 ${formatRate(channelMultiplier)} × 充值倍率 ${formatCompactNumber(rechargeMultiplier)} USD/CNY`,
    `= ${cnyUnitPriceDisplay(actual)}`,
  ].join('\n')
}

function priceFoot(model: UserSupportedModel): string {
  const pricing = model.pricing
  if (!pricing) return ''
  if (pricing.billing_mode === BILLING_MODE_IMAGE || pricing.billing_mode === BILLING_MODE_PER_REQUEST) return ''
  const parts: string[] = []
  if (pricing.per_request_price != null) parts.push(`请求价 ${priceDisplay(pricing.per_request_price)}`)
  if (pricing.image_output_price != null) parts.push(`图片输出 ${priceDisplay(pricing.image_output_price)}`)
  return parts.join(' / ')
}

async function copyModelId(model: string) {
  try {
    await navigator.clipboard?.writeText(model)
    copiedModel.value = model
    window.setTimeout(() => {
      if (copiedModel.value === model) copiedModel.value = ''
    }, 1200)
  } catch {
    copiedModel.value = ''
  }
}
</script>

<style scoped>
.available-models-shell {
  --text: #0f172a;
  --muted: #475569;
  --shadow: 0 20px 50px rgba(15, 23, 42, .08);
  --shadow-soft: 0 12px 30px rgba(20, 184, 166, .08);
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  overflow: hidden;
  border-radius: 28px;
  padding: 18px;
  color: var(--text);
  background-image:
    radial-gradient(circle at 0% 0%, rgba(45,212,191,.16), transparent 24%),
    radial-gradient(circle at 100% 0%, rgba(125,211,252,.14), transparent 22%),
    linear-gradient(rgba(15,23,42,.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(15,23,42,.04) 1px, transparent 1px);
  background-size: auto, auto, 48px 48px, 48px 48px;
}

.model-page-grid {
  display: grid;
  grid-template-columns: minmax(320px, 360px) minmax(0, 1fr);
  gap: 22px;
  align-items: start;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}

.channel-tabs-shell {
  position: relative;
  z-index: 1;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  border-radius: 28px;
  padding: 18px;
  background: rgba(255,255,255,.55);
  border: 1px solid rgba(255,255,255,.62);
  box-shadow: var(--shadow-soft);
  backdrop-filter: blur(14px);
}

.section-eyebrow {
  position: sticky;
  top: 0;
  z-index: 1;
  margin: -18px -18px 12px;
  padding: 18px 18px 12px;
  color: #0f766e;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: .08em;
  text-transform: uppercase;
  background: rgba(255,255,255,.78);
  backdrop-filter: blur(14px);
}

.group-tab {
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 10px;
  border: 1px solid transparent;
  border-radius: 16px;
  padding: 14px 12px;
  text-align: left;
  background: transparent;
  transition: background .18s ease, border-color .18s ease, box-shadow .18s ease;
}

.group-tab:hover,
.group-tab.active {
  border-color: rgba(15,23,42,.04);
  background: rgba(241,245,249,.82);
  box-shadow: inset 0 0 0 1px rgba(255,255,255,.54);
}

.group-tab-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.group-tab-meta,
.model-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.group-desc {
  color: #64748b;
  font-size: 13px;
  line-height: 1.45;
}

.group-name-pill {
  display: inline-flex;
  min-width: 0;
  max-width: min(100%, 220px);
  align-items: center;
  gap: 7px;
  border-radius: 9px;
  padding: 4px 10px;
  font-size: 16px;
  font-weight: 800;
  line-height: 1.25;
}

.group-name-pill.large {
  font-size: 22px;
  border-radius: 12px;
  padding: 6px 12px;
}

.group-name-pill.anthropic {
  color: #a85516;
  background: #fff7ed;
}

.group-name-pill.openai {
  color: #2f7d3f;
  background: #f0fdf4;
}

.group-name-pill.gemini {
  color: #2563a4;
  background: #eff6ff;
}

.group-name-pill.generic {
  color: #5b21b6;
  background: #f5f3ff;
}

.rate-badge {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  padding: 4px 9px;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.rate-badge.anthropic {
  color: #a85516;
  background: #fff7ed;
}

.rate-badge.openai {
  color: #2f7d3f;
  background: #f0fdf4;
}

.rate-badge.gemini {
  color: #2563a4;
  background: #eff6ff;
}

.rate-badge.generic {
  color: #5b21b6;
  background: #f5f3ff;
}

.mini-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid rgba(15,23,42,.07);
  border-radius: 999px;
  padding: 5px 10px;
  color: #475569;
  background: rgba(255,255,255,.72);
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}

.content-shell {
  position: relative;
  z-index: 2;
  display: grid;
  gap: 18px;
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding-right: 4px;
  align-content: start;
}

.content-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  border: 1px solid rgba(226,232,240,.9);
  border-radius: 28px;
  padding: 28px 32px;
  background: rgba(255,255,255,.82);
  box-shadow: none;
  backdrop-filter: blur(16px);
}

.content-title-row {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  min-width: 0;
}

.content-icon,
.model-avatar {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 56px;
  height: 56px;
  border-radius: 18px;
  color: #fff;
}

.content-icon {
  margin-top: 2px;
}

.content-icon.anthropic {
  color: #a85516;
  background: #fff7ed;
  box-shadow: 0 18px 34px rgba(234, 88, 12, .16);
}

.content-icon.openai {
  color: #2f7d3f;
  background: #f0fdf4;
  box-shadow: 0 18px 34px rgba(34, 197, 94, .16);
}

.content-icon.gemini {
  color: #2563a4;
  background: #eff6ff;
  box-shadow: 0 18px 34px rgba(37, 99, 235, .16);
}

.content-icon.generic {
  color: #5b21b6;
  background: #f5f3ff;
  box-shadow: 0 18px 34px rgba(124, 58, 237, .16);
}

.content-head h2 {
  margin: 0;
  color: #0f172a;
  font-size: 24px;
  font-weight: 850;
  letter-spacing: -.03em;
}

.content-title-copy {
  min-width: 0;
}

.content-heading-line {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 0;
}

.mobile-group-filter {
  display: none;
}

.mobile-filter-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 1px solid rgba(15, 23, 42, .08);
  border-radius: 9px;
  padding: 6px 8px;
  color: #0f172a;
  background: rgba(255, 255, 255, .92);
  box-shadow: 0 1px 2px rgba(15, 23, 42, .06);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.mobile-filter-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 30;
  display: grid;
  width: min(280px, calc(100vw - 32px));
  max-height: 340px;
  overflow-y: auto;
  border: 1px solid rgba(226, 232, 240, .95);
  border-radius: 14px;
  padding: 8px;
  background: rgba(255, 255, 255, .98);
  box-shadow: 0 18px 42px rgba(15, 23, 42, .16);
}

.mobile-filter-item {
  display: grid;
  gap: 4px;
  width: 100%;
  border: 1px solid transparent;
  border-radius: 11px;
  padding: 9px;
  text-align: left;
  background: transparent;
}

.mobile-filter-item.active {
  border-color: rgba(20, 184, 166, .24);
  background: #f0fdfa;
}

.mobile-filter-item-main {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  color: #0f172a;
  font-size: 13px;
  font-weight: 800;
}

.mobile-filter-item-main span:last-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-filter-item-meta {
  color: #64748b;
  font-size: 11px;
  font-weight: 700;
}

.content-meta-line {
  margin: 8px 0 0;
  color: var(--muted);
  font-size: 14px;
  line-height: 1.4;
}

.content-desc {
  margin: 8px 0 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.6;
}

.content-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
}

.content-side {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.kpi,
.price-box {
  border: 1px solid rgba(226,232,240,.9);
  border-radius: 12px;
  padding: 10px 12px;
  background: rgba(248,250,252,.72);
}

.kpi .value,
.price-box .value {
  font-size: clamp(18px, 2.4vw, 24px);
  font-weight: 800;
  letter-spacing: -.03em;
}

.price-lines {
  display: grid;
  gap: 6px;
  margin-top: 4px;
}

.price-line {
  position: relative;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  color: var(--muted);
  font-size: 12px;
}

.price-line strong {
  color: #0f172a;
  font-size: clamp(16px, 2vw, 20px);
  font-weight: 800;
  letter-spacing: -.03em;
}

.price-line:not(.actual) strong {
  font-size: 24px;
}

.price-line.actual strong {
  color: #34a780;
  font-size: 16px;
}

.unit-price-grid {
  grid-template-columns: 1fr;
}

.unit-price-box {
  padding: 0;
  border: 0;
  background: transparent;
}

.tier-price-list {
  display: grid;
  gap: 8px;
  margin-top: 8px;
}

.tier-price-row {
  display: block;
}

.unit-price-label {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.tier-label {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 38px;
  padding: 3px 8px;
  border-radius: 999px;
  color: #0f766e;
  background: rgba(20, 184, 166, .12);
  font-size: 12px;
  font-weight: 700;
}

.actual-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.price-help {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 999px;
  color: #64748b;
  cursor: help;
  outline: none;
}

.price-help:hover,
.price-help:focus-visible {
  color: #0f766e;
  background: rgba(15, 118, 110, .08);
}

.price-help::after {
  position: absolute;
  left: 0;
  bottom: calc(100% + 8px);
  z-index: 999;
  width: max-content;
  max-width: 260px;
  transform: translateX(-10px);
  border: 1px solid rgba(15, 23, 42, .08);
  border-radius: 10px;
  padding: 8px 10px;
  color: #0f172a;
  background: rgba(255, 255, 255, .96);
  box-shadow: 0 12px 30px rgba(15, 23, 42, .16);
  content: attr(data-tooltip);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.55;
  opacity: 0;
  pointer-events: none;
  text-align: left;
  white-space: pre-line;
  transition: opacity .15s ease, transform .15s ease;
}

.price-help:hover::after,
.price-help:focus-visible::after {
  opacity: 1;
  transform: translateX(-10px) translateY(-2px);
}

.kpi .subvalue,
.price-foot,
.price-box .label {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.7;
}

.section-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #0f766e;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: .08em;
  text-transform: uppercase;
}

.model-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 12px;
}

.model-card {
  position: relative;
  border: 1px solid rgba(226,232,240,.9);
  border-radius: 16px;
  padding: 16px 48px 16px 16px;
  background: rgba(255,255,255,.88);
  box-shadow: 0 1px 2px rgba(15,23,42,.04);
  transition: border-color .18s ease, box-shadow .18s ease;
}

.model-card:hover {
  border-color: rgba(20,184,166,.28);
  box-shadow: 0 8px 22px rgba(15,23,42,.06);
}

.model-header,
.model-title-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.model-header {
  justify-content: space-between;
}

.model-title-row {
  min-width: 0;
  width: 100%;
  flex: 1 1 100%;
}

.model-avatar {
  width: 36px;
  height: 36px;
  border-radius: 12px;
  flex: 0 0 36px;
  background: #fff;
  box-shadow: inset 0 0 0 1px rgba(15,23,42,.06);
}

.model-name {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 700;
  line-height: 1.25;
  overflow-wrap: anywhere;
  word-break: normal;
}

.copy-btn {
  position: absolute;
  top: 18px;
  right: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 1px solid rgba(15,23,42,.08);
  border-radius: 10px;
  padding: 0;
  color: #475569;
  background: rgba(255,255,255,.88);
  box-shadow: none;
  cursor: pointer;
  transition: color .15s ease, transform .15s ease, box-shadow .15s ease;
}

.copy-btn:hover {
  color: #0f766e;
  box-shadow: 0 6px 14px rgba(20,184,166,.12);
}

.price-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 14px;
}

.price-foot {
  margin-top: 10px;
}

.empty-state {
  border-radius: 24px;
  padding: 48px 16px;
  text-align: center;
  background: rgba(255,255,255,.72);
}

@media (max-width: 1100px) {
  .model-page-grid {
    grid-template-columns: 1fr;
    height: auto;
    overflow: visible;
  }

  .channel-tabs-shell {
    max-height: 260px;
  }

  .content-shell {
    height: auto;
    overflow: visible;
  }

  .model-card-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .available-models-shell {
    border-radius: 18px;
    padding: 10px;
    overflow: visible;
  }

  .model-page-grid {
    gap: 12px;
  }

  .channel-tabs-shell {
    display: none;
  }

  .section-eyebrow {
    position: static;
    flex: 0 0 auto;
    margin: 0;
    padding: 8px 4px;
    writing-mode: vertical-rl;
    text-orientation: mixed;
    background: transparent;
    backdrop-filter: none;
  }

  .group-tab {
    flex: 0 0 210px;
    scroll-snap-align: start;
    gap: 8px;
    margin-bottom: 0;
    border-radius: 14px;
    padding: 10px;
  }

  .group-tab-top {
    align-items: flex-start;
    flex-direction: column;
    gap: 7px;
  }

  .group-name-pill {
    max-width: 100%;
    padding: 4px 8px;
    font-size: 14px;
  }

  .rate-badge,
  .mini-badge {
    padding: 3px 7px;
    font-size: 10px;
  }

  .group-desc {
    display: -webkit-box;
    overflow: hidden;
    font-size: 12px;
    line-height: 1.35;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }

  .group-tab-meta {
    gap: 5px;
  }

  .content-shell {
    gap: 12px;
    padding-right: 0;
  }

  .content-head {
    align-items: flex-start;
    flex-direction: column;
    gap: 12px;
    border-radius: 18px;
    padding: 16px;
  }

  .content-title-row {
    width: 100%;
    gap: 10px;
  }

  .content-title-copy {
    flex: 1 1 auto;
  }

  .content-heading-line {
    position: relative;
    align-items: flex-start;
    justify-content: space-between;
  }

  .mobile-group-filter {
    position: relative;
    display: inline-flex;
    flex: 0 0 auto;
    padding-top: 1px;
  }

  .content-icon {
    width: 42px;
    height: 42px;
    border-radius: 14px;
  }

  .content-head h2 {
    font-size: 20px;
  }

  .content-meta-line,
  .content-desc {
    font-size: 12px;
  }

  .content-tags,
  .content-side {
    gap: 6px;
  }

  .kpi {
    padding: 8px 10px;
  }

  .kpi .value {
    font-size: 18px;
  }

  .price-grid {
    grid-template-columns: 1fr;
  }

  .model-card-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .model-card {
    border-radius: 14px;
    padding: 12px 10px;
  }

  .model-header,
  .model-title-row {
    gap: 8px;
  }

  .model-avatar {
    width: 30px;
    height: 30px;
    border-radius: 10px;
    flex-basis: 30px;
  }

  .model-name {
    margin-bottom: 5px;
    font-size: 12px;
    line-height: 1.25;
  }

  .model-tags {
    gap: 4px;
  }

  .copy-btn {
    top: 10px;
    right: 10px;
    width: 26px;
    height: 26px;
    border-radius: 9px;
  }

  .model-title-row {
    padding-right: 28px;
  }

  .price-grid {
    gap: 7px;
    margin-top: 10px;
  }

  .price-box {
    padding: 8px;
  }

  .price-box .label {
    font-size: 10px;
    line-height: 1.35;
  }

  .price-lines {
    gap: 4px;
  }

  .price-line {
    gap: 4px;
    font-size: 10px;
  }

  .price-line:not(.actual) strong {
    font-size: 17px;
  }

  .price-line.actual strong {
    font-size: 16px;
  }

  .price-help::after {
    right: 0;
    left: auto;
    max-width: min(240px, calc(100vw - 32px));
    transform: translateX(0);
  }

  .price-help:hover::after,
  .price-help:focus-visible::after {
    transform: translateY(-2px);
  }
}

@media (max-width: 420px) {
  .model-card-grid {
    gap: 7px;
  }

  .model-card {
    padding: 10px 8px;
  }

  .model-avatar {
    width: 28px;
    height: 28px;
    flex-basis: 28px;
  }

  .model-name {
    font-size: 11px;
  }

  .price-line:not(.actual) strong {
    font-size: 16px;
  }
}
</style>
