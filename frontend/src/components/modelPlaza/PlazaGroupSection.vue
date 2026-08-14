<template>
  <!--
    One group, one panel: a hairline box with a header rule and the price table
    flush against the edges. The border used to be tinted with the platform hue
    (`platformBorderStrongClass`), which made a page of six groups read as six
    differently-colored objects rather than one list.
  -->
  <Surface flush data-testid="plaza-group">
    <template #header>
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <PlatformIcon
            v-if="group.platform"
            :platform="group.platform as GroupPlatform"
            size="sm"
            class="shrink-0 text-ink-secondary"
          />
          <h2 class="min-w-0 truncate text-sm font-semibold text-ink">{{ group.name }}</h2>
          <!-- Exclusive / subscription are categories, not states: neutral tint. -->
          <Badge v-if="group.is_exclusive" caps>
            <Icon name="shield" size="xs" class="h-3 w-3" />
            {{ t('modelPlaza.badges.exclusive') }}
          </Badge>
          <Badge v-if="group.subscription_type === 'subscription'" caps>
            {{ t('modelPlaza.badges.subscription') }}
          </Badge>
        </div>
        <p v-if="group.description" class="mt-1.5 text-xs text-ink-secondary">
          {{ group.description }}
        </p>
        <!-- Peak-hour surcharge is a real caution, and one of the very few -->
        <!-- places on this page allowed to spend a semantic colour. -->
        <p v-if="peakNote" class="mt-1.5 inline-flex items-center gap-1 text-2xs text-warn">
          <Icon name="clock" size="xs" class="h-3 w-3 shrink-0" />
          {{ peakNote }}
        </p>
      </div>
    </template>

    <template #actions>
      <span class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
        {{ t('modelPlaza.filters.rateLabel') }}
      </span>
      <span
        v-if="hasCustomRate"
        class="font-mono text-2xs text-ink-tertiary line-through"
        data-testid="plaza-group-rate-original"
        >{{ group.rate_multiplier }}x</span
      >
      <NumCell
        :value="effectiveRate"
        unit="x"
        class="text-sm font-medium"
        data-testid="plaza-group-rate"
      />
    </template>

    <PlazaModelPricingTable
      v-if="group.models.length > 0"
      :models="group.models"
      :platform="group.platform"
      :rate-multiplier="group.rate_multiplier"
      :user-rate-multiplier="group.user_rate_multiplier ?? null"
      :image-rate-independent="group.image_rate_independent"
      :image-rate-multiplier="group.image_rate_multiplier"
    />
    <p v-else class="px-4 py-8 text-center text-xs text-ink-tertiary">
      {{ t('modelPlaza.detail.noModels') }}
    </p>
  </Surface>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Badge from '@/components/common/Badge.vue'
import NumCell from '@/components/common/NumCell.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Surface from '@/components/common/Surface.vue'
import PlazaModelPricingTable from './PlazaModelPricingTable.vue'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
import type { GroupPlatform } from '@/types'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  group: ModelPlazaGroup
}>()

const { t } = useI18n()
const appStore = useAppStore()

/** 生效倍率 = 用户专属倍率 ?? 分组默认倍率。 */
const effectiveRate = computed(
  () => props.group.user_rate_multiplier ?? props.group.rate_multiplier
)

const hasCustomRate = computed(
  () =>
    props.group.user_rate_multiplier != null &&
    props.group.user_rate_multiplier !== props.group.rate_multiplier
)

const peakNote = computed(() => {
  if (!hasPeakRate(props.group)) return ''
  const window = formatPeakRateWindow(
    props.group,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
  return t('modelPlaza.detail.peakNote', {
    window,
    multiplier: props.group.peak_rate_multiplier
  })
})
</script>
