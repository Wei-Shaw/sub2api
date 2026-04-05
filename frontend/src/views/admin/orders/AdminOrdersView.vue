<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Tab Navigation -->
      <div class="sticky top-0 z-10 overflow-x-auto settings-tabs-scroll">
        <nav class="settings-tabs">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            :class="['settings-tab', activeTab === tab.key && 'settings-tab-active']"
            @click="activeTab = tab.key"
          >
            <span class="settings-tab-icon">
              <Icon :name="tab.icon" size="sm" />
            </span>
            <span>{{ tab.label }}</span>
          </button>
        </nav>
      </div>

      <!-- Tab: Overview -->
      <div v-show="activeTab === 'overview'">
        <div class="space-y-6">
          <div v-if="dashboardLoading" class="flex items-center justify-center py-12">
            <LoadingSpinner />
          </div>
          <template v-else-if="dashboardStats">
            <OrderStatsCards :stats="dashboardStats" />
            <DailyRevenueChart :data="dashboardStats.daily_series || []" :loading="dashboardLoading" />
            <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <div class="card p-4">
                <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.paymentDistribution') }}</h3>
                <div v-if="!dashboardStats.payment_methods?.length" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noData') }}</div>
                <div v-else class="space-y-3">
                  <div v-for="method in dashboardStats.payment_methods" :key="method.type" class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <span :class="['inline-block h-3 w-3 rounded-full', methodColor(method.type)]"></span>
                      <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + method.type, method.type) }}</span>
                    </div>
                    <div class="text-right">
                      <span class="text-sm font-medium text-gray-900 dark:text-white">${{ method.amount.toFixed(2) }}</span>
                      <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">({{ method.count }})</span>
                    </div>
                  </div>
                </div>
              </div>
              <div class="card p-4">
                <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.topUsers') }}</h3>
                <div v-if="!dashboardStats.top_users?.length" class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.noData') }}</div>
                <div v-else class="space-y-2">
                  <div v-for="(user, idx) in dashboardStats.top_users" :key="user.user_id" class="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-gray-50 dark:hover:bg-dark-700">
                    <div class="flex items-center gap-3">
                      <span :class="['flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold', idx === 0 ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400' : idx === 1 ? 'bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-300' : idx === 2 ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400']">{{ idx + 1 }}</span>
                      <span class="text-sm text-gray-700 dark:text-gray-300">{{ user.email }}</span>
                    </div>
                    <span class="text-sm font-medium text-gray-900 dark:text-white">${{ user.amount.toFixed(2) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- Tab: Order List -->
      <div v-show="activeTab === 'orders'">
        <div class="space-y-4">
          <div class="card p-4">
            <div class="flex flex-wrap items-center gap-3">
              <div class="flex-1 sm:max-w-64">
                <input v-model="orderSearch" type="text" :placeholder="t('payment.admin.searchOrders')" class="input" @input="debounceLoadOrders" />
              </div>
              <Select v-model="orderFilters.status" :options="statusFilterOptions" class="w-36" @change="loadOrders" />
              <Select v-model="orderFilters.payment_type" :options="paymentTypeFilterOptions" class="w-40" @change="loadOrders" />
              <Select v-model="orderFilters.order_type" :options="orderTypeFilterOptions" class="w-36" @change="loadOrders" />
              <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
                <button @click="loadOrders" :disabled="ordersLoading" class="btn btn-secondary" :title="t('common.refresh')">
                  <Icon name="refresh" size="md" :class="ordersLoading ? 'animate-spin' : ''" />
                </button>
              </div>
            </div>
          </div>
          <DataTable :columns="orderColumns" :data="orders" :loading="ordersLoading">
            <template #cell-id="{ value }"><span class="font-mono text-sm">#{{ value }}</span></template>
            <template #cell-user_id="{ value }"><span class="text-sm text-gray-600 dark:text-gray-400">#{{ value }}</span></template>
            <template #cell-amount="{ value, row }">
              <div class="text-sm">
                <span class="font-medium text-gray-900 dark:text-white">${{ value.toFixed(2) }}</span>
                <span v-if="row.pay_amount !== value" class="ml-1 text-xs text-gray-500">({{ t('payment.orders.payAmount') }}: ${{ row.pay_amount.toFixed(2) }})</span>
              </div>
            </template>
            <template #cell-status="{ value }">
              <span :class="['badge', statusBadgeClass(value)]">{{ t('payment.status.' + value.toLowerCase(), value) }}</span>
            </template>
            <template #cell-payment_type="{ value }">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + value, value) }}</span>
            </template>
            <template #cell-created_at="{ value }">
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="flex items-center gap-1">
                <button @click="showOrderDetail(row)" class="btn-icon text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" :title="t('common.view')"><Icon name="eye" size="sm" /></button>
                <button v-if="row.status === 'PENDING'" @click="handleCancelOrder(row)" class="btn-icon text-yellow-500 hover:text-yellow-700" :title="t('payment.orders.cancel')"><Icon name="x" size="sm" /></button>
                <button v-if="row.status === 'FAILED'" @click="handleRetryOrder(row)" class="btn-icon text-blue-500 hover:text-blue-700" :title="t('payment.admin.retry')"><Icon name="refresh" size="sm" /></button>
                <button v-if="canRefund(row)" @click="openRefundDialog(row)" class="btn-icon text-red-500 hover:text-red-700" :title="t('payment.admin.refund')"><Icon name="dollar" size="sm" /></button>
              </div>
            </template>
          </DataTable>
          <Pagination v-if="orderPagination.total > 0" :page="orderPagination.page" :total="orderPagination.total" :page-size="orderPagination.page_size" @update:page="handleOrderPageChange" @update:pageSize="handleOrderPageSizeChange" />
        </div>
      </div>

      <!-- Tab: Channels -->
      <div v-show="activeTab === 'channels'">
        <div class="space-y-4">
          <div class="flex items-center justify-end gap-2">
            <button @click="loadChannels" :disabled="channelsLoading" class="btn btn-secondary" :title="t('common.refresh')"><Icon name="refresh" size="md" :class="channelsLoading ? 'animate-spin' : ''" /></button>
            <button @click="openChannelEdit(null)" class="btn btn-primary">{{ t('payment.admin.createChannel') }}</button>
          </div>
          <DataTable :columns="channelColumns" :data="channels" :loading="channelsLoading">
            <template #cell-enabled="{ value }"><span :class="['badge', value ? 'badge-success' : 'badge-secondary']">{{ value ? t('common.enabled') : t('common.disabled') }}</span></template>
            <template #cell-rate_multiplier="{ value }"><span class="font-mono text-sm">{{ value }}x</span></template>
            <template #cell-actions="{ row }">
              <div class="flex items-center gap-1">
                <button @click="openChannelEdit(row)" class="btn-icon text-blue-500 hover:text-blue-700" :title="t('common.edit')"><Icon name="edit" size="sm" /></button>
                <button @click="confirmDeleteChannel(row)" class="btn-icon text-red-500 hover:text-red-700" :title="t('common.delete')"><Icon name="trash" size="sm" /></button>
              </div>
            </template>
          </DataTable>
        </div>
      </div>

      <!-- Tab: Plans -->
      <div v-show="activeTab === 'plans'">
        <div class="space-y-4">
          <div class="flex items-center justify-end gap-2">
            <button @click="loadPlans" :disabled="plansLoading" class="btn btn-secondary" :title="t('common.refresh')"><Icon name="refresh" size="md" :class="plansLoading ? 'animate-spin' : ''" /></button>
            <button @click="openPlanEdit(null)" class="btn btn-primary">{{ t('payment.admin.createPlan') }}</button>
          </div>
          <DataTable :columns="planColumns" :data="plans" :loading="plansLoading">
            <template #cell-price="{ value, row }">
              <div class="text-sm"><span class="font-medium text-gray-900 dark:text-white">${{ value.toFixed(2) }}</span><span v-if="row.original_price" class="ml-1 text-xs text-gray-400 line-through">${{ row.original_price.toFixed(2) }}</span></div>
            </template>
            <template #cell-validity_days="{ value, row }"><span class="text-sm">{{ value }} {{ row.validity_unit || 'days' }}</span></template>
            <template #cell-for_sale="{ value }"><span :class="['badge', value ? 'badge-success' : 'badge-secondary']">{{ value ? t('payment.admin.onSale') : t('payment.admin.offSale') }}</span></template>
            <template #cell-actions="{ row }">
              <div class="flex items-center gap-1">
                <button @click="openPlanEdit(row)" class="btn-icon text-blue-500 hover:text-blue-700" :title="t('common.edit')"><Icon name="edit" size="sm" /></button>
                <button @click="confirmDeletePlan(row)" class="btn-icon text-red-500 hover:text-red-700" :title="t('common.delete')"><Icon name="trash" size="sm" /></button>
              </div>
            </template>
          </DataTable>
        </div>
      </div>
    </div>

    <!-- Order Detail Dialog -->
    <BaseDialog :show="showDetailDialog" :title="t('payment.admin.orderDetail')" width="wide" @close="showDetailDialog = false">
      <div v-if="selectedOrder" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</p><p class="font-mono text-sm font-medium text-gray-900 dark:text-white">#{{ selectedOrder.id }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.status') }}</p><span :class="['badge', statusBadgeClass(selectedOrder.status)]">{{ t('payment.status.' + selectedOrder.status.toLowerCase(), selectedOrder.status) }}</span></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">${{ selectedOrder.amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">${{ selectedOrder.pay_amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.paymentMethod') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + selectedOrder.payment_type, selectedOrder.payment_type) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.feeRate') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ (selectedOrder.fee_rate * 100).toFixed(1) }}%</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.createdAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.created_at) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.expiresAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.expires_at) }}</p></div>
          <div v-if="selectedOrder.paid_at"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.paidAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.paid_at) }}</p></div>
          <div v-if="selectedOrder.refund_amount"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundAmount') }}</p><p class="text-sm font-medium text-red-600 dark:text-red-400">${{ selectedOrder.refund_amount.toFixed(2) }}</p></div>
          <div v-if="selectedOrder.refund_reason" class="col-span-2"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundReason') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.refund_reason }}</p></div>
        </div>
      </div>
    </BaseDialog>

    <!-- Channel Edit Dialog -->
    <BaseDialog :show="showChannelDialog" :title="editingChannel ? t('payment.admin.editChannel') : t('payment.admin.createChannel')" width="normal" @close="showChannelDialog = false">
      <form id="channel-form" @submit.prevent="handleSaveChannel" class="space-y-4">
        <div><label class="input-label">{{ t('payment.admin.channelName') }}</label><input v-model="channelForm.name" type="text" class="input" required /></div>
        <div class="grid grid-cols-2 gap-4">
          <div><label class="input-label">{{ t('payment.admin.groupId') }}</label><input v-model.number="channelForm.group_id" type="number" class="input" /></div>
          <div><label class="input-label">{{ t('payment.admin.rateMultiplier') }}</label><input v-model.number="channelForm.rate_multiplier" type="number" step="0.01" min="0" class="input" required /></div>
        </div>
        <div><label class="input-label">{{ t('payment.admin.channelDescription') }}</label><textarea v-model="channelForm.description" rows="2" class="input"></textarea></div>
        <div class="flex items-center gap-2"><input id="channel-enabled" v-model="channelForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" /><label for="channel-enabled" class="text-sm text-gray-700 dark:text-gray-300">{{ t('common.enabled') }}</label></div>
      </form>
      <template #footer><div class="flex justify-end gap-3"><button type="button" @click="showChannelDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button><button type="submit" form="channel-form" :disabled="channelSaving" class="btn btn-primary">{{ channelSaving ? t('common.saving') : t('common.save') }}</button></div></template>
    </BaseDialog>

    <!-- Plan Edit Dialog -->
    <BaseDialog :show="showPlanDialog" :title="editingPlan ? t('payment.admin.editPlan') : t('payment.admin.createPlan')" width="wide" @close="showPlanDialog = false">
      <form id="plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div><label class="input-label">{{ t('payment.admin.planName') }}</label><input v-model="planForm.name" type="text" class="input" required /></div>
          <div><label class="input-label">{{ t('payment.admin.groupId') }}</label><input v-model.number="planForm.group_id" type="number" class="input" required /></div>
        </div>
        <div><label class="input-label">{{ t('payment.admin.planDescription') }}</label><textarea v-model="planForm.description" rows="2" class="input"></textarea></div>
        <div class="grid grid-cols-3 gap-4">
          <div><label class="input-label">{{ t('payment.admin.price') }}</label><input v-model.number="planForm.price" type="number" step="0.01" min="0" class="input" required /></div>
          <div><label class="input-label">{{ t('payment.admin.originalPrice') }}</label><input v-model.number="planForm.original_price" type="number" step="0.01" min="0" class="input" /></div>
          <div><label class="input-label">{{ t('payment.admin.sortOrder') }}</label><input v-model.number="planForm.sort_order" type="number" min="0" class="input" /></div>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div><label class="input-label">{{ t('payment.admin.validityDays') }}</label><input v-model.number="planForm.validity_days" type="number" min="1" class="input" required /></div>
          <div><label class="input-label">{{ t('payment.admin.validityUnit') }}</label><Select v-model="planForm.validity_unit" :options="validityUnitOptions" /></div>
        </div>
        <div><label class="input-label">{{ t('payment.admin.features') }}</label><textarea v-model="planFeaturesText" rows="3" class="input" :placeholder="t('payment.admin.featuresPlaceholder')"></textarea><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.featuresHint') }}</p></div>
        <div class="flex items-center gap-2"><input id="plan-for-sale" v-model="planForm.for_sale" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" /><label for="plan-for-sale" class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.forSale') }}</label></div>
      </form>
      <template #footer><div class="flex justify-end gap-3"><button type="button" @click="showPlanDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button><button type="submit" form="plan-form" :disabled="planSaving" class="btn btn-primary">{{ planSaving ? t('common.saving') : t('common.save') }}</button></div></template>
    </BaseDialog>

    <AdminRefundDialog :show="showRefundDialog" :order="selectedOrder" :submitting="refundSubmitting" @confirm="handleRefund" @cancel="showRefundDialog = false" />
    <ConfirmDialog :show="showDeleteChannelDialog" :title="t('payment.admin.deleteChannel')" :message="t('payment.admin.deleteChannelConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeleteChannel" @cancel="showDeleteChannelDialog = false" />
    <ConfirmDialog :show="showDeletePlanDialog" :title="t('payment.admin.deletePlan')" :message="t('payment.admin.deletePlanConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeletePlan" @cancel="showDeletePlanDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { DashboardStats, PaymentOrder, PaymentChannel, SubscriptionPlan } from '@/types/payment'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatsCards from '@/components/admin/payment/OrderStatsCards.vue'
