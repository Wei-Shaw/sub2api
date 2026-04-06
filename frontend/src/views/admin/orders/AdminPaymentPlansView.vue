<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.plansPageTitle') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.plansPageDesc') }}</p>
        </div>
      </div>

      <!-- Sub-tab Switcher -->
      <div class="flex space-x-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-800">
        <button
          v-for="tab in subTabs"
          :key="tab.key"
          class="flex-1 rounded-lg px-4 py-2.5 text-sm font-medium transition-all"
          :class="activeSubTab === tab.key
            ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white'
            : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
          @click="activeSubTab = tab.key"
        >{{ tab.label }}</button>
      </div>

      <!-- Sub-tab: Plan Configuration -->
      <template v-if="activeSubTab === 'plans'">
        <div class="space-y-4">
          <div class="flex items-center justify-end gap-2">
            <button @click="loadPlans" :disabled="plansLoading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="plansLoading ? 'animate-spin' : ''" />
            </button>
            <button @click="openPlanEdit(null)" class="btn btn-primary">{{ t('payment.admin.createPlan') }}</button>
          </div>
          <DataTable :columns="planColumns" :data="plans" :loading="plansLoading">
            <template #cell-group_id="{ value }">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ groupName(value) }}</span>
            </template>
            <template #cell-price="{ value, row }">
              <div class="text-sm">
                <span class="font-medium text-gray-900 dark:text-white">${{ value.toFixed(2) }}</span>
                <span v-if="row.original_price" class="ml-1 text-xs text-gray-400 line-through">${{ row.original_price.toFixed(2) }}</span>
              </div>
            </template>
            <template #cell-validity_days="{ value, row }">
              <span class="text-sm">{{ value }} {{ row.validity_unit || 'days' }}</span>
            </template>
            <template #cell-for_sale="{ value }">
              <span :class="['badge', value ? 'badge-success' : 'badge-secondary']">{{ value ? t('payment.admin.onSale') : t('payment.admin.offSale') }}</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="flex items-center gap-1">
                <button @click="openPlanEdit(row)" class="btn-icon text-blue-500 hover:text-blue-700" :title="t('common.edit')"><Icon name="edit" size="sm" /></button>
                <button @click="confirmDeletePlan(row)" class="btn-icon text-red-500 hover:text-red-700" :title="t('common.delete')"><Icon name="trash" size="sm" /></button>
              </div>
            </template>
          </DataTable>
        </div>
      </template>

      <!-- Sub-tab: User Subscriptions -->
      <template v-else-if="activeSubTab === 'userSubs'">
        <div class="space-y-4">
          <!-- Search Bar -->
          <div class="card p-4">
            <div class="flex items-center gap-3">
              <div class="flex-1 sm:max-w-80">
                <input
                  v-model="subsKeyword"
                  type="text"
                  :placeholder="t('payment.admin.searchUserSubs')"
                  class="input"
                  @input="debounceLoadSubs"
                />
              </div>
              <Select v-model="subsStatusFilter" :options="subsStatusOptions" class="w-36" @change="loadUserSubs" />
              <button @click="loadUserSubs" :disabled="subsLoading" class="btn btn-secondary" :title="t('common.refresh')">
                <Icon name="refresh" size="md" :class="subsLoading ? 'animate-spin' : ''" />
              </button>
            </div>
          </div>

          <!-- Subscriptions Table -->
          <DataTable :columns="subsColumns" :data="userSubs" :loading="subsLoading">
            <template #cell-user_id="{ value, row }">
              <div class="text-sm">
                <span class="font-medium text-gray-900 dark:text-white">#{{ value }}</span>
                <span v-if="row.user_email" class="ml-1 text-xs text-gray-500">{{ row.user_email }}</span>
              </div>
            </template>
            <template #cell-group_id="{ value, row }">
              <div class="text-sm">
                <span class="text-gray-700 dark:text-gray-300">{{ row.group_name || groupName(value) }}</span>
              </div>
            </template>
            <template #cell-status="{ value }">
              <span :class="['badge', subsStatusClass(value)]">{{ t('payment.admin.subsStatus.' + value, value) }}</span>
            </template>
            <template #cell-expires_at="{ value }">
              <div class="text-sm">
                <span class="text-gray-700 dark:text-gray-300">{{ formatDate(value) }}</span>
                <span v-if="daysRemaining(value) !== null" :class="['ml-1 text-xs', (daysRemaining(value) ?? 0) <= 3 ? 'text-red-500' : 'text-gray-400']">
                  ({{ daysRemaining(value) }}d)
                </span>
              </div>
            </template>
            <template #cell-usage="{ row }">
              <div class="space-y-1 text-xs">
                <UsageBar :label="t('payment.admin.daily')" :usage="row.daily_usage_usd" :limit="row.daily_limit_usd" />
                <UsageBar :label="t('payment.admin.weekly')" :usage="row.weekly_usage_usd" :limit="row.weekly_limit_usd" />
                <UsageBar :label="t('payment.admin.monthly')" :usage="row.monthly_usage_usd" :limit="row.monthly_limit_usd" />
              </div>
            </template>
          </DataTable>
          <Pagination
            v-if="subsPagination.total > 0"
            :page="subsPagination.page"
            :total="subsPagination.total"
            :page-size="subsPagination.page_size"
            @update:page="handleSubsPageChange"
            @update:pageSize="handleSubsPageSizeChange"
          />
        </div>
      </template>
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
            <Select
              v-model="planForm.group_id"
              :options="groupOptions"
              class="w-full"
            />
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
        <div class="flex items-center gap-2">
          <input id="plan-for-sale" v-model="planForm.for_sale" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <label for="plan-for-sale" class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.forSale') }}</label>
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
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import adminAPI from '@/api/admin'
import type { SubscriptionPlan } from '@/types/payment'
import type { AdminGroup, UserSubscription, PaginatedResponse } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

