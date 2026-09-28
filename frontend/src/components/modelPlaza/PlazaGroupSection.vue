<template>
  <section class="card">
    <!-- 分组头部:名称/平台/倍率徽章/专属/订阅徽章 + 描述 -->
    <header class="card-header">
      <div class="flex flex-wrap items-center gap-2">
        <GroupBadge
          :name="group.name"
          :platform="group.platform as GroupPlatform"
          :subscription-type="(group.subscription_type || 'standard') as SubscriptionType"
          :rate-multiplier="group.rate_multiplier"
          :user-rate-multiplier="group.user_rate_multiplier ?? null"
          :peak-rate-enabled="group.peak_rate_enabled"
          :peak-start="group.peak_start"
          :peak-end="group.peak_end"
          :peak-rate-multiplier="group.peak_rate_multiplier"
          always-show-rate
        />
        <span v-if="group.is_exclusive" class="badge badge-gray">
          <Icon name="shield" size="xs" class="h-3 w-3" />
          {{ t('modelPlaza.badges.exclusive') }}
        </span>
        <span v-if="group.subscription_type === 'subscription'" class="badge badge-primary">
          {{ t('modelPlaza.badges.subscription') }}
        </span>
      </div>
      <p v-if="group.description" class="mt-2 max-w-[75ch] text-body text-fg-muted">
        {{ group.description }}
      </p>
      <p v-if="peakNote" class="mt-1.5 flex items-center gap-1 text-meta font-medium text-warning">
        <Icon name="clock" size="xs" class="h-3 w-3" />
        {{ peakNote }}
      </p>
      <p v-if="longContextNote" class="mt-1.5 flex items-center gap-1 text-meta text-fg-muted">
        <Icon name="infoCircle" size="xs" class="h-3 w-3" />
        {{ longContextNote }}
      </p>
    </header>

    <!-- 模型价格表:印刷网格,边缘由分节边框提供 -->
    <PlazaModelPricingTable
      v-if="group.models.length > 0"
      :models="group.models"
      :platform="group.platform"
      :rate-multiplier="group.rate_multiplier"
      :user-rate-multiplier="group.user_rate_multiplier ?? null"
      :image-rate-independent="group.image_rate_independent"
      :image-rate-multiplier="group.image_rate_multiplier"
      :video-rate-independent="group.video_rate_independent"
      :video-rate-multiplier="group.video_rate_multiplier"
      :peak-window="peakWindow"
      :peak-rate-multiplier="group.peak_rate_multiplier"
    />
    <p v-else class="px-5 py-4 text-center text-body text-fg-subtle">
      {{ t('modelPlaza.detail.noModels') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlazaModelPricingTable from './PlazaModelPricingTable.vue'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
import type { GroupPlatform, SubscriptionType } from '@/types'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  group: ModelPlazaGroup
}>()

const { t } = useI18n()
const appStore = useAppStore()

/** 高峰窗口描述(含倍率与服务器时区标注);分组未启用高峰为空串。 */
const peakWindow = computed(() => {
  if (!hasPeakRate(props.group)) return ''
  return formatPeakRateWindow(
    props.group,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
})

const peakNote = computed(() => {
  if (!peakWindow.value) return ''
  return t('modelPlaza.detail.peakNote', {
    window: peakWindow.value,
    multiplier: props.group.peak_rate_multiplier
  })
})

/**
 * 分组关闭了长上下文阶梯、但组内有模型官方带阶梯时提示:实付列只展示基础档,
 * 官方阶梯仅供参考。字段缺失(旧后端)不提示。
 */
const longContextNote = computed(() => {
  if (props.group.long_context_pricing_enabled !== false) return ''
  const hasOfficialLadder = props.group.models.some(
    (m) => (m.official_pricing?.intervals?.length ?? 0) > 1
  )
  return hasOfficialLadder ? t('modelPlaza.detail.longContextDisabledNote') : ''
})
</script>
