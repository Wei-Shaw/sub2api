<template>
  <AppLayout>
    <div class="space-y-4">
      <!--
        A toolbar, not a card. The filter row used to sit in its own bordered
        panel directly above another bordered panel, which is two boxes for one
        idea; a single rule under the controls separates them.
      -->
      <div class="flex flex-wrap items-center gap-3 border-b border-line pb-4">
        <Select
          v-model="currentFilter"
          :options="statusFilters"
          class="w-36"
          @change="fetchOrders"
        />
        <div class="flex flex-1 items-center justify-end gap-2">
          <Button
            :loading="loading"
            :title="t('common.refresh')"
            :aria-label="t('common.refresh')"
            @click="fetchOrders"
          >
            <template #icon>
              <Icon name="refresh" size="xs" />
            </template>
          </Button>
          <Button tone="accent" variant="solid" @click="router.push('/purchase')">
            {{ t('payment.result.backToRecharge') }}
          </Button>
        </div>
      </div>

      <OrderTable :orders="orders" :loading="loading">
        <template #actions="{ row }">
          <div class="flex items-center justify-end gap-3">
            <!--
              `quiet` inside a cell: a hover ground here would fight the row
              hover, and an outlined control per row turns the column into a
              wall of boxes.
            -->
            <Button
              v-if="row.status === 'PENDING'"
              variant="quiet"
              size="xs"
              tone="danger"
              @click="handleCancel(row.id)"
            >
              {{ t('payment.orders.cancel') }}
            </Button>
            <Button
              v-if="canRequestRefund(row)"
              variant="quiet"
              size="xs"
              @click="openRefundDialog(row)"
            >
              {{ t('payment.orders.requestRefund') }}
            </Button>
          </div>
        </template>
      </OrderTable>

      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </div>

    <!-- Cancel Confirm Dialog -->
    <BaseDialog
      :show="!!cancelTargetId"
      :title="t('payment.orders.cancel')"
      width="narrow"
      @close="cancelTargetId = null"
    >
      <p class="text-sm text-ink-secondary">{{ t('payment.confirmCancel') }}</p>
      <template #footer>
        <div class="flex justify-end gap-2">
          <Button size="md" @click="cancelTargetId = null">{{ t('common.cancel') }}</Button>
          <!--
            `loading` keeps the label's box and overlays a spinner. Swapping the
            text to "Processing…" resized the button under the pointer that had
            just pressed it.
          -->
          <Button
            size="md"
            tone="danger"
            variant="solid"
            :loading="actionLoading"
            @click="confirmCancel"
          >
            {{ t('payment.orders.cancel') }}
          </Button>
        </div>
      </template>
    </BaseDialog>

    <!-- Refund Dialog -->
    <BaseDialog
      :show="!!refundTarget"
      :title="t('payment.orders.requestRefund')"
      @close="refundTarget = null"
    >
      <div v-if="refundTarget" class="space-y-4">
        <dl class="divide-y divide-line-subtle rounded border border-line">
          <div class="flex items-baseline justify-between gap-4 px-3 py-2">
            <dt class="text-xs text-ink-secondary">{{ t('payment.orders.orderId') }}</dt>
            <dd class="font-mono text-xs tabular-nums text-ink">#{{ refundTarget.id }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 px-3 py-2">
            <dt class="text-xs text-ink-secondary">{{ t('payment.orders.amount') }}</dt>
            <dd class="inline-flex items-baseline gap-0.5">
              <span class="text-2xs text-ink-tertiary">{{ usdSymbol }}</span>
              <NumCell :value="refundTarget.amount" :precision="2" />
            </dd>
          </div>
        </dl>

        <FormField
          id="refund-reason"
          :label="t('payment.refundReason')"
          required
          :error="refundReasonError"
        >
          <template #default="{ describedBy, invalid }">
            <textarea
              id="refund-reason"
              v-model="refundReason"
              rows="3"
              class="input"
              :class="invalid && 'input-error'"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
              :placeholder="t('payment.refundReasonPlaceholder')"
            />
          </template>
        </FormField>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <Button size="md" @click="refundTarget = null">{{ t('common.cancel') }}</Button>
          <Button
            size="md"
            tone="accent"
            variant="solid"
            :loading="actionLoading"
            @click="confirmRefund"
          >
            {{ t('payment.orders.requestRefund') }}
          </Button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { paymentAPI } from '@/api/payment'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import NumCell from '@/components/common/NumCell.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { currencySymbol } from '@/components/payment/currency'
import OrderTable from '@/components/payment/OrderTable.vue'
import { useAppStore } from '@/stores'
import type { PaymentOrder } from '@/types/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const loading = ref(false)
const actionLoading = ref(false)
const orders = ref<PaymentOrder[]>([])
const refundEligibleProviders = ref<Set<string>>(new Set())
const currentFilter = ref('')
const cancelTargetId = ref<number | null>(null)
const refundTarget = ref<PaymentOrder | null>(null)
const refundReason = ref('')
const refundReasonError = ref('')
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

/** Refunds are credited back to the balance, which is always USD. */
const usdSymbol = currencySymbol('USD')

const statusFilters = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'PENDING', label: t('payment.status.pending') },
  { value: 'COMPLETED', label: t('payment.status.completed') },
  { value: 'FAILED', label: t('payment.status.failed') },
])

async function fetchOrders() {
  loading.value = true
  try {
    const res = await paymentAPI.getMyOrders({
      page: pagination.page,
      page_size: pagination.page_size,
      status: currentFilter.value || undefined,
    })
    orders.value = res.data.items || []
    pagination.total = res.data.total || 0
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) { pagination.page = page; fetchOrders() }
function handlePageSizeChange(size: number) { pagination.page_size = size; pagination.page = 1; fetchOrders() }

function handleCancel(orderId: number) { cancelTargetId.value = orderId }

async function confirmCancel() {
  if (!cancelTargetId.value) return
  actionLoading.value = true
  try {
    await paymentAPI.cancelOrder(cancelTargetId.value)
    appStore.showSuccess(t('common.success'))
    cancelTargetId.value = null
    await fetchOrders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    actionLoading.value = false
  }
}

function openRefundDialog(order: PaymentOrder) {
  refundTarget.value = order
  refundReason.value = ''
  refundReasonError.value = ''
}

async function confirmRefund() {
  if (!refundTarget.value) return
  /*
   * The submit button used to be disabled until a reason was typed, which is a
   * dead control that never says why. It stays live and answers in the field's
   * own reserved message row instead.
   */
  if (!refundReason.value.trim()) {
    refundReasonError.value = `${t('payment.refundReason')} ${t('common.required')}`
    return
  }
  refundReasonError.value = ''
  actionLoading.value = true
  try {
    await paymentAPI.requestRefund(refundTarget.value.id, { reason: refundReason.value.trim() })
    appStore.showSuccess(t('common.success'))
    refundTarget.value = null
    refundReason.value = ''
    await fetchOrders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    actionLoading.value = false
  }
}

function canRequestRefund(order: PaymentOrder): boolean {
  if (order.status !== 'COMPLETED') return false
  if (!order.provider_instance_id) return false
  return refundEligibleProviders.value.has(order.provider_instance_id)
}

async function loadRefundEligibility() {
  try {
    const res = await paymentAPI.getRefundEligibleProviders()
    refundEligibleProviders.value = new Set(res.data.provider_instance_ids || [])
  } catch { /* ignore — default to hiding refund button */ }
}

onMounted(() => { fetchOrders(); loadRefundEligibility() })
</script>
