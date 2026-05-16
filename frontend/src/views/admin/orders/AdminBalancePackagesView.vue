<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('payment.admin.balancePackagesPageTitle') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.admin.balancePackagesPageDesc') }}</p>
        </div>
        <div class="flex items-center gap-2">
          <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="loadPackages">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button class="btn btn-primary" @click="openEdit(null)">{{ t('payment.admin.createBalancePackage') }}</button>
        </div>
      </div>

      <DataTable :columns="columns" :data="packages" :loading="loading">
        <template #cell-label="{ value, row }">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-primary-600 dark:text-primary-400">{{ value }}</span>
            <span v-if="row.popular" class="badge badge-warning">{{ t('payment.admin.popular') }}</span>
          </div>
          <div class="text-xs text-gray-500">{{ row.code }}</div>
        </template>
        <template #cell-amount_ledger="{ row }">
          <div class="text-sm">
            <div><span class="text-gray-500">{{ t('payment.admin.payAmount') }}:</span> <span class="font-medium text-gray-900 dark:text-white">${{ row.amount_ledger.toFixed(2) }}</span></div>
            <div><span class="text-gray-500">Tokens:</span> <span class="font-medium text-primary-600 dark:text-primary-400">{{ formatTokens(row.actual_credits) }}</span></div>
          </div>
        </template>
        <template #cell-balance_group_id="{ row }">
          <span v-if="isGroupMissing(row.balance_group_id || row.group_id)" class="text-sm">
            <span class="text-gray-400">#{{ row.balance_group_id || row.group_id }}</span>
            <span class="ml-1 badge badge-danger">{{ t('payment.admin.balanceGroupMissing') }}</span>
          </span>
          <GroupBadge
            v-else-if="getGroup(row.balance_group_id || row.group_id)"
            :name="getGroup(row.balance_group_id || row.group_id)!.name"
            :platform="getGroup(row.balance_group_id || row.group_id)!.platform"
            :subscription-type="getGroup(row.balance_group_id || row.group_id)!.subscription_type"
            :rate-multiplier="getGroup(row.balance_group_id || row.group_id)!.rate_multiplier"
          />
          <span v-else class="text-sm text-gray-400">-</span>
        </template>
        <template #cell-badge="{ value }">
          <span v-if="value" class="badge badge-success">{{ value }}</span>
          <span v-else class="text-sm text-gray-400">-</span>
        </template>
        <template #cell-for_sale="{ value, row }">
          <button type="button" :class="['relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2', value ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600']" @click="toggleForSale(row)">
            <span :class="['pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out', value ? 'translate-x-4' : 'translate-x-0']" />
          </button>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex items-center gap-2">
            <button class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400" @click="openEdit(row)">
              <Icon name="edit" size="sm" />
              <span class="text-xs">{{ t('common.edit') }}</span>
            </button>
            <button class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" @click="confirmDelete(row)">
              <Icon name="trash" size="sm" />
              <span class="text-xs">{{ t('common.delete') }}</span>
            </button>
          </div>
        </template>
      </DataTable>
    </div>

    <BalancePackageEditDialog :show="showDialog" :pkg="editingPackage" @close="showDialog = false" @saved="loadPackages" />
    <ConfirmDialog :show="showDeleteDialog" :title="t('payment.admin.deleteBalancePackage')" :message="t('payment.admin.deleteBalancePackageConfirm')" :confirm-text="t('common.delete')" danger @confirm="handleDelete" @cancel="showDeleteDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import adminAPI from '@/api/admin'
import type { BalancePackage } from '@/types/payment'
import type { AdminGroup } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import BalancePackageEditDialog from './BalancePackageEditDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const packages = ref<BalancePackage[]>([])
const showDialog = ref(false)
const showDeleteDialog = ref(false)
const editingPackage = ref<BalancePackage | null>(null)
const deletingPackageId = ref<number | null>(null)

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

const columns = computed((): Column[] => [
  { key: 'id', label: 'ID' },
  { key: 'label', label: t('payment.admin.packageLabel') },
  { key: 'amount_ledger', label: t('payment.admin.packageAmounts') },
  { key: 'balance_group_id', label: t('payment.admin.balanceGroup') },
  { key: 'badge', label: t('payment.admin.badge') },
  { key: 'for_sale', label: t('payment.admin.forSale') },
  { key: 'sort_order', label: t('payment.admin.sortOrder') },
  { key: 'actions', label: t('common.actions') },
])

async function loadPackages() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getBalancePackages()
    packages.value = res.data || []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

function formatTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(0)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)}K`
  return String(value)
}

function openEdit(pkg: BalancePackage | null) {
  editingPackage.value = pkg
  showDialog.value = true
}

async function toggleForSale(pkg: BalancePackage) {
  try {
    await adminPaymentAPI.updateBalancePackage(pkg.id, { for_sale: !pkg.for_sale })
    pkg.for_sale = !pkg.for_sale
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

function confirmDelete(pkg: BalancePackage) {
  deletingPackageId.value = pkg.id
  showDeleteDialog.value = true
}

async function handleDelete() {
  if (!deletingPackageId.value) return
  try {
    await adminPaymentAPI.deleteBalancePackage(deletingPackageId.value)
    appStore.showSuccess(t('common.deleted'))
    showDeleteDialog.value = false
    loadPackages()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

onMounted(() => {
  loadGroups()
  loadPackages()
})
</script>