import DailyRevenueChart from '@/components/admin/payment/DailyRevenueChart.vue'
import AdminRefundDialog from '@/components/admin/payment/AdminRefundDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()

type OrderTab = 'overview' | 'orders' | 'channels' | 'plans'
const activeTab = ref<OrderTab>('overview')
const tabs = computed(() => [
  { key: 'overview' as OrderTab, icon: 'chart' as const, label: t('payment.admin.tabs.overview') },
  { key: 'orders' as OrderTab, icon: 'creditCard' as const, label: t('payment.admin.tabs.orders') },
  { key: 'channels' as OrderTab, icon: 'server' as const, label: t('payment.admin.tabs.channels') },
  { key: 'plans' as OrderTab, icon: 'cube' as const, label: t('payment.admin.tabs.plans') },
])

const dashboardLoading = ref(false)
const dashboardStats = ref<DashboardStats | null>(null)
async function loadDashboard() {
  dashboardLoading.value = true
  try {
    const res = await adminPaymentAPI.getDashboard(30)
    dashboardStats.value = res.data
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally { dashboardLoading.value = false }
}
function methodColor(type: string): string {
  const c: Record<string, string> = { alipay: 'bg-blue-500', wxpay: 'bg-green-500', alipay_direct: 'bg-blue-400', wxpay_direct: 'bg-green-400', stripe: 'bg-purple-500' }
  return c[type] || 'bg-gray-400'
}

const ordersLoading = ref(false)
const orders = ref<PaymentOrder[]>([])
const orderSearch = ref('')
const orderFilters = reactive({ status: '', payment_type: '', order_type: '' })
const orderPagination = reactive({ page: 1, page_size: 20, total: 0 })
const selectedOrder = ref<PaymentOrder | null>(null)
const showDetailDialog = ref(false)
const showRefundDialog = ref(false)
const refundSubmitting = ref(false)

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function debounceLoadOrders() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => loadOrders(), 300)
}
async function loadOrders() {
  ordersLoading.value = true
  try {
    const res = await adminPaymentAPI.getOrders({
      page: orderPagination.page, page_size: orderPagination.page_size,
      keyword: orderSearch.value || undefined, status: orderFilters.status || undefined,
      payment_type: orderFilters.payment_type || undefined, order_type: orderFilters.order_type || undefined,
    })
    orders.value = res.data.data || []
    orderPagination.total = res.data.total || 0
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally { ordersLoading.value = false }
}
function handleOrderPageChange(page: number) { orderPagination.page = page; loadOrders() }
function handleOrderPageSizeChange(size: number) { orderPagination.page_size = size; orderPagination.page = 1; loadOrders() }

const statusFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allStatuses') },
  { value: 'PENDING', label: t('payment.status.pending') },
  { value: 'PAID', label: t('payment.status.paid') },
  { value: 'COMPLETED', label: t('payment.status.completed') },
  { value: 'EXPIRED', label: t('payment.status.expired') },
  { value: 'CANCELLED', label: t('payment.status.cancelled') },
  { value: 'FAILED', label: t('payment.status.failed') },
  { value: 'REFUNDED', label: t('payment.status.refunded') },
])
const paymentTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allPaymentTypes') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
])
const orderTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allOrderTypes') },
  { value: 'balance', label: t('payment.admin.balanceOrder') },
  { value: 'subscription', label: t('payment.admin.subscriptionOrder') },
])
const orderColumns: Column[] = [
  { key: 'id', label: 'ID' }, { key: 'user_id', label: 'User' }, { key: 'amount', label: 'Amount' },
  { key: 'payment_type', label: 'Method' }, { key: 'status', label: 'Status' },
  { key: 'created_at', label: 'Created' }, { key: 'actions', label: 'Actions' },
]
function statusBadgeClass(status: string): string {
  const m: Record<string, string> = { PENDING: 'badge-warning', PAID: 'badge-info', RECHARGING: 'badge-info', COMPLETED: 'badge-success', EXPIRED: 'badge-secondary', CANCELLED: 'badge-secondary', FAILED: 'badge-danger', REFUND_REQUESTED: 'badge-warning', REFUNDING: 'badge-warning', PARTIALLY_REFUNDED: 'badge-warning', REFUNDED: 'badge-info', REFUND_FAILED: 'badge-danger' }
  return m[status] || 'badge-secondary'
}
function canRefund(order: PaymentOrder): boolean { return ['COMPLETED', 'PARTIALLY_REFUNDED'].includes(order.status) }
function showOrderDetail(order: PaymentOrder) { selectedOrder.value = order; showDetailDialog.value = true }
async function handleCancelOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.cancelOrder(order.id); appStore.showSuccess(t('payment.admin.orderCancelled')); loadOrders() }
  catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
}
async function handleRetryOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.retryRecharge(order.id); appStore.showSuccess(t('payment.admin.retrySuccess')); loadOrders() }
  catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
}
function openRefundDialog(order: PaymentOrder) { selectedOrder.value = order; showRefundDialog.value = true }
async function handleRefund(data: { amount: number; reason: string; deduct_balance: boolean }) {
  if (!selectedOrder.value) return
  refundSubmitting.value = true
  try {
    await adminPaymentAPI.refundOrder(selectedOrder.value.id, { amount: data.amount, reason: data.reason, deduct_balance: data.deduct_balance })
    appStore.showSuccess(t('payment.admin.refundSuccess')); showRefundDialog.value = false; loadOrders()
  } catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
  finally { refundSubmitting.value = false }
}

