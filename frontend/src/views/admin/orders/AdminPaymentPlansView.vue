<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Actions -->
      <div class="flex items-center justify-end gap-2">
        <button @click="loadPlans" :disabled="plansLoading" class="btn btn-secondary" :title="t('common.refresh')">
          <Icon name="refresh" size="md" :class="plansLoading ? 'animate-spin' : ''" />
        </button>
        <button @click="openPlanEdit(null)" class="btn btn-primary">{{ t('payment.admin.createPlan') }}</button>
      </div>

      <!-- Plans Table -->
      <DataTable :columns="planColumns" :data="plans" :loading="plansLoading">
        <template #cell-group_id="{ value }">
          <span v-if="isGroupMissing(value)" class="text-sm">
            <span class="text-gray-400">#{{ value }}</span>
            <span class="ml-1 badge badge-danger">{{ t('payment.admin.groupMissing') }}</span>
          </span>
          <GroupBadge
            v-else-if="getGroup(value)"
            :name="getGroup(value)!.name"
            :platform="getGroup(value)!.platform"
            :rate-multiplier="getGroup(value)!.rate_multiplier"
          />
          <span v-else class="text-sm text-gray-400">-</span>
        </template>
        <template #cell-price="{ value, row }">
          <div class="text-sm">
            <span class="font-medium text-gray-900 dark:text-white">&yen;{{ value.toFixed(2) }}</span>
            <span v-if="row.original_price" class="ml-1 text-xs text-gray-400 line-through">&yen;{{ row.original_price.toFixed(2) }}</span>
          </div>
        </template>
        <template #cell-validity_days="{ value, row }">
          <span class="text-sm">{{ value }} {{ t('payment.admin.' + (row.validity_unit || 'days')) }}</span>
        </template>
        <template #cell-for_sale="{ value, row }">
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
        <template #cell-actions="{ row }">
          <div class="flex items-center gap-1">
            <button @click="openPlanEdit(row)" class="btn-icon text-blue-500 hover:text-blue-700" :title="t('common.edit')"><Icon name="edit" size="sm" /></button>
            <button @click="confirmDeletePlan(row)" class="btn-icon text-red-500 hover:text-red-700" :title="t('common.delete')"><Icon name="trash" size="sm" /></button>
          </div>
        </template>
      </DataTable>
    </div>

    <!-- Plan Edit Dialog -->
    <BaseDialog :show="showPlanDialog" :title="editingPlan ? t('payment.admin.editPlan') : t('payment.admin.createPlan')" width="wide" @close="showPlanDialog = false">
      <form id="plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('payment.admin.planName') }}</label>
            <input v-model="planForm.name" type="text" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('payment.admin.groupId') }}</label>
            <Select v-model="planForm.group_id" :options="groupOptions" class="w-full" />
          </div>
        </div>

        <!-- Group Info Preview -->
        <div v-if="selectedGroupInfo" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
          <p class="mb-2 text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('payment.admin.groupInfo') }}</p>
          <div class="grid grid-cols-2 gap-2 text-xs">
            <div><span class="text-gray-500">{{ t('payment.admin.platform') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.platform }}</span></div>
            <div><span class="text-gray-500">{{ t('payment.admin.rateMultiplierLabel') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.rate_multiplier }}x</span></div>
            <div><span class="text-gray-500">{{ t('payment.admin.dailyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.daily_limit_usd != null ? '$' + selectedGroupInfo.daily_limit_usd : t('payment.admin.unlimited') }}</span></div>
            <div><span class="text-gray-500">{{ t('payment.admin.weeklyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.weekly_limit_usd != null ? '$' + selectedGroupInfo.weekly_limit_usd : t('payment.admin.unlimited') }}</span></div>
            <div><span class="text-gray-500">{{ t('payment.admin.monthlyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ selectedGroupInfo.monthly_limit_usd != null ? '$' + selectedGroupInfo.monthly_limit_usd : t('payment.admin.unlimited') }}</span></div>
          </div>
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
        <div>
          <label class="input-label">{{ t('payment.admin.features') }}</label>
          <textarea v-model="planFeaturesText" rows="3" class="input" :placeholder="t('payment.admin.featuresPlaceholder')"></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.featuresHint') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <label class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.forSale') }}</label>
          <button
            type="button"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              planForm.for_sale ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
            ]"
            @click="planForm.for_sale = !planForm.for_sale"
          >
            <span :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              planForm.for_sale ? 'translate-x-5' : 'translate-x-0'
            ]" />
          </button>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="showPlanDialog = false" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" form="plan-form" :disabled="planSaving" class="btn btn-primary">{{ planSaving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="showDeletePlanDialog" :title="t('payment.admin.deletePlan')" :message="t('payment.admin.deletePlanConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDeletePlan" @cancel="showDeletePlanDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import adminAPI from '@/api/admin'
import type { SubscriptionPlan } from '@/types/payment'
import type { AdminGroup } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== Groups ====================

const groups = ref<AdminGroup[]>([])

async function loadGroups() {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch { /* ignore */ }
}

function getGroup(id: number): AdminGroup | undefined {
  return groups.value.find(g => g.id === id)
}

function isGroupMissing(id: number): boolean {
  return id > 0 && !groups.value.find(g => g.id === id)
}

const groupOptions = computed(() => [
  { value: 0, label: t('payment.admin.selectGroup') },
  ...groups.value.map(g => ({
    value: g.id,
    label: `${g.name} — ${g.platform} (${g.rate_multiplier}x)`,
  })),
])

const selectedGroupInfo = computed(() => {
  if (!planForm.group_id) return null
  return groups.value.find(g => g.id === planForm.group_id) || null
})

// ==================== Plans ====================

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

const planColumns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'name', label: t('payment.admin.planName') },
  { key: 'group_id', label: t('payment.admin.groupId') },
  { key: 'price', label: t('payment.admin.price') },
  { key: 'validity_days', label: t('payment.admin.validityDays') },
  { key: 'for_sale', label: t('payment.admin.forSale') },
  { key: 'sort_order', label: t('payment.admin.sortOrder') },
  { key: 'actions', label: t('common.actions') },
])

async function loadPlans() {
  plansLoading.value = true
  try {
    const res = await adminPaymentAPI.getPlans()
    // Backend returns features as newline-separated string; parse to array
    plans.value = (res.data || []).map((p: any) => ({
      ...p,
      features: typeof p.features === 'string'
        ? p.features.split('\n').map((f: string) => f.trim()).filter(Boolean)
        : (p.features || []),
    }))
  }
  catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
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

/** Build request payload with snake_case keys matching backend JSON tags */
function buildPlanPayload() {
  const features = planFeaturesText.value.split('\n').map(f => f.trim()).filter(Boolean).join('\n')
  return {
    name: planForm.name,
    group_id: planForm.group_id,
    description: planForm.description,
    price: planForm.price,
    original_price: planForm.original_price || 0,
    validity_days: planForm.validity_days,
    validity_unit: planForm.validity_unit,
    for_sale: planForm.for_sale,
    sort_order: planForm.sort_order,
    features,
  }
}

async function handleSavePlan() {
  planSaving.value = true
  try {
    const data = buildPlanPayload()
    if (editingPlan.value) { await adminPaymentAPI.updatePlan(editingPlan.value.id, data) }
    else { await adminPaymentAPI.createPlan(data) }
    appStore.showSuccess(t('common.saved')); showPlanDialog.value = false; loadPlans()
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { planSaving.value = false }
}

/** Quick toggle for_sale from the list */
async function toggleForSale(plan: SubscriptionPlan) {
  try {
    await adminPaymentAPI.updatePlan(plan.id, { for_sale: !plan.for_sale })
    plan.for_sale = !plan.for_sale
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

function confirmDeletePlan(plan: SubscriptionPlan) { deletingPlanId.value = plan.id; showDeletePlanDialog.value = true }
async function handleDeletePlan() {
  if (!deletingPlanId.value) return
  try { await adminPaymentAPI.deletePlan(deletingPlanId.value); appStore.showSuccess(t('common.deleted')); showDeletePlanDialog.value = false; loadPlans() }
  catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
}

// ==================== Lifecycle ====================

onMounted(() => {
  loadGroups()
  loadPlans()
})
</script>
