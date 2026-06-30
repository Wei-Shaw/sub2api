<template>
  <span
    class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
    :class="statusClass"
  >
    {{ statusLabel REDACTEDREDACTED
  </span>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { OrderStatus REDACTED from '@/types/payment'

const props = defineProps<{
  status: OrderStatus
REDACTED>()

const { t REDACTED = useI18n()

const statusMap: Record<OrderStatus, { key: string; class: string REDACTED> = {
  PENDING: { key: 'payment.status.pending', class: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400' REDACTED,
  PAID: { key: 'payment.status.paid', class: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400' REDACTED,
  RECHARGING: { key: 'payment.status.recharging', class: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400' REDACTED,
  COMPLETED: { key: 'payment.status.completed', class: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' REDACTED,
  EXPIRED: { key: 'payment.status.expired', class: 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400' REDACTED,
  CANCELLED: { key: 'payment.status.cancelled', class: 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400' REDACTED,
  FAILED: { key: 'payment.status.failed', class: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' REDACTED,
  REFUND_REQUESTED: { key: 'payment.status.refund_requested', class: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400' REDACTED,
  REFUNDING: { key: 'payment.status.refunding', class: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400' REDACTED,
  REFUND_PENDING: { key: 'payment.status.refund_pending', class: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400' REDACTED,
  REFUNDED: { key: 'payment.status.refunded', class: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400' REDACTED,
  PARTIALLY_REFUNDED: { key: 'payment.status.partially_refunded', class: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400' REDACTED,
  REFUND_FAILED: { key: 'payment.status.refund_failed', class: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' REDACTED,
REDACTED

const statusLabel = computed(() => {
  const entry = statusMap[props.status]
  return entry ? t(entry.key) : props.status
REDACTED)

const statusClass = computed(() => {
  const entry = statusMap[props.status]
  return entry?.class ?? 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400'
REDACTED)
</script>
