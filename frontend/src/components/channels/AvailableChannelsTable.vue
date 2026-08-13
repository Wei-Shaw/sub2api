<template>
  <!--
    .table-wrapper 是 TablePageLayout 滚动链的挂载点：外层 .table-scroll-container
    负责卡片外观并 overflow-hidden，本层接收 overflow-y-auto 才能在内容超高时滚动。
    AvailableChannelsTable.spec asserts the `<table>` follows this div with
    nothing between them, so keep prose out of that gap.

    This is the hand-rolled table the audit's `grep` for an opening table tag
    missed, because the tag wraps onto the next line. It keeps its own markup
    rather than moving to `DataTable`: `DataTable` renders one row per record
    from a flat column contract, and this surface is a nested grouping — one
    tbody per channel, with the channel name and description `rowspan`ed across
    its platform rows. Expressing that through a column contract would mean
    either flattening the grouping away or teaching `DataTable` about row
    spans, which is a far larger change than this visual pass. So it takes the
    tokenized `.table` class from style.css instead — which is the same chrome
    `DataTable` would have applied: 34px header on a sunken ground, a
    `line-strong` rule under it, 32px rows (44px on touch) separated by
    `line-subtle` hairlines, no zebra, and hover that moves the ground only.
  -->
  <div class="table-wrapper">
    <table
      data-testid="desktop-channels"
      class="table !hidden table-fixed lg:!table"
    >
      <thead>
        <tr>
          <th scope="col" class="w-[180px]">{{ columns.name }}</th>
          <th scope="col" class="w-[200px]">{{ columns.description }}</th>
          <th scope="col" class="w-[140px]">{{ columns.platform }}</th>
          <th scope="col">{{ columns.groups }}</th>
          <th scope="col">{{ columns.supportedModels }}</th>
        </tr>
      </thead>
      <tbody v-if="loading">
        <tr>
          <td colspan="5" class="py-10 text-center">
            <Icon name="refresh" size="lg" class="inline-block animate-spin text-ink-tertiary" />
          </td>
        </tr>
      </tbody>
      <tbody v-else-if="rows.length === 0">
        <tr>
          <td colspan="5" class="py-12 text-center">
            <Icon name="inbox" size="lg" class="mx-auto mb-3 text-ink-disabled" />
            <p class="text-xs text-ink-tertiary">{{ emptyLabel }}</p>
          </td>
        </tr>
      </tbody>
      <!-- 每个渠道一个 tbody：首行 td rowspan 渠道名，后续行只渲染其余三列。
           tbody 之间用 line-strong 表达"渠道边界"，tbody 内部由 .table 的
           line-subtle 行分隔线区分平台。 -->
      <tbody
        v-else
        v-for="(channel, chIdx) in rows"
        :key="`${channel.name}-${chIdx}`"
        class="border-b border-line-strong last:border-b-0"
      >
        <tr
          v-for="(section, secIdx) in channel.platforms"
          :key="`${channel.name}-${section.platform}`"
        >
          <!-- 渠道名：只在第一行渲染并用 rowspan 纵向合并 -->
          <td
            v-if="secIdx === 0"
            :rowspan="channel.platforms.length"
            class="py-2 align-top text-sm font-medium text-ink"
          >
            {{ channel.name }}
          </td>

          <!-- 描述：独立一列，同样用 rowspan 纵向合并 -->
          <td
            v-if="secIdx === 0"
            :rowspan="channel.platforms.length"
            class="py-2 align-top text-xs text-ink-tertiary"
          >
            <template v-if="channel.description">{{ channel.description }}</template>
            <span v-else class="text-ink-disabled">–</span>
          </td>

          <!-- 平台徽章：平台色是分类色（见 utils/platformColors），不是状态色。 -->
          <td class="py-2 align-top">
            <span
              :class="[
                'inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 text-2xs font-medium uppercase tracking-[0.04em]',
                platformBadgeClass(section.platform),
              ]"
            >
              <PlatformIcon :platform="section.platform as GroupPlatform" size="xs" />
              {{ section.platform }}
            </span>
          </td>

          <!-- 分组：专属分组在前，公开分组在后。区分靠图标+字重，不靠色相。 -->
          <td class="py-2 align-top">
            <div class="flex flex-col gap-1.5">
              <div
                v-if="exclusiveGroups(section).length > 0"
                class="flex flex-wrap items-center gap-1.5"
              >
                <span
                  class="inline-flex items-center gap-1 text-2xs font-medium uppercase tracking-[0.04em] text-ink"
                  :title="t('availableChannels.exclusiveTooltip')"
                >
                  <Icon name="shield" size="xs" />
                  {{ t('availableChannels.exclusive') }}
                </span>
                <div
                  v-for="g in exclusiveGroups(section)"
                  :key="`ex-${g.id}`"
                  class="inline-flex flex-wrap items-center gap-1"
                >
                  <GroupBadge
                    :name="g.name"
                    :platform="g.platform as GroupPlatform"
                    :subscription-type="(g.subscription_type || 'standard') as SubscriptionType"
                    :rate-multiplier="g.rate_multiplier"
                    :user-rate-multiplier="userGroupRates[g.id] ?? null"
                    always-show-rate
                  />
                  <Badge v-if="hasPeakRate(g)" mono :title="peakRateTitle(g)">
                    <Icon name="clock" size="xs" />
                    {{ peakRateLabel(g) }}
                  </Badge>
                </div>
              </div>
              <div
                v-if="publicGroups(section).length > 0"
                class="flex flex-wrap items-center gap-1.5"
              >
                <span
                  class="inline-flex items-center gap-1 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary"
                  :title="t('availableChannels.publicTooltip')"
                >
                  <Icon name="globe" size="xs" />
                  {{ t('availableChannels.public') }}
                </span>
                <div
                  v-for="g in publicGroups(section)"
                  :key="`pub-${g.id}`"
                  class="inline-flex flex-wrap items-center gap-1"
                >
                  <GroupBadge
                    :name="g.name"
                    :platform="g.platform as GroupPlatform"
                    :subscription-type="(g.subscription_type || 'standard') as SubscriptionType"
                    :rate-multiplier="g.rate_multiplier"
                    :user-rate-multiplier="userGroupRates[g.id] ?? null"
                    always-show-rate
                  />
                  <Badge v-if="hasPeakRate(g)" mono :title="peakRateTitle(g)">
                    <Icon name="clock" size="xs" />
                    {{ peakRateLabel(g) }}
                  </Badge>
                </div>
              </div>
              <span v-if="section.groups.length === 0" class="text-xs text-ink-disabled">–</span>
            </div>
          </td>

          <!-- 支持模型 -->
          <td class="py-2 align-top">
            <div class="flex flex-wrap gap-1">
              <SupportedModelChip
                v-for="m in section.supported_models"
                :key="`${section.platform}-${m.name}`"
                :model="m"
                :pricing-key-prefix="pricingKeyPrefix"
                :no-pricing-label="noPricingLabel"
                :show-platform="false"
                :platform-hint="section.platform"
              />
              <span v-if="section.supported_models.length === 0" class="text-xs text-ink-disabled">
                {{ noModelsLabel }}
              </span>
            </div>
          </td>
        </tr>
      </tbody>
    </table>

    <div data-testid="mobile-channels" class="w-full min-w-0 overflow-x-hidden lg:hidden">
      <div v-if="loading" data-testid="mobile-loading" class="py-10 text-center">
        <Icon name="refresh" size="lg" class="inline-block animate-spin text-ink-tertiary" />
      </div>
      <div v-else-if="rows.length === 0" data-testid="mobile-empty" class="py-12 text-center">
        <Icon name="inbox" size="lg" class="mx-auto mb-3 text-ink-disabled" />
        <p class="text-xs text-ink-tertiary">{{ emptyLabel }}</p>
      </div>
      <section
        v-else
        v-for="(channel, chIdx) in rows"
        :key="`mobile-${channel.name}-${chIdx}`"
        class="border-b border-line px-4 py-4 last:border-b-0"
      >
        <header class="mb-3 min-w-0">
          <h3 class="break-words text-sm font-semibold text-ink">
            {{ channel.name }}
          </h3>
          <p class="mt-1 break-words text-xs text-ink-tertiary">
            {{ channel.description || '–' }}
          </p>
        </header>

        <div class="divide-y divide-line-subtle">
          <div
            v-for="section in channel.platforms"
            :key="`mobile-${channel.name}-${section.platform}`"
            class="min-w-0 py-3 first:pt-0 last:pb-0"
          >
            <span
              :class="[
                'inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 text-2xs font-medium uppercase tracking-[0.04em]',
                platformBadgeClass(section.platform),
              ]"
            >
              <PlatformIcon :platform="section.platform as GroupPlatform" size="xs" />
              {{ section.platform }}
            </span>

            <dl class="mt-3 space-y-3">
              <div class="min-w-0">
                <dt class="mb-1.5 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ columns.groups }}
                </dt>
                <dd class="flex min-w-0 flex-col gap-2">
                  <div
                    v-if="exclusiveGroups(section).length > 0"
                    class="flex min-w-0 flex-wrap items-center gap-1.5"
                  >
                    <span
                      class="inline-flex items-center gap-1 text-2xs font-medium uppercase tracking-[0.04em] text-ink"
                      :title="t('availableChannels.exclusiveTooltip')"
                    >
                      <Icon name="shield" size="xs" />
                      {{ t('availableChannels.exclusive') }}
                    </span>
                    <div
                      v-for="g in exclusiveGroups(section)"
                      :key="`mobile-ex-${g.id}`"
                      class="inline-flex max-w-full min-w-0 flex-wrap items-center gap-1"
                    >
                      <GroupBadge
                        class="max-w-full"
                        :name="g.name"
                        :platform="g.platform as GroupPlatform"
                        :subscription-type="(g.subscription_type || 'standard') as SubscriptionType"
                        :rate-multiplier="g.rate_multiplier"
                        :user-rate-multiplier="userGroupRates[g.id] ?? null"
                        always-show-rate
                      />
                      <Badge v-if="hasPeakRate(g)" mono :title="peakRateTitle(g)">
                        <Icon name="clock" size="xs" />
                        {{ peakRateLabel(g) }}
                      </Badge>
                    </div>
                  </div>
                  <div
                    v-if="publicGroups(section).length > 0"
                    class="flex min-w-0 flex-wrap items-center gap-1.5"
                  >
                    <span
                      class="inline-flex items-center gap-1 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary"
                      :title="t('availableChannels.publicTooltip')"
                    >
                      <Icon name="globe" size="xs" />
                      {{ t('availableChannels.public') }}
                    </span>
                    <div
                      v-for="g in publicGroups(section)"
                      :key="`mobile-pub-${g.id}`"
                      class="inline-flex max-w-full min-w-0 flex-wrap items-center gap-1"
                    >
                      <GroupBadge
                        class="max-w-full"
                        :name="g.name"
                        :platform="g.platform as GroupPlatform"
                        :subscription-type="(g.subscription_type || 'standard') as SubscriptionType"
                        :rate-multiplier="g.rate_multiplier"
                        :user-rate-multiplier="userGroupRates[g.id] ?? null"
                        always-show-rate
                      />
                      <Badge v-if="hasPeakRate(g)" mono :title="peakRateTitle(g)">
                        <Icon name="clock" size="xs" />
                        {{ peakRateLabel(g) }}
                      </Badge>
                    </div>
                  </div>
                  <span v-if="section.groups.length === 0" class="text-xs text-ink-disabled">–</span>
                </dd>
              </div>

              <div class="min-w-0">
                <dt class="mb-1.5 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                  {{ columns.supportedModels }}
                </dt>
                <dd class="flex min-w-0 flex-wrap gap-1">
                  <SupportedModelChip
                    v-for="m in section.supported_models"
                    :key="`mobile-${section.platform}-${m.name}`"
                    class="max-w-full [&>span]:max-w-full [&>span]:truncate"
                    :model="m"
                    :pricing-key-prefix="pricingKeyPrefix"
                    :no-pricing-label="noPricingLabel"
                    :show-platform="false"
                    :platform-hint="section.platform"
                  />
                  <span v-if="section.supported_models.length === 0" class="text-xs text-ink-disabled">
                    {{ noModelsLabel }}
                  </span>
                </dd>
              </div>
            </dl>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Badge from '@/components/common/Badge.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import SupportedModelChip from './SupportedModelChip.vue'
