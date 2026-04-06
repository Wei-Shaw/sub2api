<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div class="flex items-center justify-between">
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('payment.orders.title') }}</h1>
        <div class="flex gap-2">
          <button class="btn btn-secondary btn-sm" @click="fetchOrders()">
            <Icon name="refresh" size="sm" class="mr-1" />{{ t('common.refresh') }}
          </button>
          <button class="btn btn-primary btn-sm" @click="router.push('/purchase')">{{ t('payment.result.backToRecharge') }}</button>
        </div>
      </div>
      <div class="flex flex-wrap gap-2">
        <button v-for="filter in statusFilters" :key="filter.value"
          class="rounded-full px-4 py-1.5 text-sm font-medium transition-all"
          :class="currentFilter === filter.value ? 'bg-primary-500 text-white' : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600'"
          @click="setFilter(filter.value)">{{ filter.label }}</button>
      </div>
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <div v-else-if="filteredOrders.length === 0" class="py-16 text-center text-gray-500 dark:text-gray-400">
        {{ t('payment.orders.empty') }}
      </div>
      <div v-else class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-800 dark:text-dark-400">
              <tr>
                <th class="px-4 py-3">{{ t('payment.orders.orderId') }}</th>
                <th class="px-4 py-3">{{ t('payment.orders.amount') }}</th>
                <th class="px-4 py-3">{{ t('payment.orders.payAmount') }}</th>
                <th class="px-4 py-3">{{ t('payment.orders.paymentMethod') }}</th>
                <th class="px-4 py-3">{{ t('payment.orders.status') }}</th>
                <th class="px-4 py-3">{{ t('payment.orders.createdAt') }}</th>
                <th class="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="order in filteredOrders" :key="order.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/50">
                <td class="px-4 py-3 font-mono text-xs text-gray-900 dark:text-white">#{{ order.id }}</td>
                <td class="px-4 py-3 text-gray-900 dark:text-white">&#165;{{ order.amount.toFixed(2) }}</td>
                <td class="px-4 py-3 text-gray-900 dark:text-white">&#165;{{ order.pay_amount.toFixed(2) }}</td>
                <td class="px-4 py-3 text-gray-600 dark:text-dark-300">{{ t('payment.methods.' + order.payment_type, order.payment_type) }}</td>
                <td class="px-4 py-3"><OrderStatusBadge :status="order.status" /></td>
                <td class="px-4 py-3 text-xs text-gray-500 dark:text-dark-400">{{ formatDate(order.created_at) }}</td>
                <td class="px-4 py-3">
                  <div class="flex gap-2">
                    <button v-if="order.status === 'PENDING'" class="text-xs text-red-600 hover:text-red-800 dark:text-red-400" @click="handleCancel(order.id)">{{ t('payment.orders.cancel') }}</button>
                    <button v-if="order.status === 'COMPLETED'" class="text-xs text-purple-600 hover:text-purple-800 dark:text-purple-400" @click="openRefundDialog(order)">{{ t('payment.orders.requestRefund') }}</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <div v-if="totalPages > 1" class="flex items-center justify-center gap-2">
        <button class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="goToPage(page - 1)"><Icon name="chevronLeft" size="sm" /></button>
        <span class="text-sm text-gray-500 dark:text-dark-400">{{ page }} / {{ totalPages }}</span>
        <button class="btn btn-secondary btn-sm" :disabled="page >= totalPages" @click="goToPage(page + 1)"><Icon name="chevronRight" size="sm" /></button>
      </div>
    </div>
    <BaseDialog :show="!!cancelTargetId" :title="t('payment.orders.cancel')" width="narrow" @close="cancelTargetId = null">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('payment.confirmCancel') }}</p>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" @click="cancelTargetId = null">{{ t('common.cancel') }}</button>
          <button class="btn btn-danger" :disabled="actionLoading" @click="confirmCancel">{{ actionLoading ? t('common.processing') : t('payment.orders.cancel') }}</button>
        </div>
      </template>
    </BaseDialog>
    <BaseDialog :show="!!refundTarget" :title="t('payment.orders.requestRefund')" @close="refundTarget = null">
      <div v-if="refundTarget" class="space-y-4">
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
          <div class="flex justify-between text-sm">
            <span class="text-gray-500 dark:text-dark-400">{{ t('payment.orders.orderId') }}</span>
            <span class="font-mono text-gray-900 dark:text-white">#{{ refundTarget.id }}</span>
          </div>
          <div class="mt-2 flex justify-between text-sm">
            <span class="text-gray-500 dark:text-dark-400">{{ t('payment.orders.amount') }}</span>
            <span class="text-gray-900 dark:text-white">&#165;{{ refundTarget.amount.toFixed(2) }}</span>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('payment.refundReason') }}</label>
          <textarea v-model="refundReason" rows="3" class="input mt-1 w-full" :placeholder="t('payment.refundReasonPlaceholder')" />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" @click="refundTarget = null">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="actionLoading || !refundReason.trim()" @click="confirmRefund">{{ actionLoading ? t('common.processing') : t('payment.orders.requestRefund') }}</button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const loading = ref(false)