const channelsLoading = ref(false)
const channels = ref<PaymentChannel[]>([])
const showChannelDialog = ref(false)
const showDeleteChannelDialog = ref(false)
const channelSaving = ref(false)
const editingChannel = ref<PaymentChannel | null>(null)
const deletingChannelId = ref<number | null>(null)
const channelForm = reactive({ name: '', group_id: 0, rate_multiplier: 1, description: '', enabled: true })
const channelColumns: Column[] = [
  { key: 'id', label: 'ID' }, { key: 'name', label: 'Name' },
  { key: 'rate_multiplier', label: 'Rate' }, { key: 'enabled', label: 'Status' }, { key: 'actions', label: 'Actions' },
]
async function loadChannels() {
  channelsLoading.value = true
  try { const res = await adminPaymentAPI.getChannels(); channels.value = res.data || [] }
  catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
  finally { channelsLoading.value = false }
}
function openChannelEdit(channel: PaymentChannel | null) {
  editingChannel.value = channel
  if (channel) { Object.assign(channelForm, { name: channel.name, group_id: channel.group_id || 0, rate_multiplier: channel.rate_multiplier, description: channel.description, enabled: channel.enabled }) }
  else { Object.assign(channelForm, { name: '', group_id: 0, rate_multiplier: 1, description: '', enabled: true }) }
  showChannelDialog.value = true
}
async function handleSaveChannel() {
  channelSaving.value = true
  try {
    if (editingChannel.value) { await adminPaymentAPI.updateChannel(editingChannel.value.id, { ...channelForm }) }
    else { await adminPaymentAPI.createChannel({ ...channelForm }) }
    appStore.showSuccess(t('common.saved')); showChannelDialog.value = false; loadChannels()
  } catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
  finally { channelSaving.value = false }
}
function confirmDeleteChannel(channel: PaymentChannel) { deletingChannelId.value = channel.id; showDeleteChannelDialog.value = true }
async function handleDeleteChannel() {
  if (!deletingChannelId.value) return
  try { await adminPaymentAPI.deleteChannel(deletingChannelId.value); appStore.showSuccess(t('common.deleted')); showDeleteChannelDialog.value = false; loadChannels() }
  catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
}

