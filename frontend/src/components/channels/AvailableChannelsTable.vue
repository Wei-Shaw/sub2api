<template>
  <div class="card overflow-hidden">
    <!-- 表头 -->
    <div
      class="grid items-center border-b border-gray-100 bg-gray-50/50 px-4 py-3 text-xs font-medium uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-400"
      :style="gridStyle"
    >
      <div>{{ columns.name }}</div>
      <div>{{ columns.platform }}</div>
      <div>{{ columns.groups }}</div>
      <div>{{ columns.supportedModels }}</div>
    </div>

    <div v-if="loading" class="flex justify-center py-10">
      <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
    </div>

    <div v-else-if="rows.length === 0" class="flex flex-col items-center py-12">
      <Icon name="inbox" size="xl" class="mb-3 h-12 w-12 text-gray-400" />
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyLabel }}</p>
    </div>

    <!-- 渠道分组：每个渠道一个 section，内部按 platform 顺序铺开 -->
    <div
      v-else
      v-for="(channel, chIdx) in rows"
      :key="`${channel.name}-${chIdx}`"
      class="border-b border-gray-100 last:border-b-0 dark:border-dark-700"
    >
      <div
        v-for="(section, secIdx) in channel.platforms"
        :key="`${channel.name}-${section.platform}`"
        class="grid items-center px-4 py-3 transition-colors hover:bg-gray-50/40 dark:hover:bg-dark-800/40"
        :class="{ 'border-t border-gray-100/70 dark:border-dark-700/50': secIdx > 0 }"
        :style="gridStyle"
      >
        <!-- 渠道名：仅第一行渲染，后续行留空（视觉上的 rowspan） -->
        <div>
          <template v-if="secIdx === 0">
            <div class="font-medium text-gray-900 dark:text-white">{{ channel.name }}</div>
            <div
              v-if="channel.description"
              class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
            >
              {{ channel.description }}
            </div>
          </template>
        </div>

        <!-- 平台徽章 -->
        <div>
          <span
            :class="[
              'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase',
              platformBadgeClass(section.platform),
            ]"
          >
            <PlatformIcon :platform="section.platform as GroupPlatform" size="xs" />
            {{ section.platform }}
          </span>
        </div>

        <!-- 分组 -->
        <div class="flex flex-wrap gap-1">
          <span
            v-for="g in section.groups"
            :key="g.id"
            :class="[
              'inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium',
              platformBadgeLightClass(section.platform),
            ]"
          >
            <PlatformIcon :platform="section.platform as GroupPlatform" size="xs" />
            {{ g.name }}
          </span>
          <span v-if="section.groups.length === 0" class="text-xs text-gray-400">-</span>
        </div>

        <!-- 支持模型 -->
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
          <span v-if="section.supported_models.length === 0" class="text-xs text-gray-400">
            {{ noModelsLabel }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import SupportedModelChip from './SupportedModelChip.vue'
import type { UserAvailableChannel } from '@/api/channels'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass, platformBadgeLightClass } from '@/utils/platformColors'

/** 四列 grid 的 template-columns；与表头、每个 section 行共享。 */
const gridStyle = 'grid-template-columns: 220px 140px minmax(200px, 1fr) minmax(280px, 2fr); display: grid;'

defineProps<{
  columns: {
    name: string
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
}>()
</script>