const actionLoading = ref(false)
const orders = ref<PaymentOrder[]>([])
const page = ref(1)
const totalPages = ref(1)
const pageSize = 20
const currentFilter = ref('ALL')
const cancelTargetId = ref<number | null>(null)
const refundTarget = ref<PaymentOrder | null>(null)
const refundReason = ref('')

const statusFilters = computed(() => [
  { value: 'ALL', label: t('common.all') },
  { value: 'PENDING', label: t('payment.status.pending') },
  { value: 'COMPLETED', label: t('payment.status.completed') },
  { value: 'FAILED', label: t('payment.status.failed') },
  { value: 'REFUND', label: t('payment.status.refunded') },
])

const filteredOrders = computed(() => {
  if (currentFilter.value === 'ALL') return orders.value
  if (currentFilter.value === 'FAILED') return orders.value.filter(o => ['FAILED', 'EXPIRED', 'CANCELLED'].includes(o.status))
  if (currentFilter.value === 'REFUND') return orders.value.filter(o => o.status.startsWith('REFUND') || o.status === 'PARTIALLY_REFUNDED')
  return orders.value.filter(o => o.status === currentFilter.value)
})

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }
function setFilter(value: string) { currentFilter.value = value }
function goToPage(p: number) { if (p >= 1 && p <= totalPages.value) { page.value = p; fetchOrders() } }

async function fetchOrders() {
  loading.value = true
  try {
    const res = await paymentAPI.getMyOrders({ page: page.value, page_size: pageSize })
    orders.value = res.data.items
    totalPages.value = res.data.pages
  } catch (err) { console.error('Failed to load orders:', err) }
  finally { loading.value = false }
}

function handleCancel(orderId: number) { cancelTargetId.value = orderId }

async function confirmCancel() {
  if (!cancelTargetId.value) return
  actionLoading.value = true
  try {
    await paymentAPI.cancelOrder(cancelTargetId.value)
    appStore.showSuccess(t('common.success'))
    cancelTargetId.value = null
    await fetchOrders()
  } catch (err: any) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { actionLoading.value = false }
}

function openRefundDialog(order: PaymentOrder) { refundTarget.value = order; refundReason.value = '' }

async function confirmRefund() {
  if (!refundTarget.value || !refundReason.value.trim()) return
  actionLoading.value = true
  try {
    await paymentAPI.requestRefund(refundTarget.value.id, { reason: refundReason.value.trim() })
    appStore.showSuccess(t('common.success'))
    refundTarget.value = null
    refundReason.value = ''
    await fetchOrders()
  } catch (err: any) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { actionLoading.value = false }
}

onMounted(() => fetchOrders())
</script>
