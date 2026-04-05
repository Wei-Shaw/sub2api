<template>
  <AppLayout>
    <div class="mx-auto flex max-w-md flex-col items-center space-y-6 py-16">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <template v-else>
        <div v-if="isSuccess" class="text-center">
          <div class="mx-auto flex h-20 w-20 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
            <svg class="h-10 w-10 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <h2 class="mt-4 text-2xl font-bold text-gray-900 dark:text-white">{{ t('payment.result.success') }}</h2>
        </div>
        <div v-else class="text-center">
          <div class="mx-auto flex h-20 w-20 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
            <svg class="h-10 w-10 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
          <h2 class="mt-4 text-2xl font-bold text-gray-900 dark:text-white">{{ t('payment.result.failed') }}</h2>
        </div>
        <div v-if="order" class="w-full rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
          <div class="space-y-2 text-sm">
            <div class="flex justify-between">
              <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</span>
              <span class="font-medium text-gray-900 dark:text-white">#{{ order.id }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</span>
              <span class="font-medium text-gray-900 dark:text-white">&#165;{{ order.pay_amount.toFixed(2) }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.paymentMethod') }}</span>
              <span class="font-medium text-gray-900 dark:text-white">{{ t('payment.methods.' + order.payment_type, order.payment_type) }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.status') }}</span>
              <OrderStatusBadge :status="order.status" />
            </div>
          </div>
        </div>
        <div class="flex gap-3 pt-4">
          <button class="btn btn-secondary" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
          <button class="btn btn-primary" @click="router.push('/orders')">{{ t('payment.result.viewOrders') }}</button>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { usePaymentStore } from '@/stores/payment'
import type { PaymentOrder } from '@/types/payment'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const paymentStore = usePaymentStore()

const order = ref<PaymentOrder | null>(null)
const loading = ref(true)

const isSuccess = computed(() => {
  if (route.query.status === 'success') return true
  if (!order.value) return false
  return order.value.status === 'COMPLETED' || order.value.status === 'PAID'
})

onMounted(async () => {
  const orderId = Number(route.query.order_id)
  if (orderId) {
    order.value = await paymentStore.pollOrderStatus(orderId)
  }
  loading.value = false
})
</script>
