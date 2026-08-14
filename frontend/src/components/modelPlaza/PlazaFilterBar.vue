<template>
  <!--
    Faceted filter. Every control is one flat 28px hairline chip; the selected
    one is an accent tint with an accent hairline, which is the only thing the
    accent says anywhere on this page — "this is what you picked".

    What used to be here: chips filled with a per-platform hue derived through
    `color-mix`, an active state that was a gradient-filled pill with a colored
    shadow, and `grayscale` on the disabled ones. Fifteen hues competing to be
    the selection signal means there is no selection signal.
  -->
  <div class="space-y-2" data-testid="plaza-filter-bar">
    <!-- 一级:平台 -->
    <div class="flex flex-wrap items-start gap-x-3 gap-y-1.5">
      <span :class="ROW_LABEL">{{ t('modelPlaza.filters.platformLabel') }}</span>
      <div class="flex min-w-0 flex-wrap items-center gap-1.5" data-testid="plaza-filter-platform">
        <button
          v-for="p in ['all', ...platforms]"
          :key="`platform-${p}`"
          type="button"
          :aria-pressed="platform === p"
          :class="[CHIP, platform === p ? CHIP_ON : CHIP_OFF]"
          :disabled="p !== 'all' && !platformEnabled(p)"
          @click="$emit('update:platform', p)"
        >
          <PlatformIcon v-if="p !== 'all'" :platform="p as GroupPlatform" size="xs" />
          {{ p === 'all' ? t('modelPlaza.filters.all') : p }}
        </button>
      </div>
    </div>

    <!-- 二级:分组(当前组合下无结果的置灰) -->
    <div class="flex flex-wrap items-start gap-x-3 gap-y-1.5">
      <span :class="ROW_LABEL">{{ t('modelPlaza.filters.groupLabel') }}</span>
      <div class="flex min-w-0 flex-wrap items-center gap-1.5" data-testid="plaza-filter-group">
        <button
          type="button"
          :aria-pressed="groupId === 'all'"
          :class="[CHIP, groupId === 'all' ? CHIP_ON : CHIP_OFF]"
          @click="$emit('update:groupId', 'all')"
        >
          {{ t('modelPlaza.filters.all') }}
        </button>
        <button
          v-for="g in groups"
          :key="`group-${g.id}`"
          type="button"
          :aria-pressed="groupId === g.id"
          :class="[CHIP, groupId === g.id ? CHIP_ON : CHIP_OFF]"
          :disabled="!groupEnabled(g)"
          @click="$emit('update:groupId', g.id)"
        >
          {{ g.name }}
        </button>
      </div>
    </div>

    <!-- 三级:倍率(当前组合下不存在的置灰) -->
    <div class="flex flex-wrap items-start gap-x-3 gap-y-1.5">
      <span :class="ROW_LABEL">{{ t('modelPlaza.filters.rateLabel') }}</span>
      <div class="flex min-w-0 flex-wrap items-center gap-1.5" data-testid="plaza-filter-rate">
        <button
          type="button"
          :aria-pressed="rate === 'all'"
          :class="[CHIP, rate === 'all' ? CHIP_ON : CHIP_OFF]"
          @click="$emit('update:rate', 'all')"
        >
          {{ t('modelPlaza.filters.all') }}
        </button>
        <button
          v-for="r in rates"
          :key="`rate-${r}`"
          type="button"
          :aria-pressed="rate === r"
          :class="[CHIP, 'font-mono tabular-nums', rate === r ? CHIP_ON : CHIP_OFF]"
          :disabled="!rateEnabled(r)"
          @click="$emit('update:rate', r)"
        >
          {{ r }}x
        </button>
      </div>
    </div>

    <!-- 四级:模型名搜索(纯前端过滤) -->
    <div class="flex flex-wrap items-start gap-x-3 gap-y-1.5">
      <span :class="ROW_LABEL">{{ t('modelPlaza.filters.modelLabel') }}</span>
      <div class="relative w-full sm:w-64">
        <Icon
          name="search"
          size="xs"
          class="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-ink-tertiary"
        />
        <input
          :value="search"
          type="text"
          :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          class="h-7 w-full rounded border border-line bg-surface pl-7 pr-7 text-xs text-ink transition-colors duration-fast placeholder:text-ink-disabled hover:border-line-strong focus:border-accent"
          data-testid="plaza-filter-search"
          @input="$emit('update:search', ($event.target as HTMLInputElement).value)"
        />
        <button
          v-if="search"
          type="button"
          :aria-label="t('common.clear')"
          class="absolute right-1 top-1/2 flex h-5 w-5 -translate-y-1/2 items-center justify-center rounded text-ink-tertiary transition-colors duration-fast hover:text-ink"
          data-testid="plaza-filter-search-clear"
          @click="$emit('update:search', '')"
        >
          <Icon name="x" size="xs" class="h-3 w-3" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'

const props = defineProps<{
  /** 数据中出现的平台(去重排序后)。 */
  platforms: string[]
  /** 全量分组(含平台与生效倍率),三个维度的置灰联动由此推导。 */
  groups: Array<{ id: number; name: string; platform: string; rate: number }>
  /** 全量生效倍率去重升序。 */
  rates: number[]
  platform: string
  groupId: number | 'all'
  rate: number | 'all'
  /** 模型名搜索词(纯前端过滤)。 */
  search: string
}>()

defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number | 'all']
  'update:rate': [value: number | 'all']
  'update:search': [value: string]
}>()

const { t } = useI18n()

/** The dimension label. Fixed width so the four control rows share a margin. */
const ROW_LABEL =
  'w-16 shrink-0 pt-1.5 text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary'

/** 28px, the middle step of the 24/28/32 control scale. */
const CHIP =
  'inline-flex h-7 items-center gap-1.5 whitespace-nowrap rounded border px-2.5 text-xs font-medium transition-colors duration-fast disabled:cursor-not-allowed disabled:opacity-40'
const CHIP_ON = 'border-accent bg-accent-tint text-accent'
const CHIP_OFF =
  'border-line bg-surface text-ink-secondary enabled:hover:bg-surface-hover enabled:hover:text-ink'

/**
 * 三个维度互为约束(faceted):某选项可点 ⟺ 在「其他两维」当前选择下仍有分组命中。
 * 「全部」永远可点,作为解除本维约束的出口;可点项组合恒有结果,无需选择修正。
 */
function platformEnabled(p: string): boolean {
  return props.groups.some(
    (g) =>
      g.platform === p &&
      (props.groupId === 'all' || g.id === props.groupId) &&
      (props.rate === 'all' || g.rate === props.rate)
  )
}

function groupEnabled(g: { platform: string; rate: number }): boolean {
  return (
    (props.platform === 'all' || g.platform === props.platform) &&
    (props.rate === 'all' || g.rate === props.rate)
  )
}

function rateEnabled(r: number): boolean {
  return props.groups.some(
    (g) =>
      g.rate === r &&
      (props.platform === 'all' || g.platform === props.platform) &&
      (props.groupId === 'all' || g.id === props.groupId)
  )
}
</script>