import type { UserAvailableChannel, UserAvailableGroup, UserChannelPlatformSection } from '@/api/channels'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { platformBadgeClass } from '@/utils/platformColors'
import { useAppStore } from '@/stores/app'
import { hasPeakRate as groupHasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'

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
  /** 用户专属倍率（group_id → multiplier）；无专属时由 GroupBadge 仅显示默认倍率。 */
  userGroupRates: Record<number, number>
}>()

// Suppress unused warning — props is accessed via template automatically but
// the explicit reference here keeps the linter from flagging userGroupRates.
void props.userGroupRates

const { t } = useI18n()

function exclusiveGroups(section: UserChannelPlatformSection): UserAvailableGroup[] {
  return section.groups.filter((g) => g.is_exclusive)
}

function publicGroups(section: UserChannelPlatformSection): UserAvailableGroup[] {
  return section.groups.filter((g) => !g.is_exclusive)
}

const appStore = useAppStore()

function hasPeakRate(group: UserAvailableGroup): boolean {
  return groupHasPeakRate(group)
}

function peakRateLabel(group: UserAvailableGroup): string {
  return formatPeakRateWindow(group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
}

function peakRateTitle(group: UserAvailableGroup): string {
  return t('common.peakRateTooltip', { window: peakRateLabel(group) }) + t('common.peakRateImageNote')
}
</script>
