<template>
  <!--
    A dot plus a written label, not a coloured pill.

    Two things changed and both are load-bearing. The pill was `rounded-full`
    with a tinted ground and no border — the shape read as a dismissible tag
    rather than as the state of the row, and `rounded-full` on text was the most
    repeated slop signal in the old tree. And the twelve statuses were spread
    across six hues (yellow / blue / green / gray / red / purple), so the colour
    was doing all the work: in grayscale, in a screenshot pasted into a ticket,
    or for a reader with a colour vision deficiency, "refunded" and "paid" were
    the same object.

    `StatusDot` makes the text label a required prop, so the redundant channel
    cannot be dropped later. The tone table lives in `orderUtils.ts` next to the
    refundability rules, because "which statuses are terminal" is one fact and
    it should not be stated twice.
  -->
  <StatusDot :tone="tone" :label="statusLabel" :muted="tone === 'neutral'" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import StatusDot from '@/components/common/StatusDot.vue'
import { orderStatusI18nKey, orderStatusTone } from '@/components/payment/orderUtils'
import type { OrderStatus } from '@/types/payment'

const props = defineProps<{
  status: OrderStatus
}>()

const { t } = useI18n()

const tone = computed(() => orderStatusTone(props.status))

/** An unmapped status prints its raw value — more diagnostic than "unknown". */
const statusLabel = computed(() => {
  const key = orderStatusI18nKey(props.status)
  return key ? t(key) : String(props.status ?? '')
})
</script>
