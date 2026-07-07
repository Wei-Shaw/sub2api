<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Actions -->
      <div class="flex items-center justify-end gap-2">
        <button @click="loadPlans" :disabled="plansLoading" class="btn btn-secondary" :title="t('common.refresh')">
          <Icon name="refresh" size="md" :class="plansLoading ? 'animate-spin' : ''" />
        </button>
        <button @click="openPlanEdit(null)" class="btn btn-primary">{{ t('payment.admin.createPlan') REDACTEDREDACTED</button>
      </div>

      <!-- Plans Table -->
      <DataTable :columns="planColumns" :data="plans" :loading="plansLoading">
        <template #cell-name="{ value, row REDACTED">
          <span class="text-sm font-medium" :class="getPlanNameClass(row.group_id)">{{ value REDACTEDREDACTED</span>
        </template>
        <template #cell-group_id="{ value REDACTED">
          <span v-if="isGroupMissing(value)" class="text-sm">
            <span class="text-gray-400">#{{ value REDACTEDREDACTED</span>
            <span class="ml-1 badge badge-danger">{{ t('payment.admin.groupMissing') REDACTEDREDACTED</span>
          </span>
          <GroupBadge
            v-else-if="getGroup(value)"
            :name="getGroup(value)!.name"
            :platform="getGroup(value)!.platform"
            :rate-multiplier="getGroup(value)!.rate_multiplier"
          />
          <span v-else class="text-sm text-gray-400">-</span>
        </template>
        <template #cell-price="{ value, row REDACTED">
          <div class="text-sm">
            <span class="font-medium text-gray-900 dark:text-white">${{ (value ?? 0).toFixed(2) REDACTEDREDACTED</span>
            <span v-if="row.original_price" class="ml-1 text-xs text-gray-400 line-through">${{ row.original_price.toFixed(2) REDACTEDREDACTED</span>
          </div>
        </template>
        <template #cell-validity_days="{ value, row REDACTED">
          <span class="text-sm">{{ value REDACTEDREDACTED {{ t('payment.admin.' + (row.validity_unit || 'days')) REDACTEDREDACTED</span>
        </template>
        <template #cell-for_sale="{ value, row REDACTED">
          <button
            type="button"
            :class="[
              'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              value ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
            ]"
            @click="toggleForSale(row)"
          >
            <span :class="[
              'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              value ? 'translate-x-4' : 'translate-x-0'
            ]" />
          </button>
        </template>
        <template #cell-actions="{ row REDACTED">
          <div class="flex items-center gap-2">
            <button @click="openPlanEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400">
              <Icon name="edit" size="sm" />
              <span class="text-xs">{{ t('common.edit') REDACTEDREDACTED</span>
            </button>
            <button @click="confirmDeletePlan(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400">
              <Icon name="trash" size="sm" />
              <span class="text-xs">{{ t('common.delete') REDACTEDREDACTED</span>
            </button>
          </div>
        </template>
      </DataTable>
    </div>

    <!-- Plan Edit Dialog -->
    <PlanEditDialog :show="showPlanDialog" :plan="editingPlan" :groups="groups" :payment-config="paymentConfig" @close="showPlanDialog = false" @saved="loadPlans" />

    <ConfirmDialog :show="showDeletePlanDialog" :title="t('payment.admin.deletePlan')" :message="t('payment.admin.deletePlanConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeletePlan" @cancel="showDeletePlanDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminPaymentAPI REDACTED from '@/api/admin/payment'
import type { AdminPaymentConfig REDACTED from '@/api/admin/payment'
import { extractI18nErrorMessage REDACTED from '@/utils/apiError'
import adminAPI from '@/api/admin'
import type { SubscriptionPlan REDACTED from '@/types/payment'
import type { AdminGroup REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlanEditDialog from './PlanEditDialog.vue'
import { platformTextClass REDACTED from '@/utils/platformColors'

const { t REDACTED = useI18n()
const appStore = useAppStore()

// ==================== Groups ====================

const groups = ref<AdminGroup[]>([])
const paymentConfig = ref<AdminPaymentConfig | null>(null)

async function loadGroups() {
  try {
    groups.value = await adminAPI.groups.getAll()
  REDACTED catch { /* ignore */ REDACTED
REDACTED

async function loadPaymentConfig() {
  try {
    const res = await adminPaymentAPI.getConfig()
    paymentConfig.value = res.data
  REDACTED catch { /* preview only */ REDACTED
REDACTED

function getGroup(id: number): AdminGroup | undefined {
  return groups.value.find(g => g.id === id)
REDACTED

function isGroupMissing(id: number): boolean {
  return id > 0 && !groups.value.find(g => g.id === id)
REDACTED

function getPlanNameClass(groupId: number): string {
  const group = getGroup(groupId)
  return group ? platformTextClass(group.platform) : 'text-gray-900 dark:text-white'
REDACTED


// ==================== Plans ====================

const plansLoading = ref(false)
const plans = ref<SubscriptionPlan[]>([])
const showPlanDialog = ref(false)
const showDeletePlanDialog = ref(false)
const editingPlan = ref<SubscriptionPlan | null>(null)
const deletingPlanId = ref<number | null>(null)

const planColumns = computed((): Column[] => [
  { key: 'id', label: 'ID' REDACTED,
  { key: 'name', label: t('payment.admin.planName') REDACTED,
  { key: 'group_id', label: t('payment.admin.group') REDACTED,
  { key: 'price', label: t('payment.admin.price') REDACTED,
  { key: 'validity_days', label: t('payment.admin.validityDays') REDACTED,
  { key: 'for_sale', label: t('payment.admin.forSale') REDACTED,
  { key: 'sort_order', label: t('payment.admin.sortOrder') REDACTED,
  { key: 'actions', label: t('common.actions') REDACTED,
])

async function loadPlans() {
  plansLoading.value = true
  try {
    const res = await adminPaymentAPI.getPlans()
    // Backend returns features as newline-separated string; parse to array
    plans.value = (res.data || []).map((p: Omit<SubscriptionPlan, 'features'> & { features: string | string[] REDACTED) => ({
      ...p,
      features: typeof p.features === 'string'
        ? p.features.split('\n').map((f: string) => f.trim()).filter(Boolean)
        : (p.features || []),
    REDACTED))
  REDACTED
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) REDACTED
  finally { plansLoading.value = false REDACTED
REDACTED

function openPlanEdit(plan: SubscriptionPlan | null) {
  editingPlan.value = plan
  showPlanDialog.value = true
REDACTED


/** Quick toggle for_sale from the list */
async function toggleForSale(plan: SubscriptionPlan) {
  try {
    await adminPaymentAPI.updatePlan(plan.id, { for_sale: !plan.for_sale REDACTED)
    plan.for_sale = !plan.for_sale
  REDACTED catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  REDACTED
REDACTED

function confirmDeletePlan(plan: SubscriptionPlan) { deletingPlanId.value = plan.id; showDeletePlanDialog.value = true REDACTED
async function handleDeletePlan() {
  if (!deletingPlanId.value) return
  try { await adminPaymentAPI.deletePlan(deletingPlanId.value); appStore.showSuccess(t('common.deleted')); showDeletePlanDialog.value = false; loadPlans() REDACTED
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) REDACTED
REDACTED

// ==================== Lifecycle ====================

onMounted(() => {
  loadGroups()
  loadPaymentConfig()
  loadPlans()
REDACTED)
</script>