type SubTab = 'plans' | 'userSubs'
const activeSubTab = ref<SubTab>('plans')
const subTabs = computed(() => [
  { key: 'plans' as SubTab, label: t('payment.admin.tabPlanConfig') },
  { key: 'userSubs' as SubTab, label: t('payment.admin.tabUserSubs') },
])

// ==================== Groups ====================

const groups = ref<AdminGroup[]>([])

async function loadGroups() {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch { /* ignore */ }
}

function groupName(id: number): string {
  const g = groups.value.find(g => g.id === id)
  return g ? `${g.name} (${g.platform})` : `#${id}`
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

const planColumns: Column[] = [
  { key: 'id', label: 'ID' }, { key: 'name', label: 'Name' }, { key: 'group_id', label: 'Group' },
  { key: 'price', label: 'Price' }, { key: 'validity_days', label: 'Validity' },
  { key: 'for_sale', label: 'For Sale' }, { key: 'sort_order', label: 'Sort' }, { key: 'actions', label: 'Actions' },
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

// ==================== User Subscriptions ====================

const subsLoading = ref(false)
const userSubs = ref<UserSubscription[]>([])
const subsKeyword = ref('')
const subsStatusFilter = ref('')
const subsPagination = reactive({ page: 1, page_size: 20, total: 0 })

const subsStatusOptions = computed(() => [
  { value: '', label: t('payment.admin.allStatuses') },
  { value: 'active', label: t('payment.admin.subsStatus.active') },
  { value: 'expired', label: t('payment.admin.subsStatus.expired') },
  { value: 'revoked', label: t('payment.admin.subsStatus.revoked') },
])

const subsColumns: Column[] = [
  { key: 'id', label: 'ID' }, { key: 'user_id', label: 'User' }, { key: 'group_id', label: 'Group' },
  { key: 'status', label: 'Status' }, { key: 'expires_at', label: 'Expires' }, { key: 'usage', label: 'Usage' },
]

let subsDebounceTimer: ReturnType<typeof setTimeout> | null = null
function debounceLoadSubs() {
  if (subsDebounceTimer) clearTimeout(subsDebounceTimer)
  subsDebounceTimer = setTimeout(() => loadUserSubs(), 300)
}

async function loadUserSubs() {
  subsLoading.value = true
  try {
    const filters: Record<string, any> = {}
    if (subsStatusFilter.value) filters.status = subsStatusFilter.value
    const res: PaginatedResponse<UserSubscription> = await adminAPI.subscriptions.list(
      subsPagination.page,
      subsPagination.page_size,
      filters,
    )
    userSubs.value = res.items || []
    subsPagination.total = res.total || 0
  } catch (err: unknown) {
    appStore.showError(err instanceof Error ? err.message : String(err))
  } finally { subsLoading.value = false }
}

function handleSubsPageChange(page: number) { subsPagination.page = page; loadUserSubs() }
function handleSubsPageSizeChange(size: number) { subsPagination.page_size = size; subsPagination.page = 1; loadUserSubs() }

function subsStatusClass(status: string): string {
  const m: Record<string, string> = { active: 'badge-success', expired: 'badge-secondary', revoked: 'badge-danger' }
  return m[status] || 'badge-secondary'
}

function formatDate(dateStr: string): string {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleDateString()
}

function daysRemaining(dateStr: string): number | null {
  if (!dateStr) return null
  const diff = new Date(dateStr).getTime() - Date.now()
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

// ==================== Usage Bar component ====================

const UsageBar = {
  props: {
    label: String,
    usage: { type: Number, default: 0 },
    limit: { type: Number, default: null },
  },
  setup(props: { label: string; usage: number; limit: number | null }) {
    const pct = computed(() => props.limit && props.limit > 0 ? Math.min((props.usage / props.limit) * 100, 100) : 0)
    const barColor = computed(() => pct.value > 80 ? 'bg-red-500' : pct.value > 50 ? 'bg-yellow-500' : 'bg-green-500')
    return { pct, barColor }
  },
  template: `
    <div class="flex items-center gap-2">
      <span class="w-8 text-gray-500 dark:text-gray-400">{{ label }}</span>
      <div class="flex-1">
        <div v-if="limit != null && limit > 0" class="h-1.5 w-full rounded-full bg-gray-200 dark:bg-dark-600">
          <div :class="['h-full rounded-full transition-all', barColor]" :style="{ width: pct + '%' }"></div>
        </div>
      </div>
      <span class="text-gray-600 dark:text-gray-300">\${{ usage.toFixed(2) }}<span v-if="limit != null"> / \${{ limit.toFixed(2) }}</span></span>
    </div>
  `,
}

// ==================== Lifecycle ====================

watch(activeSubTab, (tab) => {
  if (tab === 'userSubs' && userSubs.value.length === 0) loadUserSubs()
})

onMounted(() => {
  loadGroups()
  loadPlans()
})
</script>