const plansLoading = ref(false)
const plans = ref<SubscriptionPlan[]>([])
const showPlanDialog = ref(false)
const showDeletePlanDialog = ref(false)
const planSaving = ref(false)
const editingPlan = ref<SubscriptionPlan | null>(null)
const deletingPlanId = ref<number | null>(null)
const planForm = reactive({ name: '', group_id: 0, description: '', price: 0, original_price: 0, validity_days: 30, validity_unit: 'days', for_sale: true, sort_order: 0 })
const planFeaturesText = ref('')
const validityUnitOptions = computed(() => [
  { value: 'days', label: t('payment.admin.days') },
  { value: 'weeks', label: t('payment.admin.weeks') },
  { value: 'months', label: t('payment.admin.months') },
])
const planColumns: Column[] = [
  { key: 'id', label: 'ID' }, { key: 'name', label: 'Name' }, { key: 'price', label: 'Price' },
  { key: 'validity_days', label: 'Validity' }, { key: 'for_sale', label: 'For Sale' },
  { key: 'sort_order', label: 'Sort' }, { key: 'actions', label: 'Actions' },
]
async function loadPlans() {
  plansLoading.value = true
  try { const res = await adminPaymentAPI.getPlans(); plans.value = res.data || [] }
  catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
  finally { plansLoading.value = false }
}
function openPlanEdit(plan: SubscriptionPlan | null) {
  editingPlan.value = plan
  if (plan) {
    Object.assign(planForm, { name: plan.name, group_id: plan.group_id, description: plan.description, price: plan.price, original_price: plan.original_price || 0, validity_days: plan.validity_days, validity_unit: plan.validity_unit || 'days', for_sale: plan.for_sale, sort_order: plan.sort_order })
    planFeaturesText.value = (plan.features || []).join('\n')
  } else {
    Object.assign(planForm, { name: '', group_id: 0, description: '', price: 0, original_price: 0, validity_days: 30, validity_unit: 'days', for_sale: true, sort_order: 0 })
    planFeaturesText.value = ''
  }
  showPlanDialog.value = true
}
async function handleSavePlan() {
  planSaving.value = true
  try {
    const features = planFeaturesText.value.split('\n').map(f => f.trim()).filter(Boolean)
    const data = { ...planForm, features }
    if (editingPlan.value) { await adminPaymentAPI.updatePlan(editingPlan.value.id, data) }
    else { await adminPaymentAPI.createPlan(data) }
    appStore.showSuccess(t('common.saved')); showPlanDialog.value = false; loadPlans()
  } catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
  finally { planSaving.value = false }
}
function confirmDeletePlan(plan: SubscriptionPlan) { deletingPlanId.value = plan.id; showDeletePlanDialog.value = true }
async function handleDeletePlan() {
  if (!deletingPlanId.value) return
  try { await adminPaymentAPI.deletePlan(deletingPlanId.value); appStore.showSuccess(t('common.deleted')); showDeletePlanDialog.value = false; loadPlans() }
  catch (err: unknown) { appStore.showError(err instanceof Error ? err.message : String(err)) }
}

function formatDateTime(dateStr: string): string { if (!dateStr) return '-'; return new Date(dateStr).toLocaleString() }

onMounted(() => { loadDashboard(); loadOrders(); loadChannels(); loadPlans() })
</script>
